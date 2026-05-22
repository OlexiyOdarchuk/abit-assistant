package bot

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/bot/callback"
	"github.com/OlexiyOdarchuk/abit-assistant/pkg/abit"
)

const (
	pageSize      = 10
	searchTimeout = 90 * time.Second

	// FSM states. Convention: <feature>.<step>.
	fsmStateWaitingURL = "search.waiting_url"
	fsmStateViewing    = "search.viewing"
)

// FSM data keys for the search flow.
const (
	fsmKeyURL  = "url"
	fsmKeyPage = "page"
)

// --- Command handlers -----------------------------------------------------

func (b *Bot) handleStart(c tele.Context) error  { return b.renderMenu(c) }
func (b *Bot) handleMenu(c tele.Context) error   { return b.renderMenu(c) }
func (b *Bot) handleHelp(c tele.Context) error   { return c.Send(helpText, tele.ModeMarkdown) }
func (b *Bot) handleAbout(c tele.Context) error  { return b.renderAbout(c) }

func (b *Bot) handleCancel(c tele.Context) error {
	if err := b.fsm.Clear(context.Background(), senderID(c)); err != nil {
		return fmt.Errorf("не вдалося очистити стан: %w", err)
	}
	return c.Send("🚫 Поточну дію скасовано. /menu — головне меню")
}

func (b *Bot) handleProfile(c tele.Context) error {
	return c.Send("👤 *Профіль* — у розробці. Поки можеш одразу аналізувати програми через /search.",
		tele.ModeMarkdown, backToMenuKeyboard())
}

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

// handleText is the OnText catch-all. It routes the message either to
// active FSM state, or — if the message looks like an osvita URL —
// to an implicit /search.
func (b *Bot) handleText(c tele.Context) error {
	text := strings.TrimSpace(c.Text())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	state, err := b.fsm.Get(ctx, senderID(c))
	cancel()
	if err != nil {
		b.log.Warn("fsm get failed", "err", err, "user_id", senderID(c))
	}

	if state.Name == fsmStateWaitingURL {
		return b.runSearch(c, text)
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
func (b *Bot) handleProfileCB(c tele.Context) error { return b.handleProfile(c) }
func (b *Bot) handleListsCB(c tele.Context) error   { return b.handleLists(c) }

func (b *Bot) handlePagePrev(c tele.Context) error { return b.flipPage(c, -1) }
func (b *Bot) handlePageNext(c tele.Context) error { return b.flipPage(c, +1) }

func (b *Bot) flipPage(c tele.Context, delta int) error {
	uid := senderID(c)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	state, err := b.fsm.Get(ctx, uid)
	cancel()
	if err != nil {
		return fmt.Errorf("не вдалося прочитати стан: %w", err)
	}
	if state.Name != fsmStateViewing {
		return errors.New("сесію переглядання втрачено — почни знову з /search")
	}
	rawURL := state.Get(fsmKeyURL)
	curPage, _ := state.Data[fsmKeyPage].(float64) // JSON numbers come back as float64
	newPage := int(curPage) + delta

	args := callback.From(c)
	if v, ok := args.Int(0); ok {
		// Explicit page from callback — defends against state drift.
		newPage = v + delta
	}

	return b.showResultsPage(c, rawURL, newPage)
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
	return b.showResultsPage(c, rawURL, 0)
}

// showResultsPage runs (or re-runs through cache) the program lookup and
// renders the requested page in place. Also updates FSM so pagination
// buttons know which URL to flip.
func (b *Bot) showResultsPage(c tele.Context, rawURL string, page int) error {
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

	if page < 0 {
		page = 0
	}
	maxPage := (len(abits) - 1) / pageSize
	page = min(page, maxPage)

	if err := b.fsm.Set(context.Background(), senderID(c), fsmStateViewing, map[string]any{
		fsmKeyURL:  rawURL,
		fsmKeyPage: page,
	}); err != nil {
		b.log.Warn("fsm set failed", "err", err)
	}

	text, kb := buildResultsView(prog, abits, page)
	return renderOrEdit(c, text, tele.ModeMarkdown, kb, tele.NoPreview)
}

// --- View builder ---------------------------------------------------------

func buildResultsView(prog *abit.Program, abits []abit.Abiturient, page int) (string, *tele.ReplyMarkup) {
	total := len(abits)
	maxPage := (total - 1) / pageSize
	start := page * pageSize
	end := min(start+pageSize, total)

	var sb strings.Builder
	fmt.Fprintf(&sb, "📋 *%s* — %s\n",
		mdEscape(prog.UniversityName), mdEscape(prog.ProgramName))
	fmt.Fprintf(&sb, "Знайдено *%d* заяв. Сторінка %d / %d\n\n",
		total, page+1, maxPage+1)
	for i := start; i < end; i++ {
		writeApplicantLine(&sb, abits[i], i+1)
	}

	kb := &tele.ReplyMarkup{}
	rows := []tele.Row{}
	if maxPage > 0 {
		nav := []tele.Btn{}
		if page > 0 {
			nav = append(nav, kb.Data("◀️", btnUniquePagePrev, callback.Encode(formatPage(page))))
		}
		nav = append(nav, kb.Data(fmt.Sprintf("%d / %d", page+1, maxPage+1), "noop"))
		if page < maxPage {
			nav = append(nav, kb.Data("▶️", btnUniquePageNext, callback.Encode(formatPage(page))))
		}
		rows = append(rows, kb.Row(nav...))
	}
	rows = append(rows, kb.Row(kb.Data("⬅️ Меню", btnUniqueMenu)))
	kb.Inline(rows...)
	return sb.String(), kb
}

func writeApplicantLine(sb *strings.Builder, ab abit.Abiturient, rank int) {
	fmt.Fprintf(sb, "*%d.* %s — `%.3f`\n", rank, mdEscape(ab.Name), ab.Score)
	fmt.Fprintf(sb, "    %s", mdEscape(ab.Status))
	if ab.RecType != "" {
		fmt.Fprintf(sb, " · %s", mdEscape(ab.RecType))
	}
	if len(ab.Quotas) > 0 {
		fmt.Fprintf(sb, " · 🏷 %s", strings.Join(ab.Quotas, ", "))
	}
	if ab.Documents {
		sb.WriteString(" · 📄")
	}
	sb.WriteString("\n\n")
}

func formatPage(n int) string { return fmt.Sprintf("%d", n) }

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
