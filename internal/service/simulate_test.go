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
		// abit-poisk returns ALL apps for the name (incl. this program). The
		// anchor is the entry whose score matches the candidate's here; the
		// person is then confirmed by the invariant НМТ breakdown.
		"Другий О О": {
			{Priority: "3", TotalScore: "185", SubjectScores: "У 180 М 175", University: "Цей"},       // anchor (this program)
			{Priority: "1", Status: "Рекомендовано", University: "КНУ", SubjectScores: "У 180 М 175"}, // same person → he leaves
			// A same-named stranger (different НМТ) recommended elsewhere —
			// must be filtered out so it doesn't drive the departure.
			{Priority: "1", Status: "Рекомендовано", University: "ХАІ", SubjectScores: "У 150 М 140"},
		},
		// Третій's only other app is low priority and not placed → stays.
		"Третій О О": {
			{Priority: "2", TotalScore: "182", SubjectScores: "У 170 М 160", University: "Цей"}, // anchor
			{Priority: "5", Status: "Допущено", University: "ЛНУ", SubjectScores: "У 170 М 160"},
		},
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
		"Хитрий О О": {{Priority: "1", Status: "Допущено", University: "КНУ", Specialty: "Право", TotalScore: "195", SubjectScores: "У 190 М 185"}},
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

func TestSimulate_ExcludedFromOrderIsNotPlaced(t *testing.T) {
	store := newStore(t)
	prog := &abit.Program{
		EB: 40, OKR: 1, Volume: map[string]string{"Максимальний обсяг державного замовлення": "5"},
	}
	abits := []abit.Abiturient{
		{ID: 1, Name: "Спірний О О", Score: 190, Priority: 2, Status: "Допущено", StateEducation: true, Documents: true},
	}
	// His higher-priority entry says "Виключено з наказу" — that contains
	// "наказу" but means he LOST the seat, so he must NOT be removed here.
	searcher := searcherFromMap(map[string][]abit.ApplicantEntry{
		"Спірний О О": {{Priority: "1", Status: "Виключено з наказу", University: "КНУ"}},
	})
	appSvc := service.NewApplicantService(searcher, store, time.Hour)
	sim := service.NewPrioritySimulator(appSvc, nil, nil, 4, 40)

	res, err := sim.Simulate(context.Background(), prog, abits, service.SimInput{UserScore: 180})
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	if len(res.Departures) != 0 {
		t.Errorf("excluded-from-order applicant should not be a departure, got %+v", res.Departures)
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

// mapResolver resolves any (uni, spec) to a per-university URL from a map.
type mapResolver map[string]string // university → url

func (m mapResolver) Resolve(_ context.Context, university, _ string) (string, bool) {
	u, ok := m[university]
	return u, ok
}

// rawRow builds a 19-element osvita RawRequest (see decoder). status 6 =
// "Допущено" via the program's Statuses map.
func rawRow(id, priority, status int, name string, score float64, stateEdu, docs int) abit.RawRequest {
	return abit.RawRequest{
		float64(id), float64(id), float64(priority), float64(status),
		name, score,
		0.0, 0.0, 0.0,
		0.0, 0.0, 0.0, 0.0, 0.0,
		float64(docs), 0.0, 0.0, float64(stateEdu), 0.0,
	}
}

// TestSimulate_RecursiveDepthResolvesMedium: competitor A ranks above the user
// here (priority 2). A's priority-1 program Q gives A only a Medium chance —
// but at Q, applicant B (above A) leaves via a recommended higher priority. So
// at depth 0 A is kept (Medium ≠ pass); at depth ≥1 the recursion removes B,
// A clears Q, and A departs our program.
func TestSimulate_RecursiveDepthResolvesMedium(t *testing.T) {
	store := newStore(t)
	statuses := map[string]string{"6": "Допущено", "14": "До наказу"}

	// Our program P: 1 seat, user 170, competitor A (180, priority 2).
	progP := &abit.Program{
		EB: 40, OKR: 1, RK: 1.0, Statuses: statuses,
		Volume:   map[string]string{"Максимальний обсяг державного замовлення": "1"},
		Requests: []abit.RawRequest{rawRow(1, 2, 6, "А А А", 180, 1, 1)},
	}
	abitsP := abit.Decode(progP)

	// A's priority-1 program Q: 1 seat, B (190, priority 2) sits above A(180).
	progQ := &abit.Program{
		EB: 40, OKR: 1, RK: 1.0, Statuses: statuses,
		Volume:   map[string]string{"Максимальний обсяг державного замовлення": "1"},
		Requests: []abit.RawRequest{rawRow(2, 2, 6, "Б Б Б", 190, 1, 1)},
	}

	searcher := searcherFromMap(map[string][]abit.ApplicantEntry{
		// A: anchor here (P) + a not-yet-recommended priority-1 app at Q.
		"А А А": {
			{Priority: "2", TotalScore: "180", SubjectScores: "nA", University: "P"},
			{Priority: "1", Status: "Допущено", University: "Quni", Specialty: "Qspec", TotalScore: "180", SubjectScores: "nA"},
		},
		// B: anchor at Q + a recommended priority-1 app elsewhere → B leaves Q.
		"Б Б Б": {
			{Priority: "2", TotalScore: "190", SubjectScores: "nB", University: "Quni"},
			{Priority: "1", Status: "Рекомендовано", University: "Runi", SubjectScores: "nB"},
		},
	})
	appSvc := service.NewApplicantService(searcher, store, time.Hour)
	resolver := mapResolver{"Quni": "qurl"}
	fetcher := fakeFetcher{prog: progQ} // any URL → Q (only Q is resolved here)
	sim := service.NewPrioritySimulator(appSvc, resolver, fetcher, 4, 40)

	shallow, err := sim.Simulate(context.Background(), progP, abitsP, service.SimInput{UserScore: 170, Depth: 0})
	if err != nil {
		t.Fatalf("shallow: %v", err)
	}
	if len(shallow.Departures) != 0 {
		t.Fatalf("depth 0: A's Q is Medium → keep A, want 0 departures, got %d", len(shallow.Departures))
	}

	deep, err := sim.Simulate(context.Background(), progP, abitsP, service.SimInput{UserScore: 170, Depth: 3})
	if err != nil {
		t.Fatalf("deep: %v", err)
	}
	if len(deep.Departures) != 1 {
		t.Fatalf("depth 3: recursion removes B, A clears Q → A departs, want 1, got %d", len(deep.Departures))
	}
	if d := deep.Departures[0]; d.Name != "А А А" || !d.Predicted {
		t.Errorf("departure = %+v, want А А А predicted", d)
	}
}
