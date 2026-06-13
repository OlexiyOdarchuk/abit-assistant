package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/bot/callback"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/storage"
)

const listsTimeout = 5 * time.Second

// renderSavedLists shows the user's saved analyses, newest first.
func (b *Bot) renderSavedLists(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), listsTimeout)
	defer cancel()

	lists, err := b.store.ListSavedLists(ctx, senderID(c))
	if err != nil {
		return fmt.Errorf("не вдалося прочитати списки: %w", err)
	}
	text, kb := buildSavedListsView(lists)
	return renderOrEdit(c, text, tele.ModeMarkdown, kb)
}

// handleListManage opens the per-list action screen.
func (b *Bot) handleListManage(c tele.Context) error {
	id, ok := callback.From(c).Int64(0)
	if !ok {
		return errors.New("втрачено ID списку")
	}
	ctx, cancel := context.WithTimeout(context.Background(), listsTimeout)
	defer cancel()

	item, err := b.loadOwnedList(ctx, c, id)
	if err != nil {
		return err
	}
	text, kb := buildListManageView(item)
	return renderOrEdit(c, text, tele.ModeMarkdown, kb)
}

// handleListView opens the saved program as a summary screen — same
// rendering as a fresh /search, but the data comes from the snapshot,
// so there's no network call.
func (b *Bot) handleListView(c tele.Context) error {
	id, ok := callback.From(c).Int64(0)
	if !ok {
		return errors.New("втрачено ID списку")
	}
	ctx, cancel := context.WithTimeout(context.Background(), listsTimeout)
	defer cancel()

	item, err := b.loadOwnedList(ctx, c, id)
	if err != nil {
		return err
	}
	if item.Program == nil {
		return errors.New("збережений список пошкоджений")
	}
	return b.renderSummary(c, item.Program, item.URL)
}

// handleListDelete asks the user to confirm deletion. Cheap safeguard
// — saved lists carry the full snapshot, so an accidental tap is
// genuinely lossy.
func (b *Bot) handleListDelete(c tele.Context) error {
	id, ok := callback.From(c).Int64(0)
	if !ok {
		return errors.New("втрачено ID списку")
	}
	ctx, cancel := context.WithTimeout(context.Background(), listsTimeout)
	defer cancel()

	item, err := b.loadOwnedList(ctx, c, id)
	if err != nil {
		return err
	}
	text, kb := buildDeleteConfirmView(item)
	return renderOrEdit(c, text, tele.ModeMarkdown, kb)
}

// handleListDeleteConfirm is the second step — actually drops the row.
func (b *Bot) handleListDeleteConfirm(c tele.Context) error {
	id, ok := callback.From(c).Int64(0)
	if !ok {
		return errors.New("втрачено ID списку")
	}
	ctx, cancel := context.WithTimeout(context.Background(), listsTimeout)
	defer cancel()

	if _, err := b.loadOwnedList(ctx, c, id); err != nil {
		return err
	}
	if err := b.store.DeleteSavedList(ctx, id); err != nil {
		return fmt.Errorf("не вдалося видалити: %w", err)
	}
	_ = c.Respond(&tele.CallbackResponse{Text: "🗑 Видалено"})
	return b.renderSavedLists(c)
}

