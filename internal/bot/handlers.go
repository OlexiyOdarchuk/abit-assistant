package bot

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/bot/callback"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/storage"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/visualizer"
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
	// fsmKeyFromDiscover marks a viewing session that was opened from the
	// "where can I get in" results, so every re-render of the summary keeps
	// offering the "back to results" button.
	fsmKeyFromDiscover = "from_discover"
)

// Search list display modes — toggled by the user from the results page.
const (
	modeAll         = "all"
	modeCompetitors = "competitors"
)

// --- Command handlers -----------------------------------------------------

func (b *Bot) handleStart(c tele.Context) error {
	payload := strings.TrimSpace(c.Message().Payload)
	switch {
	case strings.HasPrefix(payload, "share_"):
		return b.handleStartShareClone(c, strings.TrimPrefix(payload, "share_"))
	case strings.HasPrefix(payload, "list_"):
		// Legacy format from before share tokens existed. Refuse cleanly
		// instead of resolving — the owner needs to send a fresh link.
		return errors.New("посилання старого формату вже не діє — попроси нове")
	}
	return b.renderMenu(c)
}

// handleStartShareClone resolves a share token to its source saved
// list, copies the snapshot into the current user's /lists, and shows
// it as if they had just searched. Anyone with the token can clone —
// that's the point — but tokens are 128-bit random, not enumerable.
func (b *Bot) handleStartShareClone(c tele.Context, token string) error {
	if token == "" || len(token) < 16 {
		return errors.New("некоректне посилання")
	}

	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()

	source, err := b.store.GetSavedListByToken(ctx, token)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return errors.New("список не знайдено або був видалений")
		}
		return err
	}
	if source.Program == nil {
		return errors.New("список пошкоджений")
	}

	uid := senderID(c)
	if err := b.store.UpsertUser(ctx, uid); err != nil {
		return err
	}
	name := "Копія: " + source.Name
	if _, err := b.store.SaveList(ctx, uid, name, source.URL, source.Program); err != nil {
		return fmt.Errorf("не вдалося зберегти копію: %w", err)
	}

	intro := fmt.Sprintf("📥 Отримано спільний аналіз: *%s*\nЗбережено в /lists.",
		mdEscape(source.Name))
	if err := c.Send(intro, tele.ModeMarkdown); err != nil {
		return err
	}
	return b.renderSummary(c, source.Program, source.URL, false)
}
func (b *Bot) handleMenu(c tele.Context) error  { return b.renderMenu(c) }
func (b *Bot) handleHelp(c tele.Context) error  { return c.Send(helpText, tele.ModeMarkdown) }
func (b *Bot) handleAbout(c tele.Context) error { return b.renderAbout(c) }

func (b *Bot) handleCancel(c tele.Context) error {
	if err := b.fsm.Clear(context.Background(), senderID(c)); err != nil {
		return fmt.Errorf("не вдалося очистити стан: %w", err)
	}
	return c.Send("🚫 Поточну дію скасовано. /menu — головне меню")
}

func (b *Bot) handleProfile(c tele.Context) error {
	if err := requirePrivateChat(c); err != nil {
		return err
	}
	return b.renderProfile(c)
}

func (b *Bot) handleLists(c tele.Context) error {
	if err := requirePrivateChat(c); err != nil {
		return err
	}
	return b.renderSavedLists(c)
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
	case fsmStateAdminBroadcast:
		return b.handleAdminBroadcastText(c, text)
	}
	if looksLikeOsvitaURL(text) {
		return b.runSearch(c, text)
	}
	return c.Send("Не зрозумів. /menu — головне меню, /help — список команд.")
}

// --- Callback handlers ----------------------------------------------------

// Top-level navigation buttons. Each clears any in-flight FSM state so
// a user who taps "⬅️ Меню" while mid-input doesn't have their next
// free-text message hijacked by a stale handler (admin broadcast, NMT
// score entry, creative score, URL prompt).
func (b *Bot) handleMenuCB(c tele.Context) error   { b.clearTransientFSM(c); return b.renderMenu(c) }
func (b *Bot) handleAboutCB(c tele.Context) error  { b.clearTransientFSM(c); return b.renderAbout(c) }
func (b *Bot) handleSearchCB(c tele.Context) error { b.clearTransientFSM(c); return b.askForURL(c) }
func (b *Bot) handleProfileCB(c tele.Context) error {
	b.clearTransientFSM(c)
	return b.renderProfile(c)
}
func (b *Bot) handleListsCB(c tele.Context) error {
	b.clearTransientFSM(c)
	return b.renderSavedLists(c)
}

