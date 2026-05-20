package abit

import "errors"

var (
	// ErrNoData is returned when a source replied successfully but the
	// result set is empty (no applications found, no rows on the page, ...).
	ErrNoData = errors.New("abit: no data")

	// ErrInvalidURL is returned when a URL doesn't match the pattern
	// expected by the source.
	ErrInvalidURL = errors.New("abit: invalid program url")
)
