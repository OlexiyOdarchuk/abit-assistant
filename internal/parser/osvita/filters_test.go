package osvita

import "testing"

func TestParseFilters(t *testing.T) {
	doc := loadListingDoc(t)
	f := ParseFilters(doc)

	// The form lists 25 oblasts (Київ + 24 областей) and 11 галузі —
	// placeholders excluded.
	if len(f.Regions) != 25 {
		t.Errorf("regions = %d, want 25", len(f.Regions))
	}
	if len(f.Industries) != 11 {
		t.Errorf("industries = %d, want 11", len(f.Industries))
	}

	if find(f.Regions, 27) != "Київ" {
		t.Errorf("region 27 = %q, want Київ", find(f.Regions, 27))
	}
	if got := find(f.Industries, 166); got != "Інформаційні технології" {
		t.Errorf("industry 166 = %q, want Інформаційні технології", got)
	}
	// No placeholder leaked through.
	for _, o := range append(f.Regions, f.Industries...) {
		if o.Code == 0 || o.Name == "" {
			t.Errorf("placeholder/blank option leaked: %+v", o)
		}
	}
}

func find(opts []FilterOption, code int) string {
	for _, o := range opts {
		if o.Code == code {
			return o.Name
		}
	}
	return ""
}
