package osvita

import (
	"errors"
	"strings"
	"testing"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/PuerkitoBio/goquery"
)

func TestCollectVolume_2026Inline(t *testing.T) {
	// osvita 2026 renders volumes as inline "Label: <b>value</b><br>" pairs
	// mixed with a leftover stats <table>. collectVolume must pick up both.
	html := `<html><body>
		<div class="obsjagy">
			Ліцензований обсяг прийому: <b>100</b><br>
			Максимальне держзамовлення: <b>78</b><br>
			Максимальне держзамовлення, квота 1: <b>1</b><br>
			Максимальне держзамовлення, квота 2: <b>8</b><br>
			Освітня програма: <b>Інженерія ПЗ</b><br>
		</div>
		<table><tr><td>Мінімальний бал ЗНО серед зарахованих на бюджет</td><td>134.00</td></tr></table>
	</body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	vol := map[string]string{}
	collectVolume(doc, vol)

	if vol["Максимальне держзамовлення"] != "78" {
		t.Errorf("budget = %q, want 78", vol["Максимальне держзамовлення"])
	}
	if vol["Максимальне держзамовлення, квота 1"] != "1" {
		t.Errorf("quota1 = %q, want 1", vol["Максимальне держзамовлення, квота 1"])
	}
	if vol["Максимальне держзамовлення, квота 2"] != "8" {
		t.Errorf("quota2 = %q, want 8", vol["Максимальне держзамовлення, квота 2"])
	}
	if vol["Мінімальний бал ЗНО серед зарахованих на бюджет"] != "134.00" {
		t.Errorf("table stat not collected: %q", vol["Мінімальний бал ЗНО серед зарахованих на бюджет"])
	}
	// Non-numeric bold prose (program name) must NOT leak into Volume.
	if _, ok := vol["Освітня програма"]; ok {
		t.Error("non-numeric bold prose leaked into Volume")
	}

	// End-to-end through the public accessors, including the quota-collision guard.
	prog := &abit.Program{Volume: vol}
	if prog.BudgetVolume() != 78 || !prog.BudgetVolumeIsCeiling() {
		t.Errorf("BudgetVolume=%d ceiling=%v, want 78/true", prog.BudgetVolume(), prog.BudgetVolumeIsCeiling())
	}
	if prog.Quota1Volume() != 1 || prog.Quota2Volume() != 8 {
		t.Errorf("quotas = %d/%d, want 1/8", prog.Quota1Volume(), prog.Quota2Volume())
	}
}

func TestParseProgramURL(t *testing.T) {
	tests := []struct {
		name           string
		in             string
		sid, uid, year string
		wantErr        bool
	}{
		{
			name: "canonical",
			in:   "https://vstup.osvita.ua/y2025/r14/282/1471029/",
			sid:  "1471029", uid: "282", year: "2025",
		},
		{
			name: "without trailing slash",
			in:   "https://vstup.osvita.ua/y2024/r05/100/9999",
			sid:  "9999", uid: "100", year: "2024",
		},
		{
			name:    "too few segments",
			in:      "https://vstup.osvita.ua/y2025/",
			wantErr: true,
		},
		{
			name:    "missing year prefix",
			in:      "https://vstup.osvita.ua/2025/r14/282/1471029/",
			wantErr: true,
		},
		{
			name:    "foreign host with valid path (SSRF)",
			in:      "http://169.254.169.254/y2025/r14/282/1471029/",
			wantErr: true,
		},
		{
			name:    "internal host over http (SSRF)",
			in:      "http://localhost:8080/y2025/r14/282/1471029/",
			wantErr: true,
		},
		{
			name:    "right host but http scheme",
			in:      "http://vstup.osvita.ua/y2025/r14/282/1471029/",
			wantErr: true,
		},
		{
			name:    "lookalike subdomain",
			in:      "https://vstup.osvita.ua.evil.com/y2025/r14/282/1471029/",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sid, uid, year, err := parseProgramURL(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got sid=%q uid=%q year=%q", sid, uid, year)
				}
				if !errors.Is(err, abit.ErrInvalidURL) {
					t.Fatalf("expected ErrInvalidURL, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sid != tt.sid || uid != tt.uid || year != tt.year {
				t.Fatalf("got (%q, %q, %q), want (%q, %q, %q)",
					sid, uid, year, tt.sid, tt.uid, tt.year)
			}
		})
	}
}

func TestExtractJSExpr(t *testing.T) {
	js := `
		var statuses = {'1':'Зареєстровано','6':'Рекомендовано'};
		var rec_types = ['a','b','c'];
		var nested = {'a':{'b':'c'}, 'd':'e'};
	`
	if got := extractJSExpr(js, "statuses"); got != `{'1':'Зареєстровано','6':'Рекомендовано'}` {
		t.Errorf("statuses: %q", got)
	}
	if got := extractJSExpr(js, "rec_types"); got != `['a','b','c']` {
		t.Errorf("rec_types: %q", got)
	}
	if got := extractJSExpr(js, "nested"); got != `{'a':{'b':'c'}, 'd':'e'}` {
		t.Errorf("nested: %q", got)
	}
	if got := extractJSExpr(js, "missing"); got != "" {
		t.Errorf("missing: %q", got)
	}
}

func TestParseJSStringMap(t *testing.T) {
	js := `var statuses = {'1':'Зареєстровано','6':'Рекомендовано'};`
	got := parseJSStringMap(js, "statuses")
	if got["1"] != "Зареєстровано" || got["6"] != "Рекомендовано" {
		t.Fatalf("got %v", got)
	}
}
