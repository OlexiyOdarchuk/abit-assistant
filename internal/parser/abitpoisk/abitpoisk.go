// Package abitpoisk searches the abit-poisk.org.ua applicant index.
//
// The endpoint returns a JSON envelope containing an HTML table; the table
// is then scraped into ApplicantEntry values. This is a single POST with no
// pagination, so the client is intentionally tiny.
package abitpoisk

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/httpx"
	"github.com/PuerkitoBio/goquery"
)

const (
	sourceID    = "abit-poisk"
	defaultAPI  = "https://abit-poisk.org.ua/api/statements"
	defaultUA   = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36"
	defaultTime = 15 * time.Second
)

// Client searches abit-poisk.org.ua.
type Client struct {
	http      *http.Client
	endpoint  string
	userAgent string
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the HTTP client (custom timeouts, proxies, ...).
func WithHTTPClient(c *http.Client) Option { return func(cl *Client) { cl.http = c } }

// WithEndpoint overrides the API URL (mostly for tests).
func WithEndpoint(u string) Option { return func(cl *Client) { cl.endpoint = u } }

// WithUserAgent overrides the User-Agent header.
func WithUserAgent(ua string) Option { return func(cl *Client) { cl.userAgent = ua } }

// WithInsecureTLS disables TLS verification. Use only when the
// upstream's certificate is known-broken (abit-poisk.org.ua serves an
// incomplete chain) and you accept the risk.
//
// Implementation note: mutates the existing client's Transport instead
// of replacing the whole *http.Client. That way other options
// (WithHTTPClient, custom CookieJar / CheckRedirect) survive regardless
// of declaration order.
func WithInsecureTLS() Option {
	return func(cl *Client) {
		base, ok := cl.http.Transport.(*http.Transport)
		if !ok || base == nil {
			base = http.DefaultTransport.(*http.Transport).Clone()
		} else {
			base = base.Clone()
		}
		if base.TLSClientConfig == nil {
			base.TLSClientConfig = &tls.Config{}
		}
		base.TLSClientConfig.InsecureSkipVerify = true
		cl.http.Transport = base
	}
}

// New constructs a Client with defaults overridden by opts.
func New(opts ...Option) *Client {
	c := &Client{
		http:      &http.Client{Timeout: defaultTime},
		endpoint:  defaultAPI,
		userAgent: defaultUA,
	}
	for _, o := range opts {
		o(c)
	}
	// Gate all traffic to abit-poisk.org.ua. It's rate-sensitive, so pace
	// the sustained rate low and trip the breaker quickly to fail fast on a
	// throttle/outage rather than amplifying it. Applied last so it wraps
	// the (possibly insecure-TLS) transport set by the options above.
	c.http.Transport = httpx.NewGate(c.http.Transport, httpx.Limits{
		RPS:           6,
		Burst:         4,
		FailThreshold: 5,
		OpenFor:       20 * time.Second,
	})
	return c
}

// ID returns a stable identifier for this source.
func (c *Client) ID() string { return sourceID }

// Search returns every application matching name. Returns abit.ErrNoData
// when the upstream responds successfully but with no rows.
func (c *Client) Search(ctx context.Context, name string) ([]abit.ApplicantEntry, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("abitpoisk: empty name")
	}

	form := url.Values{"search": {name}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("abitpoisk: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("abitpoisk: http %d", resp.StatusCode)
	}

	var env struct {
		HTML string `json:"html"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("abitpoisk: decode envelope: %w", err)
	}
	if env.HTML == "" {
		return nil, abit.ErrNoData
	}

	return parseHTML(env.HTML)
}

func parseHTML(htmlText string) ([]abit.ApplicantEntry, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlText))
	if err != nil {
		return nil, fmt.Errorf("abitpoisk: parse html: %w", err)
	}
	var out []abit.ApplicantEntry
	doc.Find("table.table tbody tr").Each(func(_ int, s *goquery.Selection) {
		cells := s.Find("td")
		if cells.Length() < 14 {
			return
		}
		out = append(out, abit.ApplicantEntry{
			Degree:                compact(cells.Eq(0).Text()),
			FullName:              compact(cells.Eq(1).Text()),
			Status:                compact(cells.Eq(2).Text()),
			RankingNumber:         compact(cells.Eq(3).Text()),
			Priority:              compact(cells.Eq(4).Text()),
			TotalScore:            compact(cells.Eq(6).Text()),
			EducationAvg:          compact(cells.Eq(7).Text()),
			University:            compact(cells.Eq(9).Text()),
			Faculty:               compact(cells.Eq(10).Text()),
			Specialty:             compact(cells.Eq(11).Text()),
			Quota:                 compact(cells.Eq(12).Text()),
			OriginalDocsSubmitted: compact(cells.Eq(13).Text()),
		})
	})
	if len(out) == 0 {
		return nil, abit.ErrNoData
	}
	return out, nil
}

func compact(s string) string { return strings.Join(strings.Fields(s), " ") }
