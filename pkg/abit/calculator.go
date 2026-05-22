package abit

import "math"

// RatingInput bundles everything ComputeRating needs from the user
// profile. Keeping it in a struct lets callers grow the inputs (e.g.
// extra modifiers) without breaking the call sites.
type RatingInput struct {
	// NMT maps subject name → user's score for that subject.
	NMT map[string]float64
	// CreativeScore is the user-entered prediction for programs that
	// require a creative contest. 0 means "not provided".
	CreativeScore float64
	// RegionCoef, when true, applies the program's regional coefficient
	// (prog.RK) to the final rating.
	RegionCoef bool
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
//  5. RegionCoef multiplies the result by prog.RK when enabled.
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

	// 2. Best additional non-required, non-creative subject.
	var (
		bestSubj string
		bestVal  float64
	)
	for subj, score := range in.NMT {
		if IsRequiredSubject(subj) || subj == CreativeContest {
			continue
		}
		coef := coefByName[subj]
		if coef <= 0 {
			continue
		}
		val := score * coef
		if bestSubj == "" || val > bestVal {
			bestSubj = subj
			bestVal = val
		}
	}
	if bestSubj != "" {
		sumScore += in.NMT[bestSubj] * coefByName[bestSubj]
		sumCoef += coefByName[bestSubj]
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

	// 5. Regional coefficient.
	if in.RegionCoef && prog.RK > 1 {
		rating *= prog.RK
	}

	// 6. Clamp + round to 3 decimals.
	if rating > 200 {
		rating = 200
	}
	return math.Round(rating*1000) / 1000
}

// RegionCoefRequested reports whether the user asked for the regional
// coefficient AND the program would actually apply it. False when the
// scraper couldn't determine RK (the toggle is then a silent no-op,
// which is worth surfacing in the UI).
func RegionCoefRequested(prog *Program, in RatingInput) (requested, available bool) {
	return in.RegionCoef, prog != nil && prog.RK > 1
}
