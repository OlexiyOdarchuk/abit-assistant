package osvita

// directory.go loads osvita's static university directory
// (/doc/json/universities_0.json) and matches free-text university names
// against it. The universityId it yields is the same code that appears as
// the middle segment of a program URL and as the University slot in a
// SpecFilter — so resolving "name → universityId" lets us list exactly one
// institution's programs without fuzzy-matching scraped listing rows.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode"
)

const universitiesPath = "/doc/json/universities_0.json"

// University is one entry of the osvita directory.
type University struct {
	ID        int    `json:"universityId"`
	ShortName string `json:"universityShortName"`
	FullName  string `json:"universityFullName"`
}

// FetchUniversities downloads and decodes the full university directory.
// It is ~1.4 MB; callers should fetch once and cache (the directory is
// stable across a campaign).
func (p *Parser) FetchUniversities(ctx context.Context) ([]University, error) {
	base, err := p.siteBase()
	if err != nil {
		return nil, err
	}
	var unis []University
	err = p.retry(ctx, func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+universitiesPath, nil)
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
		var out []University
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return err
		}
		unis = out
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("osvita: universities directory: %w", err)
	}
	return unis, nil
}

// MatchUniversity finds the directory entry that best matches a free-text
// university name (e.g. one scraped from abit-poisk, whose spelling/quoting
// differs from osvita's). Matching is normalized — lowercased, punctuation
// and quotes stripped, whitespace collapsed — then tried in order:
//
//  1. exact normalized equality (short or full name);
//  2. one normalized name fully contains the other, picking the longest
//     such overlap to avoid a short generic name matching many entries.
//
// ok is false when nothing clears the containment bar. Matching across two
// sources' naming is inherently imperfect — callers should treat a hit as
// high-confidence only when the names are close in length.
func MatchUniversity(dir []University, name string) (University, bool) {
	target := normalizeName(name)
	if target == "" {
		return University{}, false
	}
	var (
		best      University
		bestScore int
		found     bool
	)
	for _, u := range dir {
		for _, cand := range []string{u.FullName, u.ShortName} {
			n := normalizeName(cand)
			if n == "" {
				continue
			}
			if n == target {
				return u, true // exact wins immediately
			}
			if strings.Contains(n, target) || strings.Contains(target, n) {
				// Longer overlap = more specific match.
				if score := len(n); score > bestScore {
					best, bestScore, found = u, score, true
				}
			}
		}
	}
	return best, found
}

// normalizeName lowercases, drops quotes/punctuation, and collapses
// whitespace so two spellings of the same institution compare equal.
func normalizeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevSpace = false
		case unicode.IsSpace(r):
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		default:
			// drop quotes, dots, dashes, etc. — but keep a separator so
			// "КРОК"-style adjacent tokens don't fuse.
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}
