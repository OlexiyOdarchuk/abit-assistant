package abit

// ComputeRating returns the basic competitive rating score for an
// applicant whose НМТ/exam scores are given in nmt (keyed by subject
// name as it appears in Program.Subjects) on the given program.
//
// Formula (matches osvita.ua/consultations/konkurs-ball for EB=40):
//
//	rating = Σ ball(subject) * Subjects[subject].Coefficient
//
// where:
//   - the "Мотиваційний лист" subject (if present in the rubric) is
//     skipped — it has no numeric score
//   - the attestat (SubjectID = SubjectAttestat) is rescaled by
//     ((ball - 2) * 10 + 100) when ball >= 2, else 100 — same as
//     buildCalcInput in decoder.go does for upstream
//
// This is the basic ball — bonus modifiers (K4Max, RK, oligarch
// coefficients per row) are NOT applied here. Use this for fast
// "what would my score be?" UI; the per-applicant calculator-link
// remains the ground truth.
//
// Returns 0 when the program lacks a subject rubric, when nmt is
// empty, or when none of the rubric's subjects match the user's nmt
// keys (e.g. profile not filled for this program).
func ComputeRating(prog *Program, nmt map[string]float64) float64 {
	if prog == nil || len(prog.Subjects) == 0 || len(nmt) == 0 {
		return 0
	}
	var sum float64
	for _, subj := range prog.Subjects {
		if subj.Name == motivationalLetter {
			continue
		}
		ball, ok := nmt[subj.Name]
		if !ok {
			continue
		}
		if subj.SubjectID == SubjectAttestat && prog.EB == 40 {
			if ball >= 2 {
				ball = (ball-2)*10 + 100
			} else {
				ball = 100
			}
		}
		sum += ball * subj.Coefficient
	}
	return sum
}
