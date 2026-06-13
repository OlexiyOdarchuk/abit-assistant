package web

import (
	"strings"
	"testing"
)

// TestStaticServing checks the embedded Svelte build is served, with SPA
// deep-link fallback, and that the API isn't shadowed by the catch-all.
func TestStaticServing(t *testing.T) {
	srv := newTestServer(t)

	// Root serves the built index.html (mounts the SPA into #app).
	root := do(t, srv, "GET", "/", "")
	if root.Code != 200 {
		t.Fatalf("GET / code = %d", root.Code)
	}
	if !strings.Contains(root.Body.String(), `id="app"`) {
		t.Errorf("GET / did not serve the SPA index (no #app mount): %q", trunc(root.Body.String(), 120))
	}

	// A client-side route that isn't a real asset falls back to index.html.
	deep := do(t, srv, "GET", "/discover", "")
	if deep.Code != 200 || !strings.Contains(deep.Body.String(), `id="app"`) {
		t.Errorf("deep link /discover did not fall back to SPA index (code %d)", deep.Code)
	}

	// The API still resolves (catch-all must not swallow /api/*).
	if h := do(t, srv, "GET", "/api/health", ""); h.Code != 200 {
		t.Errorf("GET /api/health code = %d, want 200", h.Code)
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
