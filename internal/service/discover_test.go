package service_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/osvita"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
)

type fakeBrowser struct {
	progs        []osvita.SpecProgram
	err          error
	filters      osvita.Filters
	filtersCalls int
}

func (f *fakeBrowser) BrowsePrograms(_ context.Context, _ osvita.SpecFilter) ([]osvita.SpecProgram, error) {
	return f.progs, f.err
}

func (f *fakeBrowser) FetchFilters(_ context.Context) (osvita.Filters, error) {
	f.filtersCalls++
	return f.filters, nil
}

// discoverProg builds a minimal analyzable program: the three required
// subjects (so ComputeRating > 0) and an optional budget volume. budget == 0
// leaves the volume unscraped, which makes Analyze return ChanceUnknown.
func discoverProg(budget int) *abit.Program {
	p := &abit.Program{
		EB: 40, OKR: 1, K4Max: 0.35, RK: 1.0,
		Subjects: []abit.SubjectMeta{
			{ID: 1, Name: "Українська мова", Coefficient: 0.3},
			{ID: 2, Name: "Математика", Coefficient: 0.5},
			{ID: 3, Name: "Історія України", Coefficient: 0.2},
		},
		Volume: map[string]string{},
	}
	if budget > 0 {
		p.Volume["Максимальний обсяг державного замовлення"] = strconv.Itoa(budget)
	}
	return p
}

func discoverInput() service.DiscoverInput {
	return service.DiscoverInput{NMT: map[string]float64{
		"Українська мова": 180,
		"Математика":      190,
		"Історія України": 170,
	}}
}

func TestDiscover_RanksCapsAndDropsFailures(t *testing.T) {
	store := newStore(t)
	src := &fakeSource{parse: func(_ context.Context, url string) (*abit.Program, error) {
		switch url {
		case "u-high":
			return discoverProg(50), nil // budget, no competitors → ChanceHigh
		case "u-unknown":
			return discoverProg(0), nil // no budget volume → ChanceUnknown
		default:
			return nil, errors.New("dead program")
		}
	}}
	ps := service.NewProgramService(src, store, time.Hour)
	browser := &fakeBrowser{progs: []osvita.SpecProgram{
		{URL: "u-unknown"}, {URL: "u-high"}, {URL: "u-fail"},
	}}
	ds := service.NewDiscoverService(browser, ps, 4)

	res, err := ds.WhereCanIGetIn(context.Background(), discoverInput(), 0, osvita.SpecFilter{})
	if err != nil {
		t.Fatalf("WhereCanIGetIn: %v", err)
	}
	if res.Found != 3 {
		t.Errorf("Found = %d, want 3", res.Found)
	}
	// u-fail dropped; the rest ranked best-chance-first.
	if len(res.Matches) != 2 {
		t.Fatalf("got %d matches, want 2 (u-fail should be dropped)", len(res.Matches))
	}
	if res.Matches[0].Program.URL != "u-high" {
		t.Errorf("top match = %q, want u-high (ChanceHigh should outrank ChanceUnknown)", res.Matches[0].Program.URL)
	}
	if res.Matches[0].Analysis.Chance != abit.ChanceHigh {
		t.Errorf("top chance = %v, want ChanceHigh", res.Matches[0].Analysis.Chance)
	}
	if res.Matches[0].Rating <= 0 {
		t.Errorf("rating not computed: %v", res.Matches[0].Rating)
	}
}

func TestDiscover_FiltersCached(t *testing.T) {
	store := newStore(t)
	ps := service.NewProgramService(&fakeSource{parse: func(_ context.Context, _ string) (*abit.Program, error) {
		return discoverProg(10), nil
	}}, store, time.Hour)
	browser := &fakeBrowser{filters: osvita.Filters{
		Regions:    []osvita.FilterOption{{Code: 27, Name: "Київ"}},
		Industries: []osvita.FilterOption{{Code: 166, Name: "Інформаційні технології"}},
	}}
	ds := service.NewDiscoverService(browser, ps, 4)

	for range 3 {
		f, err := ds.Filters(context.Background())
		if err != nil {
			t.Fatalf("Filters: %v", err)
		}
		if len(f.Regions) != 1 || len(f.Industries) != 1 {
			t.Fatalf("unexpected filters: %+v", f)
		}
	}
	if browser.filtersCalls != 1 {
		t.Errorf("FetchFilters called %d times, want 1 (cached)", browser.filtersCalls)
	}
}

func TestDiscover_LimitCapsFetched(t *testing.T) {
	store := newStore(t)
	src := &fakeSource{parse: func(_ context.Context, _ string) (*abit.Program, error) {
		return discoverProg(10), nil
	}}
	ps := service.NewProgramService(src, store, time.Hour)
	browser := &fakeBrowser{progs: []osvita.SpecProgram{
		{URL: "a"}, {URL: "b"}, {URL: "c"}, {URL: "d"},
	}}
	ds := service.NewDiscoverService(browser, ps, 4)

	res, err := ds.WhereCanIGetIn(context.Background(), discoverInput(), 2, osvita.SpecFilter{})
	if err != nil {
		t.Fatalf("WhereCanIGetIn: %v", err)
	}
	if res.Found != 4 {
		t.Errorf("Found = %d, want 4 (full match count reported)", res.Found)
	}
	if len(res.Matches) != 2 {
		t.Errorf("Matches = %d, want 2 (limit cap)", len(res.Matches))
	}
	if src.calls.Load() != 2 {
		t.Errorf("source fetched %d programs, want 2 (only the capped set)", src.calls.Load())
	}
}
