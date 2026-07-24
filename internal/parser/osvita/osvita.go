// Package osvita is a parser.Source implementation for vstup.osvita.ua.
//
// vstup.osvita.ua exposes a two-step pagination API: a POST returns a JSON
// URL, and a GET against that URL returns one page of applicant requests.
// We fan out N workers, each striding through a disjoint set of page
// offsets (stride = N*pageSize). Each worker stops at its own first empty
// page; the first hard error cancels the shared context so siblings stop.
package osvita

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/httpx"
	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

const (
	sourceID        = "osvita"
	programHost     = "vstup.osvita.ua"
	defaultAPIURL   = "https://vstup.osvita.ua/api/"
	defaultPageSize = 500
	// defaultWorkers is 1 — sequential pagination (0, 500, 1000, …), like the
	// browser and the Python original. osvita's anti-bot flags bursty traffic
	// with "Error 300"; the old 8-worker fan-out fired 8 concurrent POSTs on the
	// very first program, which looks like a scraper and got rejected. A single
	// gentle lane has the best chance of passing on a clean IP. (We keep the Go
	// User-Agent, NOT a browser one — a browser UA made Cloudflare serve a JS
	// challenge that HANGS the connection instead of failing fast.)
	defaultWorkers = 1
	defaultTimeout = 60 * time.Second
	defaultRetries = 3

	// maxRequests caps how many applicant rows a single Parse will
	// accumulate. Real programs top out at a few thousand; this is ~100x
	// headroom. It exists only to bound memory if osvita ever malfunctions
	// and streams a runaway response — without it a broken upstream could
	// OOM the process. Hitting the cap is a hard error, not a silent trim.
	maxRequests = 200_000
)

// Parser fetches competitive offer data from vstup.osvita.ua. The zero value
// is not usable; construct with New.
type Parser struct {
	client      *http.Client
	apiURL      string
	pageSize    int
	workers     int
	maxRetries  int
	reqFallback RequestsFetcher
	dataFetcher ProgramDataFetcher
}

// RequestsFetcher fetches the applicant-requests half of a program when the
// plain HTTP API is gated behind osvita's Turnstile challenge (2026: the
// /api/ POST answers "Error 316 / Перезавантажте сторінку" without a valid
// token). Implementations solve the challenge out-of-band — e.g. a headless
// browser that renders osvita's Turnstile widget and drives the paginated API
// with a fresh single-use token per page. The static program page is NOT
// gated, so only this step needs the browser.
type RequestsFetcher interface {
	// FetchRequests returns every applicant request for the program plus the
	// per-applicant subjects map, addressed by the same (year, sid, uid) the
	// HTTP API uses. programURL is the public page URL to render.
	FetchRequests(ctx context.Context, programURL, year, sid, uid string) (requests []abit.RawRequest, subjects map[string]abit.ApplicantSubjects, err error)
}

// ProgramDataFetcher fetches a whole program in ONE browser run — both the page
// HTML and the applicant requests — for when osvita 403s even the static page
// (the 2026 escalation: whole-site edge block for flagged clients). The browser
// clears the Cloudflare challenge once and reads everything from that one
// authenticated session; the returned HTML is parsed by parseStaticHTML exactly
// like a plain HTTP body, so the resulting Program is identical.
type ProgramDataFetcher interface {
	FetchProgramData(ctx context.Context, programURL, year, sid, uid string) (staticHTML string, requests []abit.RawRequest, subjects map[string]abit.ApplicantSubjects, err error)
}

// Option configures a Parser.
type Option func(*Parser)

// WithHTTPClient overrides the HTTP client. Note: the client must carry a
// cookie jar — osvita.ua sets a session cookie on the first request.
func WithHTTPClient(c *http.Client) Option { return func(p *Parser) { p.client = c } }

// WithAPIURL overrides the API endpoint (test injection).
func WithAPIURL(u string) Option { return func(p *Parser) { p.apiURL = u } }

