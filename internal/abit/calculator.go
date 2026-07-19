package abit

import "math"

// ForeignLanguageSubject is the generic name programs use for the foreign-
// language slot in their rubric. The profile lets the user pick a specific
// language, so those names must be canonicalised to this before looking up
// the program's coefficient.
const ForeignLanguageSubject = "Іноземна мова"

// foreignLanguageAliases are profile subject names that all map to the
// rubric's generic ForeignLanguageSubject.
var foreignLanguageAliases = map[string]bool{
	"Англійська мова":    true,
	"Німецька мова":      true,
	"Французька мова":    true,
	"Іспанська мова":     true,
	"Інша іноземна":      true,
	"Інша іноземна мова": true,
}

// canonicalSubject maps a (possibly profile-specific) subject name to the name
// programs use in their rubric. Foreign languages collapse to "Іноземна мова";
// everything else passes through unchanged. This is what makes a profile score
// entered as "Англійська мова" count against a rubric that weighs "Іноземна
// мова" — without it the elective subject silently dropped and the competitive
// score was computed from three subjects instead of four.
func canonicalSubject(name string) string {
	if foreignLanguageAliases[name] {
		return ForeignLanguageSubject
	}
	return name
}

// RatingInput bundles everything ComputeRating needs from the user
// profile. Keeping it in a struct lets callers grow the inputs (e.g.
// extra modifiers) without breaking the call sites.
type RatingInput struct {
	// NMT maps subject name → user's score for that subject.
	NMT map[string]float64
	// CreativeScore is the user-entered prediction for programs that
	// require a creative contest. 0 means "not provided".
	CreativeScore float64
}

// ComputeRating returns the applicant's competitive rating on the given
// program, mirroring the algorithm used by AbitAssistant since v2:
//
//  1. Required subjects (RequiredSubjects) are taken from in.NMT, each
//     weighted by the program's coefficient for that subject.
//  2. The single best additional subject (max ball*coef) is selected
//     from the remaining user scores; CreativeContest is excluded — it
//     comes via in.CreativeScore.
//  3. CreativeScore, when > 0, is added with the program's CreativeContest
//     coefficient if defined.
//  4. The total is divided by the sum of the coefficients used — a
//     weighted average that stays in the 100..200 range regardless of
//     how many subjects the program weighs.
//  5. The program's regional coefficient (prog.RK) multiplies the result
//     when the program defines one (> 1). РК is a property of the program /
//     university, applied automatically to every applicant — it is NOT a
//     user choice, so there is no toggle.
//  6. Final result is rounded to 3 decimals and clamped to 200.
//
// Returns 0 when the program has no subject rubric, in.NMT is empty,
// or none of the rubric's required subjects are present in in.NMT.
func ComputeRating(prog *Program, in RatingInput) float64 {
	if prog == nil || len(prog.Subjects) == 0 || len(in.NMT) == 0 {
		return 0
	}

	coefByName := make(map[string]float64, len(prog.Subjects))
	for _, s := range prog.Subjects {
		coefByName[s.Name] = s.Coefficient
	}

	var sumScore, sumCoef float64

	// 1. Required subjects.
	for _, subj := range RequiredSubjects {
		coef := coefByName[subj]
		if coef <= 0 {
			continue
		}
		score, ok := in.NMT[subj]
		if !ok {
			continue
		}
		sumScore += score * coef
		sumCoef += coef
	}

	// 2. Best additional non-required, non-creative subject. Subject names are
	// canonicalised (a profile "Англійська мова" matches a rubric "Іноземна
	// мова"); without this the elective was dropped and the score came out too
	// high (three subjects instead of four).
	var (
		bestSubj string
		bestCoef float64
		bestVal  float64
	)
	for subj, score := range in.NMT {
		if IsRequiredSubject(subj) || subj == CreativeContest {
			continue
		}
		// Exact rubric name wins; fall back to the canonical name so a profile
		// "Англійська мова" matches a rubric "Іноземна мова" (but a rubric that
		// literally names "Англійська мова" still matches directly).
		coef := coefByName[subj]
		if coef <= 0 {
			coef = coefByName[canonicalSubject(subj)]
		}
		if coef <= 0 {
			continue
		}
		val := score * coef
		if bestSubj == "" || val > bestVal {
			bestSubj = subj
			bestCoef = coef
			bestVal = val
		}
	}
	if bestSubj != "" {
		coef := bestCoef
		sumScore += in.NMT[bestSubj] * coef
		// Official 2025 formula: the elective slot contributes
		// (К4макс + К4)/2 to the DENOMINATOR (numerator still uses К4).
		// This is the built-in penalty for not choosing the program's
		// highest-weighted 4th subject. Guard against a stale/“default”
		// K4Max that is smaller than the actual coef — max() keeps the
		// term ≥ coef so we never inflate the score.
		k4max := math.Max(prog.K4Max, coef)
		sumCoef += (k4max + coef) / 2
	}

	// 3. Creative contest.
	if in.CreativeScore > 0 {
		if coef := coefByName[CreativeContest]; coef > 0 {
			sumScore += in.CreativeScore * coef
			sumCoef += coef
		}
	}

	if sumCoef == 0 {
		return 0
	}
	rating := sumScore / sumCoef

	// 5. Regional coefficient — a program property, always applied.
	if prog.RK > 1 {
		rating *= prog.RK
	}

	// 6. Clamp + round to 3 decimals.
	if rating > 200 {
		rating = 200
	}
	return math.Round(rating*1000) / 1000
}
