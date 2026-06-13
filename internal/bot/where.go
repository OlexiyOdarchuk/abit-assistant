package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	tele "gopkg.in/telebot.v3"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/bot/callback"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/osvita"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
)

// "Where can I get in" flow:
//
//	galuz → multi-select regions → ranked list of budget programs.
//
// The list is grouped by reach/match/safety, can be grown ("show more"),
// filtered to "only programs I'd pass", and the safe ones saved to /lists in
// one tap. Each result opens the full analysis.

const (
	fsmStateDiscoverRegions = "discover.regions"
	fsmStateDiscover        = "discover.viewing"

	// regions step
	fsmKeyDiscGaluz    = "d_galuz" // int galuz code
	fsmKeyDiscSel      = "d_sel"   // JSON []int selected region codes
	fsmKeyDiscGalName  = "d_gname" // galuz label (header)
	fsmKeyDiscContract = "d_contract"
	// results step
	fsmKeyDiscRegNames = "d_rnames"  // joined region labels (header)
	fsmKeyDiscBrowsed  = "d_browsed" // JSON []discProg (merged browse, capped)
	fsmKeyDiscAnalyzed = "d_anz"     // how many of browsed are analyzed
	fsmKeyDiscFound    = "d_found"   // total merged matches
	fsmKeyDiscRows     = "d_rows"    // JSON []discRow (analyzed+sorted)
	fsmKeyDiscPage     = "d_page"
	fsmKeyDiscOnlyPass = "d_pass" // bool: hide reach-tier
	fsmKeyDiscSpec     = "d_spec" // selected specialty label ("" = all)

	discoverBatch    = 10 // programs analyzed per browse/"show more"
	discoverStoreMax = 60 // cap on browsed programs kept in FSM
	discoverPageSize = 6
	discoverSaveMax  = 15 // cap on one-tap "save safe to lists"
)

// discProg is a compact browsed program kept in FSM for "show more".
type discProg struct {
	URL  string `json:"u"`
	Uni  string `json:"n"`
	Prog string `json:"p"`
	Spec string `json:"s"` // specialty label, for the in-results specialty filter
}

// discRow is an analyzed, render-ready result row.
type discRow struct {
	URL   string `json:"u"`
	Name  string `json:"n"`
	Label string `json:"l"`
	Spec  string `json:"s"` // specialty label, for the specialty filter
	Tier  int    `json:"t"`
}

// --- entry + pickers ------------------------------------------------------

func (b *Bot) handleDiscover(c tele.Context) error {
	b.clearTransientFSM(c)
	if err := requirePrivateChat(c); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()
	if _, ok := b.discoverInput(ctx, senderID(c)); !ok {
		return errors.New("спочатку заповни /profile — без балів НМТ я не зможу порахувати шанси")
	}
	filters, err := b.discoverSvc.Filters(ctx)
	if err != nil {
		return fmt.Errorf("не вдалося завантажити галузі: %w", err)
	}
	text := "🧭 *Куди я вступлю*\n\nОбери галузь знань — далі вкажеш області, і я знайду бюджетні бакалаврські програми та покажу, куди ти проходиш."
	return renderOrEdit(c, text, tele.ModeMarkdown, galuzKeyboard(filters))
}

// handleDiscoverGaluz opens the region multi-select for the chosen галузь
// with an empty selection.
func (b *Bot) handleDiscoverGaluz(c tele.Context) error {
	galuz, ok := callback.From(c).Int(0)
	if !ok {
		return errors.New("втрачено галузь — почни з /menu")
	}
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()
	filters, err := b.discoverSvc.Filters(ctx)
	if err != nil {
		return fmt.Errorf("не вдалося завантажити регіони: %w", err)
	}
	data := map[string]any{
		fsmKeyDiscGaluz:    galuz,
		fsmKeyDiscGalName:  optionName(filters.Industries, galuz, "Усі галузі"),
		fsmKeyDiscSel:      "[]",
		fsmKeyDiscContract: false,
	}
	if err := b.fsm.Set(context.Background(), senderID(c), fsmStateDiscoverRegions, data); err != nil {
		b.log.Warn("discover regions fsm set", "err", err)
	}
	return b.renderRegionPicker(c)
}

