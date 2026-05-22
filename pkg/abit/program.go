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
	return matchVolume(p.Volume, budgetVolumeKeys)
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
	for _, cand := range candidates {
		for k, v := range m {
			if strings.Contains(k, cand) {
				n, err := strconv.Atoi(strings.TrimSpace(v))
				if err == nil {
					return n
				}
			}
		}
	}
	return 0
}