// WithPageSize overrides the requests-per-page batch size.
func WithPageSize(n int) Option { return func(p *Parser) { p.pageSize = n } }

// WithWorkers sets the fan-out parallelism.
func WithWorkers(n int) Option { return func(p *Parser) { p.workers = n } }

// WithMaxRetries sets the per-request retry budget for transient failures.
func WithMaxRetries(n int) Option { return func(p *Parser) { p.maxRetries = n } }

// WithRequestsFallback installs a browser-backed (or other out-of-band)
// fetcher used only when the plain HTTP API returns a Turnstile challenge.
// Without it, a challenged program fails as before.
func WithRequestsFallback(f RequestsFetcher) Option {
	return func(p *Parser) { p.reqFallback = f }
}

// WithProgramDataFetcher installs a single-shot browser fetcher used when the
// static page itself is blocked (HTTP 403). Without it, a 403 static page is a
// hard error.
func WithProgramDataFetcher(f ProgramDataFetcher) Option {
	return func(p *Parser) { p.dataFetcher = f }
}

// New builds a Parser with sensible defaults overridden by opts.
func New(opts ...Option) *Parser {
	jar, _ := cookiejar.New(nil)
	p := &Parser{
		client:     &http.Client{Timeout: defaultTimeout, Jar: jar},
		apiURL:     defaultAPIURL,
		pageSize:   defaultPageSize,
		workers:    defaultWorkers,
		maxRetries: defaultRetries,
	}
	for _, opt := range opts {
		opt(p)
	}
	// Gate all traffic to vstup.osvita.ua behind a shared rate limiter +
	// circuit breaker. The fan-out runs up to defaultWorkers requests at
	// once; the limiter paces the sustained rate so a burst of concurrent
	// user searches can't turn into a single-IP flood, and the breaker
	// fails fast (short-circuiting the retry loop) once the host starts
	// returning 429/5xx. Applied last so it wraps any custom transport.
	p.client.Transport = httpx.NewGate(p.client.Transport, httpx.Limits{
		RPS:           4, // gentle — osvita rate-flags bursty traffic with "Error 300"
		Burst:         2,
		FailThreshold: 8,
		OpenFor:       15 * time.Second,
	})
	return p
}

// ID implements parser.Source.
func (p *Parser) ID() string { return sourceID }

var programURLRe = regexp.MustCompile(`/y(\d{4})/[^/]+/(\d+)/(\d+)/?$`)

// Parse fetches a vstup.osvita.ua program by its public URL of the form
// https://vstup.osvita.ua/y2025/r14/282/1471029/.
func (p *Parser) Parse(ctx context.Context, programURL string) (*abit.Program, error) {
	sid, uid, year, err := parseProgramURL(programURL)
	if err != nil {
		return nil, err
	}

	prog, err := p.fetchStatic(ctx, programURL)
	if err != nil {
		// 2026 escalation: osvita 403s the static page too. If a browser data
		// fetcher is configured, clear the Cloudflare challenge once and read
		// the whole program (HTML + requests) from that one session.
		if p.dataFetcher == nil {
			return nil, fmt.Errorf("osvita: static page: %w", err)
		}
		return p.fetchViaBrowser(ctx, programURL, year, sid, uid, err)
	}

	if err := p.fanOut(ctx, prog, programURL, sid, uid, year); err != nil {
		return nil, fmt.Errorf("osvita: requests: %w", err)
	}
	return prog, nil
}

// fetchViaBrowser is the fully-gated path: one browser run returns the page
// HTML (parsed here) plus the applicant requests. It VALIDATES the parse so a
// Cloudflare challenge/interstitial page can never slip through as a silent
// empty Program — which downstream would look like a vanished program name,
// a zero score, and no rivals (all from one bad parse, not four bugs).
func (p *Parser) fetchViaBrowser(ctx context.Context, programURL, year, sid, uid string, staticErr error) (*abit.Program, error) {
	html, reqs, subj, err := p.dataFetcher.FetchProgramData(ctx, programURL, year, sid, uid)
	if err != nil {
		return nil, fmt.Errorf("osvita: browser fetch (static was: %v): %w", staticErr, err)
	}
	prog, err := parseStaticHTML(strings.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("osvita: parse browser html: %w", err)
	}
	if err := validateProgram(prog); err != nil {
		return nil, fmt.Errorf("osvita: browser html incomplete — challenge not cleared? %w", err)
	}
	prog.Requests = reqs
	prog.RequestSubjects = subj
	return prog, nil
}

