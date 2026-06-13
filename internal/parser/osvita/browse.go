package osvita

// browse.go implements enumeration over vstup.osvita.ua's /spec/ listing —
// the program-search page that filters by education level, admission basis,
// study form, region and specialty. It is the foundation for the reverse
// "where can I get in" search and for resolving a competitor's other
// applications to concrete program URLs.
//
// URL shape (decoded from live pages, 2026-06-13):
//
//	/spec/<okr>-<edubase>-<eduform>/<region>-<industry>-<specialty>-0-0-<offset>/
//
// First group is the program type; second group is the filter + pagination.
// Defaults (okr=1 Бакалавр, edubase=40 ПЗСО, eduform=1 Денна) match the
// bot's domain: budget bachelor admission on a complete-secondary-education
// basis. Region/industry/specialty of 0 mean "any". The offset steps by 50.

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

const (
	// specPageSize is how many programs one /spec/ listing page returns.
	specPageSize = 50
	// maxSpecPages bounds a single Browse so a misread "found" count or a
	// site change can't loop forever. 200 pages = 10k programs — far above
	// any real specialty (the largest, ~CS, is <400).
	maxSpecPages = 200

	// Defaults for the first URL group (бакалавр / ПЗСО / денна).
	defaultLevel = 1
	defaultBasis = 40
	defaultForm  = 1
)

// SpecFilter selects programs on the /spec/ listing. The zero value lists
// every full-time bachelor (ПЗСО) program in the country; set the fields to
// narrow. Codes come from osvita: Region/Industry from the form's <select>
// options, University from the universities directory (see FetchUniversities),
// Specialty from the cascade endpoint. Region uses the same numbering as the
// rNN segment in a program URL (e.g. 21 = Kharkiv, 27 = Kyiv); University uses
// the same code as the middle segment of a program URL.
//
// The full path, reproduced from osvita's own form-submit JS:
//
//	/spec/<okr>-<edubase>-<eduform>/<region>-<industryId>-<specialityId>-<universityId>-<budget>-<offset>/
type SpecFilter struct {
	Level      int  // okr: education level; 0 → бакалавр (1)
	Basis      int  // edubase: admission basis; 0 → ПЗСО (40)
	Form       int  // eduform: study form; 0 → денна (1)
	Region     int  // region/oblast code; 0 → any
	Industry   int  // industryId (галузь знань); 0 → any
	Specialty  int  // specialityId; 0 → any
	University int  // universityId; 0 → any
	BudgetOnly bool // budget slot = 1 → only state-funded offers
}

// path renders the /spec/ path for the given pagination offset.
func (f SpecFilter) path(offset int) string {
	level, basis, form := f.Level, f.Basis, f.Form
	if level == 0 {
		level = defaultLevel
	}
	if basis == 0 {
		basis = defaultBasis
	}
	if form == 0 {
		form = defaultForm
	}
	budget := 0
	if f.BudgetOnly {
		budget = 1
	}
	return fmt.Sprintf("/spec/%d-%d-%d/%d-%d-%d-%d-%d-%d/",
		level, basis, form, f.Region, f.Industry, f.Specialty, f.University, budget, offset)
}

// SpecProgram is one row of the /spec/ listing. URL is absolute and ready to
// hand to ProgramService.Fetch. The remaining fields are best-effort context
// scraped from the row — useful for display and for narrowing a name match
// without fetching every candidate, but not guaranteed populated.
type SpecProgram struct {
	URL        string // absolute program URL, e.g. https://host/y2025/r27/318/1465936/
	University string // ЗВО name as printed (often prefixed with its code, "318. …")
	Program    string // освітня програма (education-programme name)
	Specialty  string // спеціальність line (may carry a galuz code prefix, "F3 …")
	Budget     bool   // false when the row is explicitly marked "Небюджетна"
}

// BrowsePrograms walks every page of the /spec/ listing for the filter and
// returns the matched programs. It stops when it has collected the page's
// reported "Знайдено: N" total, or at the first empty page, whichever comes
// first. Results are de-duplicated by URL (osvita occasionally repeats a row
// across the page boundary).
func (p *Parser) BrowsePrograms(ctx context.Context, f SpecFilter) ([]SpecProgram, error) {
	base, err := p.siteBase()
	if err != nil {
		return nil, err
	}

	var (
		out   []SpecProgram
		seen  = map[string]struct{}{}
		total = -1
	)
	for page := range maxSpecPages {
		pageURL := base + f.path(page*specPageSize)
		doc, err := p.fetchDoc(ctx, pageURL)
		if err != nil {
			return nil, fmt.Errorf("osvita: browse %s: %w", pageURL, err)
		}
		if total < 0 {
			total = parseFoundCount(doc)
		}
		rows := parseSpecListing(doc, base)
		if len(rows) == 0 {
			break
		}
		for _, r := range rows {
			if _, dup := seen[r.URL]; dup {
				continue
			}
			seen[r.URL] = struct{}{}
			out = append(out, r)
		}
		if total >= 0 && len(out) >= total {
			break
		}
	}
	return out, nil
}

