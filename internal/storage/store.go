// Package storage is the persistence layer: it opens a SQLite database,
// applies embedded migrations, and wraps the sqlc-generated query object
// with JSON convenience helpers for the JSON-blob columns
// (users.nmt_scores / users.settings, saved_lists.data, *_cache.data).
package storage

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/storage/db"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Sentinel errors.
var (
	// ErrCacheMiss means the requested key isn't in the cache table.
	// Reserved for the program_cache / applicant_cache tables.
	ErrCacheMiss = errors.New("storage: cache miss")
	// ErrCacheStale means the cache entry exists but is older than the TTL.
	ErrCacheStale = errors.New("storage: cache stale")
	// ErrNotFound means a regular (non-cache) row wasn't found —
	// saved lists, users, etc.
	ErrNotFound = errors.New("storage: not found")
)

// readPoolSize is how many concurrent read connections the read pool keeps.
// WAL lets any number of readers proceed while the single writer holds its
// lock, so reads no longer queue behind each other or behind a write.
const readPoolSize = 4

// Store is the high-level persistence facade. It owns two connection pools —
// a single-connection writer and a multi-connection reader (WAL allows one
// writer + N readers concurrently) — and the sqlc query objects bound to
// each. Writes and the infrequent admin reads go through Queries; hot-path
// reads go through ReadQueries.
//
// DB is the write pool (kept for tests / migrations / raw access).
type Store struct {
	DB          *sql.DB
	readDB      *sql.DB
	Queries     *db.Queries // bound to the write pool
	ReadQueries *db.Queries // bound to the read pool
}

// Open opens (or creates) a SQLite database at path, applies pending
// migrations, and returns a ready-to-use Store. Pass ":memory:" for an
// ephemeral in-memory database (intended for tests).
func Open(ctx context.Context, path string) (*Store, error) {
	dsn, err := buildDSN(path)
	if err != nil {
		return nil, err
	}
	writeDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: open: %w", err)
	}
	// SQLite has a single writer; pinning to one connection serializes
	// writes deterministically (no writer-writer SQLITE_BUSY at all).
	writeDB.SetMaxOpenConns(1)
	if err := writeDB.PingContext(ctx); err != nil {
		_ = writeDB.Close()
		return nil, fmt.Errorf("storage: ping: %w", err)
	}
	// Migrations must run before the read pool starts serving.
	if err := applyMigrations(ctx, writeDB); err != nil {
		_ = writeDB.Close()
		return nil, err
	}

	s := &Store{DB: writeDB, Queries: db.New(writeDB)}

	// A separate read pool only pays off for file-backed DBs. An in-memory
	// DB (tests) can't be reliably shared across independent pools, so reads
	// and writes share the one handle there.
	if path == ":memory:" {
		s.readDB = writeDB
		s.ReadQueries = s.Queries
		return s, nil
	}

	readDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		_ = writeDB.Close()
		return nil, fmt.Errorf("storage: open read pool: %w", err)
	}
	readDB.SetMaxOpenConns(readPoolSize)
	if err := readDB.PingContext(ctx); err != nil {
		_ = readDB.Close()
		_ = writeDB.Close()
		return nil, fmt.Errorf("storage: ping read pool: %w", err)
	}
	s.readDB = readDB
	s.ReadQueries = db.New(readDB)
	return s, nil
}

// Close releases both database handles.
func (s *Store) Close() error {
	var rerr error
	if s.readDB != nil && s.readDB != s.DB {
		rerr = s.readDB.Close()
	}
	if err := s.DB.Close(); err != nil {
		return err
	}
	return rerr
}

func buildDSN(path string) (string, error) {
	// busy_timeout makes SQLite block briefly on contended writes
	// instead of returning SQLITE_BUSY immediately — even though we
	// pin MaxOpenConns=1, reads share the same pool and can race
	// against the writer's lock under load.
	const pragmas = "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"
	if path == ":memory:" {
		return "file::memory:?cache=shared&" + pragmas, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("storage: mkdir: %w", err)
	}
	return fmt.Sprintf("file:%s?%s&_pragma=journal_mode(WAL)", path, pragmas), nil
}

