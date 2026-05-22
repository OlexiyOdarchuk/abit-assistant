package bot

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/bot/callback"
	"github.com/OlexiyOdarchuk/abit-assistant/pkg/abit"
)

const (
	pageSize      = 10
	searchTimeout = 90 * time.Second
	historyLimit  = 10

	// FSM states. Convention: <feature>.<step>.
	fsmStateWaitingURL = "search.waiting_url"
	fsmStateViewing    = "search.viewing"
)

// FSM data keys for the search flow.
const (
	fsmKeyURL  = "url"
	fsmKeyPage = "page"
	fsmKeyMode = "mode"
)

// Search list display modes — toggled by the user from the results page.
const (
	modeAll         = "all"
	modeCompetitors = "competitors"
)

// --- Command handlers -----------------------------------------------------

func (b *Bot) handleStart(c tele.Context) error { return b.renderMenu(c) }
func (b *Bot) handleMenu(c tele.Context) error  { return b.renderMenu(c) }
func (b *Bot) handleHelp(c tele.Context) error  { return c.Send(helpText, tele.ModeMarkdown) }
func (b *Bot) handleAbout(c tele.Context) error { return b.renderAbout(c) }

func (b *Bot) handleCancel(c tele.Context) error {
	if err := b.fsm.Clear(context.Background(), senderID(c)); err != nil {
		return fmt.Errorf("не вдалося очистити стан: %w", err)
	}
	return c.Send("🚫 Поточну дію скасовано. /menu — головне меню")
}

func (b *Bot) handleProfile(c tele.Context) error { return b.renderProfile(c) }

func (b *Bot) handleLists(c tele.Context) error {
	return c.Send("📂 *Збережені списки* — у розробці.",
		tele.ModeMarkdown, backToMenuKeyboard())
}

func (b *Bot) handleSearch(c tele.Context) error {
	raw := strings.TrimSpace(c.Message().Payload)
	if raw == "" {
		return b.askForURL(c)
	}
	return b.runSearch(c, raw)
}

func (b *Bot) handleText(c tele.Context) error {
	text := strings.TrimSpace(c.Text())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	state, err := b.fsm.Get(ctx, senderID(c))
	cancel()
	if err != nil {
		b.log.Warn("fsm get failed", "err", err, "user_id", senderID(c))
	}

	switch state.Name {
	case fsmStateWaitingURL:
		return b.runSearch(c, text)
	case fsmStateProfileEnterScore:
		return b.handleProfileEnterScore(c, state.Data)
	case fsmStateProfileEnterCreative:
		return b.handleProfileEnterCreative(c)
	}
	if looksLikeOsvitaURL(text) {
		return b.runSearch(c, text)
	}
	return c.Send("Не зрозумів. /menu — головне меню, /help — список команд.")
}

// --- Callback handlers ----------------------------------------------------

func (b *Bot) handleMenuCB(c tele.Context) error    { return b.renderMenu(c) }
func (b *Bot) handleAboutCB(c tele.Context) error   { return b.renderAbout(c) }
func (b *Bot) handleSearchCB(c tele.Context) error  { return b.askForURL(c) }
func (b *Bot) handleProfileCB(c tele.Context) error { return b.renderProfile(c) }
func (b *Bot) handleListsCB(c tele.Context) error   { return b.handleLists(c) }

func (b *Bot) handlePagePrev(c tele.Context) error { return b.flipPage(c, -1) }
func (b *Bot) handlePageNext(c tele.Context) error { return b.flipPage(c, +1) }

func (b *Bot) flipPage(c tele.Context, delta int) error {
	rawURL, curPage, mode, err := b.viewingState(c)
	if err != nil {
		return err
	}
	args := callback.From(c)
	if v, ok := args.Int(0); ok {
		// Use the page baked into the button — defends against state drift.
		curPage = v
	}
	return b.showResultsPage(c, rawURL, curPage+delta, mode)
}

