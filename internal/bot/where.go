package bot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	tele "gopkg.in/telebot.v3"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/bot/callback"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/osvita"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
)

// "Where can I get in" flow: pick a галузь → pick a region (or all of
// Ukraine) → the bot scores the user against every budget bachelor program
// matching the filter and lists them best-chance-first. Each result opens
// the usual full analysis screen.

const (
	fsmStateDiscover = "discover.viewing"

	fsmKeyDiscRows   = "d_rows"   // JSON []discoverRow
	fsmKeyDiscGaluz  = "d_galuz"  // human label, for the header
	fsmKeyDiscRegion = "d_region" // human label, for the header
	fsmKeyDiscFound  = "d_found"  // total matched (may exceed analyzed)
	fsmKeyDiscPage   = "d_page"

	// discoverLimit caps how many programs are fetched+analyzed per run.
	// Each is a full osvita scrape, so this bounds latency and politeness;
	// the user narrows by region to bring a big galuz under the cap.
	discoverLimit    = 12
	discoverPageSize = 6
)

// discoverRow is a compact, FSM-serializable result: enough to render the
// list and to reopen the full analysis on tap.
type discoverRow struct {
	URL   string `json:"u"`
	Label string `json:"l"`
}

// handleDiscover is the menu entry point. Requires a filled profile (we
// rank the user, so we need their score) and a private chat.
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
	text := "🧭 *Куди я вступлю*\n\nОбери галузь знань — я знайду бюджетні бакалаврські програми й покажу, куди ти проходиш."
	return renderOrEdit(c, text, tele.ModeMarkdown, galuzKeyboard(filters))
}

// handleDiscoverGaluz shows the region picker for the chosen галузь.
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
	text := "🧭 *Куди я вступлю*\n\nОбери регіон (або всю Україну):"
	return renderOrEdit(c, text, tele.ModeMarkdown, regionKeyboard(filters, galuz))
}

// handleDiscoverRegion runs the discovery for galuz+region, stores the
// ranked rows in FSM, and renders the first page.
func (b *Bot) handleDiscoverRegion(c tele.Context) error {
	args := callback.From(c)
	galuz, ok1 := args.Int(0)
	region, ok2 := args.Int(1)
	if !ok1 || !ok2 {
		return errors.New("втрачено фільтр — почни з /menu")
	}

	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()
	uid := senderID(c)
	in, ok := b.discoverInput(ctx, uid)
	if !ok {
		return errors.New("спочатку заповни /profile")
	}
	_ = c.Notify(tele.Typing)

	filters, _ := b.discoverSvc.Filters(ctx)
	res, err := b.discoverSvc.WhereCanIGetIn(ctx, osvita.SpecFilter{
		Region:     region,
		Industry:   galuz,
		BudgetOnly: true,
	}, in, discoverLimit)
	if err != nil {
		return fmt.Errorf("пошук не вдався: %w", err)
	}
	if len(res.Matches) == 0 {
		return errors.New("за цим фільтром нічого не знайшов — спробуй іншу галузь чи регіон")
	}

	rows := make([]discoverRow, 0, len(res.Matches))
	for _, m := range res.Matches {
		rows = append(rows, discoverRow{URL: m.Program.URL, Label: discoverLabel(m)})
	}
	rowsJSON, _ := json.Marshal(rows)

	data := map[string]any{
		fsmKeyDiscRows:   string(rowsJSON),
		fsmKeyDiscGaluz:  optionName(filters.Industries, galuz, "Усі галузі"),
		fsmKeyDiscRegion: optionName(filters.Regions, region, "Вся Україна"),
		fsmKeyDiscFound:  res.Found,
		fsmKeyDiscPage:   0,
	}
	if err := b.fsm.Set(context.Background(), uid, fsmStateDiscover, data); err != nil {
		b.log.Warn("discover fsm set", "err", err)
	}
	return b.renderDiscoverPage(c, 0)
}

// handleDiscoverPage flips between result pages, reading the cached rows
// from FSM (no re-fetch).
func (b *Bot) handleDiscoverPage(c tele.Context) error {
	page, ok := callback.From(c).Int(0)
	if !ok {
		return errors.New("втрачено сторінку")
	}
	return b.renderDiscoverPage(c, page)
}

// handleDiscoverResult opens the full analysis for the tapped program.
func (b *Bot) handleDiscoverResult(c tele.Context) error {
	idx, ok := callback.From(c).Int(0)
	if !ok {
		return errors.New("втрачено програму")
	}
	rows, _, _, _, err := b.discoverState(c)
	if err != nil {
		return err
	}
	if idx < 0 || idx >= len(rows) {
		return errors.New("програму не знайдено — повтори пошук")
	}
	return b.showSummary(c, rows[idx].URL)
}

