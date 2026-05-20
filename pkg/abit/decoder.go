package abit

import (
	"fmt"
	"strconv"
	"strings"
)

// Indices into a RawRequest row (osvita.ua API layout).
const (
	rawIdxID             = 0
	rawIdxNum            = 1
	rawIdxPriority       = 2
	rawIdxStatus         = 3
	rawIdxName           = 4
	rawIdxScore          = 5
	rawIdxQuota1         = 6
	rawIdxQuota2         = 7
	rawIdxQuota3         = 8
	rawIdxCoefGK         = 9
	rawIdxCoefSK         = 10
	rawIdxCoefPCHK       = 11
	rawIdxCoefOL         = 12
	rawIdxCoefKR         = 13
	rawIdxDocuments      = 14
	rawIdxRecType        = 15
	rawIdxInterview      = 16
	rawIdxStateEducation = 17
	rawIdxOtherReq       = 18
	rawRowMinLen         = 19
)

// Global subject IDs (SubjectMeta.SubjectID).
const (
	SubjectAttestat = 100
	SubjectGK       = 109
	SubjectSK       = 110
	SubjectPCHK     = 111
	SubjectOL       = 112
	SubjectRK       = 120
	SubjectK4Max    = 140
)

// Status codes that get special-cased during decoding.
const (
	statusAllowed  = 6  // "Допущено" — displayed via RecType instead of plain text
	statusAccepted = 14 // "До наказу" — needs a (бюджет)/(контракт) suffix
)

const motivationalLetter = "Мотиваційний лист"

// Decode transforms Program raw requests into a slice of Abiturient
// records. The returned order mirrors Program.Requests (ranking order).
// Malformed rows are skipped silently — Decode never returns an error for
// per-row issues; callers that need strictness can wrap DecodeRow.
func Decode(p *Program) []Abiturient {
	if p == nil {
		return nil
	}
	out := make([]Abiturient, 0, len(p.Requests))
	for _, row := range p.Requests {
		ab, err := DecodeRow(p, row)
		if err != nil {
			continue
		}
		out = append(out, ab)
	}
	return out
}

// DecodeRow turns one raw applicant record into a typed Abiturient.
// Rows shorter than 19 elements are zero-padded (some pages omit trailing
// fields). An error is returned only if the row is completely unusable
// (missing ID).
func DecodeRow(p *Program, row RawRequest) (Abiturient, error) {
	if len(row) < rawRowMinLen {
		padded := make(RawRequest, rawRowMinLen)
		copy(padded, row)
		row = padded
	}

	id, ok := intAt(row, rawIdxID)
	if !ok || id == 0 {
		return Abiturient{}, fmt.Errorf("abit: missing id at row")
	}
	num, _ := intAt(row, rawIdxNum)
	priority, _ := intAt(row, rawIdxPriority)
	if priority < 0 {
		priority = 0
	}
	status, _ := intAt(row, rawIdxStatus)
	name, _ := strAt(row, rawIdxName)
	score, _ := floatAt(row, rawIdxScore)
	docs, _ := intAt(row, rawIdxDocuments)
	recType, _ := intAt(row, rawIdxRecType)
	stateEdu, _ := intAt(row, rawIdxStateEducation)
	otherReq, _ := intAt(row, rawIdxOtherReq)

	ab := Abiturient{
		ID:             id,
		Num:            num,
		Priority:       priority,
		Name:           name,
		Score:          score,
		Documents:      docs == 1,
		StateEducation: stateEdu == 1,
	}
	// OtherReq shown only when distinct from priority and not "no recommendation".
	if otherReq != 0 && otherReq != priority && recType != -1 {
		ab.OtherReq = otherReq
	}
	ab.Status = decodeStatus(p, status, stateEdu)
	ab.RecType = decodeRecType(p, status, recType, otherReq, priority)
	ab.Quota = decodeQuotas(row)
	ab.Coefficients = decodeCoefficients(p, row)

	subj := buildCalcInput(p, &ab, row)
	subj = append(subj, extraCoefficientRows(row)...)
	if p.EB == 40 && p.K4Max > 0 && hasSubjectForK4(p) && len(subj) > 3 {
		subj = append(subj, CalcInput{SubjectID: SubjectK4Max, K: p.K4Max})
	}
	if p.RK > 1 && p.EB == 40 {
		subj = append(subj, CalcInput{SubjectID: SubjectRK, K: p.RK})
	}

	ab.AbitLink = GenerateAbitPoiskLink(ab.Name)
	ab.CalcLink = GenerateCalcLink(subj, ab.Score, p.EB, p.OKR)
	return ab, nil
}

// decodeStatus returns the human-readable status name. Unlike decoder.py,
// which leaves status="" for code 6 and relies on the UI to fall back to
// rec_type, we always populate Status — RecType is additive info that the
// UI may choose to surface alongside.
func decodeStatus(p *Program, code, stateEdu int) string {
	name, ok := p.Statuses[strconv.Itoa(code)]
	if !ok {
		return ""
	}
	if code == statusAccepted {
		if stateEdu == 1 {
			return name + " (бюджет)"
		}
		return name + " (контракт)"
	}
	return name
}

func decodeRecType(p *Program, status, recType, otherReq, priority int) string {
	if status != statusAllowed || recType == 0 {
		return ""
	}
	name, ok := p.RecTypes[strconv.Itoa(recType)]
	if !ok || name == "" {
		return ""
	}
	if recType == -1 {
		return name
	}
	if otherReq == priority && recType != 1 {
		return name
	}
	return ""
}

