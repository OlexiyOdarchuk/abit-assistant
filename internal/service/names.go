package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// isMaskedName reports whether the name was privacy-masked by upstream.
// Only the explicit "###" pattern counts — a legitimately short name
// (single surname) is not masked and should reach abit-poisk.
func isMaskedName(name string) bool {
	return strings.Contains(name, "###")
}

// maskName turns an applicant's full name into a short, stable, non-reversible
// tag for logs. Applicant names are third-party PII we must not write to logs
// verbatim; a truncated SHA-256 still lets an operator correlate log lines for
// the same person without exposing who they are.
func maskName(name string) string {
	sum := sha256.Sum256([]byte(name))
	return "name#" + hex.EncodeToString(sum[:])[:10]
}
