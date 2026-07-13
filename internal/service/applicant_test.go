package service_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
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
	got, err := svc.Search(ctx, "Бовкун О В")
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
	if _, err := svc.Search(ctx, "Бовкун О В"); err != nil {
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

func TestApplicantService_Refresh_SingleflightDedupes(t *testing.T) {
	store := newStore(t)
	release := make(chan struct{})
	src := &fakeSearcher{search: func(_ context.Context, name string) ([]abit.ApplicantEntry, error) {
		<-release // hold the lookup open so all callers pile onto one flight
		return entriesFixture(name), nil
	}}
	svc := service.NewApplicantService(src, store, time.Hour)

	const callers = 20
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Refresh(context.Background(), "Популярний А Б")
			errs <- err
		}()
	}
	// Give the goroutines time to enter the flight, then let the single
	// in-flight lookup complete.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("caller error: %v", err)
		}
	}
	if got := src.calls.Load(); got != 1 {
		t.Errorf("expected 1 deduped source call for %d concurrent callers, got %d", callers, got)
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
