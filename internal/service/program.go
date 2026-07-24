// Package service composes the domain (internal/abit), the parsers
// (internal/parser/...) and the storage layer (internal/storage) into
// application-level use cases. Presentation entrypoints (bot, server,
// desktop, CLI) call service methods — they never touch parser or
// storage directly.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser"
)

// ProgramService is the read-side use case for "give me the applicants
// of program X". It transparently caches Program snapshots from a
// parser.Source through the storage layer.
//
// Threading: safe for concurrent use. A singleflight group dedupes
// concurrent Refresh requests for the same URL, so a viral list-share
// doesn't fire N parallel osvita scrapes — first caller fetches, the
// rest wait for the same result.
type ProgramService struct {
	src    parser.Source
	store  ProgramCache
	ttl    time.Duration
	log    *slog.Logger
	flight singleflight.Group
}

// NewProgramService wires a service with the given source and store.
// ttl is the cache freshness window — cached programs older than this
// trigger a re-fetch on the next Fetch call.
func NewProgramService(src parser.Source, store ProgramCache, ttl time.Duration) *ProgramService {
	return &ProgramService{
		src:   src,
		store: store,
		ttl:   ttl,
		log:   slog.Default().With("service", "program", "source", src.ID()),
	}
}

// WithLogger overrides the default slog logger used for cache-write warnings.
func (s *ProgramService) WithLogger(l *slog.Logger) *ProgramService {
	s.log = l.With("service", "program", "source", s.src.ID())
	return s
}

// Fetch returns the program data for url, using the cache when fresh.
// On cache miss or staleness it falls through to the source and updates
// the cache. Cache-write errors are logged but never surfaced — they
// shouldn't fail a user request.
func (s *ProgramService) Fetch(ctx context.Context, url string) (*abit.Program, error) {
	prog, err := s.store.GetProgramCache(ctx, url, s.ttl)
	switch {
	case err == nil:
		return prog, nil
	case errors.Is(err, abit.ErrCacheMiss), errors.Is(err, abit.ErrCacheStale):
		// fall through to refresh
	default:
		return nil, fmt.Errorf("program: cache lookup: %w", err)
	}
	return s.Refresh(ctx, url)
}

// parseTimeout caps a single shared parse so a detached in-flight fetch
// can't run forever if every caller walks away before it finishes.
const parseTimeout = 90 * time.Second

// Refresh bypasses the cache and always fetches a fresh copy from the
// source, writing it back into the cache. Useful for admin "force
// refresh" commands. Concurrent callers for the same URL share a single
// in-flight parse via singleflight — saves bandwidth and avoids
// rate-limit at the source.
//
// Each caller waits on its OWN context: if caller A cancels, only A
// returns early — the shared parse keeps running for B, C… The work
// itself runs on a context detached from any single caller (so A's
// cancellation can't poison the others) but bounded by parseTimeout.
func (s *ProgramService) Refresh(ctx context.Context, url string) (*abit.Program, error) {
	ch := s.flight.DoChan(url, func() (any, error) {
		workCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), parseTimeout)
		defer cancel()
		prog, err := s.src.Parse(workCtx, url)
		if err != nil {
			return nil, fmt.Errorf("program: parse: %w", err)
		}
		if err := s.store.PutProgramCache(workCtx, url, prog); err != nil {
			s.log.WarnContext(workCtx, "cache write failed", "err", err, "url", url)
		}
		return prog, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		if res.Err != nil {
			return nil, res.Err
		}
		return res.Val.(*abit.Program), nil
	}
}

// FetchDecoded returns the program already decoded into []Abiturient.
// Equivalent to abit.Decode(prog) on the result of Fetch.
func (s *ProgramService) FetchDecoded(ctx context.Context, url string) ([]abit.Abiturient, error) {
	prog, err := s.Fetch(ctx, url)
	if err != nil {
		return nil, err
	}
	return abit.Decode(prog), nil
}
