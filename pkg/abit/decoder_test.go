package abit

import (
	"slices"
	"strings"
	"testing"
)

// fixture builds a minimal but realistic Program with three subjects
// (Ukrainian, Math, History) and one applicant (id=42). Decoders should
// produce a fully-populated Abiturient from this.
func fixture() *Program {
	return &Program{
		EB:    40,
		OKR:   1,
		K4Max: 0.35,
		RK:    1.0,
		Statuses: map[string]string{
			"6":  "Допущено",
			"14": "До наказу",
			"16": "Деактивовано (зарах. на бюджет)",
		},
		RecTypes: map[string]string{
			"11": "За квотою 1",
			"-1": "Не реком. за жодним пріор.",
		},
		Subjects: []SubjectMeta{
			{ID: 100, SubjectID: 1, Name: "Українська мова", Coefficient: 0.35},
			{ID: 101, SubjectID: 14, Name: "Математика", Coefficient: 0.40},
			{ID: 102, SubjectID: 6, Name: "Історія України", Coefficient: 0.25},
		},
		NMTs:   []int{100, 101, 102, 0, 0},
		Sub4ar: []int{0, 0, 0, 100, 101},
		RequestSubjects: map[string]ApplicantSubjects{
			"42": {
				"100": {159, 0, 0},
				"101": {143, 0, 0},
				"102": {156, 0, 0},
			},
		},
	}
}

// row builds a 19-element RawRequest with the given fields filled.
func row(id, num, priority, status int, name string, score float64,
	q1, q2, q3 int, gk, sk, pchk, ol, kr float64,
	docs, recType, interview, stateEdu, otherReq int) RawRequest {
	return RawRequest{
		float64(id), float64(num), float64(priority), float64(status),
		name, score,
		float64(q1), float64(q2), float64(q3),
		gk, sk, pchk, ol, kr,
		float64(docs), float64(recType), float64(interview),
		float64(stateEdu), float64(otherReq),
	}
}

func TestDecodeRow_BaseFields(t *testing.T) {
	p := fixture()
	r := row(42, 5, 1, 16, "Іваненко І О", 187.5,
		0, 0, 0, 1, 1, 0, 0, 0, 1, 0, 0, 0, 0)
	ab, err := DecodeRow(p, r)
	if err != nil {
		t.Fatalf("DecodeRow: %v", err)
	}
	if ab.ID != 42 || ab.Num != 5 || ab.Priority != 1 {
		t.Errorf("ID/Num/Priority: %+v", ab)
	}
	if ab.Name != "Іваненко І О" {
		t.Errorf("Name: %q", ab.Name)
	}
	if ab.Score != 187.5 {
		t.Errorf("Score: %v", ab.Score)
	}
	if !ab.Documents {
		t.Errorf("Documents should be true")
	}
	if ab.StateEducation {
		t.Errorf("StateEducation should be false")
	}
	if ab.Status != "Деактивовано (зарах. на бюджет)" {
		t.Errorf("Status: %q", ab.Status)
	}
	if ab.AbitLink == "" {
		t.Errorf("AbitLink should be populated for a non-masked name")
	}
}

func TestDecodeRow_AcceptedStatusSuffix(t *testing.T) {
	p := fixture()
	bud := row(42, 1, 1, 14, "Шевченко Т Г", 175.0,
		0, 0, 0, 1, 1, 0, 0, 0, 1, 0, 0, 1, 0)
	ab, _ := DecodeRow(p, bud)
	if ab.Status != "До наказу (бюджет)" {
		t.Errorf("budget: %q", ab.Status)
	}

	cont := row(42, 1, 1, 14, "Шевченко Т Г", 175.0,
		0, 0, 0, 1, 1, 0, 0, 0, 1, 0, 0, 0, 0)
	ab, _ = DecodeRow(p, cont)
	if ab.Status != "До наказу (контракт)" {
		t.Errorf("contract: %q", ab.Status)
	}
}