// handleToggleMode flips the list between "all" and "competitors", then
// jumps back to page 0 so the user always sees fresh first-page results.
func (b *Bot) handleToggleMode(c tele.Context) error {
	rawURL, _, mode, err := b.viewingState(c)
	if err != nil {
		return err
	}
	if mode == modeCompetitors {
		mode = modeAll
	} else {
		mode = modeCompetitors
	}
	return b.showResultsPage(c, rawURL, 0, mode)
}

// handleSummaryCB re-renders the summary screen from the current viewing
// state. Triggered by the "🎯 Аналіз" button on the list.
func (b *Bot) handleSummaryCB(c tele.Context) error {
	rawURL, _, _, err := b.viewingState(c)
	if err != nil {
		return err
	}
	return b.showSummary(c, rawURL)
}

// handleViewListCB jumps from summary to the applicants list, restoring
// the page + mode the user last viewed (page 0, mode "all" on first entry).
func (b *Bot) handleViewListCB(c tele.Context) error {
	rawURL, page, mode, err := b.viewingState(c)
	if err != nil {
		return err
	}
	return b.showResultsPage(c, rawURL, page, mode)
}

// handleSaveListCB is the stub for the (still pending) saved-lists
// feature. Surfaced via the "💾 Зберегти" button on the summary screen.
func (b *Bot) handleSaveListCB(c tele.Context) error {
	return errors.New("збереження списків — у розробці")
}

// handleApplicantView opens the detail screen for the applicant whose ID
// is in callback args.
func (b *Bot) handleApplicantView(c tele.Context) error {
	id, ok := callback.From(c).Int(0)
	if !ok {
		return errors.New("втрачено ID абітурієнта")
	}
	rawURL, _, _, err := b.viewingState(c)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()
	abits, err := b.programSvc.FetchDecoded(ctx, rawURL)
	if err != nil {
		return fmt.Errorf("не вдалося завантажити дані: %w", err)
	}
	ab := findApplicant(abits, id)
	if ab == nil {
		return errors.New("абітурієнта не знайдено в поточному списку")
	}

	text, kb := buildApplicantDetail(*ab)
	return renderOrEdit(c, text, tele.ModeMarkdown, kb, tele.NoPreview)
}

// handleApplicantHistory shows the applicant's submissions across all
// universities via abit-poisk.
func (b *Bot) handleApplicantHistory(c tele.Context) error {
	id, ok := callback.From(c).Int(0)
	if !ok {
		return errors.New("втрачено ID абітурієнта")
	}
	rawURL, _, _, err := b.viewingState(c)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()
	abits, err := b.programSvc.FetchDecoded(ctx, rawURL)
	if err != nil {
		return fmt.Errorf("не вдалося завантажити дані: %w", err)
	}
	ab := findApplicant(abits, id)
	if ab == nil {
		return errors.New("абітурієнта не знайдено")
	}
	if isMaskedName(ab.Name) {
		return errors.New("ім'я приховане — інші заяви недоступні")
	}

	if err := c.Notify(tele.Typing); err != nil {
		b.log.Debug("notify typing", "err", err)
	}
	entries, err := b.applicantSvc.Search(ctx, ab.Name)
	if err != nil && !errors.Is(err, abit.ErrNoData) {
		return fmt.Errorf("не вдалося знайти інші заяви: %w", err)
	}

	text, kb := buildHistoryView(*ab, entries)
	return renderOrEdit(c, text, tele.ModeMarkdown, kb, tele.NoPreview)
}

// handleBackToList re-renders the page the user was on before opening a
// detail screen. The page index lives in FSM, so this works even after a
// bot restart.
func (b *Bot) handleBackToList(c tele.Context) error {
	rawURL, page, mode, err := b.viewingState(c)
	if err != nil {
		return err
	}
	return b.showResultsPage(c, rawURL, page, mode)
}

