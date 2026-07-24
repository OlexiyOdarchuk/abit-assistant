// Package apidto is the shared request/response contract between the AbitAssistant
// core (internal/service, internal/abit) and its presentation front-ends. Both
// the web server (internal/web) and the desktop app (internal/desktop) speak
// these exact JSON shapes, so the same Svelte frontend runs against either —
// over HTTP on the web, over Wails bindings on the desktop.
package apidto

import (
	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/osvita"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
)

// Profile is the stateless user profile carried in every request — there are
// no accounts, so the client sends the NMT scores and settings each time.
//
// The regional coefficient (РК) is NOT a user setting: it is a constant of the
// oblast where the university sits, applied automatically from prog.RK, so it
// is absent here.
type Profile struct {
	NMT      map[string]float64 `json:"nmt"`
	Quotas   []string           `json:"quotas"`
	Creative float64            `json:"creative"`
}

// Rating adapts the profile to abit.ComputeRating's input.
func (p Profile) Rating() abit.RatingInput {
	return abit.RatingInput{NMT: p.NMT, CreativeScore: p.Creative}
}

// DiscoverInput adapts the profile to the discover use case.
func (p Profile) DiscoverInput() service.DiscoverInput {
	return service.DiscoverInput{NMT: p.NMT, CreativeScore: p.Creative, Quotas: p.Quotas}
}

// Option is one selectable filter value (region or industry).
type Option struct {
	Code   int    `json:"code"`
	Name   string `json:"name"`
	Letter string `json:"letter,omitempty"` // galuz letter A–K (industries only)
}

// FiltersResp is the discover picker's static reference data.
type FiltersResp struct {
	Regions    []Option `json:"regions"`
	Industries []Option `json:"industries"`
}

// ProgramMeta is the identifying header of a program.
type ProgramMeta struct {
	University string `json:"university"`
	Program    string `json:"program"`
	SpecCode   string `json:"specCode"`
	URL        string `json:"url"`
	Budget     int    `json:"budget"`
	Quota1     int    `json:"quota1"`
	Quota2     int    `json:"quota2"`
	SourceAsOf string `json:"sourceAsOf,omitempty"`
}

// MetaOf builds a ProgramMeta from a fetched program.
func MetaOf(prog *abit.Program, url string) ProgramMeta {
	return ProgramMeta{
		University: prog.UniversityName, Program: prog.ProgramName, SpecCode: prog.SpecCode,
		URL: url, Budget: prog.BudgetVolume(), Quota1: prog.Quota1Volume(), Quota2: prog.Quota2Volume(),
		SourceAsOf: prog.SourceAsOf,
	}
}

// Applicant is one row of the competitive list: Abiturient flattened with the
// server-computed competitor tier (abit.CompetitorTier):
// 0 none · 1 unlikely (priority 3+, 🔴→⚪) · 2 potential (priority 2, 🟡) ·
// 3 real (priority 1 / enrolled, 🔴).
type Applicant struct {
	abit.Abiturient
	Tier int `json:"tier"`
}

// ApplicantResp wraps abit-poisk "other applications" plus a flag telling the
// client whether they were disambiguated to the same person.
type ApplicantResp struct {
	Entries   []abit.ApplicantEntry `json:"entries"`
	Confident bool                  `json:"confident"`
}

// AnalyzeResp is the full competitive analysis of one program.
type AnalyzeResp struct {
	Program   ProgramMeta   `json:"program"`
	UserScore float64       `json:"userScore"`
	Analysis  abit.Analysis `json:"analysis"`
	// AnalysisOptimistic is the same analysis with priority-3+ (⚪ unlikely)
	// rivals dropped — the client's "не рахувати пріоритет 3+" toggle swaps to
	// it without a round-trip.
	AnalysisOptimistic abit.Analysis `json:"analysisOptimistic"`
	Applicants         []Applicant   `json:"applicants"`
}

// Match is one program returned by discover.
type Match struct {
	URL        string  `json:"url"`
	University string  `json:"university"`
	Program    string  `json:"program"`
	Specialty  string  `json:"specialty"`
	Budget     bool    `json:"budget"`
	Rating     float64 `json:"rating"`
	Chance     string  `json:"chance"`
	ChanceTier int     `json:"chanceTier"`
	Emoji      string  `json:"emoji"`
	Rank       int     `json:"rank"`
	Remaining  int     `json:"remaining"`
}

// MatchOf builds a Match DTO from a service result.
func MatchOf(m service.ProgramMatch) Match {
	return Match{
		URL: m.Program.URL, University: m.Program.University, Program: m.Program.Program,
		Specialty: m.Program.Specialty, Budget: m.Program.Budget, Rating: m.Rating,
		Chance: m.Analysis.Chance.Label(), ChanceTier: int(m.Analysis.Chance.Tier()),
		Emoji: m.Analysis.Chance.Emoji(), Rank: m.Analysis.MyRealRank, Remaining: m.Analysis.RemainingSpots,
	}
}

// DiscoverResp is the discover result set.
type DiscoverResp struct {
	Found   int     `json:"found"`
	Matches []Match `json:"matches"`
}

// PredictItem is one program in the user's ranked list, scored.
type PredictItem struct {
	URL        string   `json:"url"`
	University string   `json:"university"`
	Program    string   `json:"program"`
	Score      float64  `json:"score"`
	Fetched    bool     `json:"fetched"`
	Chance     string   `json:"chance"`
	ChanceTier int      `json:"chanceTier"`
	Emoji      string   `json:"emoji"`
	Cutoff     float64  `json:"cutoff,omitempty"`
	Passes     bool     `json:"passes"`
	Warnings   []string `json:"warnings,omitempty"`
}

// PredictItemOf builds a PredictItem from a service outcome.
func PredictItemOf(o service.PriorityOutcome) PredictItem {
	return PredictItem{
		URL: o.URL, University: o.University, Program: o.Program, Score: o.Score,
		Fetched: o.Fetched, Chance: o.Analysis.Chance.Label(), ChanceTier: int(o.Analysis.Chance.Tier()),
		Emoji: o.Analysis.Chance.Emoji(), Cutoff: o.Analysis.Cutoff, Passes: o.Passes(),
		Warnings: o.Analysis.Warnings,
	}
}

// PredictResp is the ranked-list prediction.
type PredictResp struct {
	Items         []PredictItem `json:"items"`
	AdmittedIndex int           `json:"admittedIndex"`
}

// Departure is one rival predicted to leave in the priority simulation.
type Departure struct {
	Name       string `json:"name"`
	University string `json:"university"`
	Priority   int    `json:"priority"`
	Predicted  bool   `json:"predicted"`
}

// SimulateResp is the priority-simulation result.
type SimulateResp struct {
	Baseline   abit.Analysis `json:"baseline"`
	Refined    abit.Analysis `json:"refined"`
	Departures []Departure   `json:"departures"`
	LookedUp   int           `json:"lookedUp"`
	Masked     int           `json:"masked"`
	Capped     bool          `json:"capped"`
}

// IndustriesDTO maps osvita filter options to industry Options (with letters).
func IndustriesDTO(opts []osvita.FilterOption) []Option {
	out := make([]Option, 0, len(opts))
	for _, o := range opts {
		out = append(out, Option{Code: o.Code, Name: o.Name, Letter: osvita.GaluzLetters[o.Code]})
	}
	return out
}

// RegionsDTO maps osvita filter options to region Options.
func RegionsDTO(opts []osvita.FilterOption) []Option {
	out := make([]Option, 0, len(opts))
	for _, o := range opts {
		out = append(out, Option{Code: o.Code, Name: o.Name})
	}
	return out
}
