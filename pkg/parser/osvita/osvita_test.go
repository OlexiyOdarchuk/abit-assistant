package osvita

import (
	"errors"
	"testing"

	"github.com/OlexiyOdarchuk/abit-assistant/pkg/abit"
)

func TestParseProgramURL(t *testing.T) {
	tests := []struct {
		name              string
		in                string
		sid, uid, year    string
		wantErr           bool
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
