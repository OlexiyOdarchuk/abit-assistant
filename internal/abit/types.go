// Package abit defines the core domain model: applicants, programs, and
// shared types used by parsers and decoders.
package abit

// Abiturient is a decoded applicant record ready for display or analysis.
// Raw data from sources (osvita.ua, edbo.gov.ua, abit-poisk) is shaped into
// this type by the decoder.
type Abiturient struct {
	ID       int    `json:"id"`
	Num      int    `json:"num"`
	Priority int    `json:"priority"`
	OtherReq int    `json:"other_req,omitempty"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	// Quotas is the list of quota codes the applicant qualifies under
	// (QuotaKV1, QuotaKV2, QuotaKV3, QuotaSB).
	Quotas []string `json:"quotas,omitempty"`
	// Coefficients is the list of bonus-coefficient codes applied to
	// the applicant's final score (CoefGK, CoefSK, CoefPCHK, CoefOL,
	// CoefKR, CoefRK, CoefSB).
	Coefficients   []string           `json:"coefficients,omitempty"`
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
	UniversityName string `json:"university_name"`
	ProgramName    string `json:"program_name"`
	SpecCode       string `json:"spec_code"`

	// EB is the education-base code (40 = on top of complete general
	// secondary education). Most score-calculation rules only kick in
	// when EB == 40.
	EB int `json:"eb"`
	// OKR is the educational-qualification-level code (1 = bachelor, ...).
	OKR int `json:"okr"`
	// K4Max is the maximum coefficient applied to the 4th subject (default 0.35).
	K4Max float64 `json:"k4max"`
	// RK is the regional coefficient (default 1.0).
	RK float64 `json:"rk"`
	// NMTs lists program-local subject IDs that count as NMT (НМТ).
	NMTs []int `json:"nmts,omitempty"`
	// Sub4ar lists program-local subject IDs eligible for the 4th-subject
	// bonus (k4max).
	Sub4ar []int `json:"sub4ar,omitempty"`
	// Subjects is the program's subject rubric (name, weights, IDs).
	Subjects []SubjectMeta `json:"subjects,omitempty"`

	// SourceAsOf is osvita's own "data as of" stamp for the EDBO sync that
	// produced this page, e.g. "19.07.2026 12:00" (from "Дані отримані з
	// ЄДЕБО …"). Empty when not present. Used to show data freshness and to
	// fire change-notifications right after a real refresh.
	SourceAsOf string `json:"source_as_of,omitempty"`

	ProgramInfo     map[string]string            `json:"program_info,omitempty"`
	Volume          map[string]string            `json:"volume,omitempty"`
	Statuses        map[string]string            `json:"statuses,omitempty"`
	RecTypes        map[string]string            `json:"rec_types,omitempty"`
	Requests        []RawRequest                 `json:"requests,omitempty"`
	RequestSubjects map[string]ApplicantSubjects `json:"request_subjects,omitempty"`
}

// SubjectMeta describes one subject within a program's rubric.
type SubjectMeta struct {
	// ID is the program-local subject ID, referenced from
	// RequestSubjects keys, NMTs and Sub4ar.
	ID int `json:"id"`
	// SubjectID is the global subject identifier — used by the score
	// calculator (e.g. SubjectAttestat, SubjectGK, ...).
	SubjectID int `json:"si"`
	// Name is the human-readable subject name ("Українська мова",
	// "Математика", ...). Used as the key in Abiturient.DetailScores.
	Name string `json:"s"`
	// Coefficient is the weight applied to the subject score.
	Coefficient float64 `json:"k"`
	// Required is 1 when the subject is mandatory, 0 when optional.
	Required int `json:"x"`
	// EFID is the educational-form ID associated with the subject.
	EFID int `json:"ef"`
	// VW is an opaque flag carried verbatim from osvita.
	VW int `json:"vw"`
	// IsNMT mirrors the per-subject "nmt" field.
	IsNMT int `json:"nmt"`
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
	Degree        string `json:"degree"`
	FullName      string `json:"full_name"`
	Status        string `json:"status"`
	RankingNumber string `json:"ranking_number"`
	Priority      string `json:"priority"`
	TotalScore    string `json:"total_score"`
	EducationAvg  string `json:"education_avg"`
	// SubjectScores is the applicant's per-subject НМТ breakdown as abit-poisk
	// renders it ("Українська мова 177 Математика 167 …", sometimes with a
	// trailing "РК: 1.07"). It is IDENTICAL across all of one person's
	// applications, so it's the reliable person-invariant for disambiguating
	// namesakes (surname+initials collide; the attestat is no longer submitted).
	SubjectScores         string `json:"subject_scores"`
	University            string `json:"university"`
	Faculty               string `json:"faculty"`
	Specialty             string `json:"specialty"`
	Quota                 string `json:"quota"`
	OriginalDocsSubmitted string `json:"original_docs_submitted"`
}
