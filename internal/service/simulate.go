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

const (
	// MaxSimDepth is the hard ceiling on recursive placement resolution: how
	// many "and would THEY pass, and would the people above THEM pass…" levels
	// we descend to resolve a borderline (Medium) verdict.
	MaxSimDepth = 3
	// recurCandidateCap bounds how many above-competitors each recursion level
	// examines — the branching factor. Keeps the fan-out tractable.
	recurCandidateCap = 8
	// recurBudget caps the total abit-poisk lookups one Simulate call may spend
	// across the whole recursion tree. abit-poisk is rate-sensitive, so this is
	// the real cost governor (cache hits don't count against it — they never
	// reach the lookup).
	recurBudget = 80
)

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
	// Depth controls recursive placement resolution. 0 = shallow (a competitor
	// leaves only if they clearly pass a higher-priority program — the fast
	// default). Up to MaxSimDepth resolves borderline competitors by recursing
	// into who leaves ABOVE them. Clamped to [0, MaxSimDepth].
	Depth int
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

	// Recursion budget shared across the whole run: abit-poisk lookups are the
	// rate-sensitive resource, so one atomic caps them regardless of fan-out.
	depth := in.Depth
	if depth < 0 {
		depth = 0
	}
	if depth > MaxSimDepth {
		depth = MaxSimDepth
	}
	budget := &atomic.Int64{}
	budget.Store(recurBudget)

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
			if d, ok := s.leavesVia(ctx, cand.ab, cand.priority, depth, budget); ok {
				departures[i] = &d
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

// leavesVia decides whether competitor ab will vacate their seat at the current
// program by enrolling on one of their higher-priority (smaller number)
// programs. It looks ab up on abit-poisk, disambiguates to that exact person,
// and returns the Departure describing where they go.
//
//   - Confirmed: a higher-priority application already has a recommended/enrolled
//     status → they hold that seat (Predicted=false).
//   - Predicted: for each higher-priority program, ask wouldEnroll — would ab
//     actually clear it (recursively resolving borderline cases)? First pass
//     wins (Predicted=true).
//
// budget caps total abit-poisk lookups across the recursion; each call spends
// one. depth bounds recursion. ok=false = keep them as a competitor (the safe
// direction — we never remove on a guess).
func (s *PrioritySimulator) leavesVia(ctx context.Context, ab abit.Abiturient, herePriority, depth int, budget *atomic.Int64) (Departure, bool) {
	if budget.Add(-1) < 0 {
		return Departure{}, false // out of lookup budget → conservative
	}
	entries, err := s.applicants.Search(ctx, ab.Name)
	if err != nil {
		if !errors.Is(err, abit.ErrNoData) {
			s.log.WarnContext(ctx, "simulate lookup", "name", maskName(ab.Name), "err", err)
		}
		return Departure{}, false
	}
	// abit-poisk mixes namesakes (surname + initials only). Only trust entries
	// we can confidently attribute to THIS person (anchored on their competitive
	// score, grouped by НМТ) — otherwise we'd remove a real competitor because a
	// same-named stranger placed elsewhere.
	same, confident := abit.SamePersonEntries(entries, ab.Score)
	if !confident {
		return Departure{}, false
	}
	// Confirmed: already recommended/enrolled on a higher priority elsewhere.
	if uni, ok := placedHigher(same, herePriority); ok {
		return Departure{Name: ab.Name, University: uni, Priority: betterPriority(same, herePriority)}, true
	}
	// Predictive: resolve each higher-priority program and test enrolment,
	// best-priority-first.
	if s.resolver == nil || s.programs == nil {
		return Departure{}, false
	}
	for _, h := range higherEntries(same, herePriority) {
		url, ok := s.resolver.Resolve(ctx, h.uni, h.spec)
		if !ok {
			continue
		}
		prog, err := s.programs.Fetch(ctx, url)
		if err != nil {
			continue
		}
		if s.wouldEnroll(ctx, h.score, prog, depth, budget) {
			return Departure{Name: ab.Name, University: h.uni, Priority: h.prio, Predicted: true}, true
		}
	}
	return Departure{}, false
}

// wouldEnroll reports whether an applicant scoring `score` clears program `prog`
// on a budget seat. A clear pass/fail returns immediately; a borderline
// (Medium) verdict is resolved — while depth remains — by simulating who leaves
// ABOVE this applicant here (recursion) and re-ranking. Out of depth, Medium is
// treated as "won't pass" (conservative, so we don't over-remove).
func (s *PrioritySimulator) wouldEnroll(ctx context.Context, score float64, prog *abit.Program, depth int, budget *atomic.Int64) bool {
	abits := abit.Decode(prog)
	a := abit.Analyze(prog, abits, abit.AnalyzeInput{UserScore: score})
	switch a.Chance.Tier() {
	case abit.TierSafety:
		return true
	case abit.TierReach, abit.TierNone:
		return false
	}
	// Medium — resolve by removing above-competitors who themselves leave.
	if depth <= 0 {
		return false
	}
	overrides := abit.OverrideMap{}
	checked := 0
	for _, ab := range abits {
		if ab.Score <= score || ab.Priority < 2 || !abit.IsCompetitor(ab, score) || isMaskedName(ab.Name) {
			continue
		}
		if checked >= recurCandidateCap {
			break
		}
		checked++
		if _, ok := s.leavesVia(ctx, ab, ab.Priority, depth-1, budget); ok {
			overrides[strconv.Itoa(ab.ID)] = false
		}
	}
	if len(overrides) == 0 {
		return false
	}
	refined := abit.Analyze(prog, abits, abit.AnalyzeInput{UserScore: score, Overrides: overrides})
	return refined.Chance.Tier() == abit.TierSafety
}

// higherApp is one of an applicant's higher-priority applications, ready to
// resolve + rank.
type higherApp struct {
	uni, spec string
	score     float64
	prio      int
}

// higherEntries extracts an applicant's strictly-higher-priority applications
// (smaller priority number) with a usable score, sorted best-priority-first.
func higherEntries(entries []abit.ApplicantEntry, herePriority int) []higherApp {
	var highs []higherApp
	for _, e := range entries {
		p, err := strconv.Atoi(strings.TrimSpace(e.Priority))
		if err != nil || p <= 0 || p >= herePriority {
			continue
		}
		score, err := strconv.ParseFloat(strings.TrimSpace(e.TotalScore), 64)
		if err != nil || score <= 0 {
			continue
		}
		highs = append(highs, higherApp{uni: e.University, spec: e.Specialty, score: score, prio: p})
	}
	sort.Slice(highs, func(i, j int) bool { return highs[i].prio < highs[j].prio })
	return highs
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