// handleDiscoverBudgetTog flips the budget-only / +contract switch in the
// region picker.
func (b *Bot) handleDiscoverBudgetTog(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), listsTimeout)
	state, err := b.fsm.Get(ctx, senderID(c))
	cancel()
	if err != nil || state.Name != fsmStateDiscoverRegions {
		return errors.New("сесія вибору завершилась — почни з /menu")
	}
	cur, _ := state.Data[fsmKeyDiscContract].(bool)
	state.Data[fsmKeyDiscContract] = !cur
	if err := b.fsm.Set(context.Background(), senderID(c), fsmStateDiscoverRegions, state.Data); err != nil {
		b.log.Warn("discover budget set", "err", err)
	}
	return b.renderRegionPicker(c)
}

// handleDiscoverRegionTog toggles a region in the multi-select.
func (b *Bot) handleDiscoverRegionTog(c tele.Context) error {
	region, ok := callback.From(c).Int(0)
	if !ok {
		return errors.New("втрачено регіон")
	}
	ctx, cancel := context.WithTimeout(context.Background(), listsTimeout)
	state, err := b.fsm.Get(ctx, senderID(c))
	cancel()
	if err != nil || state.Name != fsmStateDiscoverRegions {
		return errors.New("сесія вибору завершилась — почни з /menu")
	}
	sel := decodeIntSlice(state.Data[fsmKeyDiscSel])
	if i := slices.Index(sel, region); i >= 0 {
		sel = slices.Delete(sel, i, i+1)
	} else {
		sel = append(sel, region)
	}
	raw, _ := json.Marshal(sel)
	state.Data[fsmKeyDiscSel] = string(raw)
	if err := b.fsm.Set(context.Background(), senderID(c), fsmStateDiscoverRegions, state.Data); err != nil {
		b.log.Warn("discover sel set", "err", err)
	}
	return b.renderRegionPicker(c)
}

func (b *Bot) renderRegionPicker(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()
	state, err := b.fsm.Get(ctx, senderID(c))
	if err != nil || state.Name != fsmStateDiscoverRegions {
		return errors.New("сесія вибору завершилась — почни з /menu")
	}
	filters, err := b.discoverSvc.Filters(ctx)
	if err != nil {
		return fmt.Errorf("не вдалося завантажити регіони: %w", err)
	}
	galName, _ := state.Data[fsmKeyDiscGalName].(string)
	sel := decodeIntSlice(state.Data[fsmKeyDiscSel])
	contract, _ := state.Data[fsmKeyDiscContract].(bool)

	var sb strings.Builder
	fmt.Fprintf(&sb, "🧭 *Куди я вступлю* · %s\n\n", mdEscape(galName))
	if len(sel) == 0 {
		sb.WriteString("Познач області (можна кілька) або тисни «🔎 Шукати» — шукатиму по всій Україні.")
	} else {
		names := make([]string, 0, len(sel))
		for _, code := range sel {
			names = append(names, optionName(filters.Regions, code, "?"))
		}
		fmt.Fprintf(&sb, "Обрано: *%s*\n\nДодай ще або тисни «🔎 Шукати».", mdEscape(strings.Join(names, ", ")))
	}
	return renderOrEdit(c, sb.String(), tele.ModeMarkdown, regionPickerKeyboard(filters, sel, contract))
}

// handleDiscoverRun launches the search for the regions chosen in the
// multi-select.
func (b *Bot) handleDiscoverRun(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()
	state, err := b.fsm.Get(ctx, senderID(c))
	if err != nil || state.Name != fsmStateDiscoverRegions {
		return errors.New("сесія вибору завершилась — почни з /menu")
	}
	contract, _ := state.Data[fsmKeyDiscContract].(bool)
	return b.runDiscovery(ctx, c,
		anyToInt(state.Data[fsmKeyDiscGaluz]), decodeIntSlice(state.Data[fsmKeyDiscSel]), contract)
}

