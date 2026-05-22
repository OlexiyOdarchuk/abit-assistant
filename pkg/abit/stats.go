package abit

import (
	"math"
	"slices"
)

// Summary is a compact descriptive-statistics view of a set of scores.
// All values are populated even for empty input (everything is zero).
type Summary struct {
	Count  int     `json:"count"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	StdDev float64 `json:"std_dev"`
}

// Summarize computes a Summary over the competitive scores of every
// applicant in the slice. Applicants with score == 0 are still counted —
// callers that want to exclude no-shows should Filter first.
func Summarize(applicants []Abiturient) Summary {
	if len(applicants) == 0 {
		return Summary{}
	}
	scores := scoresOf(applicants)
	slices.Sort(scores)

	var sum float64
	for _, s := range scores {
		sum += s
	}
	mean := sum / float64(len(scores))

	var sqDiff float64
	for _, s := range scores {
		d := s - mean
		sqDiff += d * d
	}
	stddev := math.Sqrt(sqDiff / float64(len(scores)))

	return Summary{
		Count:  len(scores),
		Min:    scores[0],
		Max:    scores[len(scores)-1],
		Mean:   mean,
		Median: median(scores),
		StdDev: stddev,
	}
}

// Bucket is one bin of a score histogram. Lo is inclusive, Hi is exclusive
// except for the topmost bucket which is inclusive on both ends.
type Bucket struct {
	Lo    float64 `json:"lo"`
	Hi    float64 `json:"hi"`
	Count int     `json:"count"`
}

// Distribution buckets applicant scores into fixed-width bins. bucketSize
// must be > 0; pass e.g. 5 for "5-point bins". The returned slice spans
// from floor(min/bucketSize) to ceil(max/bucketSize), so consecutive bins
// are guaranteed (gaps appear as Count = 0).
func Distribution(applicants []Abiturient, bucketSize float64) []Bucket {
	if len(applicants) == 0 || bucketSize <= 0 {
		return nil
	}
	scores := scoresOf(applicants)
	minS, maxS := scores[0], scores[0]
	for _, s := range scores[1:] {
		if s < minS {
			minS = s
		}
		if s > maxS {
			maxS = s
		}
	}

	loIdx := int(math.Floor(minS / bucketSize))
	hiIdx := int(math.Floor(maxS / bucketSize))
	out := make([]Bucket, 0, hiIdx-loIdx+1)
	for i := loIdx; i <= hiIdx; i++ {
		out = append(out, Bucket{
			Lo: float64(i) * bucketSize,
			Hi: float64(i+1) * bucketSize,
		})
	}
	for _, s := range scores {
		idx := int(math.Floor(s/bucketSize)) - loIdx
		if idx == len(out) {
			// the topmost score sits exactly on an upper edge — push into last bucket
			idx = len(out) - 1
		}
		out[idx].Count++
	}
	return out
}

// RankByScore returns the 1-based rank of an applicant with the given
// score among the applicants (higher score → better rank). Ties take the
// higher rank ("dense" ranking, like sport leaderboards). Returns
// len(applicants)+1 for a score strictly below the lowest entry.
func RankByScore(applicants []Abiturient, score float64) int {
	better := 0
	for _, ab := range applicants {
		if ab.Score > score {
			better++
		}
	}
	return better + 1
}

// Percentile returns the share of applicants strictly worse than score,
// in [0, 1]. Useful as "you're ahead of N% of people".
func Percentile(applicants []Abiturient, score float64) float64 {
	if len(applicants) == 0 {
		return 0
	}
	worse := 0
	for _, ab := range applicants {
		if ab.Score < score {
			worse++
		}
	}
	return float64(worse) / float64(len(applicants))
}

// RealCompetitors returns the subset of applicants who are genuinely
// competing with someone at myScore for a budget seat: they outrank
// myScore AND are committed (priority 1, documents submitted, opted for
// budget). This is the heuristic at the core of "The Technique" —
// counting only these gives a tighter ceiling than the raw ranking.
//
// Strict applies a stricter set (priority == 1 AND documents AND
// budget). Non-strict drops the budget requirement, which gives an
// upper bound that includes likely-budget contract applicants.
func RealCompetitors(applicants []Abiturient, myScore float64, strict bool) []Abiturient {
	out := make([]Abiturient, 0)
	for _, ab := range applicants {
		if ab.Score <= myScore {
			continue
		}
		if ab.Priority != 1 {
			continue
		}
		if !ab.Documents {
			continue
		}
		if strict && !ab.StateEducation {
			continue
		}
		out = append(out, ab)
	}
	return out
}

// --- helpers ---

func scoresOf(applicants []Abiturient) []float64 {
	out := make([]float64, len(applicants))
	for i, ab := range applicants {
		out[i] = ab.Score
	}
	return out
}

// median assumes scores is sorted ascending.
func median(scores []float64) float64 {
	n := len(scores)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return scores[n/2]
	}
	return (scores[n/2-1] + scores[n/2]) / 2
}
