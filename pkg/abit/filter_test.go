package abit

import (
	"testing"
)

// sample builds a small list of representative applicants:
//
//	a1 — допущено, P1, КВ1+СБ, бюджет, оригінали, score 180
//	a2 — деактивовано (бюджет), P2, no quota, бюджет, оригінали, score 175
//	a3 — до наказу (контракт), P1, КВ2, контракт, no docs, score 165
//	a4 — відмова, P3, no quota, контракт, no docs, score 150
func sample() []Abiturient {
	return []Abiturient{
		{ID: 1, Priority: 1, Status: "Допущено", Quotas: []string{QuotaKV1, QuotaSB},
			Documents: true, StateEducation: true, Score: 180},
		{ID: 2, Priority: 2, Status: "Деактивовано (зарах. на бюджет)",
			Documents: true, StateEducation: true, Score: 175},
		{ID: 3, Priority: 1, Status: "До наказу (контракт)", Quotas: []string{QuotaKV2},
			Documents: false, StateEducation: false, Score: 165},
		{ID: 4, Priority: 3, Status: "Відмова",
			Documents: false, StateEducation: false, Score: 150},
	}
}

func idsOf(ats []Abiturient) []int {
	out := make([]int, len(ats))
	for i, a := range ats {
		out[i] = a.ID
	}
	return out
}

func assertIDs(t *testing.T, got []Abiturient, want ...int) {
	t.Helper()
	gotIDs := idsOf(got)
	if len(gotIDs) != len(want) {
		t.Fatalf("len = %d, want %d (ids=%v want=%v)", len(gotIDs), len(want), gotIDs, want)
	}
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Errorf("at %d: got %d, want %d (full %v)", i, gotIDs[i], want[i], gotIDs)
		}
	}
}

func TestFilter_Zero_PassesAll(t *testing.T) {
	f := Filter{}
	if !f.IsZero() {
		t.Fatal("expected IsZero true")
	}
	assertIDs(t, f.Apply(sample()), 1, 2, 3, 4)
}

func TestFilter_Apply_NilInput(t *testing.T) {
	if got := (Filter{}).Apply(nil); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestFilter_StatusInclude(t *testing.T) {
	f := Filter{StatusInclude: []string{"Допущено", "Відмова"}}
	assertIDs(t, f.Apply(sample()), 1, 4)
}

func TestFilter_StatusExclude(t *testing.T) {
	f := Filter{StatusExclude: []string{"Відмова"}}
	assertIDs(t, f.Apply(sample()), 1, 2, 3)
}

func TestFilter_PriorityRange(t *testing.T) {
	f := Filter{PriorityMin: 1, PriorityMax: 2}
	assertIDs(t, f.Apply(sample()), 1, 2, 3)

	onlyFirst := Filter{PriorityMax: 1}
	assertIDs(t, onlyFirst.Apply(sample()), 1, 3)
}

func TestFilter_QuotaInclude(t *testing.T) {
	f := Filter{QuotaInclude: []string{QuotaKV1}}
	assertIDs(t, f.Apply(sample()), 1)

	any := Filter{QuotaInclude: []string{QuotaKV1, QuotaKV2}}
	assertIDs(t, any.Apply(sample()), 1, 3)
}

func TestFilter_QuotaExclude(t *testing.T) {
	f := Filter{QuotaExclude: []string{QuotaKV1}}
	assertIDs(t, f.Apply(sample()), 2, 3, 4)
}

func TestFilter_Documents(t *testing.T) {
	yes := Filter{Documents: BoolPtr(true)}
	assertIDs(t, yes.Apply(sample()), 1, 2)

	no := Filter{Documents: BoolPtr(false)}
	assertIDs(t, no.Apply(sample()), 3, 4)
}

func TestFilter_Funding(t *testing.T) {
	bud := Filter{Funding: FundingBudget}
	assertIDs(t, bud.Apply(sample()), 1, 2)

	con := Filter{Funding: FundingContract}
	assertIDs(t, con.Apply(sample()), 3, 4)

	any := Filter{Funding: FundingAny}
	assertIDs(t, any.Apply(sample()), 1, 2, 3, 4)
}

func TestFilter_ScoreRange(t *testing.T) {
	f := Filter{ScoreMin: 170}
	assertIDs(t, f.Apply(sample()), 1, 2)

	tight := Filter{ScoreMin: 160, ScoreMax: 175}
	assertIDs(t, tight.Apply(sample()), 2, 3)
}

func TestFilter_Combined(t *testing.T) {
	// Looking for: real top contenders on budget with originals submitted.
	f := Filter{
		StatusExclude: []string{"Відмова", "Скасовано вступником"},
		PriorityMax:   1,
		Documents:     BoolPtr(true),
		Funding:       FundingBudget,
	}
	assertIDs(t, f.Apply(sample()), 1)
}

func TestFilter_Apply_DoesNotMutateInput(t *testing.T) {
	in := sample()
	original := append([]Abiturient(nil), in...)
	_ = Filter{ScoreMin: 200}.Apply(in)
	for i := range in {
		if in[i].ID != original[i].ID {
			t.Errorf("input mutated at %d", i)
		}
	}
}