// handleDiscoverBack re-runs the user's last discovery — used by the "back
// to results" button on a program opened from the list (its results FSM
// state was overwritten when the program screen opened). Cached fetches make
// the re-run fast.
func (b *Bot) handleDiscoverBack(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()
	settings, _ := b.store.GetUserSettings(ctx, senderID(c))
	if settings.LastDiscoverGaluz == 0 {
		return errors.New("немає попереднього пошуку — почни з 🧭 у /menu")
	}
	return b.runDiscovery(ctx, c, settings.LastDiscoverGaluz, settings.LastDiscoverRegions, settings.LastDiscoverContract)
}

// runDiscovery browses galuz+regions, analyzes the first batch, stores the
// result in FSM and renders page 0. It also persists the filter so a program
// later opened from these results can re-run them ("back to results").
func (b *Bot) runDiscovery(ctx context.Context, c tele.Context, galuz int, sel []int, contract bool) error {
	uid := senderID(c)
	in, ok := b.discoverInput(ctx, uid)
	if !ok {
		return errors.New("спочатку заповни /profile")
	}
	filters, _ := b.discoverSvc.Filters(ctx)
	galName := optionName(filters.Industries, galuz, "Усі галузі")
	regNames := "Вся Україна"
	if len(sel) > 0 {
		names := make([]string, 0, len(sel))
		for _, code := range sel {
			names = append(names, optionName(filters.Regions, code, "?"))
		}
		regNames = strings.Join(names, ", ")
	}

	_ = c.Notify(tele.Typing)
	browsed, err := b.discoverSvc.Browse(ctx, discoverFilters(galuz, sel, contract))
	if err != nil {
		return fmt.Errorf("пошук не вдався: %w", err)
	}
	if len(browsed) == 0 {
		return errors.New("за цим фільтром нічого не знайшов — спробуй іншу галузь чи область")
	}
	found := len(browsed)
	if len(browsed) > discoverStoreMax {
		browsed = browsed[:discoverStoreMax]
	}

	compact := make([]discProg, len(browsed))
	for i, p := range browsed {
		compact[i] = discProg{URL: p.URL, Uni: p.University, Prog: p.Program, Spec: p.Specialty}
	}

	data := map[string]any{
		fsmKeyDiscGaluz:    galuz,
		fsmKeyDiscGalName:  galName,
		fsmKeyDiscRegNames: regNames,
		fsmKeyDiscContract: contract,
		fsmKeyDiscFound:    found,
		fsmKeyDiscPage:     0,
		fsmKeyDiscOnlyPass: false,
	}
	storeBrowsed(data, compact)
	if err := b.analyzeAndStore(ctx, data, in, min(discoverBatch, len(compact))); err != nil {
		return err
	}
	if err := b.fsm.Set(context.Background(), uid, fsmStateDiscover, data); err != nil {
		b.log.Warn("discover fsm set", "err", err)
	}
	b.saveLastDiscover(ctx, uid, galuz, sel, contract)
	return b.renderDiscoverPage(c, 0)
}

// saveLastDiscover persists the filter into user settings (read-modify-write
// so other settings survive).
func (b *Bot) saveLastDiscover(ctx context.Context, uid int64, galuz int, sel []int, contract bool) {
	settings, err := b.store.GetUserSettings(ctx, uid)
	if err != nil {
		return
	}
	settings.LastDiscoverGaluz = galuz
	settings.LastDiscoverRegions = sel
	settings.LastDiscoverContract = contract
	if err := b.store.SetUserSettings(ctx, uid, settings); err != nil {
		b.log.Warn("save last discover", "err", err)
	}
}

// --- results screen -------------------------------------------------------

func (b *Bot) handleDiscoverPage(c tele.Context) error {
	page, ok := callback.From(c).Int(0)
	if !ok {
		return errors.New("втрачено сторінку")
	}
	return b.renderDiscoverPage(c, page)
}

