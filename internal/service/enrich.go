package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"

	"github.com/OlexiyOdarchuk/abit-assistant/pkg/abit"
)

// EnrichedAbiturient pairs an Abiturient with the other applications the
// same person has submitted, as reported by abit-poisk.
type EnrichedAbiturient struct {
	abit.Abiturient
	// OtherApplications lists this applicant's other submissions across
	// all universities (excluding the current program). nil for masked
	// names or when the lookup failed (see EnrichError).
	OtherApplications []abit.ApplicantEntry `json:"other_applications,omitempty"`
	// EnrichError carries the message of any lookup failure. Empty on
	// success or when the name was masked.
	EnrichError string `json:"enrich_error,omitempty"`
}

// EnrichService runs a fan-out of abit-poisk lookups for a slice of
// applicants, gathered through the cached ApplicantService.
type EnrichService struct {
	applicants *ApplicantService
	workers    int
	log        *slog.Logger
}

// NewEnrichService wires the enrichment. workers caps the concurrency
// against abit-poisk; abit-poisk is rate-sensitive, so keep it modest
// (4..8 is a reasonable default).
func NewEnrichService(applicants *ApplicantService, workers int) *EnrichService {
	if workers <= 0 {
		workers = 4
	}
	return &EnrichService{
		applicants: applicants,
		workers:    workers,
		log:        slog.Default().With("service", "enrich"),
	}
}

// WithLogger overrides the default logger.
func (s *EnrichService) WithLogger(l *slog.Logger) *EnrichService {
	s.log = l.With("service", "enrich")
	return s
}

// Enrich returns one EnrichedAbiturient per input, in the same order.
// Lookups happen concurrently with at most s.workers in flight. Names
// masked by upstream (e.g. "Іва###") are passed through without lookup.
// Per-applicant errors are captured in EnrichError; ctx cancellation
// short-circuits remaining lookups and they come back with EnrichError
// set to the context error.
func (s *EnrichService) Enrich(ctx context.Context, in []abit.Abiturient) []EnrichedAbiturient {
	out := make([]EnrichedAbiturient, len(in))
	sem := make(chan struct{}, s.workers)
	var wg sync.WaitGroup

	for i, ab := range in {
		out[i].Abiturient = ab
		if isMaskedName(ab.Name) {
			continue
		}
		select {
		case <-ctx.Done():
			out[i].EnrichError = ctx.Err().Error()
			continue
		case sem <- struct{}{}:
		}

		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			defer func() { <-sem }()

			entries, err := s.applicants.Search(ctx, name)
			if err != nil {
				// abit.ErrNoData is expected — surface it quietly.
				if !errors.Is(err, abit.ErrNoData) {
					s.log.WarnContext(ctx, "enrich lookup failed",
						"err", err, "name", name)
				}
				out[i].EnrichError = err.Error()
				return
			}
			out[i].OtherApplications = entries
		}(i, ab.Name)
	}
	wg.Wait()
	return out
}

// isMaskedName reports whether the name has been privacy-masked by
// upstream (osvita.ua renders short names as "Іва###" for applicants
// who opted out of public listing).
func isMaskedName(name string) bool {
	return strings.Contains(name, "###") || len(strings.Fields(name)) < 2
}
