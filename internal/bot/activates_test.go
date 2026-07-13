package bot

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
)

type fakeActivateStore struct {
	mu        sync.Mutex
	upserts   map[int64]int   // how many times UpsertUser was called per uid
	added     map[int64]int64 // total delta flushed per uid
	failUpsrt bool
	failAdd   bool
}

func newFakeActivateStore() *fakeActivateStore {
	return &fakeActivateStore{upserts: map[int64]int{}, added: map[int64]int64{}}
}

func (f *fakeActivateStore) UpsertUser(_ context.Context, tgID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failUpsrt {
		return errors.New("db busy")
	}
	f.upserts[tgID]++
	return nil
}

func (f *fakeActivateStore) AddActivates(_ context.Context, tgID, delta int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failAdd {
		return errors.New("db busy")
	}
	f.added[tgID] += delta
	return nil
}

func quietTracker(store activateStore) *activateTracker {
	return newActivateTracker(store, slog.New(slog.NewTextHandler(io.Discard, nil)), 30_000_000_000)
}

func TestActivateTracker_EnsuresRowOncePerProcess(t *testing.T) {
	store := newFakeActivateStore()
	tr := quietTracker(store)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if err := tr.track(ctx, 42); err != nil {
			t.Fatalf("track: %v", err)
		}
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.upserts[42] != 1 {
		t.Fatalf("want exactly 1 upsert for the user, got %d", store.upserts[42])
	}
}

func TestActivateTracker_BuffersAndFlushesDeltas(t *testing.T) {
	store := newFakeActivateStore()
	tr := quietTracker(store)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_ = tr.track(ctx, 1)
	}
	for i := 0; i < 2; i++ {
		_ = tr.track(ctx, 2)
	}

	// Nothing written to the counter before a flush.
	store.mu.Lock()
	if len(store.added) != 0 {
		store.mu.Unlock()
		t.Fatalf("counter should not be written before flush, got %v", store.added)
	}
	store.mu.Unlock()

	tr.flush(ctx)

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.added[1] != 3 || store.added[2] != 2 {
		t.Fatalf("want {1:3, 2:2}, got %v", store.added)
	}
}

func TestActivateTracker_FirstTrackFailsWhenUpsertFails(t *testing.T) {
	store := newFakeActivateStore()
	store.failUpsrt = true
	tr := quietTracker(store)

	if err := tr.track(context.Background(), 7); err == nil {
		t.Fatal("want error when the FK-target upsert fails")
	}
}

func TestActivateTracker_RequeuesOnFlushFailure(t *testing.T) {
	store := newFakeActivateStore()
	tr := quietTracker(store)
	ctx := context.Background()

	_ = tr.track(ctx, 9)
	_ = tr.track(ctx, 9)

	store.failAdd = true
	tr.flush(ctx) // fails, should re-queue

	store.failAdd = false
	tr.flush(ctx) // succeeds with the re-queued delta

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.added[9] != 2 {
		t.Fatalf("want re-queued delta of 2 after recovery, got %d", store.added[9])
	}
}
