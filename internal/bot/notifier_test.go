package bot

import (
	"strings"
	"testing"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
)

func TestChanceChanged(t *testing.T) {
	cases := []struct {
		old, new abit.ChanceLevel
		want     bool
	}{
		{abit.ChanceHigh, abit.ChanceMedium, true},    // downgrade
		{abit.ChanceLow, abit.ChanceHigh, true},       // upgrade
		{abit.ChanceHigh, abit.ChanceHigh, false},     // no change
		{abit.ChanceUnknown, abit.ChanceHigh, true},   // became knowable
		{abit.ChanceHigh, abit.ChanceUnknown, false},  // became unknown → stay quiet
		{abit.ChanceMedium, abit.ChanceMedium, false}, // no change
	}
	for _, c := range cases {
		if got := chanceChanged(c.old, c.new); got != c.want {
			t.Errorf("chanceChanged(%s→%s) = %v, want %v",
				c.old.Label(), c.new.Label(), got, c.want)
		}
	}
}

func TestBuildChanceChangeMessage(t *testing.T) {
	prog := &abit.Program{UniversityName: "КНУ", ProgramName: "Інженерія ПЗ"}
	newA := abit.Analysis{Chance: abit.ChanceMedium, Advice: "тримайся"}

	msg := buildChanceChangeMessage("Моя F2", prog, abit.ChanceHigh, newA)

	for _, want := range []string{"Моя F2", "КНУ", "Інженерія ПЗ", "Високий", "Середній", "тримайся", "/lists"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
	// High → Medium is a downgrade (tier drops).
	if !strings.Contains(msg, "📉") {
		t.Errorf("downgrade should carry 📉:\n%s", msg)
	}

	// Upgrade carries 📈.
	up := buildChanceChangeMessage("x", prog, abit.ChanceLow, abit.Analysis{Chance: abit.ChanceHigh})
	if !strings.Contains(up, "📈") {
		t.Errorf("upgrade should carry 📈:\n%s", up)
	}
}
