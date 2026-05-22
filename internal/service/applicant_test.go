package service_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
	"github.com/OlexiyOdarchuk/abit-assistant/pkg/abit"
)

type fakeSearcher struct {
	search func(ctx context.Context, name string) ([]abit.ApplicantEntry, error)
	calls  atomic.Int64
}

func (f *fakeSearcher) Search(ctx context.Context, name string) ([]abit.ApplicantEntry, error) {
	f.calls.Add(1)
	return f.search(ctx, name)
}
func (f *fakeSearcher) ID() string { return "fake" }

func entriesFixture(name string) []abit.ApplicantEntry {
	return []abit.ApplicantEntry{
		{Degree: "Б", FullName: name, University: "ЛНУ", Specialty: "I10"},
		{Degree: "Б", FullName: name, University: "Львівська політехніка", Specialty: "I10"},
	}
}

func TestApplicantService_Search_CacheMissThenHit(t *testing.T) {
	store := newStore(t)
	src := &fakeSearcher{search: func(_ context.Context, name string) ([]abit.ApplicantEntry, error) {
		return entriesFixture(name), nil
	}}
	svc := service.NewApplicantService(src, store, time.Hour)

	ctx := context.Background()
	got, err := svc.Search(ctx, "Куцелюк Д О")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("entries len = %d, want 2", len(got))
	}
	if src.calls.Load() != 1 {
		t.Errorf("expected 1 source call, got %d", src.calls.Load())
	}

	// Cache hit — source not called.
	if _, err := svc.Search(ctx, "Куцелюк Д О"); err != nil {
		t.Fatal(err)
	}
	if src.calls.Load() != 1 {
		t.Errorf("expected still 1 source call, got %d", src.calls.Load())
	}
}

func TestApplicantService_Search_NoDataNotCached(t *testing.T) {
	store := newStore(t)
	src := &fakeSearcher{search: func(_ context.Context, _ string) ([]abit.ApplicantEntry, error) {
		return nil, abit.ErrNoData
	}}
	svc := service.NewApplicantService(src, store, time.Hour)

	ctx := context.Background()
	for i := range 3 {
		_, err := svc.Search(ctx, "Невідомий А Б")
		if !errors.Is(err, abit.ErrNoData) {
			t.Fatalf("call %d: expected ErrNoData, got %v", i, err)
		}
	}
	// Negative results aren't cached — each call hits the source again.
	if src.calls.Load() != 3 {
		t.Errorf("expected 3 source calls (no negative caching), got %d", src.calls.Load())
	}
}

func TestApplicantService_Refresh_BypassesCache(t *testing.T) {
	store := newStore(t)
	src := &fakeSearcher{search: func(_ context.Context, name string) ([]abit.ApplicantEntry, error) {
		return entriesFixture(name), nil
	}}
	svc := service.NewApplicantService(src, store, time.Hour)

	ctx := context.Background()
	if _, err := svc.Search(ctx, "Х"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Refresh(ctx, "Х"); err != nil {
		t.Fatal(err)
	}
	if got := src.calls.Load(); got != 2 {
		t.Errorf("expected 2 calls (1 search + 1 refresh), got %d", got)
	}
}
