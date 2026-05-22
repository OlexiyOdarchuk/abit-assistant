package abit

import (
	"fmt"
	"slices"
	"strings"
)

// ChanceLevel ranks the user's estimated probability of being admitted
// to the program on a budget seat.
type ChanceLevel int

const (
	ChanceUnknown ChanceLevel = iota
	// ChanceZero — no budget seats left in the general pool.
	ChanceZero
	// ChanceLow — user is ranked well below the remaining-seats cutoff.
	ChanceLow
	// ChanceMedium — user is within a small margin of the cutoff;
	// admission depends on others dropping out.
	ChanceMedium
	// ChanceHigh — user ranks within the remaining general-pool seats.
	ChanceHigh
	// ChanceHighQuota1 — user qualifies under Quota 1 and ranks within it.
	ChanceHighQuota1
	// ChanceHighQuota2 — user qualifies under Quota 2 and ranks within it.
	ChanceHighQuota2
)

// Emoji is the marker shown on the summary screen.
func (c ChanceLevel) Emoji() string {
	switch c {
	case ChanceHigh, ChanceHighQuota1, ChanceHighQuota2:
		return "🟢"
	case ChanceMedium:
		return "🟡"
	case ChanceLow:
		return "🔴"
	case ChanceZero:
		return "⚫"
	}
	return "❔"
}

// Label returns the human-readable name of the chance level.
func (c ChanceLevel) Label() string {
	switch c {
	case ChanceHigh:
		return "Високий"
	case ChanceHighQuota1:
		return "Високий (Квота 1)"
	case ChanceHighQuota2:
		return "Високий (Квота 2)"
	case ChanceMedium:
		return "Середній"
	case ChanceLow:
		return "Низький"
	case ChanceZero:
		return "Нульовий"
	}
	return "Невідомий"
}

// Analysis is the result of running a program through the matchmaking
// heuristic. Numeric fields are 0 when the underlying datum couldn't
// be inferred (e.g. licensed volume not scraped).
type Analysis struct {
	UserScore        float64     `json:"user_score"`
	BudgetTotal      int         `json:"budget_total"`      // total budget seats (licensed volume)
	Quota1Total      int         `json:"quota1_total"`      // seats reserved for Q1
	Quota2Total      int         `json:"quota2_total"`      // seats reserved for Q2
	RemainingSpots   int         `json:"remaining_spots"`   // free seats in the general pool
	AlreadyEnrolled  int         `json:"already_enrolled"`  // applicants on "до наказу/рекомендовано" — they hold seats
	CompetitorsTotal int         `json:"competitors_total"` // applicants that pass IsCompetitor
	MyRealRank       int         `json:"my_real_rank"`      // 1-based rank against real competitors
	Chance           ChanceLevel `json:"chance"`
	Advice           string      `json:"advice"`

	// Warnings are non-fatal degradation hints surfaced to the user.
	// Examples: "RegionCoef requested but RK not parsed", "budget volume
	// unknown — analysis is qualitative". Empty when everything resolved.
	Warnings []string `json:"warnings,omitempty"`
}

// AnalyzeInput captures the user-facing context the analysis needs.
type AnalyzeInput struct {
	// UserScore is the applicant's own rating, typically the result of
	// ComputeRating. 0 means "profile not filled" and the analysis falls
	// back to "Unknown" everywhere.
	UserScore float64
	// UserQuotas lists quota codes (QuotaKV1, QuotaKV2, ...) the user
	// qualifies under. Sourced from storage.UserSettings.Quotas.
	UserQuotas []string
	// Overrides is the optional per-applicant manual verdict map. Lets
	// the user say "ignore #42 — they'll pass elsewhere" or "treat #99
	// as a real threat" and have the analysis reflect that.
	Overrides OverrideMap
}