// clearTransientFSM wipes any text-waiting FSM state — admin.broadcast.*,
// profile.enter_*, search.waiting_url. Persistent "viewing" state is
// kept (deeper screens like Detail/History need to come back to it).
func (b *Bot) clearTransientFSM(c tele.Context) {
	uid := senderID(c)
	if uid == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	state, err := b.fsm.Get(ctx, uid)
	if err != nil || state.Name == "" || state.Name == fsmStateViewing {
		return
	}
	if err := b.fsm.Clear(context.Background(), uid); err != nil {
		b.log.Warn("clear transient fsm", "err", err)
	}
}

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

// handleChartCB renders a histogram of the program's score distribution
// and sends it as a Telegram photo. Coloring marks competitors red and
// non-competitors green relative to the user's own rating.
func (b *Bot) handleChartCB(c tele.Context) error {
	rawURL, _, _, err := b.viewingState(c)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()

	prog, err := b.programSvc.Fetch(ctx, rawURL)
	if err != nil {
		return fmt.Errorf("не вдалося завантажити дані: %w", err)
	}
	abits := abit.Decode(prog)
	userScore := b.userRating(ctx, senderID(c), prog)

	_ = c.Notify(tele.UploadingPhoto)
	png, err := visualizer.Histogram(abits, userScore, 5)
	if err != nil {
		return fmt.Errorf("не вдалося згенерувати графік: %w", err)
	}

	photo := &tele.Photo{
		File:    tele.File{FileReader: bytes.NewReader(png)},
		Caption: fmt.Sprintf("📊 *%s* — %s", mdEscape(prog.UniversityName), mdEscape(prog.ProgramName)),
	}
	if err := c.Send(photo, tele.ModeMarkdown); err != nil {
		return err
	}
	return c.Respond()
}

// handleSaveListCB persists the current program snapshot under the user
// for later retrieval via /lists. Returns a quick toast — no full screen
// change — so the user can continue browsing.
func (b *Bot) handleSaveListCB(c tele.Context) error {
	rawURL, _, _, err := b.viewingState(c)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()

	prog, err := b.programSvc.Fetch(ctx, rawURL)
	if err != nil {
		return fmt.Errorf("не вдалося завантажити дані: %w", err)
	}
	name := savedListName(prog)
	if _, err := b.store.SaveList(ctx, senderID(c), name, rawURL, prog); err != nil {
		return fmt.Errorf("не вдалося зберегти: %w", err)
	}
	return c.Respond(&tele.CallbackResponse{
		Text:      "✅ Збережено в /lists",
		ShowAlert: false,
	})
}

// savedListName builds a stable, trimmed label for the lists view.
func savedListName(prog *abit.Program) string {
	name := fmt.Sprintf("%s — %s", prog.UniversityName, prog.ProgramName)
	if r := []rune(name); len(r) > 80 {
		name = string(r[:77]) + "…"
	}
	return name
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
	prog, err := b.programSvc.Fetch(ctx, rawURL)
	if err != nil {
		return fmt.Errorf("не вдалося завантажити дані: %w", err)
	}
	abits := abit.Decode(prog)
	ab := findApplicant(abits, id)
	if ab == nil {
		return errors.New("абітурієнта не знайдено в поточному списку")
	}

	userScore := b.userRating(ctx, senderID(c), prog)
	text, kb := buildApplicantDetail(*ab, userScore)
	return renderOrEdit(c, text, tele.ModeMarkdown, kb, tele.NoPreview)
}