// validateProgram rejects a degenerate parse. The program name and the subjects
// config are the two fields ComputeRating/Analyze need to produce a score,
// chances and competitor tiers; if either is missing the whole analysis reads
// as blank, so treat their absence as a hard fetch error rather than serving an
// empty program.
func validateProgram(prog *abit.Program) error {
	if strings.TrimSpace(prog.ProgramName) == "" {
		return fmt.Errorf("no program name in page")
	}
	if len(prog.Subjects) == 0 {
		return fmt.Errorf("no subjects config in page")
	}
	return nil
}

func parseProgramURL(rawURL string) (sid, uid, year string, err error) {
	u, perr := url.Parse(rawURL)
	if perr != nil {
		return "", "", "", fmt.Errorf("%w: %v", abit.ErrInvalidURL, perr)
	}
	// Pin scheme+host. The URL comes straight from a request body
	// (/api/analyze, /api/simulate) and is later handed to http.Get; without
	// this the path regex alone would let an attacker point us at an internal
	// host (SSRF) as long as the path ends in /yYYYY/rNN/uid/sid/.
	if u.Scheme != "https" || !strings.EqualFold(u.Host, programHost) {
		return "", "", "", fmt.Errorf("%w: only https://%s URLs are accepted", abit.ErrInvalidURL, programHost)
	}
	m := programURLRe.FindStringSubmatch(u.Path)
	if len(m) != 4 {
		return "", "", "", fmt.Errorf("%w: path %q", abit.ErrInvalidURL, u.Path)
	}
	return m[3], m[2], m[1], nil
}

// fanOut runs p.workers goroutines, each striding through pages by
// p.pageSize * p.workers. Each worker owns a disjoint set of page offsets
// (worker w covers w*pageSize, w*pageSize+stride, …), so the lanes never
// overlap and together cover every page. A lane stops at its own first
// empty page — we deliberately do NOT let one lane's empty page truncate
// the others: a transient empty body (osvita serves these to cold
// sessions) in one lane must not silently drop another lane's data. The
// cost is at most workers-1 extra empty fetches at the true tail.
//
// On the first hard error a worker records it and cancels the shared
// context, so sibling lanes stop promptly instead of hammering osvita.
func (p *Parser) fanOut(ctx context.Context, prog *abit.Program, programURL, sid, uid, year string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		mu       sync.Mutex
		firstErr error
		requests []abit.RawRequest
		subjects = map[string]abit.ApplicantSubjects{}
		wg       sync.WaitGroup
	)

	// Warm-up doubles as a challenge probe: osvita.ua returns an empty body to
	// a "cold" session, so prime the cookie jar with one throwaway request —
	// and if that request comes back as osvita's Turnstile challenge, hand the
	// whole requests fetch to the browser fallback (when one is configured).
	// The static program page is already loaded above, so only this half was
	// missing.
	_, warmErr := p.fetchJSONURL(ctx, formValues(year, sid, uid, 0))
	if p.reqFallback != nil && errors.Is(warmErr, errChallenge) {
		reqs, subj, err := p.reqFallback.FetchRequests(ctx, programURL, year, sid, uid)
		if err != nil {
			return fmt.Errorf("fallback: %w", err)
		}
		prog.Requests = reqs
		prog.RequestSubjects = subj
		return nil
	}

	for w := 0; w < p.workers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			offset := workerID * p.pageSize
			for {
				if ctx.Err() != nil {
					return
				}
				chunk, err := p.fetchChunk(ctx, sid, uid, year, offset)
				if err != nil {
					// A cancellation triggered by a sibling's error isn't a
					// new failure — don't overwrite the real firstErr with it.
					if ctx.Err() == nil {
						mu.Lock()
						if firstErr == nil {
							firstErr = fmt.Errorf("offset %d: %w", offset, err)
						}
						mu.Unlock()
						cancel() // stop the siblings
					}
					return
				}
				if len(chunk.Requests) == 0 {
					return // end of this lane's data
				}
				mu.Lock()
				requests = append(requests, chunk.Requests...)
				maps.Copy(subjects, chunk.Subjects)
				overflow := len(requests) > maxRequests
				if overflow && firstErr == nil {
					firstErr = fmt.Errorf("runaway response: more than %d requests", maxRequests)
				}
				mu.Unlock()
				if overflow {
					cancel() // stop every lane — upstream is misbehaving
					return
				}
				offset += p.pageSize * p.workers
			}
		}(w)
	}
	wg.Wait()

	prog.Requests = requests
	prog.RequestSubjects = subjects
	return firstErr
}