// renderDiscoverPage renders one page of the cached discovery result.
func (b *Bot) renderDiscoverPage(c tele.Context, page int) error {
	rows, galuz, region, found, err := b.discoverState(c)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return errors.New("результати застаріли — повтори пошук через /menu")
	}

	maxPage := (len(rows) - 1) / discoverPageSize
	if page < 0 {
		page = 0
	}
	if page > maxPage {
		page = maxPage
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "🧭 *Куди я вступлю*\n\n📚 %s · 📍 %s\n",
		mdEscape(galuz), mdEscape(region))
	sb.WriteString("_бюджет · бакалавр · ПЗСО · денна_\n\n")
	if found > len(rows) {
		fmt.Fprintf(&sb, "Знайдено *%d* програм; показую *%d* найкращих за шансом (звузь регіоном, щоб охопити всі).\n\n",
			found, len(rows))
	} else {
		fmt.Fprintf(&sb, "Знайдено й проаналізовано *%d* програм, від найкращих шансів:\n\n", len(rows))
	}
	sb.WriteString("Тисни програму — відкриється повний аналіз 👇")

	// FSM page bookkeeping so pagination survives a restart.
	b.setDiscoverPage(c, page)

	return renderOrEdit(c, sb.String(), tele.ModeMarkdown,
		discoverResultsKeyboard(rows, page), tele.NoPreview)
}

// --- state helpers --------------------------------------------------------

// discoverInput assembles the user's rating inputs. ok is false when the
// profile has no NMT scores (nothing to rank against).
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

// discoverState reads the cached discovery result from FSM.
func (b *Bot) discoverState(c tele.Context) (rows []discoverRow, galuz, region string, found int, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), listsTimeout)
	defer cancel()
	state, gerr := b.fsm.Get(ctx, senderID(c))
	if gerr != nil || state.Name != fsmStateDiscover {
		return nil, "", "", 0, errors.New("сесія пошуку завершилась — почни з /menu")
	}
	raw, _ := state.Data[fsmKeyDiscRows].(string)
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &rows)
	}
	galuz, _ = state.Data[fsmKeyDiscGaluz].(string)
	region, _ = state.Data[fsmKeyDiscRegion].(string)
	found = anyToInt(state.Data[fsmKeyDiscFound])
	return rows, galuz, region, found, nil
}

// setDiscoverPage persists the current page without touching the rest of
// the discovery state.
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

// --- view builders --------------------------------------------------------

func galuzKeyboard(f osvita.Filters) *tele.ReplyMarkup {
	kb := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(f.Industries)+1)
	for _, opt := range f.Industries {
		rows = append(rows, kb.Row(kb.Data(
			truncateRunes(opt.Name, 40), btnUniqueDiscoverGaluz, strconv.Itoa(opt.Code))))
	}
	rows = append(rows, kb.Row(kb.Data("⬅️ Меню", btnUniqueMenu)))
	kb.Inline(rows...)
	return kb
}

func regionKeyboard(f osvita.Filters, galuz int) *tele.ReplyMarkup {
	kb := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(f.Regions)/2+3)
	rows = append(rows, kb.Row(kb.Data("🇺🇦 Вся Україна",
		btnUniqueDiscoverRegion, callback.Encode(strconv.Itoa(galuz), "0"))))

	var row []tele.Btn
	for _, opt := range f.Regions {
		row = append(row, kb.Data(truncateRunes(opt.Name, 22),
			btnUniqueDiscoverRegion, callback.Encode(strconv.Itoa(galuz), strconv.Itoa(opt.Code))))
		if len(row) == 2 {
			rows = append(rows, kb.Row(row...))
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, kb.Row(row...))
	}
	rows = append(rows, kb.Row(kb.Data("⬅️ Меню", btnUniqueMenu)))
	kb.Inline(rows...)
	return kb
}

func discoverResultsKeyboard(rows []discoverRow, page int) *tele.ReplyMarkup {
	kb := &tele.ReplyMarkup{}
	start := page * discoverPageSize
	end := min(start+discoverPageSize, len(rows))
	maxPage := (len(rows) - 1) / discoverPageSize

	kbRows := make([]tele.Row, 0, end-start+2)
	for i := start; i < end; i++ {
		kbRows = append(kbRows, kb.Row(kb.Data(
			truncateRunes(rows[i].Label, 60), btnUniqueDiscoverResult, strconv.Itoa(i))))
	}

	var nav []tele.Btn
	if page > 0 {
		nav = append(nav, kb.Data("⬅️", btnUniqueDiscoverPage, strconv.Itoa(page-1)))
	}
	nav = append(nav, kb.Data(fmt.Sprintf("%d/%d", page+1, maxPage+1), btnUniqueNoop))
	if page < maxPage {
		nav = append(nav, kb.Data("➡️", btnUniqueDiscoverPage, strconv.Itoa(page+1)))
	}
	kbRows = append(kbRows, kb.Row(nav...))
	kbRows = append(kbRows, kb.Row(kb.Data("⬅️ Меню", btnUniqueMenu)))
	kb.Inline(kbRows...)
	return kb
}

// discoverLabel builds the one-line button label for a result.
func discoverLabel(m service.ProgramMatch) string {
	name := m.Program.University
	if name == "" {
		name = m.Program.Program
	}
	return fmt.Sprintf("%s %s — %s", m.Analysis.Chance.Emoji(), name, m.Analysis.Chance.Label())
}

// --- small helpers --------------------------------------------------------

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