// handleDiscoverMore analyzes the next batch of browsed programs and
// re-renders from the top (the bigger set is globally re-sorted).
func (b *Bot) handleDiscoverMore(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()
	uid := senderID(c)
	state, err := b.fsm.Get(ctx, uid)
	if err != nil || state.Name != fsmStateDiscover {
		return errors.New("результати застаріли — повтори пошук через /menu")
	}
	in, ok := b.discoverInput(ctx, uid)
	if !ok {
		return errors.New("спочатку заповни /profile")
	}
	browsed := loadBrowsed(state.Data)
	analyzed := anyToInt(state.Data[fsmKeyDiscAnalyzed])
	if analyzed >= len(browsed) {
		_ = c.Respond(&tele.CallbackResponse{Text: "Це вже всі знайдені програми"})
		return nil
	}
	_ = c.Notify(tele.Typing)
	target := min(analyzed+discoverBatch, len(browsed))
	if err := b.analyzeAndStore(ctx, state.Data, in, target); err != nil {
		return err
	}
	if err := b.fsm.Set(context.Background(), uid, fsmStateDiscover, state.Data); err != nil {
		b.log.Warn("discover more set", "err", err)
	}
	return b.renderDiscoverPage(c, 0)
}

func (b *Bot) handleDiscoverOnlyPassTog(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), listsTimeout)
	state, err := b.fsm.Get(ctx, senderID(c))
	cancel()
	if err != nil || state.Name != fsmStateDiscover {
		return errors.New("результати застаріли — повтори пошук")
	}
	cur, _ := state.Data[fsmKeyDiscOnlyPass].(bool)
	state.Data[fsmKeyDiscOnlyPass] = !cur
	if err := b.fsm.Set(context.Background(), senderID(c), fsmStateDiscover, state.Data); err != nil {
		b.log.Warn("discover onlypass set", "err", err)
	}
	return b.renderDiscoverPage(c, 0)
}

// handleDiscoverSpec drives the specialty filter: "list" opens the picker,
// "all" clears it, a numeric arg picks the specialty at that index in the
// (stable, sorted) distinct-specialty list.
func (b *Bot) handleDiscoverSpec(c tele.Context) error {
	arg := callback.From(c).String(0)
	ctx, cancel := context.WithTimeout(context.Background(), listsTimeout)
	state, err := b.fsm.Get(ctx, senderID(c))
	cancel()
	if err != nil || state.Name != fsmStateDiscover {
		return errors.New("результати застаріли — повтори пошук")
	}

	switch arg {
	case "list":
		specs := distinctSpecs(loadBrowsed(state.Data))
		if len(specs) <= 1 {
			_ = c.Respond(&tele.CallbackResponse{Text: "У цій галузі лише одна спеціальність"})
			return nil
		}
		text := "🎓 Обери спеціальність, щоб звузити список:"
		return renderOrEdit(c, text, tele.ModeMarkdown, specPickerKeyboard(specs))
	case "all":
		state.Data[fsmKeyDiscSpec] = ""
	default:
		idx, ok := callback.From(c).Int(0)
		specs := distinctSpecs(loadBrowsed(state.Data))
		if !ok || idx < 0 || idx >= len(specs) {
			return errors.New("спеціальність не знайдено")
		}
		state.Data[fsmKeyDiscSpec] = specs[idx]
	}
	if err := b.fsm.Set(context.Background(), senderID(c), fsmStateDiscover, state.Data); err != nil {
		b.log.Warn("discover spec set", "err", err)
	}
	return b.renderDiscoverPage(c, 0)
}

// handleDiscoverSaveSafe saves every safety-tier program to /lists in one tap.
func (b *Bot) handleDiscoverSaveSafe(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()
	uid := senderID(c)
	rows, _, _, _, _, _, _, err := b.discoverState(c)
	if err != nil {
		return err
	}
	saved := 0
	for _, r := range rows {
		if r.Tier != int(abit.TierSafety) {
			continue
		}
		if saved >= discoverSaveMax {
			break
		}
		prog, ferr := b.programSvc.Fetch(ctx, r.URL)
		if ferr != nil {
			b.log.Warn("save-safe fetch", "url", r.URL, "err", ferr)
			continue
		}
		if _, serr := b.store.SaveList(ctx, uid, savedListName(prog), r.URL, prog); serr != nil {
			b.log.Warn("save-safe store", "err", serr)
			continue
		}
		saved++
	}
	msg := "Немає надійних програм для збереження"
	if saved > 0 {
		msg = fmt.Sprintf("💾 Збережено %d надійних у /lists", saved)
	}
	_ = c.Respond(&tele.CallbackResponse{Text: msg, ShowAlert: true})
	return nil
}

