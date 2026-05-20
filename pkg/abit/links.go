package abit

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// CalcInput is one entry in the score-calculator payload consumed by
// osvita.ua/consultations/konkurs-ball. Field names match the JS shape.
type CalcInput struct {
	SubjectID int     `json:"sb"`
	Points    float64 `json:"p"`
	K         float64 `json:"k"`
	NMT       int     `json:"nmt,omitempty"`
}

const (
	abitPoiskSearchBase = "https://abit-poisk.org.ua/#search-"
	calcURLBase         = "https://osvita.ua/consultations/konkurs-ball/?subjson="
)

// GenerateAbitPoiskLink builds a deep link to abit-poisk.org.ua for the
// given applicant's full name. The name is expected as "Прізвище І. Б."
// or "Прізвище Імʼя По-Батькові" — only the first rune of each non-empty
// initial token is kept. Returns "" when the name has fewer than two parts.
func GenerateAbitPoiskLink(name string) string {
	parts := strings.Fields(strings.TrimSpace(name))
	if len(parts) < 2 {
		return ""
	}
	surname := parts[0]
	initials := make([]string, 0, len(parts)-1)
	for _, p := range parts[1:] {
		p = strings.Trim(p, ".")
		if p == "" {
			continue
		}
		first := []rune(p)[0]
		initials = append(initials, string(first))
	}
	if len(initials) == 0 {
		return abitPoiskSearchBase + surname
	}
	return abitPoiskSearchBase + surname + "+" + strings.Join(initials, "+")
}

// GenerateCalcLink builds a URL to the osvita.ua score calculator with the
// applicant's subject inputs baked in. Returns "" when the calculator does
// not apply (only EB == 40 and OKR ∉ {4, 9} are supported by upstream).
func GenerateCalcLink(subj []CalcInput, score float64, eb, okr int) string {
	if eb != 40 || okr == 4 || okr == 9 {
		return ""
	}
	payload, err := json.Marshal(subj)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s%s&rbal=%.3f",
		calcURLBase, base64.StdEncoding.EncodeToString(payload), score)
}
