package service_test

import (
	"context"
	"testing"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/osvita"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
)

type fakeUniBrowser struct {
	dir         []osvita.University
	progs       map[int][]osvita.SpecProgram
	browseCalls int
}

func (f *fakeUniBrowser) FetchUniversities(_ context.Context) ([]osvita.University, error) {
	return f.dir, nil
}

func (f *fakeUniBrowser) BrowsePrograms(_ context.Context, flt osvita.SpecFilter) ([]osvita.SpecProgram, error) {
	f.browseCalls++
	return f.progs[flt.University], nil
}

func TestResolver_Resolve(t *testing.T) {
	br := &fakeUniBrowser{
		dir: []osvita.University{
			{ID: 318, ShortName: "КРОК", FullName: `Університет економіки та права "КРОК"`},
		},
		progs: map[int][]osvita.SpecProgram{
			318: {
				{URL: "https://x/y2025/r27/318/111/", Specialty: "F3 Комп'ютерні науки"},
				{URL: "https://x/y2025/r27/318/222/", Specialty: "D5 Маркетинг"},
				{URL: "https://x/y2025/r27/318/333/", Specialty: "D1 Правознавство"},
			},
		},
	}
	r := service.NewResolver(br)
	ctx := context.Background()

	// University matched by name, specialty matched despite the "F3" prefix.
	if url, ok := r.Resolve(ctx, "КРОК", "Комп'ютерні науки"); !ok || url != "https://x/y2025/r27/318/111/" {
		t.Errorf("Resolve CS = %q, %v", url, ok)
	}
	// Different specialty at the same university.
	if url, ok := r.Resolve(ctx, "КРОК", "Маркетинг"); !ok || url != "https://x/y2025/r27/318/222/" {
		t.Errorf("Resolve Marketing = %q, %v", url, ok)
	}
	// Unknown university → no match.
	if _, ok := r.Resolve(ctx, "Гогвортс", "Зілляваріння"); ok {
		t.Error("unknown university should not resolve")
	}
	// Known university, specialty absent → no match (never guesses).
	if _, ok := r.Resolve(ctx, "КРОК", "Астрофізика"); ok {
		t.Error("absent specialty should not resolve")
	}
	// "Право" must NOT latch onto "Правознавство" (token-subset, not raw
	// substring) — guards against the false-positive the audit found.
	if url, ok := r.Resolve(ctx, "КРОК", "Право"); ok {
		t.Errorf("«Право» wrongly matched %q (should not substring-match Правознавство)", url)
	}

	// The university's program list is browsed once, then cached.
	if br.browseCalls != 1 {
		t.Errorf("browseCalls = %d, want 1 (cached per university)", br.browseCalls)
	}
}