// Analyze ranks the user against the field of applicants on the given
// program and packages the result for display.
//
// Algorithm (mirrors recalculate_analysis from filter.py, adjusted for
// our isCompetitor predicate):
//
//  1. Walk the applicant list; for each one that IsCompetitor passes,
//     bucket them by their quota (KV1 / KV2 / general) OR count them
//     under "already enrolled" if the status is "до наказу" / "рекомендовано".
//  2. Apply quota caps: q1_taken = min(len(q1), Quota1Volume), same for Q2.
//  3. General-pool seats taken = min(len(general), Budget - q1_taken - q2_taken).
//  4. Remaining seats = Budget - q1_taken - q2_taken - general_taken
//     - already_enrolled. Clamped to ≥ 0.
//  5. If the user holds a quota — rank against the bucket; "high" chance
//     when rank ≤ quota volume.
//  6. Otherwise rank against the general bucket:
//     • rank ≤ remaining → High (passes through)
//     • rank ≤ remaining + 5 → Medium (others may drop)
//     • else → Low / Zero
func Analyze(prog *Program, abits []Abiturient, in AnalyzeInput) Analysis {
	out := Analysis{
		UserScore:   in.UserScore,
		BudgetTotal: prog.BudgetVolume(),
		Quota1Total: prog.Quota1Volume(),
		Quota2Total: prog.Quota2Volume(),
	}
	if in.UserScore <= 0 {
		// No profile → no meaningful analysis.
		out.Advice = "Заповни /profile, щоб побачити свої шанси."
		return out
	}

	var (
		q1, q2, general []Abiturient
		alreadyEnrolled int
	)
	for _, ab := range abits {
		if !IsCompetitorWith(ab, in.UserScore, in.Overrides) {
			continue
		}
		low := strings.ToLower(ab.Status)
		if strings.Contains(low, "до наказу") || strings.Contains(low, "рекомендовано") {
			alreadyEnrolled++
			continue
		}
		switch {
		case slices.Contains(ab.Quotas, QuotaKV1):
			q1 = append(q1, ab)
		case slices.Contains(ab.Quotas, QuotaKV2):
			q2 = append(q2, ab)
		default:
			general = append(general, ab)
		}
	}
	out.CompetitorsTotal = len(q1) + len(q2) + len(general) + alreadyEnrolled
	out.AlreadyEnrolled = alreadyEnrolled

	// If the volume scraper failed to find a license size, we cannot
	// honestly compute remaining seats — bail out with Unknown rather
	// than fabricating a budget that lands every user in "High chance".
	if out.BudgetTotal <= 0 {
		out.Chance = ChanceUnknown
		out.Advice = "Ліцензований обсяг не визначено — точний аналіз шансів недоступний."
		out.Warnings = append(out.Warnings, "license-volume-missing")
		// Still surface a rank against the general pool so the user
		// has SOMETHING — but mark it as informational only.
		out.MyRealRank = rankByScore(general, in.UserScore)
		return out
	}

	q1Taken := minInt(len(q1), out.Quota1Total)
	q2Taken := minInt(len(q2), out.Quota2Total)
	generalCap := maxInt(0, out.BudgetTotal-q1Taken-q2Taken)
	generalTaken := minInt(len(general), generalCap)
	out.RemainingSpots = maxInt(0,
		out.BudgetTotal-q1Taken-q2Taken-generalTaken-alreadyEnrolled)

	// Quota-1 / Quota-2 paths.
	if slices.Contains(in.UserQuotas, QuotaKV1) {
		rank := rankByScore(q1, in.UserScore)
		out.MyRealRank = rank
		if rank <= out.Quota1Total {
			out.Chance = ChanceHighQuota1
			out.Advice = fmt.Sprintf(
				"Проходиш по Квоті 1! (%d-й з %d місць)",
				rank, out.Quota1Total)
			return out
		}
	}
	if out.Chance == ChanceUnknown && slices.Contains(in.UserQuotas, QuotaKV2) {
		rank := rankByScore(q2, in.UserScore)
		out.MyRealRank = rank
		if rank <= out.Quota2Total {
			out.Chance = ChanceHighQuota2
			out.Advice = fmt.Sprintf(
				"Проходиш по Квоті 2! (%d-й з %d місць)",
				rank, out.Quota2Total)
			return out
		}
	}

	// General pool.
	rank := rankByScore(general, in.UserScore)
	out.MyRealRank = rank

	switch {
	case out.RemainingSpots <= 0:
		out.Chance = ChanceZero
		out.Advice = "Бюджетних місць у загальному конкурсі не залишилося."
	case rank <= out.RemainingSpots:
		out.Chance = ChanceHigh
		out.Advice = fmt.Sprintf(
			"Ти %d-й претендент на %d вільних місць. Шанси чудові! 🎉",
			rank, out.RemainingSpots)
	case rank <= out.RemainingSpots+5:
		out.Chance = ChanceMedium
		gap := rank - out.RemainingSpots
		out.Advice = fmt.Sprintf(
			"Ти %d-й на %d місць. Є шанс — якщо %d людей відмовляться.",
			rank, out.RemainingSpots, gap)
	default:
		out.Chance = ChanceLow
		out.Advice = fmt.Sprintf(
			"Ти %d-й, а місць лише %d. Шанси малі. 😔",
			rank, out.RemainingSpots)
	}
	return out
}

// rankByScore returns 1-based dense rank: 1 + number of applicants in
// `pool` with score strictly above mine.
func rankByScore(pool []Abiturient, mine float64) int {
	rank := 1
	for _, ab := range pool {
		if ab.Score > mine {
			rank++
		}
	}
	return rank
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
