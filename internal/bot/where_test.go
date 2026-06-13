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
	one := discoverFilters(166, nil)
	if len(one) != 1 || one[0].Region != 0 || one[0].Industry != 166 || !one[0].BudgetOnly {
		t.Errorf("all-Ukraine filter wrong: %+v", one)
	}
	// Multiple regions → one budget filter each.
	many := discoverFilters(166, []int{21, 27})
	if len(many) != 2 || many[0].Region != 21 || many[1].Region != 27 {
		t.Errorf("multi-region filters wrong: %+v", many)
	}
	for _, f := range many {
		if f.Industry != 166 || !f.BudgetOnly {
			t.Errorf("filter lost galuz/budget: %+v", f)
		}
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