// --- Shared rendering -----------------------------------------------------

func (b *Bot) renderMenu(c tele.Context) error {
	return renderOrEdit(c, welcomeText, tele.ModeMarkdown, mainMenuKeyboard())
}

func (b *Bot) renderAbout(c tele.Context) error {
	return renderOrEdit(c, aboutText, tele.ModeMarkdown,
		backToMenuKeyboard(), tele.NoPreview)
}

func (b *Bot) askForURL(c tele.Context) error {
	if err := b.fsm.Set(context.Background(), senderID(c),
		fsmStateWaitingURL, nil); err != nil {
		return fmt.Errorf("не вдалося зберегти стан: %w", err)
	}
	const prompt = `🔗 Надішли посилання на програму з vstup.osvita.ua.

Приклад:
` + "`https://vstup.osvita.ua/y2025/r14/282/1471029/`" + `

Або /cancel щоб вийти.`
	return renderOrEdit(c, prompt, tele.ModeMarkdown, backToMenuKeyboard())
}

func (b *Bot) runSearch(c tele.Context, rawURL string) error {
	if !looksLikeOsvitaURL(rawURL) {
		return errors.New("це не схоже на посилання vstup.osvita.ua")
	}
	if err := c.Notify(tele.Typing); err != nil {
		b.log.Debug("notify typing", "err", err)
	}
	return b.showSummary(c, rawURL)
}

// showSummary renders the analysis screen: user's rating, chance level,
// counts, verdict. The list of applicants is one click away ("Дивитись
// список"); the user can also re-open this screen later via the
// "🎯 Аналіз" button on the list page.
func (b *Bot) showSummary(c tele.Context, rawURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()

	prog, err := b.programSvc.Fetch(ctx, rawURL)
	if err != nil {
		return fmt.Errorf("не вдалося отримати дані: %w", err)
	}
	abits := abit.Decode(prog)
	if len(abits) == 0 {
		return errors.New("програма знайдена, але список порожній")
	}

	uid := senderID(c)
	nmt, err := b.store.GetUserNMT(ctx, uid)
	if err != nil {
		b.log.Warn("user nmt read failed", "err", err)
	}
	settings, err := b.store.GetUserSettings(ctx, uid)
	if err != nil {
		b.log.Warn("user settings read failed", "err", err)
	}
	userScore := abit.ComputeRating(prog, abit.RatingInput{
		NMT:           map[string]float64(nmt),
		CreativeScore: float64(settings.CreativeScorePrediction),
		RegionCoef:    settings.RegionCoef,
	})
	analysis := abit.Analyze(prog, abits, abit.AnalyzeInput{
		UserScore:  userScore,
		UserQuotas: settings.Quotas,
	})

	// Persist viewing state so subsequent clicks (list, back, etc.)
	// keep their bearings. Page 0 + mode "all" are the natural defaults.
	if err := b.fsm.Set(context.Background(), uid, fsmStateViewing, map[string]any{
		fsmKeyURL:  rawURL,
		fsmKeyPage: 0,
		fsmKeyMode: modeAll,
	}); err != nil {
		b.log.Warn("fsm set failed", "err", err)
	}

	text, kb := buildSummaryView(prog, analysis)
	return renderOrEdit(c, text, tele.ModeMarkdown, kb, tele.NoPreview)
}

