package osvita

// filters.go scrapes the option tables the /spec/ search form offers —
// regions (oblasts) and industries (галузі знань). These are the choices a
// "where can I get in" UI presents; the codes feed straight into SpecFilter.
// The lists are static across a campaign, so callers should fetch once and
// cache.

import (
	"context"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// FilterOption is one selectable filter value: its osvita code and label.
type FilterOption struct {
	Code int
	Name string
}

// GaluzLetters maps osvita's industryId to the official галузь-знань letter
// code that prefixes its specialties (e.g. F3 Комп'ютерні науки → F).
// Determined empirically from /spec/ listings; the 11 osvita industries map
// one-to-one onto letters A–K. Used by presentation layers (bot, web) to
// label the galuz picker.
var GaluzLetters = map[int]string{
	161: "A", // Освіта
	162: "B", // Культура, мистецтво та гуманітарні науки
	163: "C", // Соціальні науки, журналістика, інформація
	164: "D", // Бізнес, адміністрування та право
	165: "E", // Природничі науки, математика та статистика
	166: "F", // Інформаційні технології
	167: "G", // Інженерія, виробництво та будівництво
	168: "H", // Сільське, лісове, рибне господарство та ветеринарія
	169: "I", // Охорона здоров'я та соціальне забезпечення
	170: "J", // Транспорт та послуги
	171: "K", // Безпека та оборона
}

// Filters bundles the option tables scraped from the /spec/ form.
type Filters struct {
	// Regions are the oblasts; Code matches the rNN segment of program URLs
	// and SpecFilter.Region (e.g. Київ=27, Харківська=21).
	Regions []FilterOption
	// Industries are галузі знань; Code feeds SpecFilter.Industry
	// (e.g. Інформаційні технології=166).
	Industries []FilterOption
}

// FetchFilters loads the /spec/ form and extracts its region and industry
// option tables. The specialty list is intentionally not returned — it is
// populated by a cascade AJAX call, not present in the static form.
func (p *Parser) FetchFilters(ctx context.Context) (Filters, error) {
	base, err := p.siteBase()
	if err != nil {
		return Filters{}, err
	}
	doc, err := p.fetchDoc(ctx, base+"/spec/0-0-0/")
	if err != nil {
		return Filters{}, err
	}
	return ParseFilters(doc), nil
}

// ParseFilters extracts the region and industry <select> option tables from
// a /spec/ page. The placeholder option (code 0, "- Регіон -") is dropped.
func ParseFilters(doc *goquery.Document) Filters {
	return Filters{
		Regions:    parseSelectOptions(doc, "region"),
		Industries: parseSelectOptions(doc, "industryId"),
	}
}

func parseSelectOptions(doc *goquery.Document, name string) []FilterOption {
	var out []FilterOption
	doc.Find("select[name=" + name + "]").First().Find("option").Each(func(_ int, opt *goquery.Selection) {
		val, ok := opt.Attr("value")
		if !ok {
			return
		}
		code, err := strconv.Atoi(strings.TrimSpace(val))
		if err != nil || code == 0 { // 0 is the "- placeholder -" / "any"
			return
		}
		name := compactText(opt.Text())
		if name == "" {
			return
		}
		out = append(out, FilterOption{Code: code, Name: name})
	})
	return out
}
