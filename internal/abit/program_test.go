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

func TestAnalyze_FirmBudgetNoCeilingWarning(t *testing.T) {
	prog := &Program{Volume: map[string]string{"Обсяг бюджетних місць": "10"}}
	got := Analyze(prog, []Abiturient{ab(1, 190)}, AnalyzeInput{UserScore: 180})
	if slices.Contains(got.Warnings, "budget-volume-is-ceiling") {
		t.Errorf("firm budget should not warn about a ceiling; warnings = %v", got.Warnings)
	}
}
