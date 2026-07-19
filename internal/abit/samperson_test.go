package abit

import "testing"

func TestSamePersonEntries(t *testing.T) {
	// Person A (attestat 185.5): two apps. Namesake B (attestat 170.2): one app.
	entries := []ApplicantEntry{
		{TotalScore: "180.038", EducationAvg: "185.5", University: "ЛНУ"},   // A, anchor
		{TotalScore: "160.000", EducationAvg: "185.5", University: "КНУ"},   // A, other
		{TotalScore: "175.000", EducationAvg: "170.2", University: "НаУКМА"}, // B, namesake
	}
	out, confident := SamePersonEntries(entries, 180.038)
	if !confident {
		t.Fatal("should be confident (anchor found, attestat populated)")
	}
	if len(out) != 2 {
		t.Fatalf("want 2 entries for the same person, got %d", len(out))
	}
	for _, e := range out {
		if e.EducationAvg != "185.5" {
			t.Errorf("namesake leaked: %+v", e)
		}
	}
}

func TestSamePersonEntries_AnchorNotFound(t *testing.T) {
	entries := []ApplicantEntry{{TotalScore: "150.0", EducationAvg: "180"}}
	out, confident := SamePersonEntries(entries, 199.999)
	if confident {
		t.Error("no score match → not confident")
	}
	if len(out) != 1 {
		t.Error("should return input unchanged when unconfirmed")
	}
}

func TestSamePersonEntries_NoAttestat(t *testing.T) {
	entries := []ApplicantEntry{
		{TotalScore: "180.0", EducationAvg: "—"},
		{TotalScore: "170.0", EducationAvg: ""},
	}
	_, confident := SamePersonEntries(entries, 180.0)
	if confident {
		t.Error("attestat unavailable → cannot group → not confident")
	}
}
