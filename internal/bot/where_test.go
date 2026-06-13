package bot

import (
	"testing"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/osvita"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
)

func TestDiscoverLabel(t *testing.T) {
	m := service.ProgramMatch{
		Program:  osvita.SpecProgram{University: "ЛНУ ім. Франка", Program: "Маркетинг"},
		Analysis: abit.Analysis{Chance: abit.ChanceHigh},
	}
	got := discoverLabel(m)
	if got != "🟢 ЛНУ ім. Франка — Високий" {
		t.Errorf("discoverLabel = %q", got)
	}

	// Falls back to programme name when university is missing.
	m2 := service.ProgramMatch{
		Program:  osvita.SpecProgram{Program: "Комп'ютерні науки"},
		Analysis: abit.Analysis{Chance: abit.ChanceZero},
	}
	if got := discoverLabel(m2); got != "⚫ Комп'ютерні науки — Нульовий" {
		t.Errorf("discoverLabel fallback = %q", got)
	}
}

func TestOptionName(t *testing.T) {
	opts := []osvita.FilterOption{{Code: 27, Name: "Київ"}, {Code: 21, Name: "Харківська область"}}
	if got := optionName(opts, 21, "Вся Україна"); got != "Харківська область" {
		t.Errorf("optionName(21) = %q", got)
	}
	if got := optionName(opts, 0, "Вся Україна"); got != "Вся Україна" {
		t.Errorf("optionName(0) fallback = %q", got)
	}
}

func TestDiscoverFilters(t *testing.T) {
	// No regions → single all-Ukraine budget filter.
	one := discoverFilters(166, nil, false)
	if len(one) != 1 || one[0].Region != 0 || one[0].Industry != 166 || !one[0].BudgetOnly {
		t.Errorf("all-Ukraine filter wrong: %+v", one)
	}
	// Multiple regions → one budget filter each.
	many := discoverFilters(166, []int{21, 27}, false)
	if len(many) != 2 || many[0].Region != 21 || many[1].Region != 27 {
		t.Errorf("multi-region filters wrong: %+v", many)
	}
	for _, f := range many {
		if f.Industry != 166 || !f.BudgetOnly {
			t.Errorf("filter lost galuz/budget: %+v", f)
		}
	}
	// contract=true drops the budget-only restriction.
	withContract := discoverFilters(166, []int{21}, true)
	if len(withContract) != 1 || withContract[0].BudgetOnly {
		t.Errorf("contract filter should not be budget-only: %+v", withContract)
	}
}

func TestTierCountsAndFilter(t *testing.T) {
	rows := []discRow{
		{Tier: int(abit.TierSafety)},
		{Tier: int(abit.TierSafety)},
		{Tier: int(abit.TierMatch)},
		{Tier: int(abit.TierReach)},
		{Tier: int(abit.TierNone)},
	}
	safe, mid, reach := tierCounts(rows)
	if safe != 2 || mid != 1 || reach != 1 {
		t.Errorf("tierCounts = %d/%d/%d, want 2/1/1", safe, mid, reach)
	}
	// "Only passing" keeps safety+match, drops reach+none.
	if got := len(filterPassing(rows)); got != 3 {
		t.Errorf("filterPassing kept %d, want 3", got)
	}
}

func TestFilterBySpec(t *testing.T) {
	rows := []discRow{
		{URL: "a", Spec: "F3 Комп'ютерні науки"},
		{URL: "b", Spec: "F2 Інженерія ПЗ"},
		{URL: "c", Spec: "F3 Комп'ютерні науки"},
	}
	got := filterBySpec(rows, "F3 Комп'ютерні науки")
	if len(got) != 2 || got[0].URL != "a" || got[1].URL != "c" {
		t.Errorf("filterBySpec = %+v", got)
	}
	if len(filterBySpec(rows, "немає")) != 0 {
		t.Errorf("filterBySpec for absent spec should be empty")
	}
}

func TestDistinctSpecs(t *testing.T) {
	browsed := []discProg{
		{Spec: "F3 Комп'ютерні науки"},
		{Spec: "F2 Інженерія ПЗ"},
		{Spec: "F3 Комп'ютерні науки"}, // dup
		{Spec: ""}, // blank skipped
	}
	got := distinctSpecs(browsed)
	// Sorted, deduped, blank dropped.
	want := []string{"F2 Інженерія ПЗ", "F3 Комп'ютерні науки"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("distinctSpecs = %v, want %v", got, want)
	}
}

func TestGaluzLetters(t *testing.T) {
	// All 11 osvita industries map to a distinct letter A–K.
	if len(galuzLetter) != 11 {
		t.Fatalf("galuzLetter has %d entries, want 11", len(galuzLetter))
	}
	seen := map[string]bool{}
	for code, letter := range galuzLetter {
		if len(letter) != 1 || letter[0] < 'A' || letter[0] > 'K' {
			t.Errorf("industry %d → %q is not a single A–K letter", code, letter)
		}
		if seen[letter] {
			t.Errorf("letter %q assigned to more than one industry", letter)
		}
		seen[letter] = true
	}
	// Spot-check the anchor we verified live.
	if galuzLetter[166] != "F" {
		t.Errorf("ІТ (166) should be F, got %q", galuzLetter[166])
	}
}

func TestSelSuffix(t *testing.T) {
	if selSuffix(nil) != " (вся Україна)" {
		t.Errorf("empty: %q", selSuffix(nil))
	}
	if selSuffix([]int{1}) != " (1 область)" {
		t.Errorf("one: %q", selSuffix([]int{1}))
	}
	if selSuffix([]int{1, 2, 3}) != " (3 областей)" {
		t.Errorf("many: %q", selSuffix([]int{1, 2, 3}))
	}
}

func TestDecodeIntSlice(t *testing.T) {
	if got := decodeIntSlice("[21,27,3]"); len(got) != 3 || got[0] != 21 || got[2] != 3 {
		t.Errorf("decodeIntSlice = %v", got)
	}
	if got := decodeIntSlice(""); got != nil {
		t.Errorf("empty should be nil, got %v", got)
	}
	if got := decodeIntSlice(nil); got != nil {
		t.Errorf("nil should be nil, got %v", got)
	}
}

func TestAnyToInt(t *testing.T) {
	// FSM JSON round-trips numbers as float64; the helper must accept it.
	tests := []struct {
		in   any
		want int
	}{
		{float64(12), 12},
		{int(7), 7},
		{int64(5), 5},
		{"nope", 0},
		{nil, 0},
	}
	for _, tt := range tests {
		if got := anyToInt(tt.in); got != tt.want {
			t.Errorf("anyToInt(%#v) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