// handleListRefresh re-fetches the program from osvita (bypassing the
// program cache), updates the saved snapshot in place, and renders a
// before/after diff for the user. Same id, same created_at — only the
// data column changes, so the list keeps its place in history.
func (b *Bot) handleListRefresh(c tele.Context) error {
	id, ok := callback.From(c).Int64(0)
	if !ok {
		return errors.New("втрачено ID списку")
	}

	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()

	item, err := b.loadOwnedList(ctx, c, id)
	if err != nil {
		return err
	}
	if item.Program == nil {
		return errors.New("збережений список пошкоджений")
	}
	if item.URL == "" {
		return errors.New("у списку немає джерела для оновлення")
	}

	// Build the inputs that go into both old and new analyses.
	uid := senderID(c)
	nmt, _ := b.store.GetUserNMT(ctx, uid)
	settings, _ := b.store.GetUserSettings(ctx, uid)
	ratingIn := abit.RatingInput{
		NMT:           map[string]float64(nmt),
		CreativeScore: float64(settings.CreativeScorePrediction),
		RegionCoef:    settings.RegionCoef,
	}

	oldRating := abit.ComputeRating(item.Program, ratingIn)
	oldAnalysis := abit.Analyze(item.Program, abit.Decode(item.Program),
		abit.AnalyzeInput{UserScore: oldRating, UserQuotas: settings.Quotas})
	oldAt := item.CreatedAt

	_ = c.Notify(tele.Typing)
	fresh, err := b.programSvc.Refresh(ctx, item.URL)
	if err != nil {
		return fmt.Errorf("не вдалося оновити: %w", err)
	}
	if err := b.store.UpdateSavedListProgram(ctx, id, fresh); err != nil {
		return fmt.Errorf("не вдалося зберегти оновлення: %w", err)
	}

	newRating := abit.ComputeRating(fresh, ratingIn)
	newAnalysis := abit.Analyze(fresh, abit.Decode(fresh),
		abit.AnalyzeInput{UserScore: newRating, UserQuotas: settings.Quotas})

	text, kb := buildRefreshDiffView(id, item.Name, oldAt, time.Now(), oldAnalysis, newAnalysis)
	return renderOrEdit(c, text, tele.ModeMarkdown, kb, tele.NoPreview)
}

// handleListShare renders a deep link keyed by the list's opaque share
// token (not the numeric id). The token is unguessable — only the
// owner who taps "Поділитись" reveals it — so recipients can't brute
// force their way into other users' snapshots.
func (b *Bot) handleListShare(c tele.Context) error {
	id, ok := callback.From(c).Int64(0)
	if !ok {
		return errors.New("втрачено ID списку")
	}

	ctx, cancel := context.WithTimeout(context.Background(), listsTimeout)
	defer cancel()
	item, err := b.loadOwnedList(ctx, c, id)
	if err != nil {
		return err
	}
	if item.ShareToken == "" {
		return errors.New("у списку немає токена шерингу — пересохрани його")
	}

	if b.tg.Me == nil || b.tg.Me.Username == "" {
		return errors.New("не вдалося отримати username бота")
	}
	link := fmt.Sprintf("https://t.me/%s?start=share_%s",
		b.tg.Me.Username, item.ShareToken)

	text := fmt.Sprintf(
		"🔗 *Поділитися аналізом*\n\n"+
			"`%s`\n\n"+
			"Будь-хто, хто перейде за посиланням, отримає копію цього аналізу у свій /lists.",
		link)

	idStr := strconv.FormatInt(id, 10)
	shareURL := "https://t.me/share/url?url=" + url.QueryEscape(link)
	kb := &tele.ReplyMarkup{}
	kb.Inline(
		kb.Row(kb.URL("📤 Надіслати в Telegram", shareURL)),
		kb.Row(kb.Data("⬅️ Назад", btnUniqueListManage, callback.Encode(idStr))),
	)
	return renderOrEdit(c, text, tele.ModeMarkdown, kb, tele.NoPreview)
}

// handleListExport sends the snapshot as a downloadable JSON file.
func (b *Bot) handleListExport(c tele.Context) error {
	id, ok := callback.From(c).Int64(0)
	if !ok {
		return errors.New("втрачено ID списку")
	}
	ctx, cancel := context.WithTimeout(context.Background(), listsTimeout)
	defer cancel()
	item, err := b.loadOwnedList(ctx, c, id)
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(item.Program, "", "  ")
	if err != nil {
		return fmt.Errorf("не вдалося серіалізувати: %w", err)
	}
	doc := &tele.Document{
		File:     tele.File{FileReader: bytes.NewReader(raw)},
		FileName: fmt.Sprintf("list-%d.json", id),
		Caption:  fmt.Sprintf("📤 %s — повний експорт", item.Name),
	}
	if err := c.Send(doc); err != nil {
		return err
	}
	return c.Respond()
}

