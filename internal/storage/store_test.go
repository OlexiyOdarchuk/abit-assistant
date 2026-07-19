package storage_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/storage"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/storage/pgtest"
)

// newStore returns a Store on a throwaway Postgres database (see pgtest).
func newStore(t *testing.T) *storage.Store {
	t.Helper()
	return pgtest.New(t)
}

// TestConcurrentReadsAndReadAfterWrite: a committed write is visible to a
// later read, and many concurrent reads all succeed (Postgres MVCC).
func TestConcurrentReadsAndReadAfterWrite(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	if err := s.SetUserNMT(ctx, 1, storage.UserNMT{"Математика": 180}); err != nil {
		t.Fatal(err)
	}

	const readers = 32
	var wg sync.WaitGroup
	errs := make(chan error, readers)
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			nmt, err := s.GetUserNMT(ctx, 1)
			if err != nil {
				errs <- err
				return
			}
			if nmt["Математика"] != 180 {
				errs <- errors.New("write not visible to read pool")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent read: %v", err)
	}
}

func TestOpen_AppliesMigrations(t *testing.T) {
	s := newStore(t)

	// schema_migrations table should exist and have one row.
	var count int
	if err := s.DB.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("expected at least one migration applied")
	}
}

func TestUpsertUser_Idempotent(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	if err := s.UpsertUser(ctx, 42); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertUser(ctx, 42); err != nil {
		t.Fatal(err)
	}
	got, err := s.Queries.CountUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 {
		t.Errorf("CountUsers = %d, want 1", got)
	}
}