// showResultsPage runs (or re-runs through cache) the program lookup,
// computes the user's own rating, and renders the requested page of
// either ALL applicants or just the competitors (score > user rating).
// Persists FSM so deeper screens (detail, history) and pagination
// buttons know which URL + page + mode they're attached to.
func (b *Bot) showResultsPage(c tele.Context, rawURL string, page int, mode string) error {
	if mode != modeAll && mode != modeCompetitors {
		mode = modeAll
	}
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()

	prog, err := b.programSvc.Fetch(ctx, rawURL)
	if err != nil {
		return fmt.Errorf("не вдалося отримати дані: %w", err)
	}
	abits := abit.Decode(prog)
	if len(abits) == 0 {
		return errors.New("програма знайдена, але список порожній")
	}

	// User's own competitive rating, computed from their profile НМТ +
	// settings. 0 if profile isn't filled — the view degrades gracefully.
	uid := senderID(c)
	nmt, err := b.store.GetUserNMT(ctx, uid)
	if err != nil {
		b.log.Warn("user nmt read failed", "err", err)
	}
	settings, err := b.store.GetUserSettings(ctx, uid)
	if err != nil {
		b.log.Warn("user settings read failed", "err", err)
	}
	userScore := abit.ComputeRating(prog, abit.RatingInput{
		NMT:           map[string]float64(nmt),
		CreativeScore: float64(settings.CreativeScorePrediction),
		RegionCoef:    settings.RegionCoef,
	})

	// Competitors mode degrades to "all" when we can't tell who is who.
	if mode == modeCompetitors && userScore == 0 {
		mode = modeAll
	}

	view := abits
	if mode == modeCompetitors {
		view = filterCompetitors(abits, userScore)
	}
	if len(view) == 0 {
		// e.g. user is at the top of the field with no one to outrank
		view = abits
		mode = modeAll
	}

	if page < 0 {
		page = 0
	}
	maxPage := (len(view) - 1) / pageSize
	page = min(page, maxPage)

	if err := b.fsm.Set(context.Background(), uid, fsmStateViewing, map[string]any{
		fsmKeyURL:  rawURL,
		fsmKeyPage: page,
		fsmKeyMode: mode,
	}); err != nil {
		b.log.Warn("fsm set failed", "err", err)
	}

	text, kb := buildResultsView(prog, view, abits, page, userScore, mode)
	return renderOrEdit(c, text, tele.ModeMarkdown, kb, tele.NoPreview)
}

// filterCompetitors returns applicants that realistically compete with
// the user for a budget seat, mirroring the Python filter_data logic
// (minus the abit-poisk recheck which we delegate to enrichSvc).
func filterCompetitors(abits []abit.Abiturient, mine float64) []abit.Abiturient {
	out := make([]abit.Abiturient, 0)
	for _, ab := range abits {
		if isCompetitor(ab, mine) {
			out = append(out, ab)
		}
	}
	return out
}

// isCompetitor encapsulates the per-applicant decision:
//   - contract-only applicants don't fight for budget seats
//   - "деактивовано / скасовано / відмова / відраховано" — out of the race
//   - "до наказу / рекомендовано" — already occupies a seat, definite
//     competitor regardless of priority/score
//   - otherwise: competing only if their score strictly exceeds mine
//     (priority>1 with score<=mine almost always means they'll pass
//     elsewhere; ties go to "not competing" — same as Python)
func isCompetitor(ab abit.Abiturient, mine float64) bool {
	if !ab.StateEducation {
		return false
	}
	low := strings.ToLower(ab.Status)
	for _, drop := range []string{"деактивовано", "скасовано", "відмова", "відраховано"} {
		if strings.Contains(low, drop) {
			return false
		}
	}
	if strings.Contains(low, "до наказу") || strings.Contains(low, "рекомендовано") {
		return true
	}
	return ab.Score > mine
}

// viewingState reads the URL, page and mode from FSM. Returns a clear
// error if the user is not in the search.viewing state — handlers
// should propagate this to the user as-is.
func (b *Bot) viewingState(c tele.Context) (rawURL string, page int, mode string, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	state, err := b.fsm.Get(ctx, senderID(c))
	if err != nil {
		return "", 0, "", fmt.Errorf("не вдалося прочитати стан: %w", err)
	}
	if state.Name != fsmStateViewing {
		return "", 0, "", errors.New("сесію переглядання втрачено — почни з /search")
	}
	rawURL = state.Get(fsmKeyURL)
	if p, ok := state.Data[fsmKeyPage].(float64); ok {
		page = int(p)
	}
	mode, _ = state.Data[fsmKeyMode].(string)
	if mode == "" {
		mode = modeAll
	}
	return rawURL, page, mode, nil
}

