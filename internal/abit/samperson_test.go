package abit

import "testing"

func TestSamePersonEntries(t *testing.T) {
	// Person A ("Мельник Іван Іванович"): two apps. Namesake B (same initials,
	// different middle name) shares the surname+initials search but not the ПІБ.
	entries := []ApplicantEntry{
		{FullName: "Мельник Іван Іванович", TotalScore: "180.038", University: "ЛНУ"},   // A, anchor
		{FullName: "Мельник Іван Іванович", TotalScore: "160.000", University: "КНУ"},   // A, other
		{FullName: "Мельник Іван Петрович", TotalScore: "175.000", University: "НаУКМА"}, // B, namesake
	}
	out, confident := SamePersonEntries(entries, 180.038)
	if !confident {
		t.Fatal("should be confident (anchor found, full name present)")
	}
	if len(out) != 2 {
		t.Fatalf("want 2 entries for the same person, got %d", len(out))
	}
	for _, e := range out {
		if e.FullName != "Мельник Іван Іванович" {
			t.Errorf("namesake leaked: %+v", e)
		}
	}
}

func TestSamePersonEntries_WorksWithoutAttestat(t *testing.T) {
	// Since 2024 the attestat average is usually blank — grouping must still
	// work off the full name alone.
	entries := []ApplicantEntry{
		{FullName: "Коваль Оля Олегівна", TotalScore: "180.0", EducationAvg: "—"},
		{FullName: "Коваль Оля Олегівна", TotalScore: "170.0", EducationAvg: ""},
		{FullName: "Коваль Олена Олегівна", TotalScore: "165.0", EducationAvg: ""},
	}
	out, confident := SamePersonEntries(entries, 180.0)
	if !confident {
		t.Fatal("full name present → should be confident even with no attestat")
	}
	if len(out) != 2 {
		t.Fatalf("want 2 same-name entries, got %d", len(out))
	}
}

func TestSamePersonEntries_IdenticalNameSplitByAttestat(t *testing.T) {
	// Rare: two people with the identical ПІБ. When attestat is present it breaks
	// the tie so a true namesake isn't merged in.
	entries := []ApplicantEntry{
		{FullName: "Шевченко Тарас Григорович", TotalScore: "180.0", EducationAvg: "190.0"}, // anchor
		{FullName: "Шевченко Тарас Григорович", TotalScore: "150.0", EducationAvg: "190.0"}, // same person
		{FullName: "Шевченко Тарас Григорович", TotalScore: "175.0", EducationAvg: "160.0"}, // namesake
	}
	out, confident := SamePersonEntries(entries, 180.0)
	if !confident {
		t.Fatal("anchor found → confident")
	}
	if len(out) != 2 {
		t.Fatalf("attestat should split the identical-name namesake, got %d", len(out))
	}
}

func TestSamePersonEntries_AnchorNotFound(t *testing.T) {
	entries := []ApplicantEntry{{FullName: "Хтось Хтось Хтось", TotalScore: "150.0"}}
	out, confident := SamePersonEntries(entries, 199.999)
	if confident {
		t.Error("no score match → not confident")
	}
	if len(out) != 1 {
		t.Error("should return input unchanged when unconfirmed")
	}
}

func TestSamePersonEntries_NoName(t *testing.T) {
	entries := []ApplicantEntry{{TotalScore: "180.0"}} // no full name → nothing to group by
	_, confident := SamePersonEntries(entries, 180.0)
	if confident {
		t.Error("no full name → cannot group → not confident")
	}
}
