package desktop

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/storage"
)

func TestCache_ProgramRoundTrip(t *testing.T) {
	c := openTestCache(t)
	ctx := context.Background()
	const url = "https://vstup.osvita.ua/y2026/r27/41/1612502/"

	// Miss before write.
	if _, err := c.GetProgramCache(ctx, url, time.Hour); !errors.Is(err, storage.ErrCacheMiss) {
		t.Fatalf("empty cache: got %v, want ErrCacheMiss", err)
	}

	want := &abit.Program{ProgramName: "Інженерія ПЗ", Requests: []abit.RawRequest{{100001.0, 1.0}}}
	if err := c.PutProgramCache(ctx, url, want); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, err := c.GetProgramCache(ctx, url, time.Hour)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ProgramName != want.ProgramName || len(got.Requests) != 1 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	// Upsert overwrites.
	want.ProgramName = "Оновлено"
	if err := c.PutProgramCache(ctx, url, want); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, _ = c.GetProgramCache(ctx, url, time.Hour)
	if got.ProgramName != "Оновлено" {
		t.Fatalf("upsert not applied: %q", got.ProgramName)
	}
}

func TestCache_ProgramStale(t *testing.T) {
	c := openTestCache(t)
	ctx := context.Background()
	const url = "https://vstup.osvita.ua/y2026/r27/41/1/"
	if err := c.PutProgramCache(ctx, url, &abit.Program{ProgramName: "x"}); err != nil {
		t.Fatal(err)
	}
	// A zero/negative window means "fresh forever".
	if _, err := c.GetProgramCache(ctx, url, 0); err != nil {
		t.Fatalf("ttl<=0 should never be stale, got %v", err)
	}
	// A 1ns TTL makes any existing row stale.
	time.Sleep(2 * time.Millisecond)
	if _, err := c.GetProgramCache(ctx, url, time.Nanosecond); !errors.Is(err, storage.ErrCacheStale) {
		t.Fatalf("expected ErrCacheStale, got %v", err)
	}
}

func TestCache_ApplicantRoundTrip(t *testing.T) {
	c := openTestCache(t)
	ctx := context.Background()
	const name = "Іваненко І. І."
	if _, err := c.GetApplicantCache(ctx, name, time.Hour); !errors.Is(err, storage.ErrCacheMiss) {
		t.Fatalf("empty: got %v, want ErrCacheMiss", err)
	}
	want := []abit.ApplicantEntry{{FullName: "Іваненко Іван Іванович", TotalScore: "180.5"}}
	if err := c.PutApplicantCache(ctx, name, want); err != nil {
		t.Fatalf("put: %v", err)
	}
	got, err := c.GetApplicantCache(ctx, name, time.Hour)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got) != 1 || got[0].TotalScore != "180.5" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func openTestCache(t *testing.T) *Cache {
	t.Helper()
	c, err := OpenCache(":memory:")
	if err != nil {
		t.Fatalf("OpenCache: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}
