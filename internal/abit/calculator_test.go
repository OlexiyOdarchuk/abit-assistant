package abit

import (
	"math"
	"testing"
)

// Sample program used by most of the tests below — a typical "soft"
// specialty with 3 required НМТ subjects + 5 alternatives.
func sampleProgram() *Program {
	return &Program{
		EB:  40,
		OKR: 1,
		Subjects: []SubjectMeta{
			{Name: "Українська мова", Coefficient: 0.35},
			{Name: "Математика", Coefficient: 0.40},
			{Name: "Історія України", Coefficient: 0.25},
			{Name: "Українська література", Coefficient: 0.25},
			{Name: "Англійська мова", Coefficient: 0.25},
			{Name: "Біологія", Coefficient: 0.20},
		},
	}
}

func nearly(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.005 {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestComputeRating_WeightedAverageOfRequired(t *testing.T) {
	prog := sampleProgram()
	got := ComputeRating(prog, RatingInput{
		NMT: map[string]float64{
			"Українська мова": 180,
			"Математика":      170,
			"Історія України": 175,
		},
	})
	want := (180*0.35 + 170*0.40 + 175*0.25) / (0.35 + 0.40 + 0.25)
	nearly(t, got, want)
}

func TestComputeRating_PicksBestAdditional(t *testing.T) {
	prog := sampleProgram()
	got := ComputeRating(prog, RatingInput{
		NMT: map[string]float64{
			"Українська мова":       180,
			"Математика":            170,
			"Історія України":       175,
			"Українська література": 150, // 150*0.25 = 37.5
			"Англійська мова":       190, // 190*0.25 = 47.5 ← winner
			"Біологія":              200, // 200*0.20 = 40
		},
	})
	num := 180*0.35 + 170*0.40 + 175*0.25 + 190*0.25
	den := 0.35 + 0.40 + 0.25 + 0.25
	nearly(t, got, num/den)
}

// TestComputeRating_ElectivePenalty pins the official 2025 denominator
// term (К4макс + К4)/2 for the elective slot. The program's best
// elective coefficient (K4Max) is 0.40, but the user's best scoring
// elective uses coef 0.25 — so the denominator must charge (0.40+0.25)/2
// = 0.325 for that slot, not 0.25, lowering the score.
func TestComputeRating_ElectivePenalty(t *testing.T) {
	prog := sampleProgram()
	prog.K4Max = 0.40
	got := ComputeRating(prog, RatingInput{
		NMT: map[string]float64{
			"Українська мова": 180,
			"Математика":      170,
			"Історія України": 175,
			"Англійська мова": 190, // coef 0.25, the winning elective
		},
	})
	num := 180*0.35 + 170*0.40 + 175*0.25 + 190*0.25
	den := 0.35 + 0.40 + 0.25 + (0.40+0.25)/2
	nearly(t, got, num/den)
}

func TestComputeRating_CreativeScoreIncluded(t *testing.T) {
	prog := &Program{
		EB: 40,
		Subjects: []SubjectMeta{
			{Name: "Українська мова", Coefficient: 0.20},
			{Name: "Математика", Coefficient: 0.20},
			{Name: "Історія України", Coefficient: 0.20},
			{Name: CreativeContest, Coefficient: 0.40},
		},
	}
	got := ComputeRating(prog, RatingInput{
		NMT: map[string]float64{
			"Українська мова": 180,
			"Математика":      170,
			"Історія України": 175,
		},
		CreativeScore: 200,
	})
	num := 180*0.20 + 170*0.20 + 175*0.20 + 200*0.40
	den := 0.20 + 0.20 + 0.20 + 0.40
	nearly(t, got, num/den)
}

func TestComputeRating_RegionCoefAlwaysMultiplies(t *testing.T) {
	// РК is a program property, not a user toggle — it applies whenever the
	// program defines one (> 1).
	prog := sampleProgram()
	prog.RK = 1.05
	got := ComputeRating(prog, RatingInput{
		NMT: map[string]float64{
			"Українська мова": 180,
			"Математика":      170,
			"Історія України": 175,
		},
	})
	base := (180*0.35 + 170*0.40 + 175*0.25) / (0.35 + 0.40 + 0.25)
	nearly(t, got, base*1.05)
}

func TestComputeRating_NoRegionCoefWhenProgramHasNone(t *testing.T) {
	prog := sampleProgram()
	prog.RK = 1 // no regional coefficient → no-op
	got := ComputeRating(prog, RatingInput{
		NMT: map[string]float64{"Українська мова": 180, "Математика": 170, "Історія України": 175},
	})
	base := (180*0.35 + 170*0.40 + 175*0.25) / (0.35 + 0.40 + 0.25)
	nearly(t, got, base)
}

func TestComputeRating_ClampedAt200(t *testing.T) {
	prog := sampleProgram()
	prog.RK = 1.10
	got := ComputeRating(prog, RatingInput{
		NMT: map[string]float64{
			"Українська мова": 200,
			"Математика":      200,
			"Історія України": 200,
		},
	})
	if got != 200 {
		t.Errorf("got %v, want 200", got)
	}
}

func TestComputeRating_EdgeCases(t *testing.T) {
	if v := ComputeRating(nil, RatingInput{}); v != 0 {
		t.Errorf("nil program: %v", v)
	}
	if v := ComputeRating(sampleProgram(), RatingInput{}); v != 0 {
		t.Errorf("no nmt: %v", v)
	}
	if v := ComputeRating(&Program{}, RatingInput{NMT: map[string]float64{"X": 100}}); v != 0 {
		t.Errorf("empty subjects: %v", v)
	}
	// nmt overlaps no required subject, no additional — rating 0.
	prog := &Program{
		Subjects: []SubjectMeta{{Name: "Українська мова", Coefficient: 0.35}},
	}
	if v := ComputeRating(prog, RatingInput{NMT: map[string]float64{"Біологія": 200}}); v != 0 {
		t.Errorf("no overlap: %v", v)
	}
}

func TestComputeRating_ExcludesCreativeFromAdditional(t *testing.T) {
	// CreativeContest must NOT be chosen as the "best additional" even
	// if it happens to have a score in NMT — it goes via CreativeScore.
	prog := &Program{
		EB: 40,
		Subjects: []SubjectMeta{
			{Name: "Українська мова", Coefficient: 0.25},
			{Name: "Математика", Coefficient: 0.25},
			{Name: "Історія України", Coefficient: 0.25},
			{Name: "Англійська мова", Coefficient: 0.20},
			{Name: CreativeContest, Coefficient: 0.50}, // big coef, big trap
		},
	}
	got := ComputeRating(prog, RatingInput{
		NMT: map[string]float64{
			"Українська мова": 180,
			"Математика":      170,
			"Історія України": 175,
			"Англійська мова": 150,
			CreativeContest:   200, // should be ignored as "additional"
		},
	})
	// Best additional must be "Англійська мова", not CreativeContest.
	num := 180*0.25 + 170*0.25 + 175*0.25 + 150*0.20
	den := 0.25*3 + 0.20
	nearly(t, got, num/den)
}