func (b *Bot) handleDiscoverResult(c tele.Context) error {
	idx, ok := callback.From(c).Int(0)
	if !ok {
		return errors.New("втрачено програму")
	}
	info, err := b.discoverView(c)
	if err != nil {
		return err
	}
	// Index into the same filtered view the keyboard was built from, so the
	// tapped button opens the program it shows.
	if idx < 0 || idx >= len(info.view) {
		return errors.New("програму не знайдено — повтори пошук")
	}
	return b.showSummaryFromDiscover(c, info.view[idx].URL)
}

// discoverViewInfo is the fully-resolved results view: the filtered+sorted
// rows the keyboard renders (and result taps index into), plus header data.
type discoverViewInfo struct {
	view              []discRow
	safe, mid, reach  int
	galName, regNames string
	spec              string // active specialty filter ("" = none)
	found, analyzed   int
	onlyPass          bool
	contract          bool // search included contract offers
}

// discoverView resolves the current results view from FSM, applying the
// active specialty and "only passing" filters with a graceful fallback when
// a filter would empty the list. Both renderDiscoverPage and the result-open
// handler use it, so a tapped button always opens the program it shows.
func (b *Bot) discoverView(c tele.Context) (discoverViewInfo, error) {
	rows, galName, regNames, found, onlyPass, spec, contract, err := b.discoverState(c)
	if err != nil {
		return discoverViewInfo{}, err
	}
	if len(rows) == 0 {
		return discoverViewInfo{}, errors.New("результати застаріли — повтори пошук через /menu")
	}
	base := rows
	if spec != "" {
		if f := filterBySpec(rows, spec); len(f) > 0 {
			base = f
		} else {
			spec = "" // specialty no longer present (e.g. before "show more")
		}
	}
	view := base
	if onlyPass {
		if f := filterPassing(base); len(f) > 0 {
			view = f
		} else {
			onlyPass = false
		}
	}
	safe, mid, reach := tierCounts(base)
	return discoverViewInfo{
		view: view, safe: safe, mid: mid, reach: reach,
		galName: galName, regNames: regNames, spec: spec,
		found: found, analyzed: len(rows), onlyPass: onlyPass, contract: contract,
	}, nil
}

func (b *Bot) renderDiscoverPage(c tele.Context, page int) error {
	info, err := b.discoverView(c)
	if err != nil {
		return err
	}
	maxPage := (len(info.view) - 1) / discoverPageSize
	page = max(0, min(page, maxPage))

	var sb strings.Builder
	fmt.Fprintf(&sb, "🧭 *Куди я вступлю*\n\n📚 %s · 📍 %s\n", mdEscape(info.galName), mdEscape(info.regNames))
	funding := "бюджет"
	if info.contract {
		funding = "бюджет+контракт"
	}
	fmt.Fprintf(&sb, "_%s · бакалавр · ПЗСО · денна_\n", funding)
	if info.spec != "" {
		fmt.Fprintf(&sb, "🎓 *%s*\n", mdEscape(info.spec))
	}
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "🟢 надійних: *%d* · 🟡 на межі: *%d* · 🔴 амбіційних: *%d*\n", info.safe, info.mid, info.reach)
	if info.found > info.analyzed {
		fmt.Fprintf(&sb, "_проаналізовано %d з %d — тисни «➕ Ще» для решти_\n", info.analyzed, info.found)
	}
	sb.WriteString("\nТисни програму — відкриється повний аналіз 👇")

	b.setDiscoverPage(c, page)
	return renderOrEdit(c, sb.String(), tele.ModeMarkdown,
		discoverResultsKeyboard(info.view, page, info.onlyPass, info.spec != "", info.analyzed < info.found), tele.NoPreview)
}

// --- analyze + state helpers ----------------------------------------------

