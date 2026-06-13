package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
)

// searcherFromMap adapts a name→entries map to the fakeSearcher closure
// (defined in applicant_test.go), returning ErrNoData for unknown names.
func searcherFromMap(m map[string][]abit.ApplicantEntry) *fakeSearcher {
	return &fakeSearcher{search: func(_ context.Context, name string) ([]abit.ApplicantEntry, error) {
		if e, ok := m[name]; ok {
			return e, nil
		}
		return nil, abit.ErrNoData
	}}
}

func TestSimulate_RemovesHigherPriorityPlacement(t *testing.T) {
	store := newStore(t)

	// Budget program with 5 seats; user scores 180. Three competitors rank
	// above the user.
	prog := &abit.Program{
		EB: 40, OKR: 1, K4Max: 0.35, RK: 1.0,
		Volume: map[string]string{"Максимальний обсяг державного замовлення": "5"},
	}
	comp := func(id int, name string, score float64, prio int) abit.Abiturient {
		return abit.Abiturient{
			ID: id, Name: name, Score: score, Priority: prio,
			Status: "Допущено", StateEducation: true, Documents: true,
		}
	}
	abits := []abit.Abiturient{
		comp(1, "Перший О О", 190, 1), // priority 1 — never a candidate
		comp(2, "Другий О О", 185, 3), // priority 3 — recommended elsewhere → departs
		comp(3, "Третій О О", 182, 2), // priority 2 — stays
	}

	searcher := searcherFromMap(map[string][]abit.ApplicantEntry{
		// Другий is recommended on a priority-1 program → he leaves this one.
		"Другий О О": {{Priority: "1", Status: "Рекомендовано", University: "КНУ"}},
		// Третій's only other app is low priority and not placed → stays.
		"Третій О О": {{Priority: "5", Status: "Допущено", University: "ЛНУ"}},
	})
	appSvc := service.NewApplicantService(searcher, store, time.Hour)
	sim := service.NewPrioritySimulator(appSvc, nil, nil, 4, 40)

	res, err := sim.Simulate(context.Background(), prog, abits, service.SimInput{UserScore: 180})
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}

	// Priority-1 competitor is skipped; the other two are looked up.
	if res.LookedUp != 2 {
		t.Errorf("LookedUp = %d, want 2", res.LookedUp)
	}
	if len(res.Departures) != 1 {
		t.Fatalf("Departures = %d, want 1", len(res.Departures))
	}
	d := res.Departures[0]
	if d.Name != "Другий О О" || d.University != "КНУ" || d.Priority != 1 {
		t.Errorf("departure = %+v, want Другий/КНУ/1", d)
	}
	// Removing one above-user competitor improves the user's rank.
	if res.Refined.MyRealRank >= res.Baseline.MyRealRank {
		t.Errorf("refined rank %d should beat baseline %d", res.Refined.MyRealRank, res.Baseline.MyRealRank)
	}
	if res.Baseline.MyRealRank != 4 || res.Refined.MyRealRank != 3 {
		t.Errorf("ranks = baseline %d / refined %d, want 4 / 3", res.Baseline.MyRealRank, res.Refined.MyRealRank)
	}
}

type fakeResolver struct {
	url string
	ok  bool
}

func (f fakeResolver) Resolve(_ context.Context, _, _ string) (string, bool) { return f.url, f.ok }

type fakeFetcher struct{ prog *abit.Program }

func (f fakeFetcher) Fetch(_ context.Context, _ string) (*abit.Program, error) { return f.prog, nil }

func TestSimulate_PredictsPreWave(t *testing.T) {
	store := newStore(t)
	prog := &abit.Program{
		EB: 40, OKR: 1, Volume: map[string]string{"Максимальний обсяг державного замовлення": "5"},
	}
	// One competitor above the user, priority 2 here, NOT yet recommended.
	abits := []abit.Abiturient{
		{ID: 1, Name: "Хитрий О О", Score: 195, Priority: 2, Status: "Допущено", StateEducation: true, Documents: true},
	}
	// abit-poisk: he has a priority-1 application elsewhere, also just
	// "Допущено" (so the status signal can't act) — prediction must.
	searcher := searcherFromMap(map[string][]abit.ApplicantEntry{
		"Хитрий О О": {{Priority: "1", Status: "Допущено", University: "КНУ", Specialty: "Право", TotalScore: "195"}},
	})
	appSvc := service.NewApplicantService(searcher, store, time.Hour)

	// Resolver finds his priority-1 program; the fetched program has 50 seats
	// and nobody above him → he passes there → he'll leave this one.
	resolver := fakeResolver{url: "http://x/y2025/r1/1/2/", ok: true}
	fetcher := fakeFetcher{prog: &abit.Program{
		EB: 40, OKR: 1, Volume: map[string]string{"Максимальний обсяг державного замовлення": "50"},
	}}
	sim := service.NewPrioritySimulator(appSvc, resolver, fetcher, 4, 40)

	res, err := sim.Simulate(context.Background(), prog, abits, service.SimInput{UserScore: 180})
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	if len(res.Departures) != 1 {
		t.Fatalf("Departures = %d, want 1", len(res.Departures))
	}
	d := res.Departures[0]
	if !d.Predicted || d.University != "КНУ" || d.Priority != 1 {
		t.Errorf("departure = %+v, want predicted КНУ/1", d)
	}
	if res.Refined.MyRealRank >= res.Baseline.MyRealRank {
		t.Errorf("refined rank %d should beat baseline %d", res.Refined.MyRealRank, res.Baseline.MyRealRank)
	}
}

func TestSimulate_MaskedAndNoProfile(t *testing.T) {
	store := newStore(t)
	prog := &abit.Program{EB: 40, OKR: 1, Volume: map[string]string{"Максимальний обсяг державного замовлення": "5"}}
	appSvc := service.NewApplicantService(searcherFromMap(nil), store, time.Hour)
	sim := service.NewPrioritySimulator(appSvc, nil, nil, 4, 40)

	// No score → error.
	if _, err := sim.Simulate(context.Background(), prog, nil, service.SimInput{}); err == nil {
		t.Error("expected error when UserScore is 0")
	}

	// Masked competitor above the user is counted but not looked up.
	abits := []abit.Abiturient{
		{ID: 1, Name: "Іва###", Score: 190, Priority: 2, Status: "Допущено", StateEducation: true, Documents: true},
	}
	res, err := sim.Simulate(context.Background(), prog, abits, service.SimInput{UserScore: 180})
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	if res.Masked != 1 || res.LookedUp != 0 || len(res.Departures) != 0 {
		t.Errorf("masked=%d looked=%d dep=%d, want 1/0/0", res.Masked, res.LookedUp, len(res.Departures))
	}
}