func formValues(year, sid, uid string, last int) url.Values {
	return url.Values{
		"action": {"requests"},
		"y":      {year},
		"sid":    {sid},
		"uid":    {uid},
		"last":   {fmt.Sprintf("%d", last)},
	}
}

// rawChunk is the decoded JSON of one page.
type rawChunk struct {
	Requests []abit.RawRequest `json:"requests"`
	Subjects rawSubjects       `json:"requests_subjects"`
}

// rawSubjects is requests_subjects, which osvita serves as a JSON object
// keyed by applicant id — EXCEPT when there are none, where it sends an empty
// array `[]` instead of `{}`. Decoding `[]` into a map fails and would sink
// the whole page (observed live on some programs), so tolerate both.
type rawSubjects map[string]abit.ApplicantSubjects

func (s *rawSubjects) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || b[0] == '[' || string(b) == "null" {
		*s = rawSubjects{}
		return nil
	}
	var m map[string]abit.ApplicantSubjects
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	*s = m
	return nil
}

// errEmptyURL marks osvita's flaky "first POST for an offset returns no url"
// quirk (the Python original worked around it by POSTing twice per page). It is
// retriable — a later attempt at the SAME offset usually yields the url — but
// if every attempt comes back empty we treat that offset as the true end of the
// applicant list. Without the retry, a single flaky empty truncated the lane and
// silently dropped every applicant past the first 500.
var errEmptyURL = errors.New("osvita: empty url response")

// fetchChunk runs the two-step API dance: POST to get a signed JSON URL,
// then GET that URL.
func (p *Parser) fetchChunk(ctx context.Context, sid, uid, year string, last int) (*rawChunk, error) {
	form := formValues(year, sid, uid, last)

	var jsonURL string
	err := p.retry(ctx, func() error {
		u, err := p.fetchJSONURL(ctx, form)
		if err != nil {
			return err
		}
		if u == "" {
			return errEmptyURL // flaky first POST — retry the same offset
		}
		jsonURL = u
		return nil
	})
	if errors.Is(err, errEmptyURL) {
		return &rawChunk{}, nil // genuinely empty after every retry → end of lane
	}
	if err != nil {
		return nil, err
	}

	var chunk *rawChunk
	err = p.retry(ctx, func() error {
		c, err := p.fetchPayload(ctx, jsonURL)
		if err != nil {
			return err
		}
		chunk = c
		return nil
	})
	if err != nil {
		return nil, err
	}
	return chunk, nil
}