// fetchDoc GETs url (with the same retry/backoff as the rest of the parser)
// and parses it into a goquery document.
func (p *Parser) fetchDoc(ctx context.Context, rawURL string) (*goquery.Document, error) {
	var doc *goquery.Document
	err := p.retry(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return err
		}
		resp, err := p.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if err := checkStatus(resp.StatusCode); err != nil {
			return err
		}
		d, err := goquery.NewDocumentFromReader(resp.Body)
		if err != nil {
			return err
		}
		doc = d
		return nil
	})
	return doc, err
}

// siteBase derives "scheme://host" from the configured API URL so browse
// requests hit the same origin as the rest of the parser (and so test
// servers injected via WithAPIURL are honoured).
func (p *Parser) siteBase() (string, error) {
	u, err := url.Parse(p.apiURL)
	if err != nil {
		return "", fmt.Errorf("osvita: bad api url %q: %w", p.apiURL, err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("osvita: api url %q lacks scheme/host", p.apiURL)
	}
	return u.Scheme + "://" + u.Host, nil
}

var (
	foundCountRe = regexp.MustCompile(`Знайдено:\s*(\d+)`)
	progHrefRe   = regexp.MustCompile(`^(?:https?://[^/]+)?/y\d{4}/r[^/]+/\d+/\d+/?$`)
	instPrefixRe = regexp.MustCompile(`^\d+\.\s`)
)

// parseFoundCount reads the "Знайдено: N" total from a listing page, or -1
// when absent (so the caller falls back to "stop at first empty page").
func parseFoundCount(doc *goquery.Document) int {
	if m := foundCountRe.FindStringSubmatch(doc.Text()); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	return -1
}

// parseSpecListing extracts every program row from a /spec/ listing page.
// Each program is anchored on its "Детальніше" detail button (a.green-button)
// — exactly one per program — and the surrounding row is mined best-effort
// for the university / programme / specialty labels.
func parseSpecListing(doc *goquery.Document, base string) []SpecProgram {
	var out []SpecProgram
	doc.Find("a.green-button").Each(func(_ int, btn *goquery.Selection) {
		href, ok := btn.Attr("href")
		if !ok || !progHrefRe.MatchString(strings.TrimSpace(href)) {
			return
		}
		prog := SpecProgram{URL: absURL(base, href), Budget: true}

		row := btn.Closest(".table-of-specs-item-row")
		// Specialty: first <b> in the row carries it (e.g. "F3 Комп'ютерні науки").
		if b := row.Find("b").First(); b.Length() > 0 {
			prog.Specialty = compactText(b.Text())
		}
		// University: the row's anchor whose text starts with a "<code>. " prefix.
		row.Find("a").EachWithBreak(func(_ int, a *goquery.Selection) bool {
			t := compactText(a.Text())
			if instPrefixRe.MatchString(t) {
				prog.University = t
				return false
			}
			return true
		})
		// Programme name: the text after the "Освітня програма:" label.
		prog.Program = labelledValue(row, "Освітня програма:")
		// Budget flag: rows explicitly marked non-budget.
		if strings.Contains(row.Text(), "Небюджетна") {
			prog.Budget = false
		}
		out = append(out, prog)
	})
	return out
}

// labelledValue returns the text that follows a "<label>" <b> within row —
// osvita renders "<b>Освітня програма:</b> <name>" so the value is the
// label node's trailing sibling text.
func labelledValue(row *goquery.Selection, label string) string {
	var val string
	row.Find("b").EachWithBreak(func(_ int, b *goquery.Selection) bool {
		if strings.Contains(b.Text(), label) {
			if node := b.Get(0).NextSibling; node != nil {
				val = compactText(node.Data)
			}
			return false
		}
		return true
	})
	return val
}

// absURL turns a possibly-relative href into an absolute URL on base.
func absURL(base, href string) string {
	href = strings.TrimSpace(href)
	if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
		return href
	}
	return base + href
}

// compactText collapses internal whitespace and trims — listing cells are
// heavily indented in the source HTML.
func compactText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
