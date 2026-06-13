package osvita

import "testing"

func TestNormalizeName(t *testing.T) {
	tests := []struct{ in, want string }{
		{`Вищий навчальний заклад "Університет економіки та права "КРОК"`, "вищий навчальний заклад університет економіки та права крок"},
		{"  Львівська   політехніка  ", "львівська політехніка"},
		{`«КНУ» ім. Шевченка`, "кну ім шевченка"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := normalizeName(tt.in); got != tt.want {
			t.Errorf("normalizeName(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMatchUniversity(t *testing.T) {
	dir := []University{
		{ID: 318, ShortName: "КРОК", FullName: `Вищий навчальний заклад "Університет економіки та права "КРОК"`},
		{ID: 34, ShortName: "ХАІ", FullName: "Національний аерокосмічний університет ім. М. Є. Жуковського"},
		{ID: 61, ShortName: "КПІ", FullName: "Національний технічний університет України КПІ ім. Ігоря Сікорського"},
	}

	tests := []struct {
		name    string
		query   string
		wantID  int
		wantHit bool
	}{
		{"exact short name", "КРОК", 318, true},
		{"differently-quoted full name", `Університет економіки та права «КРОК»`, 318, true},
		{"short-name token inside query", "ХАІ", 34, true},
		{"no match", "Гогвортс", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, ok := MatchUniversity(dir, tt.query)
			if ok != tt.wantHit {
				t.Fatalf("MatchUniversity(%q) ok = %v, want %v (got id %d)", tt.query, ok, tt.wantHit, u.ID)
			}
			if ok && u.ID != tt.wantID {
				t.Errorf("MatchUniversity(%q) id = %d, want %d", tt.query, u.ID, tt.wantID)
			}
		})
	}
}

func TestMatchUniversity_TokenSubset(t *testing.T) {
	// Mirrors the live case: the /spec/ listing name omits the middle
	// "ім. М. Є. Жуковського" that the directory keeps. A substring match
	// failed here; token-subset must succeed.
	dir := []University{
		{ID: 34, ShortName: "ХАІ", FullName: `Національний аерокосмічний університет ім. М. Є. Жуковського "Харківський авіаційний інститут"`},
		{ID: 7, ShortName: "", FullName: "Київський національний університет імені Тараса Шевченка"},
	}
	if u, ok := MatchUniversity(dir, `Національний аерокосмічний університет "Харківський авіаційний інститут"`); !ok || u.ID != 34 {
		t.Errorf("token-subset match = id %d, ok %v; want 34", u.ID, ok)
	}
	// A single generic word must NOT match (token-subset needs ≥2 words).
	if u, ok := MatchUniversity(dir, "університет"); ok {
		t.Errorf("single generic word matched id %d, want no match", u.ID)
	}
}
