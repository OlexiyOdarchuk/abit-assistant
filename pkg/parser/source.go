// Package parser defines the contract every data source must satisfy
// to feed the abit pipeline. Concrete implementations live in subpackages
// (osvita, abitpoisk, edbo).
package parser

import (
	"context"

	"github.com/OlexiyOdarchuk/abit-assistant/pkg/abit"
)

// Source fetches data about a single competitive offer.
//
// Implementations are expected to be safe for concurrent use by multiple
// goroutines.
type Source interface {
	// Parse returns program data identified by programURL. The URL format
	// is source-specific; implementations should return abit.ErrInvalidURL
	// when the URL doesn't match their pattern.
	Parse(ctx context.Context, programURL string) (*abit.Program, error)

	// ID returns a short, stable identifier for the source ("osvita",
	// "edbo-2025", ...). Used for logging and diagnostics.
	ID() string
}
