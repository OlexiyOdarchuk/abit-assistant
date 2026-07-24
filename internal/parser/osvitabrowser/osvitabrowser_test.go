package osvitabrowser

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/osvita"
)

// Both drivers must satisfy osvita's fallback seam.
var (
	_ osvita.RequestsFetcher = (*Driver)(nil)
	_ osvita.RequestsFetcher = (*LocalDriver)(nil)
)

// TestResolveWS_HTTPRewritesHost checks the bare-Chrome path: /json/version is
// queried and its webSocketDebuggerUrl host is rewritten to the reachable
// sidecar host:port (Chrome reports its own bind host, unreachable elsewhere).
func TestResolveWS_HTTPRewritesHost(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/version" {
			http.NotFound(w, r)
			return
		}
		// Chrome reports 127.0.0.1:9222 — a host unreachable from other containers.
		fmt.Fprint(w, `{"webSocketDebuggerUrl":"ws://127.0.0.1:9222/devtools/browser/abc-123"}`)
	}))
	defer srv.Close()

	d := New(srv.URL)
	got, err := d.resolveWS(context.Background())
	if err != nil {
		t.Fatalf("resolveWS: %v", err)
	}
	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("returned url not parseable: %v", err)
	}
	// Host must be rewritten to the test server's host:port, path preserved.
	srvURL, _ := url.Parse(srv.URL)
	if u.Host != srvURL.Host {
		t.Errorf("ws host = %q, want %q (rewritten to reachable sidecar)", u.Host, srvURL.Host)
	}
	if u.Scheme != "ws" || u.Path != "/devtools/browser/abc-123" {
		t.Errorf("ws = %q, want scheme ws + path /devtools/browser/abc-123", got)
	}
}

// TestResolveWS_WSPassthrough checks that a ready websocket endpoint
// (browserless-style) is used verbatim, without a /json/version round-trip.
func TestResolveWS_WSPassthrough(t *testing.T) {
	const in = "ws://browser:3000/some/path"
	d := New(in)
	got, err := d.resolveWS(context.Background())
	if err != nil {
		t.Fatalf("resolveWS: %v", err)
	}
	if got != in {
		t.Errorf("ws passthrough = %q, want %q", got, in)
	}
}

// TestResolveWS_VersionError surfaces a non-200 /json/version as an error
// rather than a bogus endpoint.
func TestResolveWS_VersionError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	d := New(srv.URL)
	if _, err := d.resolveWS(context.Background()); err == nil {
		t.Fatal("expected an error from a failing /json/version")
	}
}

// TestCollectorJS_InjectsCoordinates verifies the API coordinates and the
// critical Turnstile-flow primitives are present in the injected collector.
func TestCollectorJS_InjectsCoordinates(t *testing.T) {
	js := collectorJS("2026", "1612502", "41")
	for _, want := range []string{
		`"2026"`, `"1612502"`, `"41"`, // injected as JSON string literals
		"turnstile.reset()",       // fresh token per page (single-use)
		"turnstile.getResponse()", // read the solved token
		"action: 'requests'",       // the gated API action
		"'/api/'",                  // POST target
		"data.requests_subjects",   // subjects collection
	} {
		if !strings.Contains(js, want) {
			t.Errorf("collectorJS missing %q", want)
		}
	}
}
