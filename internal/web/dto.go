package web

import "github.com/OlexiyOdarchuk/abit-assistant/internal/apidto"

// The web's request/response shapes live in the shared internal/apidto package
// (also used by the desktop build). These aliases keep the handler code in
// api.go reading in local terms while guaranteeing byte-identical JSON across
// both front-ends.
type (
	profileReq     = apidto.Profile
	optionDTO      = apidto.Option
	filtersResp    = apidto.FiltersResp
	programMeta    = apidto.ProgramMeta
	applicantDTO   = apidto.Applicant
	applicantResp  = apidto.ApplicantResp
	analyzeResp    = apidto.AnalyzeResp
	matchDTO       = apidto.Match
	discoverResp   = apidto.DiscoverResp
	predictItemDTO = apidto.PredictItem
	predictResp    = apidto.PredictResp
	departureDTO   = apidto.Departure
	simulateResp   = apidto.SimulateResp
)

// Builder helpers re-exported so api.go keeps its short call sites.
var (
	metaOf        = apidto.MetaOf
	matchOf       = apidto.MatchOf
	predictItemOf = apidto.PredictItemOf
	industriesDTO = apidto.IndustriesDTO
	regionsDTO    = apidto.RegionsDTO
)
