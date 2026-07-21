package abit

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// SamePersonEntries disambiguates abit-poisk namesakes.
//
// abit-poisk indexes applicants by surname + initials only ("Петрова В. О."),
// so a name search mixes many different people (a search for a common name
// returns half a dozen). But every result row carries the applicant's per-
// subject НМТ breakdown ("Українська мова 177 Математика 167 …"), and that is
// IDENTICAL across all of one person's applications — the reliable person-
// invariant. (The old attestat average is not: since 2024 admission is by НМТ
// only and the "бал документа про освіту" is blank; and the name column is just
// surname+initials, which collide constantly.)
//
// We always look an applicant up from a known program where we know their
// competitive score, so: find the anchor entry — the one whose competitive
// score matches anchorScore (a namesake matching it to the milli-point is
// vanishingly unlikely) — take its person key (НМТ breakdown), and keep only
// entries sharing it.
//
// confident is true when the anchor was found AND carries a usable person key.
// When false the returned slice is the input unchanged so nothing is silently
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
	key := personKey(entries[anchorIdx])
	if key == "" {
		return entries, false // nothing person-invariant to group by
	}
	out = make([]ApplicantEntry, 0, len(entries))
	for _, e := range entries {
		if personKey(e) == key {
			out = append(out, e)
		}
	}
	return out, true
}

// personKey builds a person-invariant identity key for an abit-poisk row.
// Primary signal: the per-subject НМТ breakdown (stable across a person's
// applications). Fallback for older campaigns that predate it: surname+initials
// plus the attestat average when present.
func personKey(e ApplicantEntry) string {
	if s := normSubjects(e.SubjectScores); s != "" {
		return "nmt:" + s
	}
	name := normName(e.FullName)
	if name == "" {
		return ""
	}
	if att, ok := parseScore(e.EducationAvg); ok {
		return fmt.Sprintf("name:%s|att:%.3f", name, att)
	}
	return "name:" + name
}

// normSubjects canonicalizes the НМТ breakdown for comparison. It strips the
// trailing "РК: x.xx" (the program's regional coefficient — per-program, NOT
// per-person), then collapses whitespace and lowercases.
func normSubjects(s string) string {
	if i := strings.Index(s, "РК"); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// normName canonicalizes a name for comparison: trim, collapse internal
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
