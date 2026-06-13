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

// StaticFilters returns the region + industry tables as known constants,
// avoiding a live /spec/ fetch just to populate a picker. Oblast codes (the
// rNN numbering, Київ=27) and the 11 галузі are stable across campaigns;
// FetchFilters remains for verification. The presentation layer should use
// this so the discovery picker loads instantly and offline.
func StaticFilters() Filters {
	return Filters{
		Regions: []FilterOption{
			{Code: 27, Name: "Київ"},
			{Code: 3, Name: "Вінницька область"},
			{Code: 4, Name: "Волинська область"},
			{Code: 5, Name: "Дніпропетровська область"},
			{Code: 6, Name: "Донецька область"},
			{Code: 7, Name: "Житомирська область"},
			{Code: 8, Name: "Закарпатська область"},
			{Code: 9, Name: "Запорізька область"},
			{Code: 10, Name: "Івано-Франківська область"},
			{Code: 11, Name: "Київська область"},
			{Code: 12, Name: "Кіровоградська область"},
			{Code: 13, Name: "Луганська область"},
			{Code: 14, Name: "Львівська область"},
			{Code: 15, Name: "Миколаївська область"},
			{Code: 16, Name: "Одеська область"},
			{Code: 17, Name: "Полтавська область"},
			{Code: 18, Name: "Рівненська область"},
			{Code: 19, Name: "Сумська область"},
			{Code: 20, Name: "Тернопільська область"},
			{Code: 21, Name: "Харківська область"},
			{Code: 22, Name: "Херсонська область"},
			{Code: 23, Name: "Хмельницька область"},
			{Code: 24, Name: "Черкаська область"},
			{Code: 25, Name: "Чернівецька область"},
			{Code: 26, Name: "Чернігівська область"},
		},
		Industries: []FilterOption{
			{Code: 161, Name: "Освіта"},
			{Code: 162, Name: "Культура, мистецтво та гуманітарні науки"},
			{Code: 163, Name: "Соціальні науки, журналістика, інформація та міжнародні відносини"},
			{Code: 164, Name: "Бізнес, адміністрування та право"},
			{Code: 165, Name: "Природничі науки, математика та статистика"},
			{Code: 166, Name: "Інформаційні технології"},
			{Code: 167, Name: "Інженерія, виробництво та будівництво"},
			{Code: 168, Name: "Сільське, лісове, рибне господарство та ветеринарна медицина"},
			{Code: 169, Name: "Охорона здоров’я та соціальне забезпечення"},
			{Code: 170, Name: "Транспорт та послуги"},
			{Code: 171, Name: "Безпека та оборона"},
		},
	}
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
