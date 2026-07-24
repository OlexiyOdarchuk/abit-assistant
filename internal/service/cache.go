package service

import (
	"context"
	"time"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
)

// ProgramCache and ApplicantCache are the narrow storage seams the read-side
// services depend on. The Postgres-backed *storage.Store satisfies both (server
// deploys pass it unchanged), while the desktop build supplies a lightweight
// local (SQLite) implementation — so the same services run against either
// without knowing which.
//
// Both Get* return storage.ErrCacheMiss / storage.ErrCacheStale to signal a
// fall-through to the source; implementations must reuse those sentinels so the
// services' errors.Is checks keep working.
type ProgramCache interface {
	GetProgramCache(ctx context.Context, url string, ttl time.Duration) (*abit.Program, error)
	PutProgramCache(ctx context.Context, url string, prog *abit.Program) error
}

// ApplicantCache is the applicant-lookup half of the storage seam.
type ApplicantCache interface {
	GetApplicantCache(ctx context.Context, name string, ttl time.Duration) ([]abit.ApplicantEntry, error)
	PutApplicantCache(ctx context.Context, name string, entries []abit.ApplicantEntry) error
}