func TestUserSettings_Roundtrip(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	want := storage.UserSettings{
		Quotas:                  []string{"kv1", "kv2"},
		CreativeScorePrediction: 150,
	}
	if err := s.SetUserSettings(ctx, 42, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetUserSettings(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	if got.CreativeScorePrediction != 150 || len(got.Quotas) != 2 {
		t.Errorf("got %+v", got)
	}

	// Unknown user returns zero value, no error.
	zero, err := s.GetUserSettings(ctx, 999)
	if err != nil {
		t.Fatal(err)
	}
	if zero.CreativeScorePrediction != 0 {
		t.Errorf("unknown user should be zero value: %+v", zero)
	}
}

func TestUserNMT_Roundtrip(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	want := storage.UserNMT{"Українська мова": 170, "Математика": 180}
	if err := s.SetUserNMT(ctx, 42, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetUserNMT(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	if got["Українська мова"] != 170 || got["Математика"] != 180 {
		t.Errorf("got %+v", got)
	}
}

func TestIncrementActivates_RaceSafe(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	const N = 50
	var wg sync.WaitGroup
	wg.Add(N)
	for range N {
		go func() {
			defer wg.Done()
			_ = s.Queries.IncrementActivates(ctx, 42)
		}()
	}
	wg.Wait()

	user, err := s.Queries.GetUser(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	if user.Activates != N {
		t.Errorf("activates = %d, want %d (race?)", user.Activates, N)
	}
}

func TestSavedList_Roundtrip(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	if err := s.UpsertUser(ctx, 42); err != nil {
		t.Fatal(err)
	}
	prog := &abit.Program{
		UniversityName: "ЛНУ ім. І. Франка",
		ProgramName:    "Маркетинг",
		SpecCode:       "D5",
		EB:             40, OKR: 1, K4Max: 0.35, RK: 1.0,
	}
	id, err := s.SaveList(ctx, 42, "ЛНУ Маркетинг", "https://example/", prog)
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Error("expected non-zero ID")
	}

	got, err := s.GetSavedList(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Program.UniversityName != prog.UniversityName ||
		got.Program.ProgramName != prog.ProgramName ||
		got.Program.SpecCode != prog.SpecCode {
		t.Errorf("Program mismatch: %+v", got.Program)
	}

	all, err := s.ListSavedLists(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Errorf("ListSavedLists len = %d, want 1", len(all))
	}

	if err := s.DeleteSavedList(ctx, id); err != nil {
		t.Fatal(err)
	}
	all, _ = s.ListSavedLists(ctx, 42)
	if len(all) != 0 {
		t.Errorf("after delete, len = %d, want 0", len(all))
	}
}

func TestSavedList_CascadeDelete(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	if err := s.UpsertUser(ctx, 42); err != nil {
		t.Fatal(err)
	}
	prog := &abit.Program{UniversityName: "X"}
	if _, err := s.SaveList(ctx, 42, "n", "u", prog); err != nil {
		t.Fatal(err)
	}
	// Deleting the user should cascade to saved_lists.
	if _, err := s.DB.ExecContext(ctx, "DELETE FROM users WHERE tg_id = $1", 42); err != nil {
		t.Fatal(err)
	}
	all, _ := s.ListSavedLists(ctx, 42)
	if len(all) != 0 {
		t.Errorf("cascade delete failed: %d lists remain", len(all))
	}
}

func TestProgramCache_FreshAndStale(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	url := "https://vstup.osvita.ua/y2025/r14/282/1471029/"
	prog := &abit.Program{UniversityName: "ЛНУ", ProgramName: "Маркетинг"}

	if _, err := s.GetProgramCache(ctx, url, time.Minute); !errors.Is(err, storage.ErrCacheMiss) {
		t.Errorf("expected ErrCacheMiss, got %v", err)
	}

	if err := s.PutProgramCache(ctx, url, prog); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetProgramCache(ctx, url, time.Minute)
	if err != nil {
		t.Fatalf("fresh: %v", err)
	}
	if got.UniversityName != "ЛНУ" {
		t.Errorf("got %+v", got)
	}

	// TTL = 1 nanosecond — entry should be stale.
	if _, err := s.GetProgramCache(ctx, url, time.Nanosecond); !errors.Is(err, storage.ErrCacheStale) {
		t.Errorf("expected ErrCacheStale, got %v", err)
	}
}

func TestApplicantCache_FreshAndStale(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	name := "Бовкун О В"
	entries := []abit.ApplicantEntry{{Degree: "Б", FullName: name, University: "ЛНУ"}}

	if _, err := s.GetApplicantCache(ctx, name, time.Minute); !errors.Is(err, storage.ErrCacheMiss) {
		t.Errorf("expected ErrCacheMiss, got %v", err)
	}
	if err := s.PutApplicantCache(ctx, name, entries); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetApplicantCache(ctx, name, time.Minute)
	if err != nil {
		t.Fatalf("fresh: %v", err)
	}
	if len(got) != 1 || got[0].FullName != name {
		t.Errorf("got %+v", got)
	}
}

func TestVacuumCaches(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()

	if err := s.PutProgramCache(ctx, "u1", &abit.Program{}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutApplicantCache(ctx, "name1", nil); err != nil {
		t.Fatal(err)
	}
	// Vacuum with negative TTL drops everything (now-(-1s) > all updated_at).
	if err := s.VacuumCaches(ctx, -time.Second, -time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetProgramCache(ctx, "u1", time.Hour); !errors.Is(err, storage.ErrCacheMiss) {
		t.Errorf("expected ErrCacheMiss after vacuum, got %v", err)
	}
	if _, err := s.GetApplicantCache(ctx, "name1", time.Hour); !errors.Is(err, storage.ErrCacheMiss) {
		t.Errorf("expected ErrCacheMiss after vacuum, got %v", err)
	}
}

func TestRunVacuum_ImmediateSweepThenStops(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	if err := s.PutApplicantCache(ctx, "name1", nil); err != nil {
		t.Fatal(err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		// Negative TTLs make the immediate sweep drop everything.
		s.RunVacuum(runCtx, time.Hour, -time.Second, -time.Second, nil)
		close(done)
	}()

	// The immediate sweep should evict the row shortly after start.
	evicted := false
	for i := 0; i < 100; i++ {
		if _, err := s.GetApplicantCache(ctx, "name1", time.Hour); errors.Is(err, storage.ErrCacheMiss) {
			evicted = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !evicted {
		t.Error("expected the immediate sweep to evict the row")
	}

	// Cancellation stops the loop.
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("RunVacuum did not return after ctx cancel")
	}
}