// userRating reads the user's NMT + settings and computes their rating
// against prog. Returns 0 when the profile isn't filled.
func (b *Bot) userRating(ctx context.Context, uid int64, prog *abit.Program) float64 {
	nmt, _ := b.store.GetUserNMT(ctx, uid)
	settings, _ := b.store.GetUserSettings(ctx, uid)
	return abit.ComputeRating(prog, abit.RatingInput{
		NMT:           map[string]float64(nmt),
		CreativeScore: float64(settings.CreativeScorePrediction),
	})
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

// showSummary fetches the program (cache-aware) and renders the analysis.
// Use when triggered by /search or a fresh URL — for already-loaded
// programs (e.g. saved lists) call renderSummary directly.
func (b *Bot) showSummary(c tele.Context, rawURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()

	prog, err := b.programSvc.Fetch(ctx, rawURL)
	if err != nil {
		return fmt.Errorf("не вдалося отримати дані: %w", err)
	}
	return b.renderSummary(c, prog, rawURL, false)
}

// showSummaryFromDiscover is showSummary for a program opened from the
// "where can I get in" results — the summary then offers a "back to results"
// button (the discovery FSM state is overwritten here, so back re-runs it).
func (b *Bot) showSummaryFromDiscover(c tele.Context, rawURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()
	prog, err := b.programSvc.Fetch(ctx, rawURL)
	if err != nil {
		return fmt.Errorf("не вдалося отримати дані: %w", err)
	}
	return b.renderSummary(c, prog, rawURL, true)
}

// renderSummary is the common path: takes an already-loaded Program +
// the URL it came from, reads the user's profile, computes the rating
// and analysis, persists FSM, edits the message.
func (b *Bot) renderSummary(c tele.Context, prog *abit.Program, rawURL string, backToDiscover bool) error {
	abits := abit.Decode(prog)
	if len(abits) == 0 {
		return errors.New("програма знайдена, але список порожній")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

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
	})

	// Recover the "came from discover" flag so re-renders (e.g. back from the
	// refine screen) keep the "back to results" button.
	prevState, _ := b.fsm.Get(ctx, uid)
	fromDiscover := backToDiscover
	if prevState.Name == fsmStateViewing && prevState.Get(fsmKeyURL) == rawURL {
		if v, _ := prevState.Data[fsmKeyFromDiscover].(bool); v {
			fromDiscover = true
		}
	}

	analysis := abit.Analyze(prog, abits, abit.AnalyzeInput{
		UserScore:  userScore,
		UserQuotas: settings.Quotas,
	})

	data := map[string]any{
		fsmKeyURL:  rawURL,
		fsmKeyPage: 0,
		fsmKeyMode: modeAll,
	}
	if fromDiscover {
		data[fsmKeyFromDiscover] = true
	}
	if err := b.fsm.Set(context.Background(), uid, fsmStateViewing, data); err != nil {
		b.log.Warn("fsm set failed", "err", err)
	}

	text, kb := buildSummaryView(prog, analysis, fromDiscover)
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
	userScore := b.userRating(ctx, uid, prog)

	// Keep the "came from discover" flag sticky as the user pages, when the
	// previous viewing state is for the same URL.
	prevState, _ := b.fsm.Get(ctx, uid)
	fromDiscover := false
	if prevState.Name == fsmStateViewing && prevState.Get(fsmKeyURL) == rawURL {
		fromDiscover, _ = prevState.Data[fsmKeyFromDiscover].(bool)
	}

	// Competitors mode degrades to "all" when we can't tell who is who.
	if mode == modeCompetitors && userScore == 0 {
		mode = modeAll
	}

	view := abits
	if mode == modeCompetitors {
		view = filterCompetitors(abits, userScore)
	}
	if len(view) == 0 {
		view = abits
		mode = modeAll
	}

	if page < 0 {
		page = 0
	}
	maxPage := (len(view) - 1) / pageSize
	page = min(page, maxPage)

	data := map[string]any{
		fsmKeyURL:  rawURL,
		fsmKeyPage: page,
		fsmKeyMode: mode,
	}
	if fromDiscover {
		data[fsmKeyFromDiscover] = true
	}
	if err := b.fsm.Set(context.Background(), uid, fsmStateViewing, data); err != nil {
		b.log.Warn("fsm set failed", "err", err)
	}

	text, kb := buildResultsView(prog, view, abits, page, userScore, mode)
	return renderOrEdit(c, text, tele.ModeMarkdown, kb, tele.NoPreview)
}

