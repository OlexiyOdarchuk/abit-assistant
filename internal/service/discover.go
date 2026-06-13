package service

import (
	"context"
	"log/slog"
	"sort"
	"sync"

	"github.com/OlexiyOdarchuk/abit-assistant/pkg/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/pkg/parser/osvita"
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
	RegionCoef    bool
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

// WhereCanIGetIn browses programs matching filter, scores the user against
// each (cache-aware fetch + ComputeRating + Analyze), and returns them ranked
// by chance. limit bounds how many programs are actually fetched+analyzed —
// browse can return hundreds and each fetch is a full osvita scrape, so an
// unbounded run would be slow and impolite. limit ≤ 0 means "no cap".
func (s *DiscoverService) WhereCanIGetIn(ctx context.Context, filter osvita.SpecFilter, in DiscoverInput, limit int) (DiscoverResult, error) {
	programs, err := s.browser.BrowsePrograms(ctx, filter)
	if err != nil {
		return DiscoverResult{}, err
	}
	found := len(programs)
	if limit > 0 && len(programs) > limit {
		programs = programs[:limit]
	}

	matches := make([]ProgramMatch, len(programs))
	sem := make(chan struct{}, s.workers)
	var wg sync.WaitGroup
	for i, p := range programs {
		select {
		case <-ctx.Done():
			return DiscoverResult{}, ctx.Err()
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(i int, p osvita.SpecProgram) {
			defer wg.Done()
			defer func() { <-sem }()
			m, ok := s.analyzeOne(ctx, p, in)
			if ok {
				matches[i] = m
			}
		}(i, p)
	}
	wg.Wait()

	// Drop programs that failed to fetch/decode (zero-value URL marks them).
	out := matches[:0]
	for _, m := range matches {
		if m.Program.URL != "" {
			out = append(out, m)
		}
	}
	sortMatches(out)
	return DiscoverResult{Found: found, Matches: out}, nil
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
		RegionCoef:    in.RegionCoef,
	})
	analysis := abit.Analyze(prog, abits, abit.AnalyzeInput{
		UserScore:  rating,
		UserQuotas: in.Quotas,
	})
	return ProgramMatch{Program: p, Rating: rating, Analysis: analysis}, true
}

// sortMatches ranks best-chance-first: higher ChanceLevel wins, ties broken
// by more remaining spots, then better (lower) rank.
func sortMatches(m []ProgramMatch) {
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