// --- View builders --------------------------------------------------------

// buildResultsView renders the program header as text + a grid of inline
// buttons (10 applicants per page) + pagination + mode toggle + menu.
//
// `view` is the (possibly filtered) slice the page is sliced from;
// `all` is the unfiltered list, used only for the competitor count
// shown in the header.
func buildResultsView(prog *abit.Program, view, all []abit.Abiturient, page int, userScore float64, mode string) (string, *tele.ReplyMarkup) {
	total := len(view)
	maxPage := (total - 1) / pageSize
	start := page * pageSize
	end := min(start+pageSize, total)

	var sb strings.Builder
	fmt.Fprintf(&sb, "📋 *%s* — %s\n",
		mdEscape(prog.UniversityName), mdEscape(prog.ProgramName))

	if mode == modeCompetitors {
		fmt.Fprintf(&sb, "🎯 Конкуренти: *%d* / %d · Сторінка %d / %d\n",
			total, len(all), page+1, maxPage+1)
	} else {
		competitorsTotal := countCompetitors(all, userScore)
		if userScore > 0 {
			fmt.Fprintf(&sb, "Заявок: *%d* · 🔴 Конкурентів: *%d* · Сторінка %d / %d\n",
				total, competitorsTotal, page+1, maxPage+1)
		} else {
			fmt.Fprintf(&sb, "Заявок: *%d* · Сторінка %d / %d\n",
				total, page+1, maxPage+1)
		}
	}

	if userScore > 0 {
		fmt.Fprintf(&sb, "🧮 *Твій бал:* `%.3f`\n", userScore)
	} else {
		sb.WriteString("_Заповни /profile, щоб бачити, хто реально конкурент._\n")
	}
	sb.WriteString("\nНатисни на абітурієнта для деталей 👇")

	kb := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, end-start+4)

	for i := start; i < end; i++ {
		ab := view[i]
		rows = append(rows, kb.Row(kb.Data(
			applicantButtonLabel(ab, i+1, userScore),
			btnUniqueApplicant,
			callback.Encode(strconv.Itoa(ab.ID)),
		)))
	}

	// Mode toggle — only meaningful when the user has a score and there's
	// at least one competitor above them.
	if userScore > 0 && (mode == modeCompetitors || countCompetitors(all, userScore) > 0) {
		label := "🎯 Тільки конкуренти"
		if mode == modeCompetitors {
			label = "📋 Усі заяви"
		}
		rows = append(rows, kb.Row(kb.Data(label, btnUniqueToggleMode)))
	}

	if maxPage > 0 {
		nav := make([]tele.Btn, 0, 3)
		if page > 0 {
			nav = append(nav, kb.Data("◀️", btnUniquePagePrev,
				callback.Encode(strconv.Itoa(page))))
		}
		nav = append(nav, kb.Data(
			fmt.Sprintf("%d / %d", page+1, maxPage+1), btnUniqueNoop))
		if page < maxPage {
			nav = append(nav, kb.Data("▶️", btnUniquePageNext,
				callback.Encode(strconv.Itoa(page))))
		}
		rows = append(rows, kb.Row(nav...))
	}
	rows = append(rows, kb.Row(
		kb.Data("🎯 Аналіз", btnUniqueSummary),
		kb.Data("⬅️ Меню", btnUniqueMenu),
	))
	kb.Inline(rows...)
	return sb.String(), kb
}

