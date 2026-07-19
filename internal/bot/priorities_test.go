package bot

import (
	"strings"
	"testing"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/storage"
)

func TestBuildPrioritiesView_AdmittedVerdict(t *testing.T) {
	items := []storage.PriorityItem{
		{URL: "u1", University: "Топ ВНЗ", Program: "Право"},
		{URL: "u2", University: "Другий ВНЗ", Program: "Право"},
	}
	pred := service.PriorityPrediction{
		AdmittedIndex: 1,
		Items: []service.PriorityOutcome{
			{URL: "u1", University: "Топ ВНЗ", Program: "Право", Score: 180, Fetched: true,
				Analysis: abit.Analysis{Chance: abit.ChanceLow}},
			{URL: "u2", University: "Другий ВНЗ", Program: "Право", Score: 180, Fetched: true,
				Analysis: abit.Analysis{Chance: abit.ChanceHigh, Cutoff: 150}},
		},
	}
	text, kb := buildPrioritiesView(items, pred, false)

	if !strings.Contains(text, "проходиш за пріоритетом 2") {
		t.Errorf("verdict missing admitted priority:\n%s", text)
	}
	if !strings.Contains(text, "➡️") {
		t.Errorf("admitted row not marked:\n%s", text)
	}
	if !strings.Contains(text, "Вищі пріоритети поки не проходиш") {
		t.Errorf("fall-through note missing:\n%s", text)
	}
	if kb == nil || len(kb.InlineKeyboard) == 0 {
		t.Error("keyboard should have action rows")
	}
}

func TestBuildPrioritiesView_NonePass(t *testing.T) {
	items := []storage.PriorityItem{{URL: "u1", University: "A", Program: "B"}}
	pred := service.PriorityPrediction{
		AdmittedIndex: -1,
		Items: []service.PriorityOutcome{
			{URL: "u1", University: "A", Program: "B", Score: 150, Fetched: true,
				Analysis: abit.Analysis{Chance: abit.ChanceLow}},
		},
	}
	text, _ := buildPrioritiesView(items, pred, false)
	if !strings.Contains(text, "не проходиш на жоден") {
		t.Errorf("expected none-pass verdict:\n%s", text)
	}
}
