package abit

import (
	"math"
	"strconv"
	"strings"
)

// SamePersonEntries disambiguates abit-poisk namesakes.
//
// abit-poisk indexes applicants by surname + initials only ("Мельник І. І."),
// so a name search mixes many different people. But we always look an applicant
// up from a known program where we know their competitive score, and every one
// of a person's applications carries the SAME "бал документа про освіту"
// (attestat average) while namesakes almost always differ.
//
// So: find the anchor entry — the one whose competitive score matches
// anchorScore (a specific float; a namesake matching it to the milli-point is
// vanishingly unlikely) — then keep only entries sharing its attestat average.
//
// confident is true only when the anchor was found AND its attestat average is
// populated. When false, callers should NOT trust the result as "the same
// person" (abit-poisk didn't give enough to disambiguate) — the returned slice
// is the input unchanged so nothing is silently dropped.
func SamePersonEntries(entries []ApplicantEntry, anchorScore float64) (out []ApplicantEntry, confident bool) {
	if anchorScore <= 0 || len(entries) == 0 {
		return entries, false
	}
	anchorIdx := -1
	for i, e := range entries {
		if s, ok := parseScore(e.TotalScore); ok && math.Abs(s-anchorScore) < 0.005 {
			anchorIdx = i
			break
		}
	}
	if anchorIdx < 0 {
		return entries, false // couldn't confirm which namesake is ours
	}
	attestat, ok := parseScore(entries[anchorIdx].EducationAvg)
	if !ok || attestat <= 0 {
		return entries, false // no person-invariant to group by
	}
	out = make([]ApplicantEntry, 0, len(entries))
	for _, e := range entries {
		if a, ok := parseScore(e.EducationAvg); ok && math.Abs(a-attestat) < 0.005 {
			out = append(out, e)
		}
	}
	return out, true
}

// parseScore parses an abit-poisk numeric cell ("185.500", "—", "", "0.000").
// ok is false for non-numeric or zero values.
func parseScore(s string) (float64, bool) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v == 0 {
		return 0, false
	}
	return v, true
}