// buildSummaryView renders the per-program analysis screen: user's
// rating, chance, counts, verdict + actions.
func buildSummaryView(prog *abit.Program, an abit.Analysis) (string, *tele.ReplyMarkup) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "🎓 *%s* — %s\n\n",
		mdEscape(prog.UniversityName), mdEscape(prog.ProgramName))

	if an.UserScore == 0 {
		sb.WriteString("⚠️ Заповни /profile, щоб побачити аналіз шансів.\n\n")
		sb.WriteString(fmt.Sprintf("📊 Заявок: *%d*", len(prog.Requests)))
	} else {
		fmt.Fprintf(&sb, "🧮 *Твій бал:* `%.3f`\n", an.UserScore)
		fmt.Fprintf(&sb, "%s *Шанс:* %s\n\n", an.Chance.Emoji(), an.Chance.Label())

		sb.WriteString("📊 *Розклад:*\n")
		fmt.Fprintf(&sb, "   • Реальних конкурентів: *%d*\n", an.CompetitorsTotal)
		if an.AlreadyEnrolled > 0 {
			fmt.Fprintf(&sb, "   • Вже на наказі/рекомендовано: *%d*\n", an.AlreadyEnrolled)
		}
		if an.BudgetTotal > 0 {
			fmt.Fprintf(&sb, "   • Бюджетних місць: *%d*\n", an.BudgetTotal)
		}
		if an.Quota1Total > 0 {
			fmt.Fprintf(&sb, "   • Квота 1: %d місць\n", an.Quota1Total)
		}
		if an.Quota2Total > 0 {
			fmt.Fprintf(&sb, "   • Квота 2: %d місць\n", an.Quota2Total)
		}
		fmt.Fprintf(&sb, "   • Вільних місць: *%d*\n", an.RemainingSpots)

		if an.MyRealRank > 0 {
			fmt.Fprintf(&sb, "\n🏆 *Твоє місце:* %d", an.MyRealRank)
			if an.BudgetTotal > 0 {
				fmt.Fprintf(&sb, " (бюджет %d місць)", an.BudgetTotal)
			}
			sb.WriteString("\n")
		}
		if an.Advice != "" {
			fmt.Fprintf(&sb, "\n💡 %s", mdEscape(an.Advice))
		}
	}

	kb := &tele.ReplyMarkup{}
	kb.Inline(
		kb.Row(kb.Data("📋 Дивитись список", btnUniqueViewList)),
		kb.Row(
			kb.Data("💾 Зберегти", btnUniqueSaveList),
			kb.Data("⬅️ Меню", btnUniqueMenu),
		),
	)
	return sb.String(), kb
}

func countCompetitors(abits []abit.Abiturient, mine float64) int {
	if mine <= 0 {
		return 0
	}
	n := 0
	for _, ab := range abits {
		if isCompetitor(ab, mine) {
			n++
		}
	}
	return n
}

// applicantButtonLabel is what shows up on the applicant's button.
// Compact: rank, competitor marker (when userScore > 0), status marker,
// name, score. Inline buttons have a pragmatic length cap before
// Telegram hides text on small screens.
func applicantButtonLabel(ab abit.Abiturient, rank int, userScore float64) string {
	threatMarker := ""
	if userScore > 0 {
		if isCompetitor(ab, userScore) {
			threatMarker = "🔴 "
		} else {
			threatMarker = "🟢 "
		}
	}
	statusM := statusMarker(ab.Status)
	label := fmt.Sprintf("%d. %s%s%s — %.1f",
		rank, threatMarker, statusM, ab.Name, ab.Score)
	if len(label) > 60 {
		label = label[:57] + "…"
	}
	return label
}

func statusMarker(status string) string {
	low := strings.ToLower(status)
	switch {
	case strings.HasPrefix(low, "до наказу"):
		return "✅ "
	case strings.HasPrefix(low, "рекомендовано"):
		return "🟢 "
	case strings.HasPrefix(low, "допущено"):
		return "🟡 "
	case strings.HasPrefix(low, "деактивовано"):
		return "🔄 "
	case strings.HasPrefix(low, "відхилено"), strings.HasPrefix(low, "відмова"),
		strings.HasPrefix(low, "відраховано"), strings.HasPrefix(low, "скасовано"):
		return "⛔ "
	}
	return ""
}