// handleListsBack returns from a per-list screen to the overview.
func (b *Bot) handleListsBack(c tele.Context) error { return b.renderSavedLists(c) }

// loadOwnedList fetches a saved list and refuses access when it doesn't
// belong to the caller — small but important safeguard since list IDs
// are simple ints and a curious user might guess one.
func (b *Bot) loadOwnedList(ctx context.Context, c tele.Context, id int64) (*storage.SavedList, error) {
	item, err := b.store.GetSavedList(ctx, id)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, errors.New("список не знайдено")
		}
		return nil, fmt.Errorf("не вдалося прочитати список: %w", err)
	}
	if item.UserTgID != senderID(c) {
		return nil, errors.New("цей список не твій")
	}
	return item, nil
}

// --- View builders --------------------------------------------------------

func buildSavedListsView(lists []storage.SavedList) (string, *tele.ReplyMarkup) {
	var sb strings.Builder
	sb.WriteString("📂 *Збережені списки*\n\n")
	if len(lists) == 0 {
		sb.WriteString("Поки порожньо.\n\n")
		sb.WriteString("Зроби `/search`, тоді на екрані аналізу натисни *💾 Зберегти*.")
	} else {
		fmt.Fprintf(&sb, "Усього: *%d*", len(lists))
	}

	kb := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(lists)+1)
	for _, l := range lists {
		label := fmt.Sprintf("📂 %s", truncateRunes(l.Name, 48))
		rows = append(rows, kb.Row(kb.Data(
			label, btnUniqueListManage,
			callback.Encode(strconv.FormatInt(l.ID, 10)),
		)))
	}
	rows = append(rows, kb.Row(kb.Data("⬅️ Меню", btnUniqueMenu)))
	kb.Inline(rows...)
	return sb.String(), kb
}

func buildListManageView(item *storage.SavedList) (string, *tele.ReplyMarkup) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "📂 *%s*\n\n", mdEscape(item.Name))
	fmt.Fprintf(&sb, "📅 Збережено: `%s`\n",
		item.CreatedAt.Format("2006-01-02 15:04"))
	if item.URL != "" {
		// Render as plain text + tele.NoPreview at send time — embedding
		// untrusted URLs in [text](url) markdown was a parse-entities
		// crash vector (URL with `)` breaks the link parser).
		fmt.Fprintf(&sb, "🔗 %s\n", item.URL)
	}
	if item.Program != nil && item.Program.UniversityName != "" {
		fmt.Fprintf(&sb, "\n🎓 %s\n", mdEscape(item.Program.UniversityName))
		if item.Program.ProgramName != "" {
			fmt.Fprintf(&sb, "📚 %s\n", mdEscape(item.Program.ProgramName))
		}
	}

	id := strconv.FormatInt(item.ID, 10)
	args := callback.Encode(id)
	kb := &tele.ReplyMarkup{}
	kb.Inline(
		kb.Row(kb.Data("👁 Переглянути", btnUniqueListView, args)),
		kb.Row(
			kb.Data("🔄 Оновити", btnUniqueListRefresh, args),
			kb.Data("🔗 Поділитись", btnUniqueListShare, args),
		),
		kb.Row(
			kb.Data("📤 Експорт JSON", btnUniqueListExport, args),
			kb.Data("🗑 Видалити", btnUniqueListDelete, args),
		),
		kb.Row(kb.Data("⬅️ До списків", btnUniqueListsBack)),
	)
	return sb.String(), kb
}

