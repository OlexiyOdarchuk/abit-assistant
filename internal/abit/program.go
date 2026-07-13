package abit

import (
	"strconv"
	"strings"
)

// Volume key fragments osvita.ua uses on the program page. Order matters
// — we pick the first match, so put the most specific fragments first.
var budgetVolumeKeys = []string{
	"Максимальний обсяг державного замовлення",
	"Обсяг держзамовлення",
	"Загальний обсяг бюджетних місць",
	"Обсяг бюджетних місць",
}

// BudgetVolume returns the program's licensed budget capacity parsed
// from p.Volume, or 0 if no matching key was scraped.
func (p *Program) BudgetVolume() int {
	if p == nil {
		return 0
	}
	v, _, _ := matchVolumeKey(p.Volume, budgetVolumeKeys)
	return v
}

// BudgetVolumeIsCeiling reports whether the figure BudgetVolume returned came
// from the "Максимальний обсяг державного замовлення" key — the licensing
// CEILING, not the state order actually placed on this offer. Under adaptive
// placement the real budget is frequently a fraction of the ceiling (down to
// zero on some regional offers), so when this is true the seat count — and
// therefore the estimated chance — is an optimistic upper bound. Callers
// surface this so a user isn't handed a confident "you're in" built on the
// maximum-possible rather than the likely-actual number of seats.
func (p *Program) BudgetVolumeIsCeiling() bool {
	if p == nil {
		return false
	}
	_, idx, ok := matchVolumeKey(p.Volume, budgetVolumeKeys)
	return ok && idx == 0
}

// Quota1Volume returns the licensed capacity reserved for Quota 1
// (territorial quota for war-affected regions, etc.).
func (p *Program) Quota1Volume() int {
	if p == nil {
		return 0
	}
	return matchVolume(p.Volume, []string{"Квота 1", "Квота1"})
}

// Quota2Volume returns the licensed capacity reserved for Quota 2.
func (p *Program) Quota2Volume() int {
	if p == nil {
		return 0
	}
	return matchVolume(p.Volume, []string{"Квота 2", "Квота2"})
}

// matchVolume scans m for the first key that contains any candidate
// substring and returns its value parsed as int. Returns 0 if nothing
// matches or the value isn't a valid integer.
func matchVolume(m map[string]string, candidates []string) int {
	v, _, _ := matchVolumeKey(m, candidates)
	return v
}

// matchVolumeKey is matchVolume that also reports WHICH candidate matched (its
// index in candidates) so callers can tell a firm figure from a fallback/
// ceiling one. ok is false when nothing matched a parseable integer.
func matchVolumeKey(m map[string]string, candidates []string) (val, matchedIdx int, ok bool) {
	for i, cand := range candidates {
		for k, v := range m {
			if strings.Contains(k, cand) {
				n, err := strconv.Atoi(strings.TrimSpace(v))
				if err == nil {
					return n, i, true
				}
			}
		}
	}
	return 0, -1, false
}
