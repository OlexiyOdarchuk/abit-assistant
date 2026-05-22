package service_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/storage"
	"github.com/OlexiyOdarchuk/abit-assistant/pkg/abit"
)

// fakeSource is a parser.Source double driven by a programmable Parse fn.
type fakeSource struct {
	parse func(ctx context.Context, url string) (*abit.Program, error)
	calls atomic.Int64
}

func (f *fakeSource) Parse(ctx context.Context, url string) (*abit.Program, error) {
	f.calls.Add(1)
	return f.parse(ctx, url)
}
func (f *fakeSource) ID() string { return "fake" }

func newStore(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func newFixtureProgram() *abit.Program {
	return &abit.Program{
		UniversityName: "ЛНУ ім. І. Франка",
		ProgramName:    "Маркетинг",
		SpecCode:       "D5",
		EB:             40, OKR: 1, K4Max: 0.35, RK: 1.0,
	}
}

func TestProgramService_Fetch_CacheMissThenHit(t *testing.T) {
	store := newStore(t)
	fixture := newFixtureProgram()
	src := &fakeSource{parse: func(_ context.Context, _ string) (*abit.Program, error) {
		return fixture, nil
	}}
	svc := service.NewProgramService(src, store, time.Hour)

	ctx := context.Background()
	url := "https://example/y2025/r14/282/1471029/"

	got, err := svc.Fetch(ctx, url)
	if err != nil {
		t.Fatalf("first Fetch: %v", err)
	}
	if got.UniversityName != fixture.UniversityName {
		t.Errorf("got %s, want %s", got.UniversityName, fixture.UniversityName)
	}
	if src.calls.Load() != 1 {
		t.Errorf("expected 1 source call, got %d", src.calls.Load())
	}

	// Second call should be a cache hit — source NOT called again.
	got2, err := svc.Fetch(ctx, url)
	if err != nil {
		t.Fatalf("second Fetch: %v", err)
	}
	if got2.UniversityName != fixture.UniversityName {
		t.Errorf("cached: got %s", got2.UniversityName)
	}
	if src.calls.Load() != 1 {
		t.Errorf("expected still 1 source call (cache hit), got %d", src.calls.Load())
	}
}

func TestProgramService_Fetch_CacheStaleRefreshes(t *testing.T) {
	store := newStore(t)
	fixture := newFixtureProgram()
	src := &fakeSource{parse: func(_ context.Context, _ string) (*abit.Program, error) {
		return fixture, nil
	}}
	// Tiny TTL → the entry is immediately stale.
	svc := service.NewProgramService(src, store, time.Nanosecond)

	ctx := context.Background()
	url := "https://example/y2025/r14/282/1471029/"

	if _, err := svc.Fetch(ctx, url); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Fetch(ctx, url); err != nil {
		t.Fatal(err)
	}
	if got := src.calls.Load(); got != 2 {
		t.Errorf("expected 2 source calls (stale), got %d", got)
	}
}

func TestProgramService_Refresh_BypassesCache(t *testing.T) {
	store := newStore(t)
	src := &fakeSource{parse: func(_ context.Context, _ string) (*abit.Program, error) {
		return newFixtureProgram(), nil
	}}
	svc := service.NewProgramService(src, store, time.Hour)

	ctx := context.Background()
	url := "https://example/y2025/r14/282/1471029/"

	if _, err := svc.Fetch(ctx, url); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(ctx, url); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(ctx, url); err != nil {
		t.Fatal(err)
	}
	if got := src.calls.Load(); got != 3 {
		t.Errorf("expected 3 source calls (1 + 2 refresh), got %d", got)
	}
}

func TestProgramService_Fetch_ParseErrorIsPropagated(t *testing.T) {
	store := newStore(t)
	want := errors.New("network down")
	src := &fakeSource{parse: func(_ context.Context, _ string) (*abit.Program, error) {
		return nil, want
	}}
	svc := service.NewProgramService(src, store, time.Hour)

	_, err := svc.Fetch(context.Background(), "https://example/")
	if !errors.Is(err, want) {
		t.Errorf("expected wrapped %v, got %v", want, err)
	}
}

func TestProgramService_FetchDecoded_ReturnsAbiturients(t *testing.T) {
	store := newStore(t)
	prog := newFixtureProgram()
	prog.Statuses = map[string]string{"16": "Деактивовано"}
	prog.Requests = []abit.RawRequest{
		{1.0, 1.0, 1.0, 16.0, "Тест Т Т", 175.0,
			0.0, 0.0, 0.0, 1.0, 1.0, 0.0, 0.0, 0.0, 1.0, 0.0, 0.0, 0.0, 0.0},
	}
	src := &fakeSource{parse: func(_ context.Context, _ string) (*abit.Program, error) {
		return prog, nil
	}}
	svc := service.NewProgramService(src, store, time.Hour)

	out, err := svc.FetchDecoded(context.Background(), "https://example/")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("expected 1 abiturient, got %d", len(out))
	}
	if out[0].Name != "Тест Т Т" || out[0].Status != "Деактивовано" {
		t.Errorf("decoded: %+v", out[0])
	}
}
