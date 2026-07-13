package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// stubRT is a programmable http.RoundTripper.
type stubRT struct {
	calls  atomic.Int64
	status int
	err    error
}

func (s *stubRT) RoundTrip(req *http.Request) (*http.Response, error) {
	s.calls.Add(1)
	if s.err != nil {
		return nil, s.err
	}
	rec := httptest.NewRecorder()
	rec.WriteHeader(s.status)
	return rec.Result(), nil
}

func newReq(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://x/", nil)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func TestGate_PacesRequests(t *testing.T) {
	base := &stubRT{status: 200}
	// 20 rps, burst 1 → the 2nd and 3rd requests each wait ~50ms.
	g := NewGate(base, Limits{RPS: 20, Burst: 1})

	start := time.Now()
	for range 3 {
		resp, err := g.RoundTrip(newReq(t))
		if err != nil {
			t.Fatalf("RoundTrip: %v", err)
		}
		_ = resp.Body.Close()
	}
	elapsed := time.Since(start)
	// Two inter-request gaps of ~50ms = ~100ms. Allow slack but require
	// that pacing actually happened (well above zero).
	if elapsed < 80*time.Millisecond {
		t.Errorf("expected pacing to add ~100ms, got %v", elapsed)
	}
}

func TestGate_BreakerOpensAndFailsFast(t *testing.T) {
	base := &stubRT{status: 500}
	g := NewGate(base, Limits{RPS: 1000, Burst: 100, FailThreshold: 3, OpenFor: time.Minute})

	// Three 500s trip the breaker.
	for range 3 {
		resp, err := g.RoundTrip(newReq(t))
		if err != nil {
			t.Fatalf("unexpected error before open: %v", err)
		}
		_ = resp.Body.Close()
	}
	callsBefore := base.calls.Load()

	// Next call short-circuits: ErrCircuitOpen, base NOT hit.
	_, err := g.RoundTrip(newReq(t))
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("want ErrCircuitOpen, got %v", err)
	}
	if base.calls.Load() != callsBefore {
		t.Errorf("breaker open should not call base transport")
	}
}

func TestGate_BreakerRecoversAfterCooldown(t *testing.T) {
	base := &stubRT{status: 500}
	g := NewGate(base, Limits{RPS: 1000, Burst: 100, FailThreshold: 2, OpenFor: 30 * time.Millisecond})

	for range 2 {
		resp, err := g.RoundTrip(newReq(t))
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}
	if _, err := g.RoundTrip(newReq(t)); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("want open, got %v", err)
	}

	// After cooldown, a probe is allowed; make it succeed → breaker closes.
	time.Sleep(40 * time.Millisecond)
	base.status = 200
	resp, err := g.RoundTrip(newReq(t))
	if err != nil {
		t.Fatalf("probe after cooldown: %v", err)
	}
	_ = resp.Body.Close()

	// Breaker is closed: subsequent success still passes.
	if _, err := g.RoundTrip(newReq(t)); err != nil {
		t.Fatalf("after recovery: %v", err)
	}
}

func TestGate_RespectsContextCancellation(t *testing.T) {
	base := &stubRT{status: 200}
	// RPS 1, burst 1: the second request must wait ~1s, but we cancel first.
	g := NewGate(base, Limits{RPS: 1, Burst: 1})

	resp, err := g.RoundTrip(newReq(t)) // consumes the burst token
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://x/", nil)
	if _, err := g.RoundTrip(req); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled while paced, got %v", err)
	}
}

func TestGate_BreakerDisabledWhenThresholdZero(t *testing.T) {
	base := &stubRT{status: 500}
	g := NewGate(base, Limits{RPS: 1000, Burst: 100}) // FailThreshold 0

	for range 20 {
		resp, err := g.RoundTrip(newReq(t))
		if err != nil {
			t.Fatalf("breaker should be disabled: %v", err)
		}
		_ = resp.Body.Close()
	}
}
