package abit

import "slices"

// FundingFilter selects applicants by their funding type (budget vs
// contract). FundingAny is the zero value and matches everyone.
type FundingFilter int

const (
	FundingAny FundingFilter = iota
	FundingBudget
	FundingContract
)

// Filter is a composable predicate over Abiturient slices. Every field
// is optional — the zero-value Filter is a no-op that passes everything
// through. Combine fields freely; they are AND-ed together.
//
// Use the constants from codes.go (QuotaKV1, CoefGK, ...) when populating
// Status/Quota fields to avoid typos.
type Filter struct {
	// StatusInclude, when non-empty, restricts the output to applicants
	// whose Status appears in the list. Compared verbatim — pass the
	// strings that appear in Program.Statuses.
	StatusInclude []string
	// StatusExclude drops applicants whose Status appears in the list.
	StatusExclude []string

	// PriorityMin / PriorityMax restrict by application priority.
	// 0 means "no bound" (priority values from osvita start at 1).
	PriorityMin int
	PriorityMax int

	// QuotaInclude, when non-empty, requires that the applicant holds
	// at least ONE of the listed quotas (QuotaKV1, ..., QuotaSB).
	QuotaInclude []string
	// QuotaExclude drops applicants holding ANY of the listed quotas.
	QuotaExclude []string

	// Documents is a tri-state document-submission filter:
	//   nil   — don't filter
	//   *true — only applicants who submitted originals
	//   *false — only applicants who didn't
	Documents *bool

	// Funding selects budget vs contract; FundingAny disables it.
	Funding FundingFilter

	// ScoreMin / ScoreMax restrict by final competitive score.
	// 0 means "no bound".
	ScoreMin float64
	ScoreMax float64
}

// IsZero reports whether the filter has any active criteria. A zero-value
// filter is a pass-through.
func (f Filter) IsZero() bool {
	return len(f.StatusInclude) == 0 &&
		len(f.StatusExclude) == 0 &&
		f.PriorityMin == 0 && f.PriorityMax == 0 &&
		len(f.QuotaInclude) == 0 && len(f.QuotaExclude) == 0 &&
		f.Documents == nil &&
		f.Funding == FundingAny &&
		f.ScoreMin == 0 && f.ScoreMax == 0
}

// Matches reports whether ab passes every active criterion.
func (f Filter) Matches(ab Abiturient) bool {
	if len(f.StatusInclude) > 0 && !slices.Contains(f.StatusInclude, ab.Status) {
		return false
	}
	if len(f.StatusExclude) > 0 && slices.Contains(f.StatusExclude, ab.Status) {
		return false
	}
	if f.PriorityMin > 0 && ab.Priority < f.PriorityMin {
		return false
	}
	if f.PriorityMax > 0 && ab.Priority > f.PriorityMax {
		return false
	}
	if len(f.QuotaInclude) > 0 && !sharesAny(ab.Quotas, f.QuotaInclude) {
		return false
	}
	if len(f.QuotaExclude) > 0 && sharesAny(ab.Quotas, f.QuotaExclude) {
		return false
	}
	if f.Documents != nil && ab.Documents != *f.Documents {
		return false
	}
	switch f.Funding {
	case FundingBudget:
		if !ab.StateEducation {
			return false
		}
	case FundingContract:
		if ab.StateEducation {
			return false
		}
	}
	if f.ScoreMin > 0 && ab.Score < f.ScoreMin {
		return false
	}
	if f.ScoreMax > 0 && ab.Score > f.ScoreMax {
		return false
	}
	return true
}

// Apply returns a new slice containing every element of in that Matches.
// The original slice is not modified. The result is nil when in is nil
// and an empty (non-nil) slice when everything is filtered out.
func (f Filter) Apply(in []Abiturient) []Abiturient {
	if in == nil {
		return nil
	}
	if f.IsZero() {
		// Return a copy so callers can't mutate the original list.
		return append([]Abiturient(nil), in...)
	}
	out := make([]Abiturient, 0, len(in))
	for _, ab := range in {
		if f.Matches(ab) {
			out = append(out, ab)
		}
	}
	return out
}

// BoolPtr is a tiny helper for populating Filter.Documents inline:
//
//	Filter{Documents: abit.BoolPtr(true)}.Apply(list)
func BoolPtr(v bool) *bool { return &v }

// sharesAny reports whether a and b share at least one element. Both
// slices are tiny in practice (≤ 4 quota codes), so the O(n*m) scan is
// fine.
func sharesAny(a, b []string) bool {
	for _, x := range a {
		if slices.Contains(b, x) {
			return true
		}
	}
	return false
}