// buildDeleteConfirmView is the "are you sure?" screen.
func buildDeleteConfirmView(item *storage.SavedList) (string, *tele.ReplyMarkup) {
	text := fmt.Sprintf(
		"⚠️ *Видалити список?*\n\n📂 %s\n\nДію скасувати буде неможливо.",
		mdEscape(item.Name))

	id := strconv.FormatInt(item.ID, 10)
	kb := &tele.ReplyMarkup{}
	kb.Inline(
		kb.Row(kb.Data("✓ Так, видалити", btnUniqueListDeleteConfirm, callback.Encode(id))),
		kb.Row(kb.Data("⬅️ Назад", btnUniqueListManage, callback.Encode(id))),
	)
	return text, kb
}

// buildRefreshDiffView shows what changed between two snapshots taken
// at oldAt and newAt. Empty diff lines collapse to "(без змін)".
func buildRefreshDiffView(id int64, name string, oldAt, newAt time.Time, oldA, newA abit.Analysis) (string, *tele.ReplyMarkup) {
	var sb strings.Builder
	fmt.Fprintf(&sb, "🔄 *Оновлено: %s*\n\n", mdEscape(name))
	fmt.Fprintf(&sb, "📅 Було: `%s`\n", oldAt.Format("2006-01-02 15:04"))
	fmt.Fprintf(&sb, "📅 Стало: `%s`\n\n", newAt.Format("2006-01-02 15:04"))

	sb.WriteString("📊 *Зміни:*\n")
	// User-score line — usually identical, but flags when the profile
	// changed between snapshots (e.g. RK toggled, new subject score).
	if oldA.UserScore != newA.UserScore {
		fmt.Fprintf(&sb, "🧮 Твій бал: `%.3f` → `%.3f`\n",
			oldA.UserScore, newA.UserScore)
	}
	fmt.Fprintf(&sb, "%s\n", diffLine("Конкурентів",
		oldA.CompetitorsTotal, newA.CompetitorsTotal, true))
	fmt.Fprintf(&sb, "%s\n", diffLine("На наказі",
		oldA.AlreadyEnrolled, newA.AlreadyEnrolled, true))
	fmt.Fprintf(&sb, "%s\n", diffLine("Вільних місць",
		oldA.RemainingSpots, newA.RemainingSpots, false))
	if oldA.MyRealRank > 0 || newA.MyRealRank > 0 {
		fmt.Fprintf(&sb, "%s\n", diffLine("Твоє місце",
			oldA.MyRealRank, newA.MyRealRank, true))
	}
	if oldA.Chance != newA.Chance && newA.Chance != abit.ChanceUnknown {
		fmt.Fprintf(&sb, "🎯 Шанс: %s %s → %s %s\n",
			oldA.Chance.Emoji(), oldA.Chance.Label(),
			newA.Chance.Emoji(), newA.Chance.Label())
	}
	if newA.Advice != "" {
		fmt.Fprintf(&sb, "\n💡 %s", mdEscape(newA.Advice))
	}

	idStr := strconv.FormatInt(id, 10)
	args := callback.Encode(idStr)
	kb := &tele.ReplyMarkup{}
	kb.Inline(
		kb.Row(kb.Data("👁 Переглянути", btnUniqueListView, args)),
		kb.Row(
			kb.Data("⬅️ До списку", btnUniqueListManage, args),
			kb.Data("📋 Усі списки", btnUniqueListsBack),
		),
	)
	return sb.String(), kb
}

// diffLine renders "label: old → new (±Δ)". invertedTrend=true means
// "higher = worse" (e.g. competitors, rank) — picks the emoji accordingly.
func diffLine(label string, oldV, newV int, invertedTrend bool) string {
	delta := newV - oldV
	if delta == 0 {
		return fmt.Sprintf("• %s: %d _(без змін)_", label, newV)
	}
	arrow := "↑"
	sign := "+"
	if delta < 0 {
		arrow = "↓"
		sign = ""
	}
	good := (delta > 0) != invertedTrend
	mark := "⚠️"
	if good {
		mark = "✅"
	}
	return fmt.Sprintf("%s • %s: %d → %d (%s%d %s)",
		mark, label, oldV, newV, sign, delta, arrow)
}

// truncateRunes shortens s to at most n runes (not bytes), appending "…".
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
