package abit

import "testing"

func TestGenerateAbitPoiskLink(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"surname + two initials", "Куцелюк Д О", "https://abit-poisk.org.ua/#search-Куцелюк+Д+О"},
		{"surname + dotted initials", "Іваненко І. О.", "https://abit-poisk.org.ua/#search-Іваненко+І+О"},
		{"surname + full name", "Шевченко Тарас Григорович", "https://abit-poisk.org.ua/#search-Шевченко+Т+Г"},
		{"one word — privacy-masked", "Ма###", ""},
		{"empty", "", ""},
		{"whitespace only", "   ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GenerateAbitPoiskLink(tt.in); got != tt.want {
				t.Errorf("GenerateAbitPoiskLink(%q):\ngot:  %s\nwant: %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestGenerateCalcLink(t *testing.T) {
	subj := []CalcInput{
		{SubjectID: 1, Points: 159, K: 0.35},
		{SubjectID: 14, Points: 143, K: 0.4},
	}

	t.Run("applicable: eb=40 okr=1", func(t *testing.T) {
		got := GenerateCalcLink(subj, 151.88, 40, 1)
		if got == "" {
			t.Fatal("expected non-empty link")
		}
		const wantPrefix = "https://osvita.ua/consultations/konkurs-ball/?subjson="
		if got[:len(wantPrefix)] != wantPrefix {
			t.Errorf("unexpected prefix: %s", got)
		}
		if got[len(got)-len("&rbal=151.880"):] != "&rbal=151.880" {
			t.Errorf("rbal suffix wrong: %s", got)
		}
	})

	t.Run("inapplicable: eb!=40", func(t *testing.T) {
		if got := GenerateCalcLink(subj, 100, 0, 1); got != "" {
			t.Errorf("expected empty, got %s", got)
		}
	})

	t.Run("inapplicable: okr=4", func(t *testing.T) {
		if got := GenerateCalcLink(subj, 100, 40, 4); got != "" {
			t.Errorf("expected empty for okr=4, got %s", got)
		}
	})

	t.Run("inapplicable: okr=9", func(t *testing.T) {
		if got := GenerateCalcLink(subj, 100, 40, 9); got != "" {
			t.Errorf("expected empty for okr=9, got %s", got)
		}
	})
}