// analyzeAndStore (re-)analyzes browsed[:target], sorts the whole set, and
// writes rows + analyzed count into data. Already-fetched programs come from
// cache, so re-analyzing the prefix is cheap.
func (b *Bot) analyzeAndStore(ctx context.Context, data map[string]any, in service.DiscoverInput, target int) error {
	browsed := loadBrowsed(data)
	if target > len(browsed) {
		target = len(browsed)
	}
	progs := make([]osvita.SpecProgram, target)
	for i := 0; i < target; i++ {
		progs[i] = osvita.SpecProgram{
			URL: browsed[i].URL, University: browsed[i].Uni,
			Program: browsed[i].Prog, Specialty: browsed[i].Spec,
		}
	}
	matches := b.discoverSvc.Analyze(ctx, progs, in)
	service.SortMatches(matches)

	rows := make([]discRow, 0, len(matches))
	for _, m := range matches {
		rows = append(rows, discRow{
			URL:   m.Program.URL,
			Name:  discoverName(m),
			Label: discoverLabel(m),
			Spec:  m.Program.Specialty,
			Tier:  int(m.Analysis.Chance.Tier()),
		})
	}
	raw, _ := json.Marshal(rows)
	data[fsmKeyDiscRows] = string(raw)
	data[fsmKeyDiscAnalyzed] = target
	return nil
}

func (b *Bot) discoverInput(ctx context.Context, uid int64) (service.DiscoverInput, bool) {
	nmt, _ := b.store.GetUserNMT(ctx, uid)
	if len(nmt) == 0 {
		return service.DiscoverInput{}, false
	}
	settings, _ := b.store.GetUserSettings(ctx, uid)
	return service.DiscoverInput{
		NMT:           map[string]float64(nmt),
		CreativeScore: float64(settings.CreativeScorePrediction),
		RegionCoef:    settings.RegionCoef,
		Quotas:        settings.Quotas,
	}, true
}

func (b *Bot) discoverState(c tele.Context) (rows []discRow, galName, regNames string, found int, onlyPass bool, spec string, contract bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), listsTimeout)
	defer cancel()
	state, gerr := b.fsm.Get(ctx, senderID(c))
	if gerr != nil || state.Name != fsmStateDiscover {
		return nil, "", "", 0, false, "", false, errors.New("сесія пошуку завершилась — почни з /menu")
	}
	if raw, _ := state.Data[fsmKeyDiscRows].(string); raw != "" {
		_ = json.Unmarshal([]byte(raw), &rows)
	}
	galName, _ = state.Data[fsmKeyDiscGalName].(string)
	regNames, _ = state.Data[fsmKeyDiscRegNames].(string)
	found = anyToInt(state.Data[fsmKeyDiscFound])
	onlyPass, _ = state.Data[fsmKeyDiscOnlyPass].(bool)
	spec, _ = state.Data[fsmKeyDiscSpec].(string)
	contract, _ = state.Data[fsmKeyDiscContract].(bool)
	return rows, galName, regNames, found, onlyPass, spec, contract, nil
}

func (b *Bot) setDiscoverPage(c tele.Context, page int) {
	ctx, cancel := context.WithTimeout(context.Background(), listsTimeout)
	defer cancel()
	uid := senderID(c)
	state, err := b.fsm.Get(ctx, uid)
	if err != nil || state.Name != fsmStateDiscover || state.Data == nil {
		return
	}
	state.Data[fsmKeyDiscPage] = page
	if err := b.fsm.Set(context.Background(), uid, fsmStateDiscover, state.Data); err != nil {
		b.log.Warn("discover page set", "err", err)
	}
}

func storeBrowsed(data map[string]any, progs []discProg) {
	raw, _ := json.Marshal(progs)
	data[fsmKeyDiscBrowsed] = string(raw)
}

func loadBrowsed(data map[string]any) []discProg {
	var out []discProg
	if raw, _ := data[fsmKeyDiscBrowsed].(string); raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return out
}

// --- view builders --------------------------------------------------------

