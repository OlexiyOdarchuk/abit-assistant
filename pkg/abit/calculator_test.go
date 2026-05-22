package abit

import (
	"math"
	"testing"
)

func TestComputeRating_BasicSum(t *testing.T) {
	prog := &Program{
		EB: 40,
		Subjects: []SubjectMeta{
			{Name: "Українська мова", Coefficient: 0.35, SubjectID: 1},
			{Name: "Математика", Coefficient: 0.40, SubjectID: 14},
			{Name: "Історія України", Coefficient: 0.25, SubjectID: 6},
		},
	}
	nmt := map[string]float64{
		"Українська мова":  180,
		"Математика":       170,
		"Історія України":  175,
	}
	got := ComputeRating(prog, nmt)
	want := 180*0.35 + 170*0.40 + 175*0.25
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestComputeRating_SkipsMotivationalLetter(t *testing.T) {
	prog := &Program{
		EB: 40,
		Subjects: []SubjectMeta{
			{Name: "Українська мова", Coefficient: 0.35, SubjectID: 1},
			{Name: motivationalLetter, Coefficient: 1.0, SubjectID: 0},
		},
	}
	nmt := map[string]float64{
		"Українська мова":  180,
		motivationalLetter: 200, // user mistakenly added it — should be ignored
	}
	got := ComputeRating(prog, nmt)
	want := 180 * 0.35
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestComputeRating_AttestatRescaledForEB40(t *testing.T) {
	prog := &Program{
		EB: 40,
		Subjects: []SubjectMeta{
			{Name: "Атестат", Coefficient: 0.10, SubjectID: SubjectAttestat},
		},
	}
	// ball=11.5 → (11.5-2)*10 + 100 = 195
	got := ComputeRating(prog, map[string]float64{"Атестат": 11.5})
	want := 195.0 * 0.10
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestComputeRating_AttestatBelowTwoFloors(t *testing.T) {
	prog := &Program{
		EB: 40,
		Subjects: []SubjectMeta{
			{Name: "Атестат", Coefficient: 0.10, SubjectID: SubjectAttestat},
		},
	}
	got := ComputeRating(prog, map[string]float64{"Атестат": 1.5})
	want := 100.0 * 0.10
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestComputeRating_AttestatRescaleSkippedWhenEBNot40(t *testing.T) {
	prog := &Program{
		EB: 30, // not 40 → no rescale
		Subjects: []SubjectMeta{
			{Name: "Атестат", Coefficient: 0.10, SubjectID: SubjectAttestat},
		},
	}
	got := ComputeRating(prog, map[string]float64{"Атестат": 11.5})
	want := 11.5 * 0.10
	if math.Abs(got-want) > 1e-9 {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestComputeRating_EdgeCases(t *testing.T) {
	prog := &Program{Subjects: []SubjectMeta{{Name: "X", Coefficient: 1}}}

	if v := ComputeRating(nil, nil); v != 0 {
		t.Errorf("nil program: %v", v)
	}
	if v := ComputeRating(prog, nil); v != 0 {
		t.Errorf("nil nmt: %v", v)
	}
	if v := ComputeRating(&Program{}, map[string]float64{"X": 100}); v != 0 {
		t.Errorf("empty subjects: %v", v)
	}
	if v := ComputeRating(prog, map[string]float64{"Y": 100}); v != 0 {
		t.Errorf("no overlap: %v", v)
	}
}