func (p *Parser) fetchJSONURL(ctx context.Context, form url.Values) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp.StatusCode); err != nil {
		return "", err
	}
	var out struct {
		URL   string `json:"url"`
		Msg   string `json:"msg"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		if errors.Is(err, io.EOF) {
			return "", nil
		}
		return "", err
	}
	// osvita gates the applicant API behind a Cloudflare Turnstile challenge
	// (2026): without a valid `token` it answers {"msg"/"error":"Перезавантажте
	// сторінку! Error 300"} instead of a data url. Surface this loudly — silently
	// returning an empty program would make the whole analysis lie ("ти перший").
	if out.URL == "" && (out.Msg != "" || out.Error != "") {
		return "", fmt.Errorf("%w: %s", errChallenge, firstNonEmpty(out.Msg, out.Error))
	}
	return out.URL, nil
}

// errChallenge marks osvita's anti-bot (Cloudflare Turnstile) rejection — the
// API needs a Turnstile token we don't have. Not retriable: retrying without a
// token only hammers the challenge.
var errChallenge = errors.New("osvita: заблоковано анти-ботом (Cloudflare Turnstile) — потрібен токен")

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (p *Parser) fetchPayload(ctx context.Context, jsonURL string) (*rawChunk, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jsonURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp.StatusCode); err != nil {
		return nil, err
	}
	var c rawChunk
	if err := json.NewDecoder(resp.Body).Decode(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

// retriableError marks transient HTTP failures that warrant a retry.
type retriableError struct{ code int }

func (e retriableError) Error() string { return fmt.Sprintf("http %d", e.code) }

func checkStatus(code int) error {
	if code/100 == 2 {
		return nil
	}
	if code == http.StatusTooManyRequests || code >= 500 {
		return retriableError{code: code}
	}
	return fmt.Errorf("http %d", code)
}

func (p *Parser) retry(ctx context.Context, fn func() error) error {
	backoff := 200 * time.Millisecond
	var err error
	for attempt := 0; attempt < p.maxRetries; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		var r retriableError
		if !errors.As(err, &r) && !errors.Is(err, errEmptyURL) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	return err
}

// --- static page (DOM scraping) ---

var (
	reSpec = regexp.MustCompile(`Спеціальність:\s*(\S+)`)
	reProg = regexp.MustCompile(`Освітня програма:\s*(.+?)\.`)
	// osvita stamps every page with its EDBO-sync time, e.g.
	// "Дані отримані з ЄДЕБО 19.07.2026 12:00 (наступне оновлення …)".
	reSourceAsOf = regexp.MustCompile(`Дані отримані з ЄДЕБО\s*(\d{2}\.\d{2}\.\d{4}\s+\d{1,2}:\d{2})`)
)

func (p *Parser) fetchStatic(ctx context.Context, programURL string) (*abit.Program, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, programURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp.StatusCode); err != nil {
		return nil, err
	}
	return parseStaticHTML(resp.Body)
}

// parseStaticHTML scrapes a program page's HTML into a Program. It is the sole
// place that knows the page's DOM shape, so it works identically on a plain
// HTTP response body and on document.documentElement.outerHTML captured from a
// browser after a Cloudflare challenge (the 2026 gated path). The <script>
// config block (subjects/rk/statuses/…) the decoder relies on survives in
// outerHTML, so both inputs yield the same Program.
func parseStaticHTML(r io.Reader) (*abit.Program, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, err
	}

	prog := &abit.Program{
		ProgramInfo: map[string]string{},
		Volume:      map[string]string{},
	}
	prog.UniversityName = strings.TrimSpace(doc.Find(".page-vnz-detail-title h2").First().Text())

	title := doc.Find(".page-vnz-detail-title h1").Text()
	if m := reSpec.FindStringSubmatch(title); len(m) > 1 {
		prog.SpecCode = strings.TrimSuffix(m[1], ".")
	}
	if m := reProg.FindStringSubmatch(title); len(m) > 1 {
		prog.ProgramName = strings.TrimSpace(m[1])
	}

	doc.Find(".table-of-specs-item b").Each(func(_ int, b *goquery.Selection) {
		key := strings.TrimSpace(strings.ReplaceAll(b.Text(), ":", ""))
		if key == "" {
			return
		}
		val := siblingText(b)
		if val == "" {
			val = strings.TrimSpace(b.NextAllFiltered("span").First().Text())
		}
		if val == "" {
			val = strings.TrimSpace(b.NextAllFiltered("a").First().Text())
		}
		if val != "" {
			prog.ProgramInfo[key] = val
		}
	})

	// Volume / statistics block. osvita renders the numeric table —
	// "Максимальний обсяг державного замовлення", "Зараховано на бюджет
	// всього", "Ліцензійний обсяг", "Залишилося невикористаних
	// ліцензійних місць" and friends — as plain <table><tr><td>k</td>
	// <td>v</td></tr></table>. The previous parser looked for <b>
	// children inside .block-pro-vnz, which matched nothing on the
	// real site and left Volume empty (Analyze then bailed with
	// ChanceUnknown / "ліцензований обсяг не визначено").
	//
	// New approach: scan every two-cell table row in the document and
	// load it into Volume. abit.Program.BudgetVolume() matches by
	// substring, so it picks up "Максимальний обсяг…" as the budget
	// figure regardless of any unrelated stats also present in the table.
	collectVolume(doc, prog.Volume)

	if m := reSourceAsOf.FindStringSubmatch(doc.Text()); len(m) > 1 {
		prog.SourceAsOf = strings.Join(strings.Fields(m[1]), " ")
	}

	var js strings.Builder
	doc.Find("script").Each(func(_ int, s *goquery.Selection) {
		if text := s.Text(); text != "" {
			js.WriteString(text)
			js.WriteByte('\n')
		}
	})
	if jsText := js.String(); jsText != "" {
		extractJSConfig(jsText, prog)
	}

	return prog, nil
}

// extractJSConfig pulls every JS-defined config value the decoder needs.
// Missing values fall back to sensible defaults (RK=1.0, K4Max=0.35).
func extractJSConfig(js string, prog *abit.Program) {
	prog.Statuses = parseJSStringMap(js, "statuses")
	prog.RecTypes = parseJSStringMap(js, "rec_types")
	if v, ok := parseJSInt(js, "eb"); ok {
		prog.EB = v
	}
	if v, ok := parseJSInt(js, "okr"); ok {
		prog.OKR = v
	}
	prog.K4Max = 0.35
	if v, ok := parseJSFloat(js, "k4max"); ok {
		prog.K4Max = v
	}
	prog.RK = 1.0
	if v, ok := parseJSFloat(js, "rk"); ok {
		prog.RK = v
	}
	if v, ok := parseJSIntSlice(js, "nmts"); ok {
		prog.NMTs = v
	}
	if v, ok := parseJSIntSlice(js, "sub4ar"); ok {
		prog.Sub4ar = v
	}
	if v, ok := parseJSSubjects(js, "subjects"); ok {
		prog.Subjects = v
	}
}

// collectVolume loads the program's numeric "volume/statistics" figures into
// vol from BOTH layouts osvita has used:
//
//   - 2025: a two-cell "<table><tr><td>key</td><td>value</td></tr>" — the
//     licensed-volume / enrolment stats block.
//   - 2026: inline "Label: <b>value</b><br>" pairs (e.g. "Максимальне
//     держзамовлення: <b>78</b>") rendered outside any table.
//
// abit.Program.BudgetVolume()/Quota*Volume() match keys by substring, so any
// unrelated stats also collected here are harmless. The inline pass keeps only
// numeric values (volumes are numbers) so bold prose doesn't leak in, and
// never overwrites a table entry.
func collectVolume(doc *goquery.Document, vol map[string]string) {
	doc.Find("table tr").Each(func(_ int, tr *goquery.Selection) {
		tds := tr.Find("td")
		if tds.Length() != 2 {
			return
		}
		key := strings.Join(strings.Fields(tds.Eq(0).Text()), " ")
		val := strings.Join(strings.Fields(tds.Eq(1).Text()), " ")
		if key == "" || val == "" {
			return
		}
		vol[key] = val
	})

	doc.Find("b").Each(func(_ int, b *goquery.Selection) {
		if len(b.Nodes) == 0 {
			return
		}
		prev := b.Nodes[0].PrevSibling
		if prev == nil || prev.Type != html.TextNode {
			return
		}
		label := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(prev.Data), ":"))
		val := strings.TrimSpace(b.Text())
		if label == "" || val == "" {
			return
		}
		if _, err := strconv.ParseFloat(strings.ReplaceAll(val, ",", "."), 64); err != nil {
			return // volumes are numeric; skip bold prose
		}
		if _, exists := vol[label]; !exists {
			vol[label] = val
		}
	})
}

func siblingText(b *goquery.Selection) string {
	n := b.Get(0).NextSibling
	if n == nil || n.Type != html.TextNode {
		return ""
	}
	return strings.TrimSpace(n.Data)
}

// extractJSExpr extracts the right-hand side of an assignment "name = ..."
// (either {...} or [...]) by balancing brackets. Word-boundary matching
// avoids false positives from substring hits (e.g. "statuses" inside a
// callback). Returns "" if not found.
func extractJSExpr(js, name string) string {
	rest := findAssignmentRHS(js, name)
	if rest == "" {
		return ""
	}
	open := rest[0]
	var closeCh byte
	switch open {
	case '{':
		closeCh = '}'
	case '[':
		closeCh = ']'
	default:
		return ""
	}
	depth := 0
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case open:
			depth++
		case closeCh:
			depth--
			if depth == 0 {
				return rest[:i+1]
			}
		}
	}
	return ""
}

// findAssignmentRHS returns the text immediately after `<name> =` (with
// optional whitespace), but only for occurrences of name surrounded by
// word boundaries.
func findAssignmentRHS(js, name string) string {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\s*=\s*`)
	loc := re.FindStringIndex(js)
	if loc == nil {
		return ""
	}
	return js[loc[1]:]
}

