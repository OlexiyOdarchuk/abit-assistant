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
	if err := p.fanOut(context.Background(), prog, srv.URL, "sid", "uid", "2025"); err != nil {
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
