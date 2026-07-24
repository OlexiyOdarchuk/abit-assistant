package desktop

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/apidto"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/abitpoisk"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/osvita"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/osvitabrowser"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
)

// Desktop tuning. Unlike the server (shared cache, single scrape serves all
// users), every program fetch here launches a local headful browser to clear
// Turnstile (~20s cold). So caches live longer and fan-out is gentle — we must
// not spawn a dozen Chromes at once.
const (
	programCacheTTL   = 30 * time.Minute
	applicantCacheTTL = 24 * time.Hour
	discoverWorkers   = 2
	simWorkers        = 2
	simMaxLookups     = 40
)

// Core is the desktop application backend: it composes the reused services over
// a local SQLite cache and a local headful-browser osvita source, and exposes
// the same use cases the web API does, returning identical apidto shapes. It is
// UI-agnostic and ctx-explicit — the Wails layer is a thin adapter over it.
type Core struct {
	program   *service.ProgramService
	discover  *service.DiscoverService
	simulate  *service.PrioritySimulator
	applicant *service.ApplicantService
	predict   *service.PriorityPredictor
	log       *slog.Logger
}

// NewCore wires the backend against the given local cache. osvita's applicant
// API is Turnstile-gated, so we install the local headful-browser driver as its
// requests fallback — the static program page still loads over plain HTTP.
func NewCore(cache *Cache, log *slog.Logger) *Core {
	if log == nil {
		log = slog.Default()
	}
	browser := osvitabrowser.NewLocal(osvitabrowser.WithLocalLogger(log))
	osvitaSrc := osvita.New(osvita.WithRequestsFallback(browser))
	abitpoiskSrc := abitpoisk.New(abitpoisk.WithInsecureTLS())

	program := service.NewProgramService(osvitaSrc, cache, programCacheTTL).WithLogger(log)
	applicant := service.NewApplicantService(abitpoiskSrc, cache, applicantCacheTTL)
	discover := service.NewDiscoverService(osvitaSrc, program, discoverWorkers)
	resolver := service.NewResolver(osvitaSrc)
	simulate := service.NewPrioritySimulator(applicant, resolver, program, simWorkers, simMaxLookups)
	predict := service.NewPriorityPredictor(program)

	return &Core{
		program: program, discover: discover, simulate: simulate,
		applicant: applicant, predict: predict, log: log,
	}
}

// GetFilters returns the static discover pickers (offline reference data).
func (c *Core) GetFilters(_ context.Context) (apidto.FiltersResp, error) {
	f := osvita.StaticFilters()
	return apidto.FiltersResp{
		Regions:    apidto.RegionsDTO(f.Regions),
		Industries: apidto.IndustriesDTO(f.Industries),
	}, nil
}

// Analyze fetches a program, scores the user, and returns the analysis plus the
// competitive list. Mirrors the web handler exactly.
func (c *Core) Analyze(ctx context.Context, url string, profile apidto.Profile) (apidto.AnalyzeResp, error) {
	prog, err := c.program.Fetch(ctx, url)
	if err != nil {
		c.log.WarnContext(ctx, "analyze: fetch program", "url", url, "err", err)
		return apidto.AnalyzeResp{}, errDataFetch
	}
	abits := abit.Decode(prog)
	score := abit.ComputeRating(prog, profile.Rating())
	analysis := abit.Analyze(prog, abits, abit.AnalyzeInput{UserScore: score, UserQuotas: profile.Quotas})
	optimistic := abit.Analyze(prog, abits, abit.AnalyzeInput{UserScore: score, UserQuotas: profile.Quotas, ExcludeUnlikely: true})

	apps := make([]apidto.Applicant, len(abits))
	for i, ab := range abits {
		tier := abit.CompetitorNone
		if score > 0 {
			tier = abit.CompetitorTier(ab, score)
		}
		apps[i] = apidto.Applicant{Abiturient: ab, Tier: tier}
	}
	return apidto.AnalyzeResp{
		Program: apidto.MetaOf(prog, url), UserScore: score,
		Analysis: analysis, AnalysisOptimistic: optimistic, Applicants: apps,
	}, nil
}

// DiscoverRequest mirrors the web /api/discover body.
type DiscoverRequest struct {
	Galuz      int            `json:"galuz"`
	Regions    []int          `json:"regions"`
	BudgetOnly bool           `json:"budgetOnly"`
	Limit      int            `json:"limit"`
	Profile    apidto.Profile `json:"profile"`
}

