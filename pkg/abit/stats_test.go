package abit

import (
	"math"
	"testing"
)

func TestSummarize_Empty(t *testing.T) {
	got := Summarize(nil)
	if got != (Summary{}) {
		t.Errorf("expected zero, got %+v", got)
	}
}

func TestSummarize_BasicArithmetic(t *testing.T) {
	in := []Abiturient{
		{ID: 1, Score: 100},
		{ID: 2, Score: 200},
		{ID: 3, Score: 150},
		{ID: 4, Score: 120},
		{ID: 5, Score: 180},
	}
	got := Summarize(in)
	if got.Count != 5 {
		t.Errorf("Count = %d", got.Count)
	}
	if got.Min != 100 || got.Max != 200 {
		t.Errorf("Min/Max = %v/%v", got.Min, got.Max)
	}
	wantMean := (100.0 + 200 + 150 + 120 + 180) / 5
	if math.Abs(got.Mean-wantMean) > 1e-9 {
		t.Errorf("Mean = %v, want %v", got.Mean, wantMean)
	}
	if got.Median != 150 {
		t.Errorf("Median = %v, want 150", got.Median)
	}
	// stddev should be > 0 for any varied set
	if got.StdDev <= 0 {
		t.Errorf("StdDev = %v", got.StdDev)
	}
}

func TestSummarize_MedianEvenCount(t *testing.T) {
	in := []Abiturient{{Score: 100}, {Score: 200}, {Score: 150}, {Score: 120}}
	if got := Summarize(in); got.Median != (120+150)/2.0 {
		t.Errorf("Median = %v, want 135", got.Median)
	}
}

func TestDistribution_BucketsCoverRange(t *testing.T) {
	in := []Abiturient{
		{Score: 142}, {Score: 145}, {Score: 158}, {Score: 161}, {Score: 178},
	}
	got := Distribution(in, 5)
	if len(got) == 0 {
		t.Fatal("expected non-empty distribution")
	}
	// Total count == input length.
	total := 0
	for _, b := range got {
		total += b.Count
	}
	if total != len(in) {
		t.Errorf("total %d != input %d", total, len(in))
	}
	// Buckets contiguous: each Hi == next Lo.
	for i := 1; i < len(got); i++ {
		if got[i].Lo != got[i-1].Hi {
			t.Errorf("gap at %d: %v / %v", i, got[i-1], got[i])
		}
	}
	// First bucket contains the min, last contains the max.
	if got[0].Lo > 142 || got[len(got)-1].Hi < 178 {
		t.Errorf("range mismatch: first=%+v last=%+v", got[0], got[len(got)-1])
	}
}

func TestDistribution_EdgeCases(t *testing.T) {
	if Distribution(nil, 5) != nil {
		t.Error("nil input should yield nil")
	}
	if Distribution([]Abiturient{{Score: 100}}, 0) != nil {
		t.Error("bucketSize 0 should yield nil")
	}
}

func TestRankByScore(t *testing.T) {
	in := []Abiturient{
		{Score: 200}, {Score: 180}, {Score: 175}, {Score: 150},
	}
	// I'd come second after the 200-er.
	if got := RankByScore(in, 190); got != 2 {
		t.Errorf("rank(190) = %d, want 2", got)
	}
	// At the top.
	if got := RankByScore(in, 250); got != 1 {
		t.Errorf("rank(top) = %d, want 1", got)
	}
	// Tie with 180 — dense ranking puts you AT the same rank as the 180-er, so
	// strictly more applicants are still 1 (the 200-er).
	if got := RankByScore(in, 180); got != 2 {
		t.Errorf("rank(tie) = %d, want 2", got)
	}
	// Below everyone.
	if got := RankByScore(in, 100); got != len(in)+1 {
		t.Errorf("rank(bottom) = %d, want %d", got, len(in)+1)
	}
}

func TestPercentile(t *testing.T) {
	in := []Abiturient{
		{Score: 100}, {Score: 120}, {Score: 150}, {Score: 180}, {Score: 200},
	}
	// score=160 beats 3 of 5 → 0.6
	if got := Percentile(in, 160); math.Abs(got-0.6) > 1e-9 {
		t.Errorf("percentile = %v, want 0.6", got)
	}
	if got := Percentile(nil, 100); got != 0 {
		t.Errorf("empty percentile = %v", got)
	}
}

func TestRealCompetitors(t *testing.T) {
	in := []Abiturient{
		{ID: 1, Score: 200, Priority: 1, Documents: true, StateEducation: true},  // counts
		{ID: 2, Score: 195, Priority: 2, Documents: true, StateEducation: true},  // wrong priority
		{ID: 3, Score: 190, Priority: 1, Documents: false, StateEducation: true}, // no docs
		{ID: 4, Score: 185, Priority: 1, Documents: true, StateEducation: false}, // contract — strict drops, lax keeps
		{ID: 5, Score: 170, Priority: 1, Documents: true, StateEducation: true},  // below myScore
	}
	const myScore = 175.0

	strict := RealCompetitors(in, myScore, true)
	if len(strict) != 1 || strict[0].ID != 1 {
		t.Errorf("strict: got %v", idsOf(strict))
	}

	lax := RealCompetitors(in, myScore, false)
	if len(lax) != 2 || lax[0].ID != 1 || lax[1].ID != 4 {
		t.Errorf("lax: got %v", idsOf(lax))
	}
}
