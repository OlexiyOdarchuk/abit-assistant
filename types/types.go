package types

// ApplicantEntry описує заяву абітурієнта на Abit-poisk
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
