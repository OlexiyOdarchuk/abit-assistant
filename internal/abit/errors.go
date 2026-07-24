package abit

import "errors"

var (
	// ErrNoData is returned when a source replied successfully but the
	// result set is empty (no applications found, no rows on the page, ...).
	ErrNoData = errors.New("abit: no data")

	// ErrInvalidURL is returned when a URL doesn't match the pattern
	// expected by the source.
	ErrInvalidURL = errors.New("abit: invalid program url")

	// ErrCacheMiss / ErrCacheStale are the cache-lookup sentinels shared by
	// every cache backend (Postgres store, local SQLite). They live in this
	// leaf package so the read services and the desktop build can reference
	// them WITHOUT importing internal/storage — which would drag the whole
	// Postgres driver (pgx) into the desktop binary for two error values.
	ErrCacheMiss  = errors.New("cache miss")
	ErrCacheStale = errors.New("cache stale")
)