func TestDecodeRow_QuotasAndCoefficients(t *testing.T) {
	p := fixture()
	r := row(42, 1, 1, 16, "А А", 180,
		1, 0, 1, // КВ1, -, КВ3
		1.05, 1.0, 0.05, 0.10, 0.0,
		1, 0, 1, 0, 0) // interview=1 → СБ
	ab, _ := DecodeRow(p, r)
	wantQuotas := []string{QuotaKV1, QuotaKV3, QuotaSB}
	if !slices.Equal(ab.Quotas, wantQuotas) {
		t.Errorf("Quotas: got %v, want %v", ab.Quotas, wantQuotas)
	}
	if !slices.Contains(ab.Coefficients, CoefGK) || !slices.Contains(ab.Coefficients, CoefPCHK) {
		t.Errorf("Coefficients: %v", ab.Coefficients)
	}
}

func TestDecodeRow_CalcInputAndK4Max(t *testing.T) {
	p := fixture()
	r := row(42, 1, 1, 16, "А А", 180,
		0, 0, 0, 1, 1, 0, 0, 0, 1, 0, 0, 0, 0)
	ab, _ := DecodeRow(p, r)

	if ab.CalcLink == "" {
		t.Fatal("CalcLink should be non-empty for eb=40, okr=1")
	}
	if !strings.Contains(ab.CalcLink, "subjson=") {
		t.Errorf("CalcLink missing subjson: %s", ab.CalcLink)
	}
	if ab.DetailScores["Українська мова"] != 159 {
		t.Errorf("DetailScores[Українська]: %v", ab.DetailScores)
	}
	if ab.DetailScores["Математика"] != 143 {
		t.Errorf("DetailScores[Математика]: %v", ab.DetailScores)
	}
}

func TestDecodeRow_OtherReqOnlyWhenDifferent(t *testing.T) {
	p := fixture()
	same := row(42, 1, 2, 16, "А А", 180,
		0, 0, 0, 1, 1, 0, 0, 0, 1, 0, 0, 0, 2) // other_req == priority
	ab, _ := DecodeRow(p, same)
	if ab.OtherReq != 0 {
		t.Errorf("OtherReq should be 0 when equal to Priority, got %d", ab.OtherReq)
	}

	diff := row(42, 1, 2, 16, "А А", 180,
		0, 0, 0, 1, 1, 0, 0, 0, 1, 0, 0, 0, 1) // other_req=1, priority=2
	ab, _ = DecodeRow(p, diff)
	if ab.OtherReq != 1 {
		t.Errorf("OtherReq should be 1 when distinct, got %d", ab.OtherReq)
	}
}

func TestDecodeRow_ShortRowGetsPadded(t *testing.T) {
	p := fixture()
	r := RawRequest{42.0, 1.0, 1.0, 6.0, "А А", 150.0} // only 6 elements
	ab, err := DecodeRow(p, r)
	if err != nil {
		t.Fatalf("DecodeRow on short row: %v", err)
	}
	if ab.ID != 42 || ab.Name != "А А" || ab.Status != "Допущено" {
		t.Errorf("short row decoded incorrectly: %+v", ab)
	}
}

func TestDecodeRow_MissingIDIsError(t *testing.T) {
	p := fixture()
	r := RawRequest{0.0, 1.0, 1.0, 6.0, "А А", 150.0}
	if _, err := DecodeRow(p, r); err == nil {
		t.Fatal("expected error for missing ID")
	}
}

func TestDecode_SkipsMalformedRows(t *testing.T) {
	p := fixture()
	good := row(42, 1, 1, 16, "Іваненко І О", 187.5,
		0, 0, 0, 1, 1, 0, 0, 0, 1, 0, 0, 1, 0)
	bad := RawRequest{0.0} // missing ID
	p.Requests = []RawRequest{good, bad, good}

	out := Decode(p)
	if len(out) != 2 {
		t.Errorf("expected 2 decoded (one bad skipped), got %d", len(out))
	}
}

func TestDecode_NilProgram(t *testing.T) {
	if out := Decode(nil); out != nil {
		t.Errorf("expected nil, got %v", out)
	}
}