// Discover runs the "where can I get in" search.
func (c *Core) Discover(ctx context.Context, req DiscoverRequest) (apidto.DiscoverResp, error) {
	if req.Limit <= 0 || req.Limit > 60 {
		req.Limit = 20
	}
	res, err := c.discover.WhereCanIGetIn(ctx, req.Profile.DiscoverInput(), req.Limit,
		discoverFilters(req.Galuz, req.Regions, req.BudgetOnly)...)
	if err != nil {
		c.log.WarnContext(ctx, "discover", "err", err)
		return apidto.DiscoverResp{}, errors.New("пошук не вдався")
	}
	matches := make([]apidto.Match, len(res.Matches))
	for i, m := range res.Matches {
		matches[i] = apidto.MatchOf(m)
	}
	return apidto.DiscoverResp{Found: res.Found, Matches: matches}, nil
}

// Simulate runs the priority simulation on a program. deep enables recursive
// (depth-3) resolution of borderline rivals.
func (c *Core) Simulate(ctx context.Context, url string, profile apidto.Profile, deep bool) (apidto.SimulateResp, error) {
	prog, err := c.program.Fetch(ctx, url)
	if err != nil {
		c.log.WarnContext(ctx, "simulate: fetch program", "url", url, "err", err)
		return apidto.SimulateResp{}, errDataFetch
	}
	score := abit.ComputeRating(prog, profile.Rating())
	if score <= 0 {
		return apidto.SimulateResp{}, errors.New("заповни профіль — без власного балу немає що уточнювати")
	}
	depth := 0
	if deep {
		depth = service.MaxSimDepth
	}
	res, err := c.simulate.Simulate(ctx, prog, abit.Decode(prog),
		service.SimInput{UserScore: score, UserQuotas: profile.Quotas, Depth: depth})
	if err != nil {
		c.log.WarnContext(ctx, "simulate", "err", err)
		return apidto.SimulateResp{}, errors.New("симуляція не вдалася")
	}
	deps := make([]apidto.Departure, len(res.Departures))
	for i, d := range res.Departures {
		deps[i] = apidto.Departure{Name: d.Name, University: d.University, Priority: d.Priority, Predicted: d.Predicted}
	}
	return apidto.SimulateResp{
		Baseline: res.Baseline, Refined: res.Refined, Departures: deps,
		LookedUp: res.LookedUp, Masked: res.Masked, Capped: res.Capped,
	}, nil
}

// Applicant returns an applicant's other applications from abit-poisk, filtered
// to the same person by their competitive score.
func (c *Core) Applicant(ctx context.Context, name string, score float64) (apidto.ApplicantResp, error) {
	entries, err := c.applicant.Search(ctx, name)
	if err != nil {
		if errors.Is(err, abit.ErrNoData) {
			return apidto.ApplicantResp{Entries: []abit.ApplicantEntry{}}, nil
		}
		c.log.WarnContext(ctx, "applicant", "err", err)
		return apidto.ApplicantResp{}, errors.New("не вдалося знайти інші заяви")
	}
	same, confident := abit.SamePersonEntries(entries, score)
	return apidto.ApplicantResp{Entries: same, Confident: confident}, nil
}

// maxPredictURLs caps the ranked list — matches the campaign's 5-priority ceiling.
const maxPredictURLs = 5

// Predict scores the user's ranked application list and returns the predicted
// placement (highest priority they clear).
func (c *Core) Predict(ctx context.Context, urls []string, profile apidto.Profile, excludeUnlikely bool) (apidto.PredictResp, error) {
	if len(urls) == 0 {
		return apidto.PredictResp{Items: []apidto.PredictItem{}, AdmittedIndex: -1}, nil
	}
	if len(urls) > maxPredictURLs {
		urls = urls[:maxPredictURLs]
	}
	pred := c.predict.Predict(ctx, urls, service.PredictInput{
		NMT:             profile.NMT,
		CreativeScore:   profile.Creative,
		Quotas:          profile.Quotas,
		ExcludeUnlikely: excludeUnlikely,
	})
	items := make([]apidto.PredictItem, len(pred.Items))
	for i, o := range pred.Items {
		items[i] = apidto.PredictItemOf(o)
	}
	return apidto.PredictResp{Items: items, AdmittedIndex: pred.AdmittedIndex}, nil
}

// errDataFetch is the user-facing message when a program can't be fetched
// (osvita unreachable, or the browser failed to clear Turnstile).
var errDataFetch = errors.New("не вдалося отримати дані програми")

// discoverFilters builds one SpecFilter per chosen region (or a single
// all-Ukraine filter). Mirrors the web handler.
func discoverFilters(galuz int, regions []int, budgetOnly bool) []osvita.SpecFilter {
	if len(regions) == 0 {
		return []osvita.SpecFilter{{Industry: galuz, BudgetOnly: budgetOnly}}
	}
	out := make([]osvita.SpecFilter, 0, len(regions))
	for _, region := range regions {
		out = append(out, osvita.SpecFilter{Industry: galuz, Region: region, BudgetOnly: budgetOnly})
	}
	return out
}