// galuzLetter maps osvita's industryId to the official галузь-знань letter
// code that prefixes its specialties (e.g. F3 Комп'ютерні науки → F).
// Determined empirically from /spec/ listings; the 11 osvita industries map
// one-to-one onto letters A–K.
var galuzLetter = map[int]string{
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

func galuzKeyboard(f osvita.Filters) *tele.ReplyMarkup {
	// Order by the galuz letter (A, B, C…) rather than the form's by-name
	// order — the letters double as a quick alphabetical index.
	opts := append([]osvita.FilterOption(nil), f.Industries...)
	slices.SortFunc(opts, func(a, b osvita.FilterOption) int {
		return strings.Compare(galuzLetter[a.Code], galuzLetter[b.Code])
	})

	kb := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(opts)+1)
	for _, opt := range opts {
		label := opt.Name
		if l := galuzLetter[opt.Code]; l != "" {
			label = l + " — " + opt.Name
		}
		rows = append(rows, kb.Row(kb.Data(
			truncateRunes(label, 42), btnUniqueDiscoverGaluz, strconv.Itoa(opt.Code))))
	}
	rows = append(rows, kb.Row(kb.Data("⬅️ Меню", btnUniqueMenu)))
	kb.Inline(rows...)
	return kb
}

func regionPickerKeyboard(f osvita.Filters, sel []int, contract bool) *tele.ReplyMarkup {
	kb := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(f.Regions)/2+4)
	rows = append(rows, kb.Row(kb.Data(
		fmt.Sprintf("🔎 Шукати%s", selSuffix(sel)), btnUniqueDiscoverRun)))
	budgetLabel := "💰 Лише бюджет"
	if contract {
		budgetLabel = "💰 Бюджет + контракт"
	}
	rows = append(rows, kb.Row(kb.Data(budgetLabel, btnUniqueDiscoverBudgetTog)))

	var row []tele.Btn
	for _, opt := range f.Regions {
		label := truncateRunes(opt.Name, 22)
		if slices.Contains(sel, opt.Code) {
			label = "✅ " + truncateRunes(opt.Name, 20)
		}
		row = append(row, kb.Data(label, btnUniqueDiscoverRegionTog, strconv.Itoa(opt.Code)))
		if len(row) == 2 {
			rows = append(rows, kb.Row(row...))
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, kb.Row(row...))
	}
	rows = append(rows, kb.Row(
		kb.Data("⬅️ Галузі", btnUniqueDiscover),
		kb.Data("⬅️ Меню", btnUniqueMenu),
	))
	kb.Inline(rows...)
	return kb
}

func discoverResultsKeyboard(rows []discRow, page int, onlyPass, specActive, hasMore bool) *tele.ReplyMarkup {
	kb := &tele.ReplyMarkup{}
	start := page * discoverPageSize
	end := min(start+discoverPageSize, len(rows))
	maxPage := (len(rows) - 1) / discoverPageSize

	kbRows := make([]tele.Row, 0, end-start+4)
	for i := start; i < end; i++ {
		kbRows = append(kbRows, kb.Row(kb.Data(
			truncateRunes(rows[i].Label, 60), btnUniqueDiscoverResult, strconv.Itoa(i))))
	}

	if maxPage > 0 {
		var nav []tele.Btn
		if page > 0 {
			nav = append(nav, kb.Data("⬅️", btnUniqueDiscoverPage, strconv.Itoa(page-1)))
		}
		nav = append(nav, kb.Data(fmt.Sprintf("%d/%d", page+1, maxPage+1), btnUniqueNoop))
		if page < maxPage {
			nav = append(nav, kb.Data("➡️", btnUniqueDiscoverPage, strconv.Itoa(page+1)))
		}
		kbRows = append(kbRows, kb.Row(nav...))
	}

	passLabel := "🎯 Тільки прохідні"
	if onlyPass {
		passLabel = "📋 Показати всі"
	}
	actions := []tele.Btn{kb.Data(passLabel, btnUniqueDiscoverOnlyPassTog)}
	if hasMore {
		actions = append(actions, kb.Data("➕ Ще", btnUniqueDiscoverMore))
	}
	kbRows = append(kbRows, kb.Row(actions...))

	specLabel, specArg := "🎓 Спеціальність", "list"
	if specActive {
		specLabel, specArg = "🎓 Спеціальність: скинути", "all"
	}
	kbRows = append(kbRows, kb.Row(
		kb.Data(specLabel, btnUniqueDiscoverSpec, specArg),
		kb.Data("💾 Надійні в /lists", btnUniqueDiscoverSaveSafe),
	))
	kbRows = append(kbRows, kb.Row(kb.Data("⬅️ Меню", btnUniqueMenu)))
	kb.Inline(kbRows...)
	return kb
}

