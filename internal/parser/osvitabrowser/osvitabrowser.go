// Package osvitabrowser is a browser-backed osvita.RequestsFetcher: it fetches
// the applicant-requests half of a vstup.osvita.ua program by driving a real
// (remote, headless) Chromium that solves osvita's Cloudflare Turnstile
// challenge.
//
// Why a browser at all. Since 2026 osvita gates its applicant API
// (POST /api/ action=requests) behind a Turnstile token: without a valid
// `token` form field the server answers "Перезавантажте сторінку! Error 316"
// instead of data. The token is produced by a Turnstile widget rendered on the
// program page, and — critically — it is SINGLE-USE: one fresh token buys
// exactly one successful POST, after which the server reports "Сесія
// застаріла". So the browser must stay in the loop for every page, calling
// turnstile.reset() and awaiting a new token before each request.
//
// What runs where. The whole pagination loop (reset → token → POST →
// optional signed-url GET → collect) runs IN THE PAGE via one injected async
// function, so it inherits the page's origin, cookies, and Turnstile widget
// exactly as a real visitor would. Go only connects to the remote browser,
// navigates, and decodes the collected rows into abit types — reusing the
// same abit.RawRequest / abit.ApplicantSubjects shapes the HTTP driver emits.
//
// The static program page is NOT gated, so osvita's HTTP driver still scrapes
// it directly; this package supplies only the requests step it can't reach.
package osvitabrowser

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// Driver connects to a remote Chromium (a sidecar) over the DevTools protocol
// and drives osvita's Turnstile-gated API. It is safe for concurrent use: each
// FetchRequests opens its own isolated browser context (tab).
type Driver struct {
	base       string        // sidecar URL: http(s)://host:port or ws(s)://…
	resolveTO  time.Duration // budget for discovering the CDP websocket endpoint
	log        *slog.Logger
	httpClient *http.Client
}

// Option configures a Driver.
type Option func(*Driver)

// WithLogger attaches a logger (used for progress/diagnostics).
func WithLogger(l *slog.Logger) Option { return func(d *Driver) { d.log = l } }

