package osvita

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
)

// TestFanOut_RetriesFlakyEmptyURL reproduces osvita's "the first POST for an
// offset returns no url" quirk and verifies the parser retries it instead of
// treating it as end-of-data — otherwise every applicant past the first page
// is silently dropped.
func TestFanOut_RetriesFlakyEmptyURL(t *testing.T) {
	const (
		total = 1300 // 3 pages of 500 (last one partial)
		pg    = 500
	)
	var (
		mu    sync.Mutex
		posts = map[string]int{} // last → number of POSTs seen for that offset
	)

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_ = r.ParseForm()
			last := r.PostFormValue("last")
			mu.Lock()
			posts[last]++
			n := posts[last]
			mu.Unlock()
			// Flaky: the FIRST POST for any offset yields no url.
			if n == 1 {
				fmt.Fprint(w, `{"url":""}`)
				return
			}
			fmt.Fprintf(w, `{"url":%q}`, srv.URL+"/page?last="+last)
			return
		}
		// GET: return up to `pg` rows starting at `last` (empty past the end).
		last, _ := strconv.Atoi(r.URL.Query().Get("last"))
		var items []string
		for i := last; i < last+pg && i < total; i++ {
			items = append(items, fmt.Sprintf("[%d,%d,1]", 100000+i, i+1))
		}
		fmt.Fprintf(w, `{"requests":[%s],"requests_subjects":{}}`, strings.Join(items, ","))
	}))
	defer srv.Close()

	p := New(WithAPIURL(srv.URL), WithWorkers(2), WithPageSize(pg), WithMaxRetries(3))
	prog := &abit.Program{}
	if err := p.fanOut(context.Background(), prog, "https://vstup.osvita.ua/y2025/r14/uid/sid/", "sid", "uid", "2025"); err != nil {
		t.Fatalf("fanOut: %v", err)
	}
	if len(prog.Requests) != total {
		t.Fatalf("collected %d requests, want %d — flaky empty url must not truncate the lane",
			len(prog.Requests), total)
	}
	// Every offset must have been POSTed at least twice (flaky first + real).
	mu.Lock()
	defer mu.Unlock()
	for _, off := range []string{"0", "500", "1000"} {
		if posts[off] < 2 {
			t.Errorf("offset %s POSTed %d times, expected a retry after the empty url", off, posts[off])
		}
	}
}

// fakeFetcher is a RequestsFetcher double that records the coordinates it was
// called with and returns a canned result (or error).
type fakeFetcher struct {
	reqs   []abit.RawRequest
	subj   map[string]abit.ApplicantSubjects
	err    error
	called bool
	gotURL string
	gotY   string
	gotSID string
	gotUID string
}

func (f *fakeFetcher) FetchRequests(_ context.Context, programURL, year, sid, uid string) ([]abit.RawRequest, map[string]abit.ApplicantSubjects, error) {
	f.called = true
	f.gotURL, f.gotY, f.gotSID, f.gotUID = programURL, year, sid, uid
	return f.reqs, f.subj, f.err
}

// challengeServer answers every POST with osvita's Turnstile rejection (no url
// + a message), which fetchJSONURL surfaces as errChallenge.
func challengeServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"msg":"Перезавантажте сторінку! Error 316"}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestFanOut_FallbackOnChallenge verifies that when the HTTP API is gated
// behind Turnstile and a fallback is configured, the requests fetch is handed
// off to the fallback and its result populates the program.
func TestFanOut_FallbackOnChallenge(t *testing.T) {
	srv := challengeServer(t)
	fb := &fakeFetcher{
		reqs: []abit.RawRequest{{100001.0, 1.0, 1.0}, {100002.0, 2.0, 1.0}},
		subj: map[string]abit.ApplicantSubjects{"1": {"161": {170, 0, 0}}},
	}
	p := New(WithAPIURL(srv.URL), WithRequestsFallback(fb))

	prog := &abit.Program{}
	const url = "https://vstup.osvita.ua/y2026/r27/41/1612502/"
	if err := p.fanOut(context.Background(), prog, url, "1612502", "41", "2026"); err != nil {
		t.Fatalf("fanOut: %v", err)
	}
	if !fb.called {
		t.Fatal("fallback was not invoked on a challenged API")
	}
	if fb.gotURL != url || fb.gotY != "2026" || fb.gotSID != "1612502" || fb.gotUID != "41" {
		t.Errorf("fallback got (%q,%q,%q,%q), want (%q,2026,1612502,41)",
			fb.gotURL, fb.gotY, fb.gotSID, fb.gotUID, url)
	}
	if len(prog.Requests) != 2 || len(prog.RequestSubjects) != 1 {
		t.Fatalf("program not populated from fallback: %d reqs, %d subjects",
			len(prog.Requests), len(prog.RequestSubjects))
	}
}

// TestFanOut_ChallengeNoFallback confirms that without a fallback a challenged
// API still fails loudly (no silent empty program).
func TestFanOut_ChallengeNoFallback(t *testing.T) {
	srv := challengeServer(t)
	p := New(WithAPIURL(srv.URL))
	prog := &abit.Program{}
	err := p.fanOut(context.Background(), prog, "https://vstup.osvita.ua/y2026/r27/41/1612502/", "1612502", "41", "2026")
	if err == nil {
		t.Fatal("expected an error from a challenged API without a fallback")
	}
	if len(prog.Requests) != 0 {
		t.Errorf("expected empty requests, got %d", len(prog.Requests))
	}
}

// TestFanOut_NoFallbackWhenOpen ensures the fallback is NOT used when the HTTP
// API answers normally — the fast path must win when osvita is ungated.
func TestFanOut_NoFallbackWhenOpen(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			last := r.PostFormValue("last")
			fmt.Fprintf(w, `{"url":%q}`, srv.URL+"/page?last="+last)
			return
		}
		last, _ := strconv.Atoi(r.URL.Query().Get("last"))
		if last == 0 {
			fmt.Fprint(w, `{"requests":[[100001,1,1]],"requests_subjects":{}}`)
			return
		}
		fmt.Fprint(w, `{"requests":[],"requests_subjects":{}}`)
	}))
	defer srv.Close()

	fb := &fakeFetcher{}
	p := New(WithAPIURL(srv.URL), WithWorkers(1), WithRequestsFallback(fb))
	prog := &abit.Program{}
	if err := p.fanOut(context.Background(), prog, "https://vstup.osvita.ua/y2026/r27/41/1/", "1", "41", "2026"); err != nil {
		t.Fatalf("fanOut: %v", err)
	}
	if fb.called {
		t.Error("fallback was used even though the HTTP API answered normally")
	}
	if len(prog.Requests) != 1 {
		t.Errorf("expected 1 request from the HTTP path, got %d", len(prog.Requests))
	}
}
