package abit

import "testing"

func TestSamePersonEntries_ByNMT(t *testing.T) {
	// Real shape: a common surname+initials search mixes people; the per-subject
	// НМТ breakdown separates them. One row carries a trailing "РК: 1.07" (the
	// program's regional coefficient) which must NOT split the person.
	me := "Українська мова 177 Математика 167 Історія України 144 Іноземна мова 137"
	entries := []ApplicantEntry{
		{FullName: "Петрова В. О.", TotalScore: "155.067", SubjectScores: me, Priority: "К"},                                                          // anchor
		{FullName: "Петрова В. О.", TotalScore: "154.400", SubjectScores: me, Priority: "1"},                                                          // same
		{FullName: "Петрова В. О.", TotalScore: "165.208", SubjectScores: me + " РК: 1.07", Priority: "5"},                                            // same (РК suffix)
		{FullName: "Петрова В. О.", TotalScore: "134.000", SubjectScores: "Українська мова 148 Математика 115 Історія України 124 Іноземна мова 130"}, // namesake
	}
	out, confident := SamePersonEntries(entries, 155.067)
	if !confident {
		t.Fatal("anchor found + НМТ present → confident")
	}
	if len(out) != 3 {
		t.Fatalf("want 3 entries for the same person (incl. the РК-suffixed one), got %d", len(out))
	}
	for _, e := range out {
		if e.Priority == "" && e.TotalScore == "134.000" {
			t.Error("namesake with different НМТ leaked in")
		}
	}
}

func TestSamePersonEntries_FallbackNameAttestat(t *testing.T) {
	// Older campaigns with no per-subject breakdown fall back to name+attestat.
	entries := []ApplicantEntry{
		{FullName: "Коваль О. О.", TotalScore: "180.0", EducationAvg: "190.0"},
		{FullName: "Коваль О. О.", TotalScore: "150.0", EducationAvg: "190.0"},
		{FullName: "Коваль О. О.", TotalScore: "175.0", EducationAvg: "160.0"}, // namesake (diff attestat)
	}
	out, confident := SamePersonEntries(entries, 180.0)
	if !confident {
		t.Fatal("anchor found + name/attestat → confident")
	}
	if len(out) != 2 {
		t.Fatalf("attestat should split the namesake, got %d", len(out))
	}
}

func TestSamePersonEntries_AnchorNotFound(t *testing.T) {
	entries := []ApplicantEntry{{FullName: "Хтось Х. Х.", TotalScore: "150.0", SubjectScores: "Математика 150"}}
	out, confident := SamePersonEntries(entries, 199.999)
	if confident {
		t.Error("no score match → not confident")
	}
	if len(out) != 1 {
		t.Error("should return input unchanged when unconfirmed")
	}
}

func TestSamePersonEntries_NoInvariant(t *testing.T) {
	entries := []ApplicantEntry{{TotalScore: "180.0"}} // no НМТ, no name, no attestat
	_, confident := SamePersonEntries(entries, 180.0)
	if confident {
		t.Error("nothing person-invariant → not confident")
	}
}
