// Package abit defines the core domain model: applicants, programs, and
// shared types used by parsers and decoders.
package abit

// Abiturient is a decoded applicant record ready for display or analysis.
// Raw data from sources (osvita.ua, edbo.gov.ua, abit-poisk) is shaped into
// this type by the decoder.
type Abiturient struct {
	ID             int                `json:"id"`
	Num            int                `json:"num"`
	Priority       int                `json:"priority"`
	OtherReq       int                `json:"other_req,omitempty"`
	Name           string             `json:"name"`
	Status         string             `json:"status"`
	Quota          string             `json:"quota,omitempty"`
	Coefficients   string             `json:"coefficients,omitempty"`
	RecType        string             `json:"rec_type,omitempty"`
	Score          float64            `json:"score"`
	Documents      bool               `json:"documents"`
	StateEducation bool               `json:"state_education"`
	CalcLink       string             `json:"calc_link,omitempty"`
	AbitLink       string             `json:"abit_link,omitempty"`
	DetailScores   map[string]float64 `json:"detail_scores,omitempty"`
}

// Program is everything we know about a single competitive offer
// (educational program at a university) from a source.
type Program struct {
	UniversityName  string                 `json:"university_name"`
	ProgramName     string                 `json:"program_name"`
	SpecCode        string                 `json:"spec_code"`
	ProgramInfo     map[string]string      `json:"program_info,omitempty"`
	Volume          map[string]string      `json:"volume,omitempty"`
	Statuses        map[string]string      `json:"statuses,omitempty"`
	RecTypes        map[string]string      `json:"rec_types,omitempty"`
	Requests        []RawRequest                 `json:"requests,omitempty"`
	RequestSubjects map[string]ApplicantSubjects `json:"request_subjects,omitempty"`
}

// RawRequest is a single raw applicant row as returned by the source. The
// element layout is source-specific; the decoder is the only place that
// knows what each index means.
type RawRequest = []any

// SubjectScore is a triple [score, bonus, intlOlympiadFlag] for one subject.
type SubjectScore = []float64

// ApplicantSubjects maps subject ID → SubjectScore for a single applicant.
type ApplicantSubjects = map[string]SubjectScore

// ApplicantEntry is one row from an abit-poisk search — a single application
// of a specific applicant to a specific program.
type ApplicantEntry struct {
	Degree                string `json:"degree"`
	FullName              string `json:"full_name"`
	Status                string `json:"status"`
	RankingNumber         string `json:"ranking_number"`
	Priority              string `json:"priority"`
	TotalScore            string `json:"total_score"`
	EducationAvg          string `json:"education_avg"`
	University            string `json:"university"`
	Faculty               string `json:"faculty"`
	Specialty             string `json:"specialty"`
	Quota                 string `json:"quota"`
	OriginalDocsSubmitted string `json:"original_docs_submitted"`
}
