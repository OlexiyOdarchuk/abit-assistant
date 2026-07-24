package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
)

// ApplicantSearcher is the minimal interface ApplicantService needs from
// any "search applicants by name" backend. The concrete impl today is
// abitpoisk.Client; keeping it abstract lets us swap in alternatives or
// fakes without touching the service.
type ApplicantSearcher interface {
	Search(ctx context.Context, name string) ([]abit.ApplicantEntry, error)
	ID() string
}

// ApplicantService caches abit-poisk lookups in the local store and
// surfaces them to the rest of the application.
//
// Threading: safe for concurrent use. A singleflight group dedupes
// concurrent Refresh requests for the same name, so a popular applicant
// looked up by many users (enrich/simulate fan-outs, viewing history)
// doesn't fire N parallel POSTs at rate-sensitive abit-poisk.
type ApplicantService struct {
	src    ApplicantSearcher
	store  ApplicantCache
	ttl    time.Duration
	log    *slog.Logger
	flight singleflight.Group
}

// NewApplicantService wires the service. ttl is the cache freshness
// window — defaults around 24h are reasonable for a season-long
// admission campaign.
func NewApplicantService(src ApplicantSearcher, store ApplicantCache, ttl time.Duration) *ApplicantService {
	return &ApplicantService{
		src:   src,
		store: store,
		ttl:   ttl,
		log:   slog.Default().With("service", "applicant", "source", src.ID()),
	}
}

// WithLogger overrides the default logger.
func (s *ApplicantService) WithLogger(l *slog.Logger) *ApplicantService {
	s.log = l.With("service", "applicant", "source", s.src.ID())
	return s
}

// Search returns the applicant's other applications, using the cache when
// fresh. abit.ErrNoData is returned for genuinely-unknown applicants;
// negative results are NOT cached (we'd rather pay an upstream call than
// serve a stale "not found" once the applicant publishes their data).
func (s *ApplicantService) Search(ctx context.Context, name string) ([]abit.ApplicantEntry, error) {
	entries, err := s.store.GetApplicantCache(ctx, name, s.ttl)
	switch {
	case err == nil:
		return entries, nil
	case errors.Is(err, abit.ErrCacheMiss), errors.Is(err, abit.ErrCacheStale):
		// fall through
	default:
		return nil, fmt.Errorf("applicant: cache lookup: %w", err)
	}
	return s.Refresh(ctx, name)
}

// searchTimeout caps a single shared abit-poisk lookup so a detached
// in-flight search can't run forever if every caller walks away.
const searchTimeout = 60 * time.Second

// Refresh bypasses the cache and hits the source directly. Successful
// responses are written back to the cache. Concurrent callers for the same
// name share a single in-flight lookup via singleflight — abit-poisk is
// rate-sensitive, so a popular name isn't searched N times in parallel.
//
// Each caller waits on its OWN context; the shared work runs on a context
// detached from any single caller (so one cancellation can't poison the
// others) but bounded by searchTimeout.
func (s *ApplicantService) Refresh(ctx context.Context, name string) ([]abit.ApplicantEntry, error) {
	ch := s.flight.DoChan(name, func() (any, error) {
		workCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), searchTimeout)
		defer cancel()
		entries, err := s.src.Search(workCtx, name)
		if err != nil {
			return nil, fmt.Errorf("applicant: search: %w", err)
		}
		if err := s.store.PutApplicantCache(workCtx, name, entries); err != nil {
			s.log.WarnContext(workCtx, "cache write failed", "err", err, "name", maskName(name))
		}
		return entries, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err
		}
		return res.Val.([]abit.ApplicantEntry), nil
	}
}