// New builds a Driver talking to the Chromium sidecar at browserURL. The URL
// may be an HTTP DevTools endpoint (http://chromium:9222 — the bare-Chrome /
// alpine-chrome case, resolved via /json/version) or a ready websocket URL
// (ws://host:3000, the browserless case), used as-is.
func New(browserURL string, opts ...Option) *Driver {
	d := &Driver{
		base:       strings.TrimRight(browserURL, "/"),
		resolveTO:  10 * time.Second,
		log:        slog.Default(),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// browserResult is the JSON shape the injected page function returns. Subjects
// are normalised to an object in-page (osvita sends [] instead of {} when
// empty), so a plain map decodes cleanly here.
type browserResult struct {
	Requests []abit.RawRequest                 `json:"requests"`
	Subjects map[string]abit.ApplicantSubjects `json:"requests_subjects"`
	Pages    int                               `json:"pages"`
	Err      string                            `json:"error"`
}

// FetchRequests implements osvita.RequestsFetcher. It renders programURL in the
// remote browser and returns every applicant request plus the per-applicant
// subjects map. The (year, sid, uid) triple addresses the same API the HTTP
// driver uses.
func (d *Driver) FetchRequests(ctx context.Context, programURL, year, sid, uid string) ([]abit.RawRequest, map[string]abit.ApplicantSubjects, error) {
	wsURL, err := d.resolveWS(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve devtools endpoint: %w", err)
	}

	allocCtx, cancelAlloc := chromedp.NewRemoteAllocator(ctx, wsURL)
	defer cancelAlloc()
	tabCtx, cancelTab := chromedp.NewContext(allocCtx)
	defer cancelTab()

	js := collectorJS(year, sid, uid)
	var res browserResult
	start := time.Now()
	err = chromedp.Run(tabCtx,
		chromedp.Navigate(programURL),
		chromedp.Evaluate(js, &res, func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
			return p.WithAwaitPromise(true)
		}),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("browser run: %w", err)
	}
	if res.Err != "" {
		return nil, nil, fmt.Errorf("osvita page: %s", res.Err)
	}
	d.log.InfoContext(ctx, "osvitabrowser: fetched requests",
		"url", programURL, "requests", len(res.Requests), "pages", res.Pages,
		"took", time.Since(start).Round(time.Millisecond))
	if res.Subjects == nil {
		res.Subjects = map[string]abit.ApplicantSubjects{}
	}
	return res.Requests, res.Subjects, nil
}

// resolveWS returns a websocket DevTools URL to hand chromedp. A ws(s):// base
// is used verbatim (browserless-style). An http(s):// base is a bare Chrome
// DevTools endpoint: we query /json/version for its webSocketDebuggerUrl.
//
// Two Docker-networking gotchas are handled here:
//   - Chrome rejects DevTools connections whose Host header is a DNS name that
//     is not "localhost" (its DNS-rebinding guard). So we resolve the sidecar
//     hostname to an IP and address it by IP — an IP Host header is accepted.
//   - The webSocketDebuggerUrl Chrome reports carries its own bind host
//     (often 127.0.0.1), unreachable from another container; we rewrite its
//     host to the sidecar IP:port we actually reached.
func (d *Driver) resolveWS(ctx context.Context) (string, error) {
	u, err := url.Parse(d.base)
	if err != nil {
		return "", fmt.Errorf("bad browser url %q: %w", d.base, err)
	}
	if u.Scheme == "ws" || u.Scheme == "wss" {
		return d.base, nil // ready endpoint (browserless) — use as-is
	}

	ctx, cancel := context.WithTimeout(ctx, d.resolveTO)
	defer cancel()

	host, port := u.Hostname(), u.Port()
	if port == "" {
		port = "9222"
	}
	// Resolve to an IP so Chrome's Host-header guard accepts the connection.
	// Prefer IPv4: Chrome's --remote-debugging-address=0.0.0.0 binds IPv4, and
	// a hostname like "localhost" often resolves to ::1 first, which Chrome
	// isn't listening on (connection reset).
	addr := host
	if ips, lerr := net.DefaultResolver.LookupIPAddr(ctx, host); lerr == nil && len(ips) > 0 {
		addr = ips[0].IP.String()
		for _, ip := range ips {
			if ip.IP.To4() != nil {
				addr = ip.IP.String()
				break
			}
		}
	}
	hostPort := net.JoinHostPort(addr, port)

	versionURL := (&url.URL{Scheme: "http", Host: hostPort, Path: "/json/version"}).String()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, versionURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("GET %s: %w", versionURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: http %d", versionURL, resp.StatusCode)
	}
	var v struct {
		WS string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", fmt.Errorf("decode /json/version: %w", err)
	}
	if v.WS == "" {
		return "", fmt.Errorf("no webSocketDebuggerUrl at %s", versionURL)
	}
	ws, err := url.Parse(v.WS)
	if err != nil {
		return "", fmt.Errorf("bad webSocketDebuggerUrl %q: %w", v.WS, err)
	}
	ws.Host = hostPort // rewrite to the reachable sidecar IP:port
	return ws.String(), nil
}

// collectorJS builds the in-page async collector. year/sid/uid are injected as
// JSON string literals (they are digit strings, but encode defensively). The
// function mirrors the HTTP driver's semantics:
//   - one fresh single-use Turnstile token per page (reset + await),
//   - POST /api/ returns either inline {requests,…} or {url} to GET,
//   - osvita's flaky "first POST returns {url:''}" is retried with a new token
//     rather than mistaken for end-of-data (which would truncate the list),
//   - a page whose authoritative `requests` array is empty ends pagination.
func collectorJS(year, sid, uid string) string {
	q := func(s string) string { b, _ := json.Marshal(s); return string(b) }
	return `(async () => {
  const Y = ` + q(year) + `, SID = ` + q(sid) + `, UID = ` + q(uid) + `;
  const READY_MS = 20000, TOKEN_MS = 20000, PAGE_CAP = 400, ROW_CAP = 200000, FLAKY_RETRIES = 4;
  const sleep = ms => new Promise(r => setTimeout(r, ms));
  const ready = () => window.turnstile && typeof window.turnstile.getResponse === 'function';
  async function waitReady() {
    const t0 = Date.now();
    while (Date.now() - t0 < READY_MS) {
      if (ready() && window.turnstile.getResponse()) return true;
      await sleep(200);
    }
    return ready();
  }
  async function freshToken(prev) {
    try { window.turnstile.reset(); } catch (e) {}
    const t0 = Date.now();
    while (Date.now() - t0 < TOKEN_MS) {
      const t = window.turnstile.getResponse();
      if (t && t !== prev) return t;
      await sleep(200);
    }
    return null;
  }
  async function postPage(last, token) {
    const body = new URLSearchParams({ action: 'requests', y: Y, sid: SID, uid: UID, last: String(last), token });
    const r = await fetch('/api/', { method: 'POST', headers: { 'Content-Type': 'application/x-www-form-urlencoded' }, body });
    let j = await r.json();
    if (j && typeof j.url === 'string' && j.url) {
      const r2 = await fetch(j.url);
      j = await r2.json();
    }
    return j;
  }
  const out = { requests: [], requests_subjects: {}, pages: 0, error: '' };
  if (!(await waitReady())) {
    out.error = 'turnstile not ready (typeof=' + (typeof window.turnstile) + ')';
    return out;
  }
  let prev = '', last = 0;
  for (let page = 0; page < PAGE_CAP; page++) {
    let data = null;
    for (let attempt = 0; attempt < FLAKY_RETRIES; attempt++) {
      const tok = await freshToken(prev);
      if (!tok) {
        const cur = (() => { try { return window.turnstile.getResponse(); } catch (e) { return 'err:' + e.message; } })();
        out.error = 'no fresh turnstile token (curLen=' + (cur ? cur.length : 0) + ', same=' + (cur === prev) + ')';
        return out;
      }
      prev = tok;
      let j;
      try { j = await postPage(last, tok); }
      catch (e) { out.error = 'fetch failed: ' + (e && e.message || e); return out; }
      if (j && (j.msg || j.error)) { out.error = 'osvita: ' + (j.msg || j.error); return out; }
      if (j && Array.isArray(j.requests)) { data = j; break; } // authoritative page
      // else flaky (e.g. {url:''}) → retry this same offset with a new token
    }
    if (!data) break; // exhausted flaky retries → treat as end of data
    const reqs = data.requests;
    if (reqs.length === 0) break; // genuine end of list
    out.requests.push(...reqs);
    const subj = data.requests_subjects;
    if (subj && !Array.isArray(subj)) Object.assign(out.requests_subjects, subj);
    out.pages++;
    last += reqs.length;
    if (out.requests.length > ROW_CAP) { out.error = 'runaway response'; return out; }
  }
  return out;
})()`
}
