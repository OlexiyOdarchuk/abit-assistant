package web

import (
	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/osvita"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
)

// profileReq is the stateless user profile carried in request bodies — the
// web has no accounts (v1), so the client sends the NMT scores and settings
// with each request. Mirrors what ComputeRating / Analyze / Discover need.
type profileReq struct {
	NMT        map[string]float64 `json:"nmt"`
	Quotas     []string           `json:"quotas"`
	RegionCoef bool               `json:"regionCoef"`
	Creative   float64            `json:"creative"`
}

func (p profileReq) rating() abit.RatingInput {
	return abit.RatingInput{NMT: p.NMT, CreativeScore: p.Creative, RegionCoef: p.RegionCoef}
}

func (p profileReq) discoverInput() service.DiscoverInput {
	return service.DiscoverInput{NMT: p.NMT, CreativeScore: p.Creative, RegionCoef: p.RegionCoef, Quotas: p.Quotas}
}

// --- responses ---

type optionDTO struct {
	Code   int    `json:"code"`
	Name   string `json:"name"`
	Letter string `json:"letter,omitempty"` // galuz letter A–K (industries only)
}

type filtersResp struct {
	Regions    []optionDTO `json:"regions"`
	Industries []optionDTO `json:"industries"`
}

type programMeta struct {
	University string `json:"university"`
	Program    string `json:"program"`
	SpecCode   string `json:"specCode"`
	URL        string `json:"url"`
	Budget     int    `json:"budget"`
	Quota1     int    `json:"quota1"`
	Quota2     int    `json:"quota2"`
}

func metaOf(prog *abit.Program, url string) programMeta {
	return programMeta{
		University: prog.UniversityName, Program: prog.ProgramName, SpecCode: prog.SpecCode,
		URL: url, Budget: prog.BudgetVolume(), Quota1: prog.Quota1Volume(), Quota2: prog.Quota2Volume(),
	}
}

// applicantDTO is one row of the competitive list, flattening Abiturient plus
// a server-computed "is this a real competitor for me" flag.
type applicantDTO struct {
	abit.Abiturient
	Competitor bool `json:"competitor"`
}

type analyzeResp struct {
	Program    programMeta    `json:"program"`
	UserScore  float64        `json:"userScore"`
	Analysis   abit.Analysis  `json:"analysis"`
	Applicants []applicantDTO `json:"applicants"`
}

type matchDTO struct {
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

func matchOf(m service.ProgramMatch) matchDTO {
	return matchDTO{
		URL: m.Program.URL, University: m.Program.University, Program: m.Program.Program,
		Specialty: m.Program.Specialty, Budget: m.Program.Budget, Rating: m.Rating,
		Chance: m.Analysis.Chance.Label(), ChanceTier: int(m.Analysis.Chance.Tier()),
		Emoji: m.Analysis.Chance.Emoji(), Rank: m.Analysis.MyRealRank, Remaining: m.Analysis.RemainingSpots,
	}
}

type discoverResp struct {
	Found   int        `json:"found"`
	Matches []matchDTO `json:"matches"`
}

type departureDTO struct {
	Name       string `json:"name"`
	University string `json:"university"`
	Priority   int    `json:"priority"`
	Predicted  bool   `json:"predicted"`
}

type simulateResp struct {
	Baseline   abit.Analysis  `json:"baseline"`
	Refined    abit.Analysis  `json:"refined"`
	Departures []departureDTO `json:"departures"`
	LookedUp   int            `json:"lookedUp"`
	Masked     int            `json:"masked"`
	Capped     bool           `json:"capped"`
}

func industriesDTO(opts []osvita.FilterOption) []optionDTO {
	out := make([]optionDTO, 0, len(opts))
	for _, o := range opts {
		out = append(out, optionDTO{Code: o.Code, Name: o.Name, Letter: osvita.GaluzLetters[o.Code]})
	}
	return out
}

func regionsDTO(opts []osvita.FilterOption) []optionDTO {
	out := make([]optionDTO, 0, len(opts))
	for _, o := range opts {
		out = append(out, optionDTO{Code: o.Code, Name: o.Name})
	}
	return out
}