// filterCompetitors returns applicants that realistically compete with
// the user for a budget seat.
func filterCompetitors(abits []abit.Abiturient, mine float64) []abit.Abiturient {
	out := make([]abit.Abiturient, 0)
	for _, ab := range abits {
		if abit.IsCompetitor(ab, mine) {
			out = append(out, ab)
		}
	}
	return out
}

// viewingState reads the URL, page and mode from FSM. Returns a clear error
// if the user is not in the search.viewing state — handlers should propagate
// this to the user.
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
func buildSummaryView(prog *abit.Program, an abit.Analysis, backToDiscover bool) (string, *tele.ReplyMarkup) {
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
		switch {
		case an.Cutoff > 0:
			// Ground truth published — the cutoff is the headline number.
			fmt.Fprintf(&sb, "   • 🎯 Прохідний бал (за результатами): *%.2f*\n", an.Cutoff)
			if an.SeatsFilled > 0 {
				fmt.Fprintf(&sb, "   • Зараховано на бюджет: *%d*\n", an.SeatsFilled)
			}
		case an.BudgetTotal > 0:
			// Only meaningful when we actually know the seat count; when the
			// volume wasn't parsed, "Вільних місць: 0" is misleading (the
			// advice/warning already explain the volume is unknown).
			fmt.Fprintf(&sb, "   • Вільних місць: *%d*\n", an.RemainingSpots)
		}

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
		for _, w := range an.Warnings {
			switch w {
			case "license-volume-missing":
				sb.WriteString("\n⚠️ Ліцензований обсяг не вдалося розпарсити — місце вище — оцінка лише за рангом.")
			case "budget-volume-is-ceiling":
				sb.WriteString("\n⚠️ Кількість місць — це *максимальний* обсяг держзамовлення (стеля). Реальних бюджетних місць може бути менше, тож шанс — оптимістична оцінка.")
			case "field-undersubscribed":
				sb.WriteString("\nℹ️ Заяв поки менше, ніж бюджетних місць — тож майже всі проходять. Якщо кампанія щойно почалась, більшість заяв подадуть в останні дні й прохідний бал ще зросте.")
			}
		}
		if prog.SourceAsOf != "" {
			fmt.Fprintf(&sb, "\n\n🕒 _Дані osvita станом на %s._", mdEscape(prog.SourceAsOf))
		}
	}

	kb := &tele.ReplyMarkup{}
	rows := []tele.Row{
		kb.Row(kb.Data("📋 Дивитись список", btnUniqueViewList)),
		kb.Row(
			kb.Data("📊 Графік балів", btnUniqueChart),
			kb.Data("💾 Зберегти", btnUniqueSaveList),
		),
	}
	// Priority simulation only makes sense once the user's score is known AND
	// we're still estimating from the competitor field. When osvita has
	// published the real cutoff (an.Cutoff > 0) the verdict is ground truth —
	// removing competitors who place elsewhere can't change it, so the
	// (slow, rate-sensitive) simulation would just report "nothing changed".
	if an.UserScore > 0 && an.Cutoff <= 0 {
		rows = append(rows, kb.Row(kb.Data("🔮 Уточнити: хто піде деінде", btnUniqueRefine)))
	}
	// When opened from "where can I get in", offer a way back to that list.
	if backToDiscover {
		rows = append(rows, kb.Row(kb.Data("⬅️ До результатів", btnUniqueDiscoverBack)))
	}
	rows = append(rows, kb.Row(kb.Data("⬅️ Меню", btnUniqueMenu)))
	kb.Inline(rows...)
	return sb.String(), kb
}

