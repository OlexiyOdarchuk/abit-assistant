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
