package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
)

// PrioritySimulator refines a program's budget competition by removing
// competitors who will actually be placed on a higher-priority program
// elsewhere. The adaptive-placement algorithm recommends each applicant to
// exactly one budget program — their highest priority where they qualify —
// so a competitor ranked above the user, but for whom THIS program is only
// priority 2/3, frees their seat here once they're recommended on a program
// they prefer more. Confirmed: between applicants the higher score wins, and
// priority gives no edge over another applicant (deep research 2026-06-13).
//
// Signal used: a competitor's other abit-poisk applications. If any has a
// strictly-higher priority (smaller number) than its priority here AND a
// "recommended/enrolled" status, the competitor is treated as gone. This is
// robust once recommendation waves start (statuses populate); it removes
// nobody while everyone is still merely "Допущено" — pre-wave prediction by
// fetching+ranking those programs is a separate, heavier step.
type PrioritySimulator struct {
	applicants *ApplicantService
	workers    int
	maxLookups int
	log        *slog.Logger
}

// NewPrioritySimulator wires the simulator. workers bounds concurrent
// abit-poisk lookups; maxLookups caps how many above-user competitors are
// checked per run (abit-poisk is rate-sensitive).
func NewPrioritySimulator(applicants *ApplicantService, workers, maxLookups int) *PrioritySimulator {
	if workers <= 0 {
		workers = 4
	}
	if maxLookups <= 0 {
		maxLookups = 40
	}
	return &PrioritySimulator{
		applicants: applicants,
		workers:    workers,
		maxLookups: maxLookups,
		log:        slog.Default().With("service", "simulate"),
	}
}

// WithLogger overrides the default logger.
func (s *PrioritySimulator) WithLogger(l *slog.Logger) *PrioritySimulator {
	s.log = l.With("service", "simulate")
	return s
}

// SimInput is the user context for the simulation.
type SimInput struct {
	UserScore  float64
	UserQuotas []string
}

// Departure records a competitor removed from the program's competition
// because they place higher elsewhere.
type Departure struct {
	Name       string
	University string // where they're recommended instead
	Priority   int    // their priority there (lower than here)
}

// SimResult bundles the baseline and refined analyses plus the removals.
type SimResult struct {
	Baseline   abit.Analysis // original analysis (no removals)
	Refined    abit.Analysis // analysis after removing departures
	Departures []Departure
	LookedUp   int  // competitors checked on abit-poisk
	Masked     int  // candidates skipped (privacy-masked names)
	Capped     bool // hit maxLookups (more candidates went unchecked)
}

// Simulate computes the baseline analysis, looks up above-user competitors
// on abit-poisk, removes those recommended on a higher-priority program, and
// returns the refined analysis. abits should be abit.Decode(prog).
func (s *PrioritySimulator) Simulate(ctx context.Context, prog *abit.Program, abits []abit.Abiturient, in SimInput) (SimResult, error) {
	if in.UserScore <= 0 {
		return SimResult{}, errors.New("simulate: user score required")
	}
	baseline := abit.Analyze(prog, abits, abit.AnalyzeInput{
		UserScore:  in.UserScore,
		UserQuotas: in.UserQuotas,
	})

	// Candidates: real competitors ranked strictly above the user whose
	// priority here is ≥ 2 (priority 1 is their top choice — they won't
	// leave). Only strictly-higher scores affect the user's rank.
	type candidate struct {
		ab       abit.Abiturient
		priority int
	}
	var candidates []candidate
	masked := 0
	for _, ab := range abits {
		if ab.Score <= in.UserScore || ab.Priority < 2 {
			continue
		}
		if !abit.IsCompetitor(ab, in.UserScore) {
			continue
		}
		if isMaskedName(ab.Name) {
			masked++
			continue
		}
		candidates = append(candidates, candidate{ab: ab, priority: ab.Priority})
	}

	capped := false
	if len(candidates) > s.maxLookups {
		candidates = candidates[:s.maxLookups]
		capped = true
	}

	// Look up each candidate concurrently; collect departures.
	departures := make([]*Departure, len(candidates))
	sem := make(chan struct{}, s.workers)
	var wg sync.WaitGroup
	for i, cand := range candidates {
		select {
		case <-ctx.Done():
			return SimResult{}, ctx.Err()
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(i int, cand candidate) {
			defer wg.Done()
			defer func() { <-sem }()
			entries, err := s.applicants.Search(ctx, cand.ab.Name)
			if err != nil {
				if !errors.Is(err, abit.ErrNoData) {
					s.log.WarnContext(ctx, "simulate lookup", "name", cand.ab.Name, "err", err)
				}
				return
			}
			if uni, ok := placedHigher(entries, cand.priority); ok {
				departures[i] = &Departure{Name: cand.ab.Name, University: uni, Priority: betterPriority(entries, cand.priority)}
			}
		}(i, cand)
	}
	wg.Wait()

	// Build overrides marking departures as non-competitors, then re-analyze.
	overrides := abit.OverrideMap{}
	var out []Departure
	for i, d := range departures {
		if d == nil {
			continue
		}
		overrides[strconv.Itoa(candidates[i].ab.ID)] = false
		out = append(out, *d)
	}

	refined := baseline
	if len(overrides) > 0 {
		refined = abit.Analyze(prog, abits, abit.AnalyzeInput{
			UserScore:  in.UserScore,
			UserQuotas: in.UserQuotas,
			Overrides:  overrides,
		})
	}

	return SimResult{
		Baseline:   baseline,
		Refined:    refined,
		Departures: out,
		LookedUp:   len(candidates),
		Masked:     masked,
		Capped:     capped,
	}, nil
}

// placedHigher reports whether any of the applicant's other applications has
// a strictly-higher priority (smaller number) than herePriority AND a
// recommended/enrolled status — i.e. they take that seat instead of this one.
// Returns the university name of that placement.
func placedHigher(entries []abit.ApplicantEntry, herePriority int) (string, bool) {
	for _, e := range entries {
		p, err := strconv.Atoi(strings.TrimSpace(e.Priority))
		if err != nil || p <= 0 || p >= herePriority {
			continue
		}
		if isPlacedStatus(e.Status) {
			return e.University, true
		}
	}
	return "", false
}

// betterPriority returns the best (smallest) higher-priority placement
// priority for display.
func betterPriority(entries []abit.ApplicantEntry, herePriority int) int {
	best := herePriority
	for _, e := range entries {
		p, err := strconv.Atoi(strings.TrimSpace(e.Priority))
		if err != nil || p <= 0 || p >= herePriority {
			continue
		}
		if isPlacedStatus(e.Status) && p < best {
			best = p
		}
	}
	return best
}

// isPlacedStatus reports whether an abit-poisk status means the applicant
// holds/was offered a budget seat there ("рекомендовано", "до наказу",
// "включено до наказу", "зараховано").
func isPlacedStatus(status string) bool {
	low := strings.ToLower(status)
	for _, m := range []string{"рекомендов", "до наказу", "наказ", "зарахов"} {
		if strings.Contains(low, m) {
			return true
		}
	}
	return false
}
