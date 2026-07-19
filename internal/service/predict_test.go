package service_test

import (
	"context"
	"testing"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
)

// mapFetcher returns a distinct program per URL, or an error for unknown ones.
type mapFetcher struct {
	progs map[string]*abit.Program
	err   error
}

func (m mapFetcher) Fetch(_ context.Context, url string) (*abit.Program, error) {
	if m.err != nil {
		return nil, m.err
	}
	if p, ok := m.progs[url]; ok {
		return p, nil
	}
	return nil, abit.ErrNoData
}

// progWithCutoff builds a scoring program whose verdict is driven purely by the
// published cutoff, so the user's ~180 score deterministically passes or fails.
func progWithCutoff(uni string, cutoff string) *abit.Program {
	return &abit.Program{
		RK: 1.0,
		Subjects: []abit.SubjectMeta{
			{Name: "Українська мова", Coefficient: 0.4},
			{Name: "Історія України", Coefficient: 0.4},
			{Name: "Математика", Coefficient: 0.4},
		},
		UniversityName: uni,
		ProgramName:    "Тестова",
		Volume: map[string]string{
			"Максимальний обсяг державного замовлення":                "10",
			"Мінімальний рейтинговий бал серед зарахованих на бюджет": cutoff,
		},
	}
}

func predictProfile() service.PredictInput {
	return service.PredictInput{NMT: map[string]float64{
		"Українська мова": 180, "Історія України": 180, "Математика": 180,
	}} // → competitive score 180
}

func TestPredict_AdmitsHighestPassingPriority(t *testing.T) {
	fetcher := mapFetcher{progs: map[string]*abit.Program{
		"p1": progWithCutoff("Топ ВНЗ", "199"),    // 180 < 199 → fail
		"p2": progWithCutoff("Другий ВНЗ", "150"), // 180 ≥ 150 → pass
		"p3": progWithCutoff("Третій ВНЗ", "100"), // also passes, but lower priority
	}}
	pred := service.NewPriorityPredictor(fetcher)

	got := pred.Predict(context.Background(), []string{"p1", "p2", "p3"}, predictProfile())
	if len(got.Items) != 3 {
		t.Fatalf("Items = %d, want 3", len(got.Items))
	}
	if got.AdmittedIndex != 1 {
		t.Fatalf("AdmittedIndex = %d, want 1 (priority 2)", got.AdmittedIndex)
	}
	adm, ok := got.Admitted()
	if !ok || adm.University != "Другий ВНЗ" {
		t.Errorf("Admitted = %+v (ok=%v), want Другий ВНЗ", adm, ok)
	}
	if got.Items[0].Passes() {
		t.Error("priority 1 should not pass (score below cutoff)")
	}
	if !got.Items[2].Passes() {
		t.Error("priority 3 should still be marked as passing")
	}
}

func TestPredict_NonePass(t *testing.T) {
	fetcher := mapFetcher{progs: map[string]*abit.Program{
		"a": progWithCutoff("A", "199"),
		"b": progWithCutoff("B", "195"),
	}}
	pred := service.NewPriorityPredictor(fetcher)
	got := pred.Predict(context.Background(), []string{"a", "b"}, predictProfile())
	if got.AdmittedIndex != -1 {
		t.Errorf("AdmittedIndex = %d, want -1", got.AdmittedIndex)
	}
	if _, ok := got.Admitted(); ok {
		t.Error("Admitted should be false when nothing passes")
	}
}

func TestPredict_UnfetchableKeptButNotAdmitted(t *testing.T) {
	fetcher := mapFetcher{progs: map[string]*abit.Program{
		"ok": progWithCutoff("OK", "150"), // passes
	}}
	pred := service.NewPriorityPredictor(fetcher)
	// "dead" is unknown → fetch fails; it must stay in the list at its slot.
	got := pred.Predict(context.Background(), []string{"dead", "ok"}, predictProfile())
	if len(got.Items) != 2 {
		t.Fatalf("Items = %d, want 2", len(got.Items))
	}
	if got.Items[0].Fetched {
		t.Error("priority 1 should be marked unfetched")
	}
	if got.AdmittedIndex != 1 {
		t.Errorf("AdmittedIndex = %d, want 1 (the fetchable pass)", got.AdmittedIndex)
	}
}
