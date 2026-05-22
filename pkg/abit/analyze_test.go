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
	// 5 competitors above me; budget 8 → all 5 take general seats,
	// leaving 3. My rank is 6 → 6 > 3 but ≤ 3+5, hits the medium band.
	prog := progWithVolume(8, 0, 0)
	abits := []Abiturient{
		ab(1, 200), ab(2, 195), ab(3, 190), ab(4, 188), ab(5, 180),
	}
	got := Analyze(prog, abits, AnalyzeInput{UserScore: 175})
	if got.Chance != ChanceMedium {
		t.Errorf("Chance = %v (%s), want Medium (rank=%d, remaining=%d)",
			got.Chance, got.Chance.Label(), got.MyRealRank, got.RemainingSpots)
	}
	if got.MyRealRank != 6 {
		t.Errorf("rank = %d", got.MyRealRank)
	}
}

func TestAnalyze_GeneralPool_LowChance(t *testing.T) {
	// 10 competitors above me; budget 15 → 10 of them take general
	// seats, leaving 5. My rank is 11 → 11 > 5+5, so Low.
	prog := progWithVolume(15, 0, 0)
	abits := make([]Abiturient, 10)
	for i := range abits {
		abits[i] = ab(i+1, 180+float64(i))
	}
	got := Analyze(prog, abits, AnalyzeInput{UserScore: 170})
	if got.Chance != ChanceLow {
		t.Errorf("Chance = %v (%s), want Low (rank=%d, remaining=%d)",
			got.Chance, got.Chance.Label(), got.MyRealRank, got.RemainingSpots)
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

func TestAnalyze_NonCompetitorsExcluded(t *testing.T) {
	prog := progWithVolume(10, 0, 0)
	abits := []Abiturient{
		ab(1, 200, contract),                          // contract — out
		ab(2, 195, withStatus("Деактивовано (зарах. на бюджет)")), // gone elsewhere
		ab(3, 190, withStatus("Скасовано")),           // out
		ab(4, 180),                                    // genuine competitor
	}
	got := Analyze(prog, abits, AnalyzeInput{UserScore: 170})
	if got.CompetitorsTotal != 1 {
		t.Errorf("CompetitorsTotal = %d, want 1", got.CompetitorsTotal)
	}
}