func applyMigrations(ctx context.Context, sqlDB *sql.DB) error {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("storage: read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	if _, err := sqlDB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name       TEXT    PRIMARY KEY,
			applied_at INTEGER NOT NULL DEFAULT (unixepoch())
		) STRICT
	`); err != nil {
		return fmt.Errorf("storage: schema_migrations: %w", err)
	}

	for _, name := range names {
		var count int
		if err := sqlDB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM schema_migrations WHERE name = ?", name,
		).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		body, err := fs.ReadFile(migrationsFS, "migrations/"+name)
		if err != nil {
			return fmt.Errorf("storage: read %s: %w", name, err)
		}
		if err := execMigration(ctx, sqlDB, name, string(body)); err != nil {
			return err
		}
	}
	return nil
}

func execMigration(ctx context.Context, sqlDB *sql.DB, name, body string) error {
	tx, err := sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, body); err != nil {
		return fmt.Errorf("storage: migration %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations(name) VALUES (?)", name,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// -----------------------------------------------------------------------
// User settings & NMT scores
// -----------------------------------------------------------------------

// UserSettings is the typed view of the JSON blob in users.settings.
// Adding fields here is a backwards-compatible change — old payloads
// missing the new fields will deserialize with their Go zero values.
type UserSettings struct {
	// Quotas lists quota codes the user wants applied (e.g. "kv1").
	Quotas []string `json:"quotas,omitempty"`
	// CreativeScorePrediction is the assumed creative-contest score
	// (used when the program requires one and the user hasn't taken it
	// yet — feeds into the calculator).
	CreativeScorePrediction int `json:"creative_score_prediction,omitempty"`

	// LastDiscoverGaluz / LastDiscoverRegions remember the user's most
	// recent "where can I get in" filter so a program opened from those
	// results can offer a "back to results" button that re-runs it (the
	// search FSM state is overwritten when a program screen opens).
	LastDiscoverGaluz   int   `json:"last_discover_galuz,omitempty"`
	LastDiscoverRegions []int `json:"last_discover_regions,omitempty"`
	// LastDiscoverContract true means the last search included contract
	// offers (default is budget-only).
	LastDiscoverContract bool `json:"last_discover_contract,omitempty"`
}

// UserNMT maps subject name → applicant's score for it. Subject names
// match the keys used by abit.Abiturient.DetailScores.
type UserNMT map[string]float64

// UpsertUser ensures the row exists (creates with defaults if not).
func (s *Store) UpsertUser(ctx context.Context, tgID int64) error {
	return s.Queries.UpsertUser(ctx, tgID)
}

// AddActivates adds delta to the user's activates counter in one write,
// creating the row if missing. Callers buffer the per-update +1s in memory
// and flush them here in batches so the single SQLite connection isn't hit
// on every update (see bot.activateTracker).
func (s *Store) AddActivates(ctx context.Context, tgID, delta int64) error {
	return s.Queries.AddActivates(ctx, db.AddActivatesParams{TgID: tgID, Activates: delta})
}

// GetUserSettings returns the typed settings; the zero value is returned
// for a non-existent user (no error).
func (s *Store) GetUserSettings(ctx context.Context, tgID int64) (UserSettings, error) {
	raw, err := s.ReadQueries.GetUserSettings(ctx, tgID)
	if errors.Is(err, sql.ErrNoRows) {
		return UserSettings{}, nil
	}
	if err != nil {
		return UserSettings{}, err
	}
	if raw == "" {
		return UserSettings{}, nil
	}
	var out UserSettings
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return UserSettings{}, fmt.Errorf("storage: settings decode: %w", err)
	}
	return out, nil
}

// SetUserSettings stores the typed settings.
func (s *Store) SetUserSettings(ctx context.Context, tgID int64, settings UserSettings) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	return s.Queries.SetUserSettings(ctx, db.SetUserSettingsParams{
		TgID: tgID, Settings: string(raw),
	})
}

// GetUserNMT returns the stored NMT scores; nil for an unknown user.
func (s *Store) GetUserNMT(ctx context.Context, tgID int64) (UserNMT, error) {
	raw, err := s.ReadQueries.GetUserNMT(ctx, tgID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	var out UserNMT
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("storage: nmt decode: %w", err)
	}
	return out, nil
}

// SetUserNMT stores the user's NMT scores.
func (s *Store) SetUserNMT(ctx context.Context, tgID int64, nmt UserNMT) error {
	raw, err := json.Marshal(nmt)
	if err != nil {
		return err
	}
	return s.Queries.SetUserNMT(ctx, db.SetUserNMTParams{
		TgID: tgID, NmtScores: string(raw),
	})
}

// -----------------------------------------------------------------------
// Saved lists (program snapshots)
// -----------------------------------------------------------------------

// SavedList is one persisted program snapshot owned by a user.
type SavedList struct {
	ID         int64
	UserTgID   int64
	Name       string
	URL        string
	Program    *abit.Program
	ShareToken string
	CreatedAt  time.Time
}

// SaveList persists a program snapshot for tgID and returns its row ID.
// A random share token is generated server-side and stored alongside the
// row — callers later use GetSavedListByToken to resolve shared deep-links
// without exposing the (predictable) numeric id.
func (s *Store) SaveList(ctx context.Context, tgID int64, name, url string, prog *abit.Program) (int64, error) {
	if prog == nil {
		return 0, errors.New("storage: nil program")
	}
	raw, err := json.Marshal(prog)
	if err != nil {
		return 0, err
	}
	token, err := newShareToken()
	if err != nil {
		return 0, fmt.Errorf("storage: token: %w", err)
	}
	return s.Queries.SaveList(ctx, db.SaveListParams{
		UserTgID: tgID, Name: name, URL: url, Data: string(raw),
		ShareToken: token,
	})
}

// GetSavedListByToken resolves a shared list by its opaque token.
// Returns ErrNotFound when no row matches.
func (s *Store) GetSavedListByToken(ctx context.Context, token string) (*SavedList, error) {
	if token == "" {
		return nil, ErrNotFound
	}
	r, err := s.ReadQueries.GetSavedListByToken(ctx, token)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	prog, err := decodeProgram(r.Data)
	if err != nil {
		return nil, err
	}
	return &SavedList{
		ID: r.ID, UserTgID: r.UserTgID,
		Name: r.Name, URL: r.URL,
		Program:    prog,
		ShareToken: r.ShareToken,
		CreatedAt:  time.Unix(r.CreatedAt, 0),
	}, nil
}

// newShareToken returns 16 cryptographically-random bytes hex-encoded
// (32 chars, ~128 bits of entropy). Matches the size of the backfill
// token from migration 0003.
func newShareToken() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

// ListSavedLists returns every saved list for tgID, newest first.
func (s *Store) ListSavedLists(ctx context.Context, tgID int64) ([]SavedList, error) {
	rows, err := s.ReadQueries.ListSavedLists(ctx, tgID)
	if err != nil {
		return nil, err
	}
	out := make([]SavedList, 0, len(rows))
	for _, r := range rows {
		prog, err := decodeProgram(r.Data)
		if err != nil {
			return nil, err
		}
		out = append(out, SavedList{
			ID: r.ID, UserTgID: r.UserTgID,
			Name: r.Name, URL: r.URL,
			Program:    prog,
			ShareToken: r.ShareToken,
			CreatedAt:  time.Unix(r.CreatedAt, 0),
		})
	}
	return out, nil
}

// GetSavedList loads a single saved list by ID.
func (s *Store) GetSavedList(ctx context.Context, id int64) (*SavedList, error) {
	r, err := s.ReadQueries.GetSavedList(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	prog, err := decodeProgram(r.Data)
	if err != nil {
		return nil, err
	}
	return &SavedList{
		ID: r.ID, UserTgID: r.UserTgID,
		Name: r.Name, URL: r.URL,
		Program:    prog,
		ShareToken: r.ShareToken,
		CreatedAt:  time.Unix(r.CreatedAt, 0),
	}, nil
}

// DeleteSavedList removes a saved list by ID.
func (s *Store) DeleteSavedList(ctx context.Context, id int64) error {
	return s.Queries.DeleteSavedList(ctx, id)
}

// UpdateSavedListProgram replaces the program snapshot of an existing
// saved list — used by /lists refresh to swap stale data with a fresh
// fetch without changing the list's id or created_at.
func (s *Store) UpdateSavedListProgram(ctx context.Context, id int64, prog *abit.Program) error {
	if prog == nil {
		return errors.New("storage: nil program")
	}
	raw, err := json.Marshal(prog)
	if err != nil {
		return err
	}
	return s.Queries.UpdateSavedListData(ctx, db.UpdateSavedListDataParams{
		ID: id, Data: string(raw),
	})
}

// -----------------------------------------------------------------------
// Caches (with TTL applied at read time)
// -----------------------------------------------------------------------

// GetProgramCache returns a cached Program if it exists and isn't older
// than ttl. Returns ErrCacheMiss / ErrCacheStale to let callers distinguish.
func (s *Store) GetProgramCache(ctx context.Context, url string, ttl time.Duration) (*abit.Program, error) {
	row, err := s.ReadQueries.GetProgramCache(ctx, url)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCacheMiss
	}
	if err != nil {
		return nil, err
	}
	if isStale(row.UpdatedAt, ttl) {
		return nil, ErrCacheStale
	}
	return decodeProgram(row.Data)
}

// PutProgramCache caches a Program under url (upsert).
func (s *Store) PutProgramCache(ctx context.Context, url string, prog *abit.Program) error {
	raw, err := json.Marshal(prog)
	if err != nil {
		return err
	}
	return s.Queries.PutProgramCache(ctx, db.PutProgramCacheParams{URL: url, Data: string(raw)})
}

// GetApplicantCache returns cached abit-poisk entries for a person.
func (s *Store) GetApplicantCache(ctx context.Context, name string, ttl time.Duration) ([]abit.ApplicantEntry, error) {
	row, err := s.ReadQueries.GetApplicantCache(ctx, name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCacheMiss
	}
	if err != nil {
		return nil, err
	}
	if isStale(row.UpdatedAt, ttl) {
		return nil, ErrCacheStale
	}
	var out []abit.ApplicantEntry
	if err := json.Unmarshal([]byte(row.Data), &out); err != nil {
		return nil, fmt.Errorf("storage: applicant decode: %w", err)
	}
	return out, nil
}

// PutApplicantCache caches abit-poisk entries for a person.
func (s *Store) PutApplicantCache(ctx context.Context, name string, entries []abit.ApplicantEntry) error {
	raw, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	return s.Queries.PutApplicantCache(ctx, db.PutApplicantCacheParams{Name: name, Data: string(raw)})
}

// VacuumCaches drops cache rows older than the given TTLs.
func (s *Store) VacuumCaches(ctx context.Context, programTTL, applicantTTL time.Duration) error {
	now := time.Now().Unix()
	if err := s.Queries.VacuumProgramCache(ctx, now-int64(programTTL.Seconds())); err != nil {
		return err
	}
	return s.Queries.VacuumApplicantCache(ctx, now-int64(applicantTTL.Seconds()))
}

// RunVacuum periodically evicts stale cache rows until ctx is cancelled.
// Without it the TTLs only gate reads — rows are never physically deleted,
// so third-party applicant names (applicant_cache) accumulate in the DB
// indefinitely. Runs one sweep immediately, then every interval. Errors are
// logged and the loop continues. Intended to be started in its own goroutine.
func (s *Store) RunVacuum(ctx context.Context, interval, programTTL, applicantTTL time.Duration, log *slog.Logger) {
	if interval <= 0 {
		interval = time.Hour
	}
	sweep := func() {
		if err := s.VacuumCaches(ctx, programTTL, applicantTTL); err != nil {
			if log != nil {
				log.WarnContext(ctx, "cache vacuum failed", "err", err)
			}
		}
	}
	sweep()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}

func decodeProgram(raw string) (*abit.Program, error) {
	var prog abit.Program
	if err := json.Unmarshal([]byte(raw), &prog); err != nil {
		return nil, fmt.Errorf("storage: program decode: %w", err)
	}
	return &prog, nil
}

func isStale(updatedAt int64, ttl time.Duration) bool {
	if ttl <= 0 {
		return false
	}
	return time.Since(time.Unix(updatedAt, 0)) > ttl
}