// extractJSScalar returns the literal between "<name> =" and the next
// terminator (`;`, `,`, `\n`, or `)`). Used for primitive values like
// `var eb = 40`. Returns "" if not found.
func extractJSScalar(js, name string) string {
	rest := findAssignmentRHS(js, name)
	if rest == "" {
		return ""
	}
	end := len(rest)
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case ';', '\n', ',', ')':
			end = i
			i = len(rest)
		}
	}
	return strings.TrimSpace(rest[:end])
}

// parseJSStringMap parses a JS object literal of string→string pairs. The
// page's JSON is already well-formed (double quotes); the single→double
// replacement is a defensive fallback. Returns nil on miss/parse fail.
func parseJSStringMap(js, name string) map[string]string {
	raw := extractJSExpr(js, name)
	if raw == "" {
		return nil
	}
	s := strings.ReplaceAll(raw, `'`, `"`)
	var out map[string]string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

// parseJSInt parses `<name> = <integer>` and returns (value, true) on success.
func parseJSInt(js, name string) (int, bool) {
	s := extractJSScalar(js, name)
	if s == "" {
		return 0, false
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return v, true
}

// parseJSFloat parses `<name> = <float>` (or integer). Returns (value, true)
// on success. Non-numeric RHS (e.g. `parseFloat(...)`) yields (0, false).
func parseJSFloat(js, name string) (float64, bool) {
	s := extractJSScalar(js, name)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// parseJSIntSlice parses `<name> = [1, 2, 3, ...]`.
func parseJSIntSlice(js, name string) ([]int, bool) {
	raw := extractJSExpr(js, name)
	if raw == "" {
		return nil, false
	}
	var out []int
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, false
	}
	return out, true
}

// parseJSSubjects parses the page's `subjects = [...]` array directly into
// the typed SubjectMeta slice.
func parseJSSubjects(js, name string) ([]abit.SubjectMeta, bool) {
	raw := extractJSExpr(js, name)
	if raw == "" {
		return nil, false
	}
	var out []abit.SubjectMeta
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, false
	}
	return out, true
}