// handleRefine runs the priority simulation for the program in the current
// viewing state: it removes competitors who place higher elsewhere and shows
// the refined chance. abit-poisk lookups make it slow, so it runs under the
// search timeout with a typing indicator.
func (b *Bot) handleRefine(c tele.Context) error {
	rawURL, _, _, err := b.viewingState(c)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()

	prog, err := b.programSvc.Fetch(ctx, rawURL)
	if err != nil {
		return fmt.Errorf("не вдалося завантажити дані: %w", err)
	}
	uid := senderID(c)
	userScore := b.userRating(ctx, uid, prog)
	if userScore == 0 {
		return errors.New("спочатку заповни /profile — без власного балу немає що уточнювати")
	}
	settings, _ := b.store.GetUserSettings(ctx, uid)

	_ = c.Notify(tele.Typing)
	res, err := b.simSvc.Simulate(ctx, prog, abit.Decode(prog), service.SimInput{
		UserScore:  userScore,
		UserQuotas: settings.Quotas,
	})
	if err != nil {
		return fmt.Errorf("симуляція не вдалася: %w", err)
	}
	text, kb := buildRefineView(prog, res)
	return renderOrEdit(c, text, tele.ModeMarkdown, kb, tele.NoPreview)
}

// buildRefineView renders the priority-simulation result: baseline vs
// refined chance, who was removed and why.
func buildRefineView(prog *abit.Program, res service.SimResult) (string, *tele.ReplyMarkup) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "🔮 *Уточнення шансів* — %s\n\n", mdEscape(prog.ProgramName))

	if len(res.Departures) == 0 {
		sb.WriteString("Поки нікого не вдалося зняти з конкуренції: ніхто з тих, хто вище за тебе, ще не отримав рекомендацію на вищий пріоритет деінде.\n\n")
		sb.WriteString("_Це працює, коли вже йдуть хвилі рекомендацій. До них — усі ще «Допущено», і знімати нема кого._")
		if res.LookedUp > 0 {
			fmt.Fprintf(&sb, "\n\n🔍 Перевірено конкурентів: %d", res.LookedUp)
		}
	} else {
		fmt.Fprintf(&sb, "🎯 *%s → %s*\n", res.Baseline.Chance.Emoji()+" "+res.Baseline.Chance.Label(),
			res.Refined.Chance.Emoji()+" "+res.Refined.Chance.Label())
		if res.Baseline.MyRealRank > 0 && res.Refined.MyRealRank > 0 {
			fmt.Fprintf(&sb, "🏆 Місце: %d → *%d*\n", res.Baseline.MyRealRank, res.Refined.MyRealRank)
		}
		confirmed, predicted := 0, 0
		for _, d := range res.Departures {
			if d.Predicted {
				predicted++
			} else {
				confirmed++
			}
		}
		fmt.Fprintf(&sb, "📉 Підуть на вищий пріоритет деінде: *%d*", len(res.Departures))
		if predicted > 0 {
			fmt.Fprintf(&sb, " (✅ %d підтверджено · 🔮 %d прогноз)", confirmed, predicted)
		}
		sb.WriteString("\n\n")
		shown := res.Departures
		if len(shown) > 8 {
			shown = shown[:8]
		}
		for _, d := range shown {
			where := d.University
			if where == "" {
				where = "інший ЗВО"
			}
			mark := "✅"
			if d.Predicted {
				mark = "🔮"
			}
			fmt.Fprintf(&sb, "  %s %s → %s (пріоритет %d)\n", mark, mdEscape(d.Name), mdEscape(where), d.Priority)
		}
		if len(res.Departures) > len(shown) {
			fmt.Fprintf(&sb, "  …і ще %d\n", len(res.Departures)-len(shown))
		}
		if predicted > 0 {
			sb.WriteString("\n_🔮 прогноз — за балом проходить на свій вищий пріоритет (ще до офіційних рекомендацій)._")
		}
	}
	if res.Masked > 0 {
		fmt.Fprintf(&sb, "\n🙈 Прихованих імен (не перевірити): %d", res.Masked)
	}
	if res.Capped {
		sb.WriteString("\n⚠️ Перевірено лише найближчих конкурентів (список великий).")
	}

	kb := &tele.ReplyMarkup{}
	kb.Inline(kb.Row(kb.Data("⬅️ Назад до аналізу", btnUniqueSummary)))
	return sb.String(), kb
}

