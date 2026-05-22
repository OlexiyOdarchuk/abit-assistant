package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/storage"
	"github.com/OlexiyOdarchuk/abit-assistant/pkg/abit"
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
type ApplicantService struct {
	src   ApplicantSearcher
	store *storage.Store
	ttl   time.Duration
	log   *slog.Logger
}

// NewApplicantService wires the service. ttl is the cache freshness
// window — defaults around 24h are reasonable for a season-long
// admission campaign.
func NewApplicantService(src ApplicantSearcher, store *storage.Store, ttl time.Duration) *ApplicantService {
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
	case errors.Is(err, storage.ErrCacheMiss), errors.Is(err, storage.ErrCacheStale):
		// fall through
	default:
		return nil, fmt.Errorf("applicant: cache lookup: %w", err)
	}
	return s.Refresh(ctx, name)
}

// Refresh bypasses the cache and hits the source directly. Successful
// responses are written back to the cache.
func (s *ApplicantService) Refresh(ctx context.Context, name string) ([]abit.ApplicantEntry, error) {
	entries, err := s.src.Search(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("applicant: search: %w", err)
	}
	if err := s.store.PutApplicantCache(ctx, name, entries); err != nil {
		s.log.WarnContext(ctx, "cache write failed", "err", err, "name", name)
	}
	return entries, nil
}