func decodeQuotas(row RawRequest) string {
	out := make([]string, 0, 4)
	if v, _ := intAt(row, rawIdxQuota1); v > 0 {
		out = append(out, "КВ1")
	}
	if v, _ := intAt(row, rawIdxQuota2); v > 0 {
		out = append(out, "КВ2")
	}
	if v, _ := intAt(row, rawIdxQuota3); v > 0 {
		out = append(out, "КВ3")
	}
	if v, _ := intAt(row, rawIdxInterview); v > 0 {
		out = append(out, "СБ")
	}
	return strings.Join(out, ", ")
}

func decodeCoefficients(p *Program, row RawRequest) string {
	out := make([]string, 0, 7)
	if v, _ := floatAt(row, rawIdxCoefGK); v > 1 {
		out = append(out, "ГК")
	}
	if v, _ := floatAt(row, rawIdxCoefSK); v > 1 {
		out = append(out, "СК")
	}
	if v, _ := floatAt(row, rawIdxCoefPCHK); v > 0 {
		out = append(out, "ПЧК")
	}
	if v, _ := floatAt(row, rawIdxCoefOL); v > 0 {
		out = append(out, "ОЛ")
	}
	if v, _ := floatAt(row, rawIdxCoefKR); v > 0 {
		out = append(out, "КР")
	}
	if p.RK > 1 && p.EB == 40 {
		out = append(out, "РК")
	}
	if v, _ := intAt(row, rawIdxInterview); v > 0 {
		out = append(out, "СБ")
	}
	return strings.Join(out, ", ")
}

// buildCalcInput assembles the per-subject input rows for the calculator
// and, as a side effect, populates ab.DetailScores with the per-subject
// raw scores keyed by subject name.
func buildCalcInput(p *Program, ab *Abiturient, row RawRequest) []CalcInput {
	received := p.RequestSubjects[strconv.Itoa(ab.ID)]
	if len(received) == 0 || len(p.Subjects) == 0 {
		return nil
	}
	if ab.DetailScores == nil {
		ab.DetailScores = make(map[string]float64, len(p.Subjects))
	}
	nmtSet := intSet(p.NMTs)

	out := make([]CalcInput, 0, len(p.Subjects))
	for _, subj := range p.Subjects {
		if subj.Name == motivationalLetter {
			continue
		}
		scores, ok := received[strconv.Itoa(subj.ID)]
		if !ok || len(scores) < 3 {
			continue
		}
		ball := scores[0]
		if scores[2] == 1 { // international olympiad → max
			ball = 200
		}
		if scores[1] > 0 { // additional points
			ball += scores[1]
			if ball > 200 {
				ball = 200
			}
		}
		entry := CalcInput{SubjectID: subj.SubjectID, Points: ball, K: subj.Coefficient}
		if subj.SubjectID == SubjectAttestat && p.EB == 40 {
			// Attestat is rescaled by upstream: ((ball-2)*10 + 100) for ball>=2.
			if ball >= 2 {
				entry.Points = (ball-2)*10 + 100
			} else {
				entry.Points = 100
			}
		}
		if _, isNMT := nmtSet[subj.ID]; isNMT {
			entry.NMT = 1
		}
		out = append(out, entry)
		ab.DetailScores[subj.Name] = ball
	}
	return out
}

func extraCoefficientRows(row RawRequest) []CalcInput {
	out := make([]CalcInput, 0, 4)
	if v, _ := floatAt(row, rawIdxCoefOL); v > 0 {
		out = append(out, CalcInput{SubjectID: SubjectOL, Points: 10, K: 1})
	}
	if v, _ := floatAt(row, rawIdxCoefPCHK); v > 0 {
		out = append(out, CalcInput{SubjectID: SubjectPCHK, K: v})
	}
	if v, _ := floatAt(row, rawIdxCoefSK); v > 0 {
		out = append(out, CalcInput{SubjectID: SubjectSK, K: v})
	}
	if v, _ := floatAt(row, rawIdxCoefGK); v > 1 {
		out = append(out, CalcInput{SubjectID: SubjectGK, K: v})
	}
	return out
}

func hasSubjectForK4(p *Program) bool {
	set := intSet(p.Sub4ar)
	for _, s := range p.Subjects {
		if _, ok := set[s.ID]; ok {
			return true
		}
	}
	return false
}

func intSet(ids []int) map[int]struct{} {
	out := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if id != 0 {
			out[id] = struct{}{}
		}
	}
	return out
}

// --- typed indexed access into RawRequest ---

func intAt(row RawRequest, i int) (int, bool) {
	if i >= len(row) {
		return 0, false
	}
	switch x := row[i].(type) {
	case float64:
		return int(x), true
	case int:
		return x, true
	case string:
		v, err := strconv.Atoi(x)
		return v, err == nil
	case nil:
		return 0, true
	default:
		return 0, false
	}
}

func floatAt(row RawRequest, i int) (float64, bool) {
	if i >= len(row) {
		return 0, false
	}
	switch x := row[i].(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case string:
		v, err := strconv.ParseFloat(x, 64)
		return v, err == nil
	case nil:
		return 0, true
	default:
		return 0, false
	}
}

func strAt(row RawRequest, i int) (string, bool) {
	if i >= len(row) {
		return "", false
	}
	s, ok := row[i].(string)
	return s, ok
}
