package osvitabrowser

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestLive_FetchRequests exercises the full browser chain against a real
// Chromium sidecar and the live osvita site. It is opt-in: set
// OSVITA_LIVE_BROWSER_URL to the sidecar DevTools endpoint (e.g.
// http://localhost:9222) to run it. OSVITA_LIVE_URL overrides the target
// program (default: a known 2026 program).
//
//	docker run -d --rm --name chrome -p 9222:9222 zenika/alpine-chrome \
//	  --headless --no-sandbox --disable-gpu --disable-dev-shm-usage \
//	  --remote-debugging-address=0.0.0.0 --remote-debugging-port=9222 \
//	  --remote-allow-origins=* about:blank
//	OSVITA_LIVE_BROWSER_URL=http://localhost:9222 \
//	  go test -run TestLive_FetchRequests -v ./internal/parser/osvitabrowser/
func TestLive_FetchRequests(t *testing.T) {
	base := os.Getenv("OSVITA_LIVE_BROWSER_URL")
	if base == "" {
		t.Skip("set OSVITA_LIVE_BROWSER_URL to run the live browser smoke test")
	}
	target := os.Getenv("OSVITA_LIVE_URL")
	if target == "" {
		target = "https://vstup.osvita.ua/y2026/r27/41/1612502/"
	}

	d := New(base)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Coordinates for the default target: year=2026, sid=1612502, uid=41.
	year, sid, uid := envOr("OSVITA_LIVE_Y", "2026"), envOr("OSVITA_LIVE_SID", "1612502"), envOr("OSVITA_LIVE_UID", "41")
	reqs, subj, err := d.FetchRequests(ctx, target, year, sid, uid)
	if err != nil {
		t.Fatalf("FetchRequests: %v", err)
	}
	t.Logf("live: %d requests, %d subject rows", len(reqs), len(subj))
	if len(reqs) == 0 {
		t.Fatal("expected a non-empty applicant list from the live site")
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
