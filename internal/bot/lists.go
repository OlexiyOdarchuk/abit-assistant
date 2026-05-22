package bot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"

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

// handleListDelete deletes a saved list and returns to the list overview.
func (b *Bot) handleListDelete(c tele.Context) error {
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

// handleListsBack returns from a per-list screen to the overview.
func (b *Bot) handleListsBack(c tele.Context) error { return b.renderSavedLists(c) }

// loadOwnedList fetches a saved list and refuses access when it doesn't
// belong to the caller — small but important safeguard since list IDs
// are simple ints and a curious user might guess one.
func (b *Bot) loadOwnedList(ctx context.Context, c tele.Context, id int64) (*storage.SavedList, error) {
	item, err := b.store.GetSavedList(ctx, id)
	if err != nil {
		if errors.Is(err, storage.ErrCacheMiss) {
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
		fmt.Fprintf(&sb, "🔗 [Джерело](%s)\n", item.URL)
	}
	if item.Program != nil && item.Program.UniversityName != "" {
		fmt.Fprintf(&sb, "\n🎓 %s\n", mdEscape(item.Program.UniversityName))
		if item.Program.ProgramName != "" {
			fmt.Fprintf(&sb, "📚 %s\n", mdEscape(item.Program.ProgramName))
		}
	}

	id := strconv.FormatInt(item.ID, 10)
	kb := &tele.ReplyMarkup{}
	kb.Inline(
		kb.Row(kb.Data("👁 Переглянути", btnUniqueListView, callback.Encode(id))),
		kb.Row(kb.Data("🗑 Видалити", btnUniqueListDelete, callback.Encode(id))),
		kb.Row(kb.Data("⬅️ До списків", btnUniqueListsBack)),
	)
	return sb.String(), kb
}

// truncateRunes shortens s to at most n runes (not bytes), appending "…".
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