// countCompetitors counts REAL competitors (priority-1 / enrolled who rank
// above) — matching the 🔴 markers in the list. Potential (🟡) leavers are
// excluded so the number isn't inflated by high-scored, low-priority
// applicants who almost always place elsewhere.
func countCompetitors(abits []abit.Abiturient, mine float64) int {
	if mine <= 0 {
		return 0
	}
	n := 0
	for _, ab := range abits {
		if abit.CompetitorTier(ab, mine) == abit.CompetitorReal {
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
		switch abit.CompetitorTier(ab, userScore) {
		case abit.CompetitorReal:
			threatMarker = "🔴 "
		case abit.CompetitorPotential:
			threatMarker = "🟡 "
		default:
			threatMarker = "🟢 "
		}
	}
	statusM := statusMarker(ab.Status)
	label := fmt.Sprintf("%d. %s%s%s — %.1f",
		rank, threatMarker, statusM, ab.Name, ab.Score)
	// Cyrillic is multi-byte in UTF-8 — truncate by rune, not by byte,
	// or Telegram rejects the keyboard with an invalid-UTF-8 error.
	return truncateRunes(label, 60)
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
func buildApplicantDetail(ab abit.Abiturient, userScore float64) (string, *tele.ReplyMarkup) {
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
	// OtherReq makes sense only when paired with a RecType — without it
	// the bare "Інший пріоритет: N" looks like noise to the user.
	if ab.OtherReq > 0 && ab.RecType != "" {
		fmt.Fprintf(&sb, "🔀 *Інший пріоритет:* %d\n", ab.OtherReq)
	}

	if len(ab.DetailScores) > 0 {
		sb.WriteString("\n📚 *Бали з предметів:*\n")
		for _, subj := range sortedKeys(ab.DetailScores) {
			fmt.Fprintf(&sb, "   • %s: `%g`\n",
				mdEscape(subj), ab.DetailScores[subj])
		}
	}

	// Verdict line — priority-aware, because adaptive placement sends an
	// applicant to their highest-priority program where they qualify.
	if userScore > 0 {
		switch abit.CompetitorTier(ab, userScore) {
		case abit.CompetitorReal:
			sb.WriteString("\n🔴 _Реальний конкурент за твій бюджетний вступ_\n")
		case abit.CompetitorPotential:
			fmt.Fprintf(&sb, "\n🟡 _Вище за балом, але пріоритет %d — імовірно пройде на вищий пріоритет і звільнить це місце. Натисни «🔮 хто піде деінде», щоб перевірити._\n", ab.Priority)
		default:
			sb.WriteString("\n🟢 _Не конкурент_\n")
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

// isMaskedName reports whether upstream privacy-masked the applicant
// name. osvita.ua replaces consenting-out applicants with "Іва###"
// patterns — that's the only signal worth treating as masked. A single-
// token name (e.g. legitimately just a surname) is a valid real name
// and should NOT be blocked from abit-poisk lookups.
func isMaskedName(name string) bool {
	return strings.Contains(name, "###")
}

// mdEscape escapes characters reserved by Telegram's legacy Markdown.
// Legacy Markdown is used (not MarkdownV2) — fewer reserved chars,
// friendlier for Ukrainian punctuation. The backslash is escaped FIRST
// so the others' escape sequences aren't double-mangled.
func mdEscape(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		"*", `\*`,
		"_", `\_`,
		"`", "\\`",
		"[", `\[`,
		"]", `\]`,
	)
	return r.Replace(s)
}

// isPrivateChat reports whether the update came from a 1-on-1 chat.
// Group/channel chats render to many people, so anything that touches
// the caller's profile (their NMT, their settings) belongs in a DM.
func isPrivateChat(c tele.Context) bool {
	if c == nil || c.Chat() == nil {
		return false
	}
	return c.Chat().Type == tele.ChatPrivate
}

// requirePrivateChat gates command handlers — returns a friendly error
// in non-private chats. handleText, which fires implicitly, uses the
// silent isPrivateChat variant instead.
func requirePrivateChat(c tele.Context) error {
	if isPrivateChat(c) {
		return nil
	}
	return errors.New("ця команда працює лише в особистих повідомленнях боту")
}

// looksLikeOsvitaURL is the pre-filter for the OnText catch-all and
// /search payload. Strict on host + scheme to avoid spoof domains
// (e.g. `phishing-osvita.ua`) and SSRF-adjacent shapes.
func looksLikeOsvitaURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return false
	}
	if u.Host != "vstup.osvita.ua" {
		return false
	}
	return strings.Contains(u.Path, "/y")
}
