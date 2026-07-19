package abit

import (
	"slices"
	"testing"
)

func TestBudgetVolume_FirmKeyPreferredOverCeiling(t *testing.T) {
	// When only a firm "Обсяг бюджетних місць" key is present, it is used and
	// is NOT flagged as a ceiling.
	firm := &Program{Volume: map[string]string{"Обсяг бюджетних місць": "30"}}
	if got := firm.BudgetVolume(); got != 30 {
		t.Errorf("BudgetVolume = %d, want 30", got)
	}
	if firm.BudgetVolumeIsCeiling() {
		t.Error("firm budget key should not be flagged as a ceiling")
	}
}

func TestBudgetVolume_CeilingKeyFlagged(t *testing.T) {
	ceil := &Program{Volume: map[string]string{"Максимальний обсяг державного замовлення": "50"}}
	if got := ceil.BudgetVolume(); got != 50 {
		t.Errorf("BudgetVolume = %d, want 50", got)
	}
	if !ceil.BudgetVolumeIsCeiling() {
		t.Error("Максимальний обсяг key must be flagged as a ceiling")
	}
}

func TestBudgetVolume_2026InlineFormat(t *testing.T) {
	// 2026 wording: budget is "Максимальне держзамовлення" and the quotas are
	// "…, квота 1/2" (lowercase). The budget key is a substring of its own
	// quota variants, so the matcher must NOT return a quota value as budget.
	prog := &Program{Volume: map[string]string{
		"Максимальне держзамовлення":          "78",
		"Максимальне держзамовлення, квота 1": "1",
		"Максимальне держзамовлення, квота 2": "8",
		"Ліцензований обсяг прийому":          "100",
	}}
	if got := prog.BudgetVolume(); got != 78 {
		t.Errorf("BudgetVolume = %d, want 78 (not a quota value)", got)
	}
	if !prog.BudgetVolumeIsCeiling() {
		t.Error("Максимальне держзамовлення is a ceiling")
	}
	if got := prog.Quota1Volume(); got != 1 {
		t.Errorf("Quota1Volume = %d, want 1", got)
	}
	if got := prog.Quota2Volume(); got != 8 {
		t.Errorf("Quota2Volume = %d, want 8", got)
	}
}

func TestBudgetVolume_NoKeyIsNotCeiling(t *testing.T) {
	empty := &Program{Volume: map[string]string{"щось інше": "5"}}
	if got := empty.BudgetVolume(); got != 0 {
		t.Errorf("BudgetVolume = %d, want 0", got)
	}
	if empty.BudgetVolumeIsCeiling() {
		t.Error("no matching key → not a ceiling")
	}
}

func TestAnalyze_CeilingBudgetAddsWarning(t *testing.T) {
	// progWithVolume uses the "Максимальний обсяг" ceiling key, so a normal
	// analysis over it must carry the ceiling warning.
	prog := progWithVolume(10, 0, 0)
	got := Analyze(prog, []Abiturient{ab(1, 190)}, AnalyzeInput{UserScore: 180})
	if !slices.Contains(got.Warnings, "budget-volume-is-ceiling") {
		t.Errorf("warnings = %v, want budget-volume-is-ceiling", got.Warnings)
	}
}

func TestAnalyze_UndersubscribedFieldWarns(t *testing.T) {
	// 78 seats, only 3 competitors and no published cutoff → everyone
	// trivially passes, which is near-meaningless early in a campaign.
	prog := progWithVolume(78, 0, 0)
	abits := []Abiturient{ab(1, 195), ab(2, 190), ab(3, 185)}
	got := Analyze(prog, abits, AnalyzeInput{UserScore: 180})
	if !slices.Contains(got.Warnings, "field-undersubscribed") {
		t.Errorf("warnings = %v, want field-undersubscribed (3 competitors < 78 seats)", got.Warnings)
	}
}

func TestAnalyze_OversubscribedFieldNoUndersubWarning(t *testing.T) {
	prog := progWithVolume(3, 0, 0)
	abits := make([]Abiturient, 10)
	for i := range abits {
		abits[i] = ab(i+1, 180+float64(i))
	}
	got := Analyze(prog, abits, AnalyzeInput{UserScore: 170})
	if slices.Contains(got.Warnings, "field-undersubscribed") {
		t.Errorf("10 competitors > 3 seats should not warn undersubscribed; got %v", got.Warnings)
	}
}

func TestAnalyze_FirmBudgetNoCeilingWarning(t *testing.T) {
	prog := &Program{Volume: map[string]string{"Обсяг бюджетних місць": "10"}}
	got := Analyze(prog, []Abiturient{ab(1, 190)}, AnalyzeInput{UserScore: 180})
	if slices.Contains(got.Warnings, "budget-volume-is-ceiling") {
		t.Errorf("firm budget should not warn about a ceiling; warnings = %v", got.Warnings)
	}
}
