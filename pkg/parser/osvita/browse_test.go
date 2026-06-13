package osvita

import (
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func loadListingDoc(t *testing.T) *goquery.Document {
	t.Helper()
	f, err := os.Open("testdata/spec_listing.html")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()
	doc, err := goquery.NewDocumentFromReader(f)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return doc
}

func TestParseFoundCount(t *testing.T) {
	doc := loadListingDoc(t)
	if got := parseFoundCount(doc); got != 370 {
		t.Errorf("parseFoundCount = %d, want 370", got)
	}
}

func TestParseSpecListing(t *testing.T) {
	const base = "https://vstup.osvita.ua"
	doc := loadListingDoc(t)
	progs := parseSpecListing(doc, base)

	if len(progs) != specPageSize {
		t.Fatalf("got %d programs, want %d", len(progs), specPageSize)
	}

	// Every row must yield an absolute, well-formed program URL.
	for i, p := range progs {
		if !strings.HasPrefix(p.URL, base+"/y") {
			t.Errorf("program %d: URL %q is not absolute on base", i, p.URL)
		}
		if !progHrefRe.MatchString(p.URL) {
			t.Errorf("program %d: URL %q fails program-href shape", i, p.URL)
		}
	}

	// First row is a known fixture entry — pin its fields.
	first := progs[0]
	if first.URL != base+"/y2025/r27/318/1465936/" {
		t.Errorf("first URL = %q", first.URL)
	}
	if !strings.Contains(first.Specialty, "Комп'ютерні науки") {
		t.Errorf("first specialty = %q, want it to contain the spec name", first.Specialty)
	}
	if !strings.Contains(first.University, "КРОК") {
		t.Errorf("first university = %q, want it to contain ЗВО name", first.University)
	}
	if !strings.Contains(first.Program, "UI/UX") {
		t.Errorf("first programme = %q, want the освітня програма name", first.Program)
	}
	// The fixture's first row is explicitly "Небюджетна".
	if first.Budget {
		t.Errorf("first program marked budget, fixture says Небюджетна")
	}
}

func TestSpecFilterPath(t *testing.T) {
	tests := []struct {
		name   string
		filter SpecFilter
		offset int
		want   string
	}{
		{
			name:   "zero value defaults to bachelor/pzso/full-time",
			filter: SpecFilter{},
			offset: 0,
			want:   "/spec/1-40-1/0-0-0-0-0-0/",
		},
		{
			name:   "computer science, all regions, page 2",
			filter: SpecFilter{Specialty: 2548},
			offset: 50,
			want:   "/spec/1-40-1/0-0-2548-0-0-50/",
		},
		{
			name:   "kharkiv region filter",
			filter: SpecFilter{Region: 21},
			offset: 0,
			want:   "/spec/1-40-1/21-0-0-0-0-0/",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.path(tt.offset); got != tt.want {
				t.Errorf("path = %q, want %q", got, tt.want)
			}
		})
	}
}