// buildApplicantDetail renders the full record + actions for one applicant.
func buildApplicantDetail(ab abit.Abiturient) (string, *tele.ReplyMarkup) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "📄 *%s*\n\n", mdEscape(ab.Name))
	fmt.Fprintf(&sb, "📊 *Статус:* %s\n", mdEscape(ab.Status))
	if ab.RecType != "" {
		fmt.Fprintf(&sb, "🏆 *Рекомендація:* %s\n", mdEscape(ab.RecType))
	}
	fmt.Fprintf(&sb, "🎯 *Пріоритет:* %d\n", ab.Priority)
	fmt.Fprintf(&sb, "📈 *Конкурсний бал:* `%.3f`\n", ab.Score)

	fundingMarker := "контракт"
	if ab.StateEducation {
		fundingMarker = "бюджет"
	}
	fmt.Fprintf(&sb, "💰 *Подавався на:* %s\n", fundingMarker)
	if ab.Documents {
		sb.WriteString("📄 *Оригінали:* подані\n")
	}
	if len(ab.Quotas) > 0 {
		fmt.Fprintf(&sb, "🏷 *Квоти:* %s\n", strings.Join(ab.Quotas, ", "))
	}
	if len(ab.Coefficients) > 0 {
		fmt.Fprintf(&sb, "⚙️ *Коефіцієнти:* %s\n", strings.Join(ab.Coefficients, ", "))
	}
	if ab.OtherReq > 0 {
		fmt.Fprintf(&sb, "🔀 *Інший пріоритет:* %d\n", ab.OtherReq)
	}

	if len(ab.DetailScores) > 0 {
		sb.WriteString("\n📚 *Бали з предметів:*\n")
		for _, subj := range sortedKeys(ab.DetailScores) {
			fmt.Fprintf(&sb, "   • %s: `%g`\n",
				mdEscape(subj), ab.DetailScores[subj])
		}
	}

	kb := &tele.ReplyMarkup{}
	idStr := strconv.Itoa(ab.ID)
	rows := make([]tele.Row, 0, 4)

	if !isMaskedName(ab.Name) {
		rows = append(rows, kb.Row(kb.Data(
			"📋 Інші заяви", btnUniqueApplicantHistory, callback.Encode(idStr))))
	}

	// External links: abit-poisk + the konkurs-ball calculator.
	extra := []tele.Btn{}
	if ab.AbitLink != "" {
		extra = append(extra, kb.URL("🔎 abit-poisk", ab.AbitLink))
	}
	if ab.CalcLink != "" {
		extra = append(extra, kb.URL("🧮 Калькулятор", ab.CalcLink))
	}
	if len(extra) > 0 {
		rows = append(rows, kb.Row(extra...))
	}

	rows = append(rows, kb.Row(
		kb.Data("⬅️ До списку", btnUniqueBackToList),
		kb.Data("🏠 Меню", btnUniqueMenu),
	))
	kb.Inline(rows...)
	return sb.String(), kb
}

