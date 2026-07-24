// Package desktop assembles the AbitAssistant core (internal/abit, service,
// parser) into a local, single-user application backend for the Wails desktop
// build. It swaps the server's Postgres cache for a local SQLite file and the
// Turnstile-gated osvita HTTP source for a local headful-browser driver — the
// services themselves are reused unchanged.
package desktop

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (no CGO → clean cross-compile)
)

// Cache is a local SQLite implementation of service.ProgramCache and
// service.ApplicantCache. It persists JSON snapshots across app restarts —
// important because a single osvita fetch costs ~20s (browser + Turnstile), so
// re-fetching everything on every launch would be painful.
//
// It mirrors the Postgres store's contract exactly: entries carry a Unix
// updated_at, reads past the TTL return abit.ErrCacheStale, and absent keys
// return abit.ErrCacheMiss — so the reused services' errors.Is checks work
// identically against either backend.
type Cache struct {
	db *sql.DB
}

// Compile-time proof the local cache satisfies the seams the services expect.
var (
	_ service.ProgramCache   = (*Cache)(nil)
	_ service.ApplicantCache = (*Cache)(nil)
)

// OpenCache opens (creating if needed) a SQLite cache at path. Use ":memory:"
// for an ephemeral cache.
func OpenCache(path string) (*Cache, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("desktop cache: open %q: %w", path, err)
	}
	// Single-user desktop app: one writer at a time. WAL keeps reads
	// non-blocking; busy_timeout avoids spurious "database is locked".
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.ExecContext(context.Background(), pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("desktop cache: %s: %w", pragma, err)
		}
	}
	if err := createSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Cache{db: db}, nil
}

// Close releases the underlying database.
func (c *Cache) Close() error { return c.db.Close() }

func createSchema(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS program_cache (
	url        TEXT PRIMARY KEY,
	data       TEXT NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS applicant_cache (
	name       TEXT PRIMARY KEY,
	data       TEXT NOT NULL,
	updated_at INTEGER NOT NULL
);`
	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		return fmt.Errorf("desktop cache: schema: %w", err)
	}
	return nil
}

// GetProgramCache returns the cached program for url, or abit.ErrCacheMiss /
// abit.ErrCacheStale.
func (c *Cache) GetProgramCache(ctx context.Context, url string, ttl time.Duration) (*abit.Program, error) {
	var data string
	var updatedAt int64
	err := c.db.QueryRowContext(ctx, `SELECT data, updated_at FROM program_cache WHERE url = ?`, url).
		Scan(&data, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, abit.ErrCacheMiss
		}
		return nil, err
	}
	if isStale(updatedAt, ttl) {
		return nil, abit.ErrCacheStale
	}
	var prog abit.Program
	if err := json.Unmarshal([]byte(data), &prog); err != nil {
		return nil, fmt.Errorf("desktop cache: program decode: %w", err)
	}
	return &prog, nil
}

// PutProgramCache upserts the program snapshot under url.
func (c *Cache) PutProgramCache(ctx context.Context, url string, prog *abit.Program) error {
	raw, err := json.Marshal(prog)
	if err != nil {
		return err
	}
	_, err = c.db.ExecContext(ctx,
		`INSERT INTO program_cache (url, data, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(url) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at`,
		url, string(raw), time.Now().Unix())
	return err
}

// GetApplicantCache returns cached abit-poisk entries for a person, or the
// miss/stale sentinels.
func (c *Cache) GetApplicantCache(ctx context.Context, name string, ttl time.Duration) ([]abit.ApplicantEntry, error) {
	var data string
	var updatedAt int64
	err := c.db.QueryRowContext(ctx, `SELECT data, updated_at FROM applicant_cache WHERE name = ?`, name).
		Scan(&data, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, abit.ErrCacheMiss
		}
		return nil, err
	}
	if isStale(updatedAt, ttl) {
		return nil, abit.ErrCacheStale
	}
	var out []abit.ApplicantEntry
	if err := json.Unmarshal([]byte(data), &out); err != nil {
		return nil, fmt.Errorf("desktop cache: applicant decode: %w", err)
	}
	return out, nil
}

// PutApplicantCache upserts abit-poisk entries for a person.
func (c *Cache) PutApplicantCache(ctx context.Context, name string, entries []abit.ApplicantEntry) error {
	raw, err := json.Marshal(entries)
	if err != nil {
		return err
	}
	_, err = c.db.ExecContext(ctx,
		`INSERT INTO applicant_cache (name, data, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at`,
		name, string(raw), time.Now().Unix())
	return err
}

// isStale mirrors storage.isStale: ttl<=0 never expires.
func isStale(updatedAt int64, ttl time.Duration) bool {
	if ttl <= 0 {
		return false
	}
	return time.Since(time.Unix(updatedAt, 0)) > ttl
}
