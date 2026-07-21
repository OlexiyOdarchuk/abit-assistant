package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/osvita"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
)

// handleFilters returns the galuz + region option tables for the discover
// pickers. These are static reference data (oblast + галузь codes are stable
// across campaigns), so we serve them instantly without touching osvita —
// the picker must never wait on a live scrape.
func (s *Server) handleFilters(w http.ResponseWriter, _ *http.Request) {
	f := osvita.StaticFilters()
	writeJSON(w, http.StatusOK, filtersResp{
		Regions:    regionsDTO(f.Regions),
		Industries: industriesDTO(f.Industries),
	})
}

// handleAnalyze fetches a program, scores the user, and returns the analysis
// plus the competitive list.
func (s *Server) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL     string     `json:"url"`
		Profile profileReq `json:"profile"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "некоректний запит")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), apiTimeout)
	defer cancel()

	prog, err := s.deps.Program.Fetch(ctx, req.URL)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "не вдалося отримати дані програми")
		return
	}
	abits := abit.Decode(prog)
	score := abit.ComputeRating(prog, req.Profile.rating())
	analysis := abit.Analyze(prog, abits, abit.AnalyzeInput{UserScore: score, UserQuotas: req.Profile.Quotas})
	optimistic := abit.Analyze(prog, abits, abit.AnalyzeInput{UserScore: score, UserQuotas: req.Profile.Quotas, ExcludeUnlikely: true})

	apps := make([]applicantDTO, len(abits))
	for i, ab := range abits {
		tier := abit.CompetitorNone
		if score > 0 {
			tier = abit.CompetitorTier(ab, score)
		}
		apps[i] = applicantDTO{Abiturient: ab, Tier: tier}
	}
	writeJSON(w, http.StatusOK, analyzeResp{
		Program: metaOf(prog, req.URL), UserScore: score,
		Analysis: analysis, AnalysisOptimistic: optimistic, Applicants: apps,
	})
}

// handleDiscover runs the "where can I get in" search.
func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Galuz      int        `json:"galuz"`
		Regions    []int      `json:"regions"`
		BudgetOnly bool       `json:"budgetOnly"`
		Limit      int        `json:"limit"`
		Profile    profileReq `json:"profile"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "некоректний запит")
		return
	}
	if req.Limit <= 0 || req.Limit > 60 {
		req.Limit = 20
	}
	ctx, cancel := context.WithTimeout(r.Context(), apiTimeout)
	defer cancel()

	res, err := s.deps.Discover.WhereCanIGetIn(ctx, req.Profile.discoverInput(), req.Limit,
		discoverFilters(req.Galuz, req.Regions, req.BudgetOnly)...)
	if err != nil {
		s.log.WarnContext(ctx, "discover", "err", err)
		writeErr(w, http.StatusBadGateway, "пошук не вдався")
		return
	}
	matches := make([]matchDTO, len(res.Matches))
	for i, m := range res.Matches {
		matches[i] = matchOf(m)
	}
	writeJSON(w, http.StatusOK, discoverResp{Found: res.Found, Matches: matches})
}

// handleSimulate runs the priority simulation on a program.
func (s *Server) handleSimulate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL     string     `json:"url"`
		Profile profileReq `json:"profile"`
		Deep    bool       `json:"deep"` // recursive (depth-3) resolution of borderline rivals
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "некоректний запит")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), apiTimeout)
	defer cancel()

	prog, err := s.deps.Program.Fetch(ctx, req.URL)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "не вдалося отримати дані програми")
		return
	}
	score := abit.ComputeRating(prog, req.Profile.rating())
	if score <= 0 {
		writeErr(w, http.StatusBadRequest, "заповни профіль — без власного балу немає що уточнювати")
		return
	}
	depth := 0
	if req.Deep {
		depth = service.MaxSimDepth
	}
	res, err := s.deps.Simulate.Simulate(ctx, prog, abit.Decode(prog),
		service.SimInput{UserScore: score, UserQuotas: req.Profile.Quotas, Depth: depth})
	if err != nil {
		s.log.WarnContext(ctx, "simulate", "err", err)
		writeErr(w, http.StatusBadGateway, "симуляція не вдалася")
		return
	}
	deps := make([]departureDTO, len(res.Departures))
	for i, d := range res.Departures {
		deps[i] = departureDTO{Name: d.Name, University: d.University, Priority: d.Priority, Predicted: d.Predicted}
	}
	writeJSON(w, http.StatusOK, simulateResp{
		Baseline: res.Baseline, Refined: res.Refined, Departures: deps,
		LookedUp: res.LookedUp, Masked: res.Masked, Capped: res.Capped,
	})
}

// handleApplicant returns an applicant's other applications from abit-poisk.
func (s *Server) handleApplicant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string  `json:"name"`
		Score float64 `json:"score"` // anchor: this applicant's competitive score
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "некоректний запит")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), apiTimeout)
	defer cancel()

	entries, err := s.deps.Applicant.Search(ctx, req.Name)
	if err != nil {
		if errors.Is(err, abit.ErrNoData) {
			writeJSON(w, http.StatusOK, applicantResp{Entries: []abit.ApplicantEntry{}})
			return
		}
		s.log.WarnContext(ctx, "applicant", "err", err)
		writeErr(w, http.StatusBadGateway, "не вдалося знайти інші заяви")
		return
	}
	// abit-poisk mixes namesakes (surname + initials); filter to the same
	// person, anchored on their competitive score.
	same, confident := abit.SamePersonEntries(entries, req.Score)
	writeJSON(w, http.StatusOK, applicantResp{Entries: same, Confident: confident})
}

// maxPredictURLs caps the ranked list the predictor scores per request —
// matches the campaign's 5-priority ceiling.
const maxPredictURLs = 5

// handlePredict scores the user's ranked application list and returns the
// predicted placement (highest priority they clear).
func (s *Server) handlePredict(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URLs            []string   `json:"urls"`
		Profile         profileReq `json:"profile"`
		ExcludeUnlikely bool       `json:"excludeUnlikely"`
	}
	if err := decodeBody(r, &req); err != nil {
		writeErr(w, http.StatusBadRequest, "некоректний запит")
		return
	}
	if len(req.URLs) == 0 {
		writeJSON(w, http.StatusOK, predictResp{Items: []predictItemDTO{}, AdmittedIndex: -1})
		return
	}
	if len(req.URLs) > maxPredictURLs {
		req.URLs = req.URLs[:maxPredictURLs]
	}
	ctx, cancel := context.WithTimeout(r.Context(), apiTimeout)
	defer cancel()

	pred := s.predict.Predict(ctx, req.URLs, service.PredictInput{
		NMT:             req.Profile.NMT,
		CreativeScore:   req.Profile.Creative,
		Quotas:          req.Profile.Quotas,
		ExcludeUnlikely: req.ExcludeUnlikely,
	})
	items := make([]predictItemDTO, len(pred.Items))
	for i, o := range pred.Items {
		items[i] = predictItemOf(o)
	}
	writeJSON(w, http.StatusOK, predictResp{Items: items, AdmittedIndex: pred.AdmittedIndex})
}

// discoverFilters builds one SpecFilter per chosen region (or a single
// all-Ukraine filter), mirroring the bot's logic. budgetOnly defaults the
// funding scope.
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