func specPickerKeyboard(specs []string) *tele.ReplyMarkup {
	kb := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(specs)+2)
	for i, s := range specs {
		rows = append(rows, kb.Row(kb.Data(truncateRunes(s, 56), btnUniqueDiscoverSpec, strconv.Itoa(i))))
	}
	rows = append(rows, kb.Row(kb.Data("📋 Усі спеціальності", btnUniqueDiscoverSpec, "all")))
	rows = append(rows, kb.Row(kb.Data("⬅️ До результатів", btnUniqueDiscoverPage, "0")))
	kb.Inline(rows...)
	return kb
}

// --- small helpers --------------------------------------------------------

func discoverFilters(galuz int, regions []int, contract bool) []osvita.SpecFilter {
	budgetOnly := !contract
	if len(regions) == 0 {
		return []osvita.SpecFilter{{Industry: galuz, BudgetOnly: budgetOnly}}
	}
	out := make([]osvita.SpecFilter, 0, len(regions))
	for _, r := range regions {
		out = append(out, osvita.SpecFilter{Industry: galuz, Region: r, BudgetOnly: budgetOnly})
	}
	return out
}

func discoverName(m service.ProgramMatch) string {
	if m.Program.University != "" {
		return m.Program.University
	}
	return m.Program.Program
}

// discoverLabel is the one-line button label: chance emoji, ЗВО, and the
// user's standing (rank / free seats) when known.
func discoverLabel(m service.ProgramMatch) string {
	a := m.Analysis
	standing := a.Chance.Label()
	if a.MyRealRank > 0 && a.RemainingSpots >= 0 && a.Chance != abit.ChanceUnknown {
		standing = fmt.Sprintf("%d-й, місць %d", a.MyRealRank, a.RemainingSpots)
	}
	return fmt.Sprintf("%s %s — %s", a.Chance.Emoji(), discoverName(m), standing)
}

func tierCounts(rows []discRow) (safe, mid, reach int) {
	for _, r := range rows {
		switch abit.Tier(r.Tier) {
		case abit.TierSafety:
			safe++
		case abit.TierMatch:
			mid++
		case abit.TierReach:
			reach++
		}
	}
	return safe, mid, reach
}

func filterPassing(rows []discRow) []discRow {
	out := make([]discRow, 0, len(rows))
	for _, r := range rows {
		if abit.Tier(r.Tier) == abit.TierSafety || abit.Tier(r.Tier) == abit.TierMatch {
			out = append(out, r)
		}
	}
	return out
}

func filterBySpec(rows []discRow, spec string) []discRow {
	out := make([]discRow, 0, len(rows))
	for _, r := range rows {
		if r.Spec == spec {
			out = append(out, r)
		}
	}
	return out
}

// distinctSpecs returns the unique specialty labels among browsed programs,
// in stable (sorted) order — so a button's index maps to the same specialty
// across renders.
func distinctSpecs(browsed []discProg) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, p := range browsed {
		if p.Spec == "" {
			continue
		}
		if _, ok := seen[p.Spec]; ok {
			continue
		}
		seen[p.Spec] = struct{}{}
		out = append(out, p.Spec)
	}
	slices.Sort(out)
	return out
}

func selSuffix(sel []int) string {
	switch {
	case len(sel) == 0:
		return " (вся Україна)"
	case len(sel) == 1:
		return " (1 область)"
	default:
		return fmt.Sprintf(" (%d областей)", len(sel))
	}
}

func optionName(opts []osvita.FilterOption, code int, fallback string) string {
	for _, o := range opts {
		if o.Code == code {
			return o.Name
		}
	}
	return fallback
}

func anyToInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

// decodeIntSlice reads a JSON-encoded []int stored in FSM data.
func decodeIntSlice(v any) []int {
	raw, _ := v.(string)
	if raw == "" {
		return nil
	}
	var out []int
	_ = json.Unmarshal([]byte(raw), &out)
	return out
}
