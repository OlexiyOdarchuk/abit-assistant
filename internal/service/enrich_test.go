package service_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/storage"
)

func newEnrichSvc(t *testing.T, searcher service.ApplicantSearcher) *service.EnrichService {
	t.Helper()
	store, err := storage.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("storage.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	app := service.NewApplicantService(searcher, store, time.Hour)
	return service.NewEnrichService(app, 4)
}

func TestEnrich_FillsOtherApplications(t *testing.T) {
	searcher := &fakeSearcher{search: func(_ context.Context, name string) ([]abit.ApplicantEntry, error) {
		return []abit.ApplicantEntry{{Degree: "Б", FullName: name, University: "ЛНУ"}}, nil
	}}
	svc := newEnrichSvc(t, searcher)

	in := []abit.Abiturient{
		{ID: 1, Name: "Іваненко І О", Score: 180},
		{ID: 2, Name: "Шевченко Т Г", Score: 175},
	}
	got := svc.Enrich(context.Background(), in)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2", len(got))
	}
	for i, e := range got {
		if e.ID != in[i].ID {
			t.Errorf("order broken at %d", i)
		}
		if e.EnrichError != "" {
			t.Errorf("[%d] unexpected error: %s", i, e.EnrichError)
		}
		if len(e.OtherApplications) != 1 {
			t.Errorf("[%d] OtherApplications len = %d", i, len(e.OtherApplications))
		}
	}
}

func TestEnrich_SkipsMaskedNames(t *testing.T) {
	searcher := &fakeSearcher{search: func(_ context.Context, name string) ([]abit.ApplicantEntry, error) {
		return []abit.ApplicantEntry{{FullName: name}}, nil
	}}
	svc := newEnrichSvc(t, searcher)

	in := []abit.Abiturient{
		{ID: 1, Name: "Іва###"},                   // masked → no lookup
		{ID: 2, Name: "Шевченко Т Г", Score: 175}, // real → looked up
		{ID: 3, Name: "Одинак"},                   // single word — now a valid name, looked up
	}
	got := svc.Enrich(context.Background(), in)

	if searcher.calls.Load() != 2 {
		t.Errorf("expected 2 search calls (masked skipped), got %d", searcher.calls.Load())
	}
	if len(got[0].OtherApplications) != 0 || got[0].EnrichError != "" {
		t.Errorf("[0] masked, should be untouched: %+v", got[0])
	}
	if len(got[1].OtherApplications) != 1 {
		t.Errorf("[1] should have entry: %+v", got[1])
	}
	if len(got[2].OtherApplications) != 1 {
		t.Errorf("[2] single-word real name should be looked up: %+v", got[2])
	}
}

func TestEnrich_CapturesLookupErrors(t *testing.T) {
	wantErr := errors.New("upstream 502")
	searcher := &fakeSearcher{search: func(_ context.Context, _ string) ([]abit.ApplicantEntry, error) {
		return nil, wantErr
	}}
	svc := newEnrichSvc(t, searcher)

	in := []abit.Abiturient{{ID: 1, Name: "Іваненко І О"}}
	got := svc.Enrich(context.Background(), in)

	if got[0].EnrichError == "" || !strings.Contains(got[0].EnrichError, "upstream") {
		t.Errorf("expected wrapped error, got %q", got[0].EnrichError)
	}
	if got[0].OtherApplications != nil {
		t.Errorf("OtherApplications should be nil on error")
	}
}

func TestEnrich_PreservesOrderUnderConcurrency(t *testing.T) {
	var seq atomic.Int64
	searcher := &fakeSearcher{search: func(_ context.Context, name string) ([]abit.ApplicantEntry, error) {
		// stagger so callers complete out of order
		time.Sleep(time.Duration(seq.Add(1)%5) * time.Millisecond)
		return []abit.ApplicantEntry{{FullName: name}}, nil
	}}
	svc := newEnrichSvc(t, searcher)

	in := make([]abit.Abiturient, 20)
	for i := range in {
		in[i] = abit.Abiturient{ID: i + 1, Name: "Тест Т Т"}
	}
	got := svc.Enrich(context.Background(), in)

	wantIDs := make([]int, len(in))
	for i := range in {
		wantIDs[i] = i + 1
	}
	gotIDs := make([]int, len(got))
	for i, e := range got {
		gotIDs[i] = e.ID
	}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Errorf("order broken: %v", gotIDs)
	}
}

func TestEnrich_ContextCancellation(t *testing.T) {
	searcher := &fakeSearcher{search: func(ctx context.Context, name string) ([]abit.ApplicantEntry, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
			return []abit.ApplicantEntry{{FullName: name}}, nil
		}
	}}
	svc := newEnrichSvc(t, searcher)

	in := make([]abit.Abiturient, 8)
	for i := range in {
		in[i] = abit.Abiturient{ID: i + 1, Name: "Тест Т Т"}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	got := svc.Enrich(ctx, in)

	// Every result should either be untouched (still scheduled past cancel)
	// or carry the context-error string — none should have entries.
	for i, e := range got {
		if len(e.OtherApplications) != 0 {
			t.Errorf("[%d] should not have entries after cancel: %+v", i, e)
		}
	}
}
