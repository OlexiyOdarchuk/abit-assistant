// Package httpx provides an http.RoundTripper that protects a single
// upstream host from our own traffic: a token-bucket rate limiter paces
// outgoing requests, and a circuit breaker fails fast once the host starts
// returning errors instead of hammering it (and amplifying the outage via
// retries).
package httpx

import (
	"errors"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ErrCircuitOpen is returned by a Gate when the breaker is open: the upstream
// recently failed repeatedly, so we shed load instead of piling on. Callers
// see it as a normal RoundTrip error.
var ErrCircuitOpen = errors.New("httpx: upstream circuit open")

// Limits configures a Gate.
type Limits struct {
	// RPS is the sustained request rate to the host (requests per second).
	RPS float64
	// Burst is the token-bucket depth — how many requests may fire back to
	// back before pacing kicks in. Should be ≥ the caller's fan-out width.
	Burst int
	// FailThreshold is how many consecutive failures (transport error, 429,
	// or 5xx) trip the breaker open. 0 disables the breaker.
	FailThreshold int
	// OpenFor is how long the breaker stays open before allowing a probe.
	OpenFor time.Duration
}

// Gate is a rate-limiting, circuit-breaking http.RoundTripper for one host.
// Construct with NewGate and install as a client's Transport.
type Gate struct {
	base    http.RoundTripper
	limiter *rate.Limiter
	limits  Limits

	mu          sync.Mutex
	consecFails int
	openUntil   time.Time
}

// NewGate wraps base (nil → http.DefaultTransport) with the given limits.
func NewGate(base http.RoundTripper, l Limits) *Gate {
	if base == nil {
		base = http.DefaultTransport
	}
	if l.RPS <= 0 {
		l.RPS = 10
	}
	if l.Burst <= 0 {
		l.Burst = 1
	}
	return &Gate{
		base:    base,
		limiter: rate.NewLimiter(rate.Limit(l.RPS), l.Burst),
		limits:  l,
	}
}

// RoundTrip implements http.RoundTripper.
func (g *Gate) RoundTrip(req *http.Request) (*http.Response, error) {
	if !g.allow() {
		return nil, ErrCircuitOpen
	}
	// Pace against the token bucket, respecting the request's context so a
	// cancelled/timed-out caller doesn't sit in the queue.
	if err := g.limiter.Wait(req.Context()); err != nil {
		return nil, err
	}
	resp, err := g.base.RoundTrip(req)
	g.record(err, resp)
	return resp, err
}

// allow reports whether a request may proceed. Closed → yes; open and still
// cooling down → no; open but cooldown elapsed → yes (a half-open probe,
// whose outcome record() uses to re-close or re-open the breaker).
func (g *Gate) allow() bool {
	if g.limits.FailThreshold <= 0 {
		return true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.openUntil.IsZero() {
		return true
	}
	return !time.Now().Before(g.openUntil)
}

// record folds a request outcome into the breaker state.
func (g *Gate) record(err error, resp *http.Response) {
	if g.limits.FailThreshold <= 0 {
		return
	}
	failed := err != nil || (resp != nil && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500))

	g.mu.Lock()
	defer g.mu.Unlock()
	if !failed {
		g.consecFails = 0
		g.openUntil = time.Time{}
		return
	}
	g.consecFails++
	if g.consecFails >= g.limits.FailThreshold {
		openFor := g.limits.OpenFor
		if openFor <= 0 {
			openFor = 10 * time.Second
		}
		g.openUntil = time.Now().Add(openFor)
	}
}
