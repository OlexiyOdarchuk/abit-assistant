package abit

import "testing"

// ab builds a fixture Abiturient with the fields Analyze actually
// inspects. Defaults: budget applicant, no quotas, "Допущено" status.
func ab(id int, score float64, opts ...func(*Abiturient)) Abiturient {
	a := Abiturient{
		ID:             id,
		Score:          score,
		StateEducation: true,
		Status:         "Допущено",
		Priority:       1,
	}
	for _, opt := range opts {
		opt(&a)
	}
	return a
}

func withStatus(s string) func(*Abiturient) { return func(a *Abiturient) { a.Status = s } }
func withQuotas(qs ...string) func(*Abiturient) {
	return func(a *Abiturient) { a.Quotas = append([]string{}, qs...) }
}
func contract(a *Abiturient) { a.StateEducation = false }

func progWithVolume(budget, q1, q2 int) *Program {
	return &Program{
		Volume: map[string]string{
			"Максимальний обсяг державного замовлення": itoaForTest(budget),
			"Квота 1": itoaForTest(q1),
			"Квота 2": itoaForTest(q2),
		},
	}
}
func itoaForTest(n int) string {
	if n == 0 {
		return ""
	}
	return formatInt(n)
}
func formatInt(n int) string {
	// avoid importing strconv into the test fixture
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func TestAnalyze_NoProfile_ReturnsHintOnly(t *testing.T) {
	got := Analyze(&Program{}, nil, AnalyzeInput{UserScore: 0})
	if got.Chance != ChanceUnknown {
		t.Errorf("Chance = %v, want Unknown", got.Chance)
	}
	if got.Advice == "" {
		t.Error("Advice should suggest filling profile")
	}
}

func TestAnalyze_GeneralPool_HighChance(t *testing.T) {
	prog := progWithVolume(50, 0, 0)
	abits := []Abiturient{
		ab(1, 195),
		ab(2, 190),
		ab(3, 185), // 3 above me
		ab(4, 160), // 1 below me (not a competitor)
	}
	got := Analyze(prog, abits, AnalyzeInput{UserScore: 175})
	if got.Chance != ChanceHigh {
		t.Errorf("Chance = %v (%s), want High", got.Chance, got.Chance.Label())
	}
	if got.MyRealRank != 4 {
		t.Errorf("rank = %d, want 4", got.MyRealRank)
	}
	if got.BudgetTotal != 50 {
		t.Errorf("BudgetTotal = %d", got.BudgetTotal)
	}
}

func TestAnalyze_GeneralPool_MediumChance(t *testing.T) {
	// budget 5, nobody enrolled. 9 competitors above me → rank 10. That's
	// just past capacity (5) but within the +5 "others may drop" band.
	prog := progWithVolume(5, 0, 0)
	abits := make([]Abiturient, 9)
	for i := range abits {
		abits[i] = ab(i+1, 180+float64(i)) // all > my 170
	}
	got := Analyze(prog, abits, AnalyzeInput{UserScore: 170})
	if got.Chance != ChanceMedium {
		t.Errorf("Chance = %v (%s), want Medium (rank=%d, seats=5)",
			got.Chance, got.Chance.Label(), got.MyRealRank)
	}
	if got.MyRealRank != 10 {
		t.Errorf("rank = %d, want 10", got.MyRealRank)
	}
}

func TestAnalyze_GeneralPool_HighWhenRankFitsBudget(t *testing.T) {
	// 10 competitors above me, budget 15, none enrolled. Rank 11 ≤ 15 →
	// the applicant clearly gets a seat. (Regression: the old double-count
	// reported Low here, discouraging a student who actually passes.)
	prog := progWithVolume(15, 0, 0)
	abits := make([]Abiturient, 10)
	for i := range abits {
		abits[i] = ab(i+1, 180+float64(i))
	}
	got := Analyze(prog, abits, AnalyzeInput{UserScore: 170})
	if got.Chance != ChanceHigh {
		t.Errorf("Chance = %v (%s), want High (rank=%d, 15 seats)",
			got.Chance, got.Chance.Label(), got.MyRealRank)
	}
	if got.MyRealRank != 11 {
		t.Errorf("rank = %d, want 11", got.MyRealRank)
	}
}

func TestAnalyze_GeneralPool_LowChance(t *testing.T) {
	// budget 3, 10 competitors above me → rank 11, far past 3+5. Low.
	prog := progWithVolume(3, 0, 0)
	abits := make([]Abiturient, 10)
	for i := range abits {
		abits[i] = ab(i+1, 180+float64(i))
	}
	got := Analyze(prog, abits, AnalyzeInput{UserScore: 170})
	if got.Chance != ChanceLow {
		t.Errorf("Chance = %v (%s), want Low (rank=%d, seats=3)",
			got.Chance, got.Chance.Label(), got.MyRealRank)
	}
}

func TestAnalyze_AlreadyEnrolledTakesSeats(t *testing.T) {
	prog := progWithVolume(3, 0, 0)
	// 3 "до наказу" (all already taking seats), 0 left → Zero
	abits := []Abiturient{
		ab(1, 200, withStatus("До наказу (бюджет)")),
		ab(2, 195, withStatus("До наказу (бюджет)")),
		ab(3, 190, withStatus("Рекомендовано (бюджет)")),
	}
	got := Analyze(prog, abits, AnalyzeInput{UserScore: 175})
	if got.AlreadyEnrolled != 3 {
		t.Errorf("AlreadyEnrolled = %d, want 3", got.AlreadyEnrolled)
	}
	if got.RemainingSpots != 0 {
		t.Errorf("RemainingSpots = %d, want 0", got.RemainingSpots)
	}
	if got.Chance != ChanceZero {
		t.Errorf("Chance = %v (%s), want Zero", got.Chance, got.Chance.Label())
	}
}

func TestAnalyze_QuotaPath_PassesQuota1(t *testing.T) {
	prog := progWithVolume(10, 2, 1)
	// One competitor in Q1, both budget seats free for me.
	abits := []Abiturient{
		ab(1, 180, withQuotas(QuotaKV1)),
	}
	got := Analyze(prog, abits, AnalyzeInput{
		UserScore:  185,
		UserQuotas: []string{QuotaKV1},
	})
	if got.Chance != ChanceHighQuota1 {
		t.Errorf("Chance = %v (%s), want HighQuota1", got.Chance, got.Chance.Label())
	}
	if got.MyRealRank != 1 {
		t.Errorf("rank = %d, want 1 (nobody above me in Q1)", got.MyRealRank)
	}
}

func TestAnalyze_SBExcludedFromGeneralPool(t *testing.T) {
	// A general (non-quota) user with budget 2. Three higher-scored
	// applicants: two are СБ (співбесіда — reserved track), one is a real
	// general competitor. Only the genuine general competitor should rank
	// against the user, so rank = 2 (1 above + me) and the seat is winnable.
	prog := progWithVolume(2, 0, 0)
	abits := []Abiturient{
		ab(1, 195, withQuotas(QuotaSB)),
		ab(2, 192, withQuotas(QuotaSB)),
		ab(3, 188), // the only general competitor above me
	}
	got := Analyze(prog, abits, AnalyzeInput{UserScore: 180})
	if got.CompetitorsTotal != 3 {
		t.Errorf("CompetitorsTotal = %d, want 3 (СБ still counted overall)", got.CompetitorsTotal)
	}
	if got.MyRealRank != 2 {
		t.Errorf("MyRealRank = %d, want 2 (only the 1 general competitor above me)", got.MyRealRank)
	}
	if got.Chance != ChanceHigh {
		t.Errorf("Chance = %v (%s), want High — СБ entrants must not crowd the general pool",
			got.Chance, got.Chance.Label())
	}
}

func TestAnalyze_EnrolledQuotaHolderConsumesSeat(t *testing.T) {
	// Quota-1 has 1 seat. A higher-scored КВ1 applicant is already "до
	// наказу" — they took the only quota seat. The user (also КВ1) must
	// NOT be told they pass Q1.
	prog := progWithVolume(10, 1, 0)
	abits := []Abiturient{
		ab(1, 190, withQuotas(QuotaKV1), withStatus("До наказу (бюджет)")),
	}
	got := Analyze(prog, abits, AnalyzeInput{
		UserScore:  185,
		UserQuotas: []string{QuotaKV1},
	})
	if got.Chance == ChanceHighQuota1 {
		t.Errorf("Chance = HighQuota1, but the only Q1 seat is already taken by an enrolled holder")
	}
}

func TestAnalyze_LowScoredQuotaHolderShrinksGeneralPool(t *testing.T) {
	// budget 5, Q1 reserves 2. Two КВ1 applicants score BELOW the user but
	// still occupy their reserved seats, leaving only 3 seats in the general
	// pool. With 3 general competitors above the user (rank 4), the honest
	// verdict is Medium — not the inflated High the old score-filter produced
	// by dropping the low-scored quota holders and pretending all 5 seats
	// were up for general grabs.
	prog := progWithVolume(5, 2, 0)
	abits := []Abiturient{
		ab(1, 150, withQuotas(QuotaKV1)), // below me, but takes a Q1 seat
		ab(2, 155, withQuotas(QuotaKV1)), // below me, but takes a Q1 seat
		ab(3, 180),
		ab(4, 185),
		ab(5, 190), // 3 general competitors above me
	}
	got := Analyze(prog, abits, AnalyzeInput{UserScore: 170})
	if got.MyRealRank != 4 {
		t.Errorf("rank = %d, want 4", got.MyRealRank)
	}
	if got.Chance != ChanceMedium {
		t.Errorf("Chance = %v (%s), want Medium — two Q1 seats are gone, only 3 general seats left",
			got.Chance, got.Chance.Label())
	}
}

func TestAnalyze_NoQuotaHolders_KeepsFullGeneralPool(t *testing.T) {
	// Control for the test above: identical general field, but no quota
	// holders eating into the budget → all 5 seats are general → rank 4
	// passes comfortably (High).
	prog := progWithVolume(5, 2, 0)
	abits := []Abiturient{
		ab(3, 180),
		ab(4, 185),
		ab(5, 190),
	}
	got := Analyze(prog, abits, AnalyzeInput{UserScore: 170})
	if got.Chance != ChanceHigh {
		t.Errorf("Chance = %v (%s), want High (rank 4, 5 general seats)",
			got.Chance, got.Chance.Label())
	}
}

func TestAnalyze_QuotaConsumptionCappedAtVolume(t *testing.T) {
	// More КВ1 applicants (3) than Q1 seats (1): consumption is capped at 1,
	// so exactly one general seat is removed, not three.
	prog := progWithVolume(4, 1, 0)
	abits := []Abiturient{
		ab(1, 150, withQuotas(QuotaKV1)),
		ab(2, 152, withQuotas(QuotaKV1)),
		ab(3, 154, withQuotas(QuotaKV1)),
		ab(4, 185),
		ab(5, 190),
		ab(6, 195), // 3 general competitors above me → rank 4
	}
	got := Analyze(prog, abits, AnalyzeInput{UserScore: 170})
	// generalSeats = 4 - min(1,3) = 3. rank 4 > 3 but ≤ 3+5 → Medium.
	if got.Chance != ChanceMedium {
		t.Errorf("Chance = %v (%s), want Medium (1 Q1 seat consumed → 3 general)",
			got.Chance, got.Chance.Label())
	}
}

func TestAnalyze_NonCompetitorsExcluded(t *testing.T) {
	prog := progWithVolume(10, 0, 0)
	abits := []Abiturient{
		ab(1, 200, contract), // contract — out
		ab(2, 195, withStatus("Деактивовано (зарах. на бюджет)")), // gone elsewhere
		ab(3, 190, withStatus("Скасовано")),                       // out
		ab(4, 180), // genuine competitor
	}
	got := Analyze(prog, abits, AnalyzeInput{UserScore: 170})
	if got.CompetitorsTotal != 1 {
		t.Errorf("CompetitorsTotal = %d, want 1", got.CompetitorsTotal)
	}
}
