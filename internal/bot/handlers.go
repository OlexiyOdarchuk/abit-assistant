package bot

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"

	"github.com/OlexiyOdarchuk/abit-assistant/pkg/abit"
)

const (
	greeting = `🎓 *AbitAssistant*

Це бот для абітурієнтів. Він тягне конкурсні списки з vstup.osvita.ua, декодує статуси й квоти, рахує конкурентів за бажаними фільтрами.

Що далі:
• надішли посилання на програму з vstup.osvita.ua, або
• скористайся /search ` + "`<url>`" + `
• /help — повний перелік команд`

	helpText = `*Доступні команди*

` + "`/start`" + ` — почати знову
` + "`/help`" + ` — це повідомлення
` + "`/search <url>`" + ` — розпарсити програму, дати топ-20 абітурієнтів

Можна просто скинути URL прямо в чат — бот зрозуміє.`

	searchTimeout = 90 * time.Second
	pageSize      = 10
)

func (b *Bot) handleStart(c tele.Context) error {
	return c.Send(greeting, tele.ModeMarkdown)
}

func (b *Bot) handleHelp(c tele.Context) error {
	return c.Send(helpText, tele.ModeMarkdown)
}

// handleSearch reads its URL from /search args.
func (b *Bot) handleSearch(c tele.Context) error {
	raw := strings.TrimSpace(c.Message().Payload)
	if raw == "" {
		return c.Send("Дай URL: `/search https://vstup.osvita.ua/y2025/...`", tele.ModeMarkdown)
	}
	return b.runSearch(c, raw)
}

// handleText accepts a bare URL message as an implicit /search.
func (b *Bot) handleText(c tele.Context) error {
	text := strings.TrimSpace(c.Text())
	if !looksLikeOsvitaURL(text) {
		return c.Send("Я приймаю посилання з vstup.osvita.ua. Спробуй /help.")
	}
	return b.runSearch(c, text)
}

func (b *Bot) runSearch(c tele.Context, raw string) error {
	if !looksLikeOsvitaURL(raw) {
		return c.Send("Це не схоже на посилання vstup.osvita.ua.")
	}
	if err := c.Notify(tele.Typing); err != nil {
		b.log.Debug("notify typing", "err", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()

	abits, err := b.programSvc.FetchDecoded(ctx, raw)
	if err != nil {
		return c.Send("⚠️ Не вдалося отримати дані: " + err.Error())
	}
	if len(abits) == 0 {
		return c.Send("Програма знайдена, але вона порожня.")
	}

	// Sort: server already orders by rank, but stay defensive.
	// (osvita's list IS the ranking.)

	page := 0
	msg, kb := renderPage(abits, page, raw)
	return c.Send(msg, tele.ModeMarkdown, kb, tele.NoPreview)
}

// --- pagination via inline buttons ---

// We encode the URL into the callback data: callback payloads are
// limited to 64 bytes by Telegram, so we use a short opaque key — we
// look up the URL from message text "🔎 <url>" rendered above.
var (
	btnPagePrev = tele.Btn{Unique: "page_prev"}
	btnPageNext = tele.Btn{Unique: "page_next"}
)

func (b *Bot) handlePagePrev(c tele.Context) error { return b.repaginate(c, -1) }
func (b *Bot) handlePageNext(c tele.Context) error { return b.repaginate(c, +1) }

func (b *Bot) repaginate(c tele.Context, delta int) error {
	// Callback data format: "<page>|<url>"
	parts := strings.SplitN(c.Callback().Data, "|", 2)
	if len(parts) != 2 {
		return c.Respond(&tele.CallbackResponse{Text: "Некоректні дані."})
	}
	page, err := strconv.Atoi(parts[0])
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Некоректна сторінка."})
	}
	rawURL := parts[1]

	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()

	abits, err := b.programSvc.FetchDecoded(ctx, rawURL)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "Помилка завантаження."})
	}

	newPage := page + delta
	maxPage := (len(abits) - 1) / pageSize
	if newPage < 0 {
		newPage = 0
	}
	newPage = min(newPage, maxPage)

	msg, kb := renderPage(abits, newPage, rawURL)
	if err := c.Edit(msg, tele.ModeMarkdown, kb, tele.NoPreview); err != nil {
		// Telegram returns "message is not modified" when the user spams
		// the boundary buttons; surface that quietly.
		if !strings.Contains(err.Error(), "not modified") {
			return err
		}
	}
	return c.Respond()
}

// renderPage builds a Telegram message + inline keyboard for the given
// page of decoded applicants.
func renderPage(abits []abit.Abiturient, page int, srcURL string) (string, *tele.ReplyMarkup) {
	total := len(abits)
	maxPage := (total - 1) / pageSize
	if page < 0 {
		page = 0
	}
	if page > maxPage {
		page = maxPage
	}
	start := page * pageSize
	end := min(start+pageSize, total)

	var sb strings.Builder
	fmt.Fprintf(&sb, "📋 Знайдено *%d* заяв (сторінка %d / %d)\n\n",
		total, page+1, maxPage+1)
	for i := start; i < end; i++ {
		writeApplicantLine(&sb, abits[i], i+1)
	}

	kb := &tele.ReplyMarkup{}
	prev := kb.Data("◀️", btnPagePrev.Unique, fmt.Sprintf("%d|%s", page, srcURL))
	next := kb.Data("▶️", btnPageNext.Unique, fmt.Sprintf("%d|%s", page, srcURL))
	kb.Inline(kb.Row(prev, next))
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
	sb.WriteString("\n")
	if ab.AbitLink != "" {
		fmt.Fprintf(sb, "    [abit-poisk](%s)\n", ab.AbitLink)
	}
	sb.WriteString("\n")
}

// mdEscape escapes characters reserved by Telegram's legacy Markdown.
// We use legacy Markdown (not MarkdownV2) because it has fewer reserved
// chars and is easier on Ukrainian text.
func mdEscape(s string) string {
	r := strings.NewReplacer(
		"*", `\*`,
		"_", `\_`,
		"`", "'",
		"[", `\[`,
	)
	return r.Replace(s)
}

func looksLikeOsvitaURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}
	return strings.HasSuffix(u.Host, "osvita.ua") && strings.Contains(u.Path, "/y")
}