// buildHistoryView renders the applicant's other applications.
func buildHistoryView(ab abit.Abiturient, entries []abit.ApplicantEntry) (string, *tele.ReplyMarkup) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "📋 *%s* — інші заяви\n\n", mdEscape(ab.Name))

	if len(entries) == 0 {
		sb.WriteString("_На abit-poisk нічого не знайдено._")
	} else {
		// Sort by priority asc, then by total score desc so the most
		// relevant submissions come first.
		sort.SliceStable(entries, func(i, j int) bool {
			pi, _ := strconv.Atoi(strings.TrimSpace(entries[i].Priority))
			pj, _ := strconv.Atoi(strings.TrimSpace(entries[j].Priority))
			if pi != pj {
				if pi == 0 {
					return false
				}
				if pj == 0 {
					return true
				}
				return pi < pj
			}
			return entries[i].TotalScore > entries[j].TotalScore
		})

		limit := min(len(entries), historyLimit)
		for _, e := range entries[:limit] {
			marker := historyMarker(e.Status)
			fmt.Fprintf(&sb, "%s *%s* · %s\n",
				marker, mdEscape(truncated(e.University, 40)),
				mdEscape(truncated(e.Specialty, 40)))
			details := []string{}
			if p := strings.TrimSpace(e.Priority); p != "" {
				details = append(details, "#"+p)
			}
			if s := strings.TrimSpace(e.TotalScore); s != "" {
				details = append(details, "бал "+s)
			}
			if d := strings.TrimSpace(e.Degree); d != "" {
				details = append(details, "("+d+")")
			}
			if len(details) > 0 {
				fmt.Fprintf(&sb, "   %s\n", mdEscape(strings.Join(details, " · ")))
			}
			if s := strings.TrimSpace(e.Status); s != "" {
				fmt.Fprintf(&sb, "   _%s_\n", mdEscape(s))
			}
			sb.WriteString("\n")
		}
		if len(entries) > limit {
			fmt.Fprintf(&sb, "_…та ще %d заяв_\n", len(entries)-limit)
		}
	}

	kb := &tele.ReplyMarkup{}
	idStr := strconv.Itoa(ab.ID)
	rows := []tele.Row{
		kb.Row(kb.Data("⬅️ До абітурієнта", btnUniqueApplicant, callback.Encode(idStr))),
		kb.Row(
			kb.Data("📋 До списку", btnUniqueBackToList),
			kb.Data("🏠 Меню", btnUniqueMenu),
		),
	}
	kb.Inline(rows...)
	return sb.String(), kb
}

// historyMarker reflects the status of an abit-poisk row.
func historyMarker(status string) string {
	low := strings.ToLower(status)
	switch {
	case strings.Contains(low, "до наказу"):
		return "✅"
	case strings.Contains(low, "рекомендовано"):
		return "🟢"
	case strings.Contains(low, "допущено"):
		return "🟡"
	case strings.Contains(low, "деактивовано"):
		return "🔄"
	case strings.Contains(low, "відмова"), strings.Contains(low, "відхилено"),
		strings.Contains(low, "скасовано"), strings.Contains(low, "відраховано"):
		return "⛔"
	}
	return "•"
}

// --- helpers --------------------------------------------------------------

// findApplicant locates an Abiturient by ID in a decoded list. Linear is
// fine — the lists are <10k entries and we hit them at most once per
// callback. The program lookup before this is cache-served.
func findApplicant(abits []abit.Abiturient, id int) *abit.Abiturient {
	for i := range abits {
		if abits[i].ID == id {
			return &abits[i]
		}
	}
	return nil
}

func sortedKeys(m map[string]float64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// isMaskedName reports whether the upstream privacy-masked the applicant
// name (e.g. "Іва###" or a single word). Mirrors the same check used by
// the enrich service.
func isMaskedName(name string) bool {
	return strings.Contains(name, "###") || len(strings.Fields(name)) < 2
}

// mdEscape escapes characters reserved by Telegram's legacy Markdown.
// Legacy Markdown is used (not MarkdownV2) — fewer reserved chars,
// friendlier for Ukrainian punctuation.
func mdEscape(s string) string {
	r := strings.NewReplacer(
		"*", `\*`,
		"_", `\_`,
		"`", "'",
		"[", `\[`,
	)
	return r.Replace(s)
}

// looksLikeOsvitaURL is a coarse pre-filter for the OnText catch-all.
// Strict shape validation happens inside the parser.
func looksLikeOsvitaURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return strings.HasSuffix(u.Host, "osvita.ua") && strings.Contains(u.Path, "/y")
}
