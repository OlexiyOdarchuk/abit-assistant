package service

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
)

// predictCap bounds how many not-yet-recommended competitors get the
// expensive pre-wave prediction (each costs a resolve + program fetch).
const predictCap = 12

// ProgramResolver maps a competitor's (university, specialty) to an osvita
// program URL. Satisfied by *Resolver. nil disables pre-wave prediction.
type ProgramResolver interface {
	Resolve(ctx context.Context, university, specialty string) (string, bool)
}

// ProgramFetcher fetches a program by URL (cache-aware). Satisfied by
// *ProgramService. nil disables pre-wave prediction.
type ProgramFetcher interface {
	Fetch(ctx context.Context, url string) (*abit.Program, error)
}

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
	resolver   ProgramResolver // nil → no pre-wave prediction
	programs   ProgramFetcher  // nil → no pre-wave prediction
	workers    int
	maxLookups int
	log        *slog.Logger
}

// NewPrioritySimulator wires the simulator. resolver+programs enable pre-wave
// prediction (pass nil for the status-only mode). workers bounds concurrent
// abit-poisk lookups; maxLookups caps how many above-user competitors are
// checked per run (abit-poisk is rate-sensitive).
func NewPrioritySimulator(applicants *ApplicantService, resolver ProgramResolver, programs ProgramFetcher, workers, maxLookups int) *PrioritySimulator {
	if workers <= 0 {
		workers = 4
	}
	if maxLookups <= 0 {
		maxLookups = 40
	}
	return &PrioritySimulator{
		applicants: applicants,
		resolver:   resolver,
		programs:   programs,
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
	Predicted  bool   // true = pre-wave estimate (would pass there), not a published recommendation
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
	predictEnabled := s.resolver != nil && s.programs != nil
	var predictUsed atomic.Int64
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
					s.log.WarnContext(ctx, "simulate lookup", "name", maskName(cand.ab.Name), "err", err)
				}
				return
			}
			// Confirmed: already recommended on a higher priority elsewhere.
			if uni, ok := placedHigher(entries, cand.priority); ok {
				departures[i] = &Departure{Name: cand.ab.Name, University: uni, Priority: betterPriority(entries, cand.priority)}
				return
			}
			// Pre-wave: predict by fetching+ranking their higher-priority
			// programs (bounded — each one is a resolve + program fetch).
			if predictEnabled && predictUsed.Add(1) <= predictCap {
				if uni, prio, ok := s.predictDeparture(ctx, entries, cand.priority); ok {
					departures[i] = &Departure{Name: cand.ab.Name, University: uni, Priority: prio, Predicted: true}
				}
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

// predictDeparture estimates whether a not-yet-recommended competitor will be
// placed on one of their higher-priority programs: it resolves each
// higher-priority application to an osvita program, fetches it, and ranks the
// competitor (by their score there) — if they pass on any, they'll take that
// seat instead of this one. Checks them best-priority-first and returns on the
// first pass. ok=false when nothing resolves or none pass (conservative — no
// guesses).
func (s *PrioritySimulator) predictDeparture(ctx context.Context, entries []abit.ApplicantEntry, herePriority int) (string, int, bool) {
	type higher struct {
		uni, spec, score string
		prio             int
	}
	var highs []higher
	for _, e := range entries {
		p, err := strconv.Atoi(strings.TrimSpace(e.Priority))
		if err != nil || p <= 0 || p >= herePriority {
			continue
		}
		highs = append(highs, higher{uni: e.University, spec: e.Specialty, score: e.TotalScore, prio: p})
	}
	sort.Slice(highs, func(i, j int) bool { return highs[i].prio < highs[j].prio })

	for _, h := range highs {
		score, err := strconv.ParseFloat(strings.TrimSpace(h.score), 64)
		if err != nil || score <= 0 {
			continue
		}
		url, ok := s.resolver.Resolve(ctx, h.uni, h.spec)
		if !ok {
			continue
		}
		prog, err := s.programs.Fetch(ctx, url)
		if err != nil {
			continue
		}
		a := abit.Analyze(prog, abit.Decode(prog), abit.AnalyzeInput{UserScore: score})
		if isPassingChance(a.Chance) {
			return h.uni, h.prio, true
		}
	}
	return "", 0, false
}

// isPassingChance reports whether a chance level means the applicant clears
// the budget cutoff. The prediction ranks competitors in the GENERAL pool
// (their quota status on the target program is unknown to us), so only
// ChanceHigh is reachable here — the quota-pass levels need quota input we
// don't pass, and Medium is too uncertain to act on. Erring toward "won't
// pass" keeps the simulator from over-removing.
func isPassingChance(c abit.ChanceLevel) bool {
	return c == abit.ChanceHigh
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
// "включено до наказу", "зараховано"). Negative statuses are excluded first
// because some of them also contain "наказ" (e.g. "виключено з наказу",
// "наказ про відрахування") and must NOT count as a held seat.
func isPlacedStatus(status string) bool {
	low := strings.ToLower(status)
	for _, neg := range []string{"виключено", "відрахов", "деактив", "скасов", "відмов"} {
		if strings.Contains(low, neg) {
			return false
		}
	}
	for _, m := range []string{"рекомендов", "до наказу", "наказ", "зарахов"} {
		if strings.Contains(low, m) {
			return true
		}
	}
	return false
}
