package abit

import (
	"fmt"
	"slices"
	"strconv"
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

// Tier is the reach/match/safety bucket used by the "where can I get in"
// list — the standard admissions framing for spreading priorities so a
// candidate doesn't burn them all on long shots.
type Tier int

const (
	TierNone   Tier = iota // chance couldn't be classified (no budget data)
	TierReach              // ambitious: low chance or no free seats
	TierMatch              // borderline: depends on others dropping out
	TierSafety             // confident: ranks within the free seats
)

// Tier maps a ChanceLevel into its reach/match/safety bucket.
func (c ChanceLevel) Tier() Tier {
	switch c {
	case ChanceHigh, ChanceHighQuota1, ChanceHighQuota2:
		return TierSafety
	case ChanceMedium:
		return TierMatch
	case ChanceLow, ChanceZero:
		return TierReach
	}
	return TierNone
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
	// Example: "license-volume-missing" — budget volume unknown, analysis is
	// qualitative. Empty when everything resolved.
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
//     bucket them by their admission track: KV1 / KV2 / general, OR a
//     separate reserved track (СБ співбесіда / KV3) that does NOT compete
//     for general seats, OR count them under "already enrolled" if the
//     status is "до наказу" / "рекомендовано" (tracking which quota seat
//     an enrolled quota holder occupies).
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
		q1, q2, general                         []Abiturient
		alreadyEnrolled                         int
		q1Enrolled, q2Enrolled, generalEnrolled int // enrolled holders already consuming a seat on each track
		reservedOther                           int // СБ/КВ3-only entrants — admitted on a separate (співбесіда/quota-3) track, NOT the general competition
	)
	for _, ab := range abits {
		hasKV1 := slices.Contains(ab.Quotas, QuotaKV1)
		hasKV2 := slices.Contains(ab.Quotas, QuotaKV2)
		// СБ (співбесіда) and КВ3 are alternative admission tracks onto
		// reserved seats, not the general NMT competition. An applicant
		// who is ALSO КВ1/КВ2 is a quota entrant via співбесіда → counted
		// under that quota (КВ1/КВ2 take precedence below).
		reservedTrack := slices.Contains(ab.Quotas, QuotaSB) || slices.Contains(ab.Quotas, QuotaKV3)
		enrolled := IsEnrolledStatus(ab.Status)
		quotaHolder := hasKV1 || hasKV2

		// Admission gate. A manual override wins outright (false → ignore
		// this applicant everywhere). Otherwise the applicant must be alive
		// on the budget track; and a PLAIN general/reserved applicant must
		// additionally score at least as high as the user — someone below
		// the user can neither outrank them nor take a seat they'd contest.
		//
		// Quota holders and already-enrolled applicants are counted
		// regardless of the user's score: a КВ1 entrant with a lower score
		// still occupies a reserved КВ1 seat (decided within the quota), and
		// an enrolled holder has already claimed one. Excluding those
		// lower-scored quota holders is exactly what used to under-count
		// quota consumption and inflate the free general pool.
		if v, forced := in.Overrides[strconv.Itoa(ab.ID)]; forced {
			if !v {
				continue
			}
		} else if !IsBudgetContender(ab) {
			continue
		} else if !enrolled && !quotaHolder && ab.Score < in.UserScore {
			continue
		}

		if enrolled {
			alreadyEnrolled++
			// Track which track's seat each enrolled holder occupies. These
			// are committed (до наказу = order issued) — they will NOT drop,
			// so they permanently subtract from that track's capacity.
			switch {
			case hasKV1:
				q1Enrolled++
			case hasKV2:
				q2Enrolled++
			case reservedTrack:
				// reserved-track enrolment — doesn't touch general seats
			default:
				generalEnrolled++
			}
			continue
		}
		switch {
		case hasKV1:
			q1 = append(q1, ab)
		case hasKV2:
			q2 = append(q2, ab)
		case reservedTrack:
			reservedOther++ // counted as a competitor, but not in the general pool
		default:
			general = append(general, ab)
		}
	}
	out.CompetitorsTotal = len(q1) + len(q2) + len(general) + reservedOther + alreadyEnrolled
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

	// Seats consumed by each quota (capped at its reservation). Unused
	// quota seats roll into the general pool automatically because we
	// subtract only what each quota actually USES, not its full size.
	q1Used := minInt(out.Quota1Total, len(q1)+q1Enrolled)
	q2Used := minInt(out.Quota2Total, len(q2)+q2Enrolled)
	generalSeats := maxInt(0, out.BudgetTotal-q1Used-q2Used)

	// Quota-1 / Quota-2 paths. Quota seats already taken by enrolled
	// quota holders are gone, so the user competes for the seats that
	// remain (q*Avail), ranked against the not-yet-enrolled quota pool.
	if slices.Contains(in.UserQuotas, QuotaKV1) {
		rank := rankByScore(q1, in.UserScore)
		out.MyRealRank = rank
		q1Avail := maxInt(0, out.Quota1Total-q1Enrolled)
		if rank <= q1Avail {
			out.Chance = ChanceHighQuota1
			out.Advice = fmt.Sprintf(
				"Проходиш по Квоті 1! (%d-й на %d вільних із %d місць)",
				rank, q1Avail, out.Quota1Total)
			return out
		}
	}
	if out.Chance == ChanceUnknown && slices.Contains(in.UserQuotas, QuotaKV2) {
		rank := rankByScore(q2, in.UserScore)
		out.MyRealRank = rank
		q2Avail := maxInt(0, out.Quota2Total-q2Enrolled)
		if rank <= q2Avail {
			out.Chance = ChanceHighQuota2
			out.Advice = fmt.Sprintf(
				"Проходиш по Квоті 2! (%d-й на %d вільних із %d місць)",
				rank, q2Avail, out.Quota2Total)
			return out
		}
	}

	// General pool. Committed enrolees (generalEnrolled) hold their seats
	// permanently; the rest are contested by score. The user passes if
	// their rank among the not-yet-enrolled pool fits within the seats
	// left after the enrolees. Crucially we do NOT also subtract the
	// higher-ranked-but-not-enrolled people from capacity — they ARE the
	// rank, so subtracting them too would double-count (the old bug that
	// reported "Low" for a rank-11 applicant with 15 seats).
	freeAfterEnrolled := maxInt(0, generalSeats-generalEnrolled)
	rank := rankByScore(general, in.UserScore)
	out.MyRealRank = rank
	out.RemainingSpots = maxInt(0, freeAfterEnrolled-(rank-1))

	switch {
	case freeAfterEnrolled <= 0:
		out.Chance = ChanceZero
		out.Advice = "Усі бюджетні місця вже зайняті зарахованими — у загальному конкурсі вільних немає."
	case rank <= freeAfterEnrolled:
		out.Chance = ChanceHigh
		out.Advice = fmt.Sprintf(
			"Ти %d-й у загальному конкурсі на %d бюджетних місць. Проходиш! 🎉",
			rank, freeAfterEnrolled)
	case rank <= freeAfterEnrolled+5:
		out.Chance = ChanceMedium
		gap := rank - freeAfterEnrolled
		out.Advice = fmt.Sprintf(
			"Ти %d-й на %d бюджетних місць. Є шанс — якщо %d вище відмовляться.",
			rank, freeAfterEnrolled, gap)
	default:
		out.Chance = ChanceLow
		out.Advice = fmt.Sprintf(
			"Ти %d-й, а бюджетних місць лише %d. Шанси малі. 😔",
			rank, freeAfterEnrolled)
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
