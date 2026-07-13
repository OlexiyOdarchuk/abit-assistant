package service

import (
	"context"
	"log/slog"
	"sort"
	"sync"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/osvita"
)

// ProgramBrowser enumerates programs matching a /spec/ filter and exposes
// the form's filter option tables. Satisfied by *osvita.Parser; kept as an
// interface so DiscoverService can be tested with a double.
type ProgramBrowser interface {
	BrowsePrograms(ctx context.Context, f osvita.SpecFilter) ([]osvita.SpecProgram, error)
	FetchFilters(ctx context.Context) (osvita.Filters, error)
}

// DiscoverService powers the "where can I get in" mode: enumerate the
// programs matching a filter (e.g. a galuz in chosen regions, budget only),
// score the user against each, and rank them by chance.
type DiscoverService struct {
	browser  ProgramBrowser
	programs *ProgramService
	workers  int
	log      *slog.Logger

	// filters caches the (static-per-campaign) region/industry option
	// tables so the picker UI doesn't re-scrape the form on every open.
	filtersMu sync.Mutex
	filters   *osvita.Filters
}

// NewDiscoverService wires the discovery use case. workers caps concurrent
// per-program fetches against osvita; keep it modest (4..8) to stay polite.
func NewDiscoverService(browser ProgramBrowser, programs *ProgramService, workers int) *DiscoverService {
	if workers <= 0 {
		workers = 6
	}
	return &DiscoverService{
		browser:  browser,
		programs: programs,
		workers:  workers,
		log:      slog.Default().With("service", "discover"),
	}
}

// WithLogger overrides the default logger.
func (s *DiscoverService) WithLogger(l *slog.Logger) *DiscoverService {
	s.log = l.With("service", "discover")
	return s
}

// Filters returns the region and industry option tables for the picker UI,
// caching the result after the first successful fetch (they're static across
// a campaign).
func (s *DiscoverService) Filters(ctx context.Context) (osvita.Filters, error) {
	s.filtersMu.Lock()
	defer s.filtersMu.Unlock()
	if s.filters != nil {
		return *s.filters, nil
	}
	f, err := s.browser.FetchFilters(ctx)
	if err != nil {
		return osvita.Filters{}, err
	}
	s.filters = &f
	return f, nil
}

// DiscoverInput is the user's profile, enough to compute their rating and
// analysis on any program. Mirrors the fields ComputeRating/Analyze need.
type DiscoverInput struct {
	NMT           map[string]float64
	CreativeScore float64
	Quotas        []string
}

// ProgramMatch pairs an enumerated program with the user's computed standing
// on it.
type ProgramMatch struct {
	Program  osvita.SpecProgram
	Rating   float64
	Analysis abit.Analysis
}

// DiscoverResult is the outcome of WhereCanIGetIn. Found is how many programs
// the filter matched; Matches holds the analyzed subset (≤ limit), ranked
// best-chance-first. When Found > len(Matches) the caller should tell the
// user the list was capped — never present a truncated set as exhaustive.
type DiscoverResult struct {
	Found   int
	Matches []ProgramMatch
}

// Browse enumerates programs for every filter and merges them, de-duped by
// URL (so multi-region searches don't double-count). Merged order follows
// the filter order, then listing order within each.
func (s *DiscoverService) Browse(ctx context.Context, filters []osvita.SpecFilter) ([]osvita.SpecProgram, error) {
	var (
		out  []osvita.SpecProgram
		seen = map[string]struct{}{}
	)
	for _, f := range filters {
		progs, err := s.browser.BrowsePrograms(ctx, f)
		if err != nil {
			return nil, err
		}
		for _, p := range progs {
			if _, dup := seen[p.URL]; dup {
				continue
			}
			seen[p.URL] = struct{}{}
			out = append(out, p)
		}
	}
	return out, nil
}

// Analyze scores the user against the given programs (cache-aware fetch +
// ComputeRating + Analyze), concurrently and bounded by s.workers. Programs
// that fail to fetch/decode are dropped. The result is NOT sorted — callers
// growing the set incrementally ("show more") merge then SortMatches once.
func (s *DiscoverService) Analyze(ctx context.Context, programs []osvita.SpecProgram, in DiscoverInput) []ProgramMatch {
	matches := make([]ProgramMatch, len(programs))
	sem := make(chan struct{}, s.workers)
	var wg sync.WaitGroup
	for i, p := range programs {
		select {
		case <-ctx.Done():
			return nil
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(i int, p osvita.SpecProgram) {
			defer wg.Done()
			defer func() { <-sem }()
			if m, ok := s.analyzeOne(ctx, p, in); ok {
				matches[i] = m
			}
		}(i, p)
	}
	wg.Wait()

	out := matches[:0]
	for _, m := range matches {
		if m.Program.URL != "" {
			out = append(out, m)
		}
	}
	return out
}

// WhereCanIGetIn is the one-shot convenience: browse every filter, analyze
// the first `limit` merged programs, return them ranked by chance. limit ≤ 0
// means "no cap". Found is the full merged count (≥ analyzed) so callers can
// flag a capped list.
func (s *DiscoverService) WhereCanIGetIn(ctx context.Context, in DiscoverInput, limit int, filters ...osvita.SpecFilter) (DiscoverResult, error) {
	programs, err := s.Browse(ctx, filters)
	if err != nil {
		return DiscoverResult{}, err
	}
	found := len(programs)
	if limit > 0 && len(programs) > limit {
		programs = programs[:limit]
	}
	matches := s.Analyze(ctx, programs, in)
	SortMatches(matches)
	return DiscoverResult{Found: found, Matches: matches}, nil
}

// analyzeOne fetches a single program and computes the user's standing on it.
// ok is false when the fetch or decode failed — a single dead program must
// not sink the whole discovery run.
func (s *DiscoverService) analyzeOne(ctx context.Context, p osvita.SpecProgram, in DiscoverInput) (ProgramMatch, bool) {
	prog, err := s.programs.Fetch(ctx, p.URL)
	if err != nil {
		s.log.WarnContext(ctx, "discover: fetch failed", "url", p.URL, "err", err)
		return ProgramMatch{}, false
	}
	abits := abit.Decode(prog)
	rating := abit.ComputeRating(prog, abit.RatingInput{
		NMT:           in.NMT,
		CreativeScore: in.CreativeScore,
	})
	analysis := abit.Analyze(prog, abits, abit.AnalyzeInput{
		UserScore:  rating,
		UserQuotas: in.Quotas,
	})
	return ProgramMatch{Program: p, Rating: rating, Analysis: analysis}, true
}

// SortMatches ranks best-chance-first: higher ChanceLevel wins, ties broken
// by more remaining spots, then better (lower) rank.
func SortMatches(m []ProgramMatch) {
	sort.SliceStable(m, func(i, j int) bool {
		a, b := m[i].Analysis, m[j].Analysis
		if a.Chance != b.Chance {
			return a.Chance > b.Chance
		}
		if a.RemainingSpots != b.RemainingSpots {
			return a.RemainingSpots > b.RemainingSpots
		}
		return a.MyRealRank < b.MyRealRank
	})
}
