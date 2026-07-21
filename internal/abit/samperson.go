package abit

import (
	"math"
	"strconv"
	"strings"
)

// SamePersonEntries disambiguates abit-poisk namesakes.
//
// abit-poisk indexes applicants by surname + initials only ("Мельник І. І."),
// so a name search mixes many different people. But every result row carries the
// applicant's FULL name (ПІБ), and every one of a person's applications repeats
// it — so the full name is a reliable person-invariant. (The old attestat
// average is not: since 2024 admission is by НМТ only and the "бал документа про
// освіту" is usually blank.)
//
// We always look an applicant up from a known program where we know their
// competitive score, so: find the anchor entry — the one whose competitive score
// matches anchorScore (a specific float; a namesake matching it to the
// milli-point is vanishingly unlikely) — take its full name, and keep only
// entries sharing it. When an attestat average IS present on both the anchor and
// a candidate row, it's used as an extra tiebreaker (guards the rare
// identical-full-name collision).
//
// confident is true when the anchor was found AND carries a usable full name.
// When false, the returned slice is the input unchanged so nothing is silently
// dropped.
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
	name := normName(entries[anchorIdx].FullName)
	if name == "" {
		return entries, false // no person-invariant to group by
	}
	anchorAtt, hasAtt := parseScore(entries[anchorIdx].EducationAvg)

	out = make([]ApplicantEntry, 0, len(entries))
	for _, e := range entries {
		if normName(e.FullName) != name {
			continue
		}
		// Same full name but a different (populated) attestat ⇒ a rare true
		// namesake with identical ПІБ — drop it.
		if hasAtt {
			if a, ok := parseScore(e.EducationAvg); ok && math.Abs(a-anchorAtt) >= 0.005 {
				continue
			}
		}
		out = append(out, e)
	}
	return out, true
}

// normName canonicalizes a full name for comparison: trim, collapse internal
// whitespace, lowercase.
func normName(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
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
