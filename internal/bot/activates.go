package bot

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// activateStore is the subset of storage.Store the tracker needs.
type activateStore interface {
	UpsertUser(ctx context.Context, tgID int64) error
	AddActivates(ctx context.Context, tgID, delta int64) error
}

// activateTracker keeps the per-update "activates" counter off the hot path.
//
// Previously trackUser ran an upsert-and-increment on every single update,
// serialized through the one SQLite connection and fatal on failure — so
// under peak load (results day) the write queue outran busy_timeout and every
// user got "server busy" before any real work happened.
//
// Instead we: (1) ensure a user's row exists exactly once per process (the
// foreign-key target for downstream FSM/saved-list writes), and (2) buffer the
// +1s in memory and flush the accumulated deltas in batches on a ticker. The
// DB is touched once per new user plus once per active user per flush interval,
// not once per update.
type activateTracker struct {
	store    activateStore
	log      *slog.Logger
	interval time.Duration

	mu      sync.Mutex
	seen    map[int64]struct{} // users whose row we've ensured this process
	pending map[int64]int64    // buffered activates awaiting flush
}

func newActivateTracker(store activateStore, log *slog.Logger, interval time.Duration) *activateTracker {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &activateTracker{
		store:    store,
		log:      log,
		interval: interval,
		seen:     make(map[int64]struct{}),
		pending:  make(map[int64]int64),
	}
}

// track records one activation for uid. The first time a uid is seen this
// process it ensures the user row exists synchronously (a hard failure here is
// returned to the caller, because downstream writes have a FK to users). On
// every subsequent update it only bumps an in-memory counter and returns nil —
// no DB round-trip on the hot path.
func (t *activateTracker) track(ctx context.Context, uid int64) error {
	t.mu.Lock()
	_, known := t.seen[uid]
	if known {
		t.pending[uid]++
		t.mu.Unlock()
		return nil
	}
	t.mu.Unlock()

	// First contact this process: guarantee the FK target exists.
	if err := t.store.UpsertUser(ctx, uid); err != nil {
		return err
	}

	t.mu.Lock()
	t.seen[uid] = struct{}{}
	t.pending[uid]++
	t.mu.Unlock()
	return nil
}

// run flushes buffered counters on a ticker until ctx is cancelled, then
// performs one final flush so a graceful shutdown doesn't drop counts.
func (t *activateTracker) run(ctx context.Context) {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			// Detach: ctx is already cancelled, but we still want the
			// final write to land.
			t.flush(context.WithoutCancel(ctx))
			return
		case <-ticker.C:
			t.flush(ctx)
		}
	}
}

// flush drains the pending counters and writes each user's accumulated delta.
// On write failure the delta is put back so it's retried on the next tick.
func (t *activateTracker) flush(ctx context.Context) {
	t.mu.Lock()
	if len(t.pending) == 0 {
		t.mu.Unlock()
		return
	}
	batch := t.pending
	t.pending = make(map[int64]int64)
	t.mu.Unlock()

	for uid, delta := range batch {
		if delta <= 0 {
			continue
		}
		if err := t.store.AddActivates(ctx, uid, delta); err != nil {
			t.log.Warn("activates flush failed", "err", err, "user_id", uid, "delta", delta)
			// Re-queue so the count isn't lost.
			t.mu.Lock()
			t.pending[uid] += delta
			t.mu.Unlock()
		}
	}
}
