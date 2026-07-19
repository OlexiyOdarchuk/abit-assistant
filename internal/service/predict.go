package service

import (
	"context"
	"log/slog"
	"sync"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
)

// PriorityPredictor walks a user's ranked application list and predicts the
// highest priority where they clear the budget cutoff — the essence of the
// Ukrainian priority-placement model: an applicant is admitted to exactly one
// program, the top one on their list where they pass.
type PriorityPredictor struct {
	programs ProgramFetcher // cache-aware program loader
	log      *slog.Logger
}

// NewPriorityPredictor wires the predictor over a program fetcher (usually
// *ProgramService, so lookups hit the shared cache).
func NewPriorityPredictor(programs ProgramFetcher) *PriorityPredictor {
	return &PriorityPredictor{
		programs: programs,
		log:      slog.Default().With("service", "predict"),
	}
}

// WithLogger overrides the default logger.
func (p *PriorityPredictor) WithLogger(l *slog.Logger) *PriorityPredictor {
	p.log = l.With("service", "predict")
	return p
}

// PredictInput is the user profile the prediction scores against. It mirrors
// what ComputeRating + Analyze need; ExcludeUnlikely toggles the optimistic
// view (drop priority-3+ rivals from each program's rank).
type PredictInput struct {
	NMT             map[string]float64
	CreativeScore   float64
	Quotas          []string
	ExcludeUnlikely bool
}

// PriorityOutcome is the analysis of one program in the ranked list.
type PriorityOutcome struct {
	URL        string
	University string
	Program    string
	Score      float64       // the user's competitive score on THIS program
	Analysis   abit.Analysis // full analysis (chance, rank, cutoff, ...)
	Fetched    bool          // false when the program couldn't be loaded
}

// Passes reports whether the user clears this program on a budget seat — the
// "admit here" signal. Uses the confident tier (published cutoff cleared, or
// ranks within the free seats / a quota).
func (o PriorityOutcome) Passes() bool {
	return o.Fetched && o.Analysis.Chance.Tier() == abit.TierSafety
}

// PriorityPrediction bundles the per-program outcomes with the predicted
// placement: the first (highest-priority) program the user passes.
type PriorityPrediction struct {
	Items         []PriorityOutcome
	AdmittedIndex int // 0-based index into Items; -1 if none pass
}

// Admitted returns the predicted placement outcome, or false if none pass.
func (p PriorityPrediction) Admitted() (PriorityOutcome, bool) {
	if p.AdmittedIndex < 0 || p.AdmittedIndex >= len(p.Items) {
		return PriorityOutcome{}, false
	}
	return p.Items[p.AdmittedIndex], true
}

// Predict fetches and analyzes every program in priority order (concurrently,
// order preserved) and marks the highest-priority pass as the placement.
// Programs that fail to load are kept in the list with Fetched=false so the UI
// can flag them without dropping the priority. A nil/empty urls yields an empty
// prediction with AdmittedIndex -1.
func (p *PriorityPredictor) Predict(ctx context.Context, urls []string, in PredictInput) PriorityPrediction {
	items := make([]PriorityOutcome, len(urls))
	var wg sync.WaitGroup
	for i, url := range urls {
		wg.Add(1)
		go func(i int, url string) {
			defer wg.Done()
			items[i] = p.analyzeOne(ctx, url, in)
		}(i, url)
	}
	wg.Wait()

	admitted := -1
	for i := range items {
		if items[i].Passes() {
			admitted = i
			break
		}
	}
	return PriorityPrediction{Items: items, AdmittedIndex: admitted}
}

// analyzeOne loads one program and scores the user against it.
func (p *PriorityPredictor) analyzeOne(ctx context.Context, url string, in PredictInput) PriorityOutcome {
	out := PriorityOutcome{URL: url}
	prog, err := p.programs.Fetch(ctx, url)
	if err != nil {
		p.log.WarnContext(ctx, "predict fetch", "url", url, "err", err)
		return out
	}
	out.Fetched = true
	out.University = prog.UniversityName
	out.Program = prog.ProgramName
	out.Score = abit.ComputeRating(prog, abit.RatingInput{NMT: in.NMT, CreativeScore: in.CreativeScore})
	out.Analysis = abit.Analyze(prog, abit.Decode(prog), abit.AnalyzeInput{
		UserScore:       out.Score,
		UserQuotas:      in.Quotas,
		ExcludeUnlikely: in.ExcludeUnlikely,
	})
	return out
}
