package bot

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	tele "gopkg.in/telebot.v3"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/bot/callback"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/storage"
)

// fsmStatePrioAddURL waits for the user to paste an osvita link to append to
// their priority list.
const fsmStatePrioAddURL = "prio.add_url"

// handlePrioritiesCB opens the "🎯 Мій прогноз" screen (conservative view).
func (b *Bot) handlePrioritiesCB(c tele.Context) error {
	b.clearTransientFSM(c)
	return b.renderPriorities(c, false)
}

// handlePrioToggleUnlikely re-renders the prediction with the opposite
// "count priority-3+ rivals" setting (passed in the callback arg).
func (b *Bot) handlePrioToggleUnlikely(c tele.Context) error {
	b.clearTransientFSM(c) // leaving the add-URL prompt behind, if any
	excl, _ := callback.From(c).Int(0)
	return b.renderPriorities(c, excl == 1)
}

// renderPriorities loads the user's ranked list, predicts their placement, and
// renders the screen. exclUnlikely drops priority-3+ rivals from each program's
// rank (optimistic).
func (b *Bot) renderPriorities(c tele.Context, exclUnlikely bool) error {
	uid := senderID(c)
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()

	nmt, _ := b.store.GetUserNMT(ctx, uid)
	settings, _ := b.store.GetUserSettings(ctx, uid)

	if len(nmt) == 0 {
		return renderOrEdit(c,
			"🎯 *Мій прогноз вступу*\n\nСпочатку заповни свій НМТ у /profile — без балів я не можу порахувати, куди ти проходиш.",
			tele.ModeMarkdown, backToMenuKeyboard())
	}
	if len(settings.Priorities) == 0 {
		return renderOrEdit(c, prioEmptyText, tele.ModeMarkdown, prioAddKeyboard(true))
	}

	if err := c.Notify(tele.Typing); err != nil {
		b.log.Debug("notify typing", "err", err)
	}
	urls := make([]string, len(settings.Priorities))
	for i, p := range settings.Priorities {
		urls[i] = p.URL
	}
	pred := b.predictSvc.Predict(ctx, urls, service.PredictInput{
		NMT:             map[string]float64(nmt),
		CreativeScore:   float64(settings.CreativeScorePrediction),
		Quotas:          settings.Quotas,
		ExcludeUnlikely: exclUnlikely,
	})
	text, kb := buildPrioritiesView(settings.Priorities, pred, exclUnlikely)
	return renderOrEdit(c, text, tele.ModeMarkdown, kb)
}

const prioEmptyText = `🎯 *Мій прогноз вступу*

Додай свої програми в порядку пріоритету (до 5), і я спрогнозую, за яким пріоритетом ти реально вступиш.

Як це працює: вступ — це пріоритетна модель. Тебе зараховують на *одну* програму — найвищий пріоритет, де ти проходиш прохідний бал. Я пройду твій список згори вниз і знайду першу, де проходиш.

Список поки порожній.`

// buildPrioritiesView renders the ranked list + the placement verdict.
func buildPrioritiesView(items []storage.PriorityItem, pred service.PriorityPrediction, exclUnlikely bool) (string, *tele.ReplyMarkup) {
	var sb strings.Builder
	sb.WriteString("🎯 *Мій прогноз вступу*\n\n")

	if adm, ok := pred.Admitted(); ok {
		fmt.Fprintf(&sb, "✅ *Прогноз: проходиш за пріоритетом %d*\n%s — %s\n",
			pred.AdmittedIndex+1, mdEscape(adm.University), mdEscape(adm.Program))
		if adm.Analysis.Cutoff > 0 {
			fmt.Fprintf(&sb, "Твій бал `%.2f` ≥ прохідного `%.2f`.\n", adm.Score, adm.Analysis.Cutoff)
		} else {
			fmt.Fprintf(&sb, "Твій бал `%.2f`, за поточним рейтингом проходиш.\n", adm.Score)
		}
		if pred.AdmittedIndex > 0 {
			sb.WriteString("_Вищі пріоритети поки не проходиш — вони згорять, і ти впадеш сюди._\n")
		}
		if cav := prioCaveat(adm.Analysis.Warnings); cav != "" {
			fmt.Fprintf(&sb, "%s\n", cav)
		}
	} else {
		sb.WriteString("😔 *За поточними даними не проходиш на жоден пріоритет.*\nСпробуй додати запасні варіанти з нижчим прохідним балом.\n")
	}

	sb.WriteString("\n📋 *Твій список:*\n")
	for i, it := range items {
		marker := "•"
		if i == pred.AdmittedIndex {
			marker = "➡️"
		}
		uni := it.University
		prog := it.Program
		var tail string
		if i < len(pred.Items) {
			o := pred.Items[i]
			if !o.Fetched {
				tail = " · ⚠️ не вдалося завантажити"
			} else {
				if o.University != "" {
					uni = o.University
					prog = o.Program
				}
				tail = fmt.Sprintf(" · бал `%.1f` · %s %s", o.Score, o.Analysis.Chance.Emoji(), o.Analysis.Chance.Label())
			}
		}
		if uni == "" {
			uni = it.URL
		}
		fmt.Fprintf(&sb, "%s *%d.* %s — %s%s\n", marker, i+1, mdEscape(uni), mdEscape(prog), tail)
	}

	if exclUnlikely {
		sb.WriteString("\n⚪ _Пріоритет 3+ суперників не враховано (оптимістична оцінка)._")
	}

	return sb.String(), prioKeyboard(len(items), exclUnlikely)
}

// prioCaveat softens an over-optimistic "проходиш" when the admitted program's
// analysis carries a data-quality warning (thin field early in the campaign, or
// a ceiling-only volume). Empty when the verdict is solid.
func prioCaveat(warnings []string) string {
	for _, w := range warnings {
		switch w {
		case "field-undersubscribed":
			return "⚠️ _Але заяв поки менше, ніж місць — більшість подають в останні дні, тож прохідний ще зросте. Прогноз оптимістичний._"
		case "budget-volume-is-ceiling":
			return "⚠️ _Кількість місць — це стеля держзамовлення; реальних може бути менше, тож прохід не гарантований._"
		}
	}
	return ""
}

// prioKeyboard builds the action rows: per-item reorder/remove, add, toggle,
// recompute, back.
func prioKeyboard(n int, exclUnlikely bool) *tele.ReplyMarkup {
	kb := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, n+3)
	for i := 0; i < n; i++ {
		btns := make([]tele.Btn, 0, 3)
		if i > 0 {
			btns = append(btns, kb.Data(fmt.Sprintf("⬆️ %d", i+1), btnUniquePrioUp, callback.Encode(strconv.Itoa(i))))
		}
		if i < n-1 {
			btns = append(btns, kb.Data(fmt.Sprintf("⬇️ %d", i+1), btnUniquePrioDown, callback.Encode(strconv.Itoa(i))))
		}
		btns = append(btns, kb.Data(fmt.Sprintf("🗑 %d", i+1), btnUniquePrioRemove, callback.Encode(strconv.Itoa(i))))
		rows = append(rows, kb.Row(btns...))
	}
	if n < storage.MaxPriorities {
		rows = append(rows, kb.Row(kb.Data("➕ Додати програму", btnUniquePrioAdd)))
	}
	toggle := "⚪ Не рахувати пріоритет 3+"
	next := "1"
	if exclUnlikely {
		toggle = "⚪ Рахувати пріоритет 3+"
		next = "0"
	}
	rows = append(rows,
		kb.Row(kb.Data(toggle, btnUniquePrioToggleUnlik, callback.Encode(next))),
		kb.Row(kb.Data("⬅️ Меню", btnUniqueMenu)),
	)
	kb.Inline(rows...)
	return kb
}

// prioAddKeyboard is shown on the empty screen / the "add" chooser.
func prioAddKeyboard(withBackToMenu bool) *tele.ReplyMarkup {
	kb := &tele.ReplyMarkup{}
	rows := []tele.Row{
		kb.Row(kb.Data("🔗 Вставити посилання", btnUniquePrioAddURL)),
		kb.Row(kb.Data("📂 З моїх списків", btnUniquePrioFromSaved)),
	}
	if withBackToMenu {
		rows = append(rows, kb.Row(kb.Data("⬅️ Меню", btnUniqueMenu)))
	} else {
		rows = append(rows, kb.Row(kb.Data("⬅️ Назад", btnUniquePriorities)))
	}
	kb.Inline(rows...)
	return kb
}

// handlePrioAdd shows the "how to add" chooser.
func (b *Bot) handlePrioAdd(c tele.Context) error {
	b.clearTransientFSM(c)
	return renderOrEdit(c, "➕ *Додати програму до пріоритетів*\n\nОбери спосіб:",
		tele.ModeMarkdown, prioAddKeyboard(false))
}

// handlePrioAddURL prompts for an osvita link and waits for it.
func (b *Bot) handlePrioAddURL(c tele.Context) error {
	if err := b.fsm.Set(context.Background(), senderID(c), fsmStatePrioAddURL, nil); err != nil {
		return fmt.Errorf("не вдалося зберегти стан: %w", err)
	}
	const prompt = `🔗 Надішли посилання на програму з vstup.osvita.ua — додам її в кінець твого списку пріоритетів.

Приклад:
` + "`https://vstup.osvita.ua/y2025/r14/282/1471029/`" + `

Або /cancel щоб вийти.`
	return renderOrEdit(c, prompt, tele.ModeMarkdown, backToMenuKeyboard())
}

// runPrioAddURL is called from the OnText catch-all: validate the pasted link,
// fetch it (for labels + to reject dead URLs), append it, and re-render.
func (b *Bot) runPrioAddURL(c tele.Context, rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if !looksLikeOsvitaURL(rawURL) {
		return errors.New("це не схоже на посилання vstup.osvita.ua")
	}
	if err := c.Notify(tele.Typing); err != nil {
		b.log.Debug("notify typing", "err", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), searchTimeout)
	defer cancel()

	prog, err := b.programSvc.Fetch(ctx, rawURL)
	if err != nil {
		return fmt.Errorf("не вдалося завантажити програму: %w", err)
	}
	if err := b.appendPriority(ctx, senderID(c), storage.PriorityItem{
		URL: rawURL, University: prog.UniversityName, Program: prog.ProgramName,
	}); err != nil {
		return err
	}
	_ = b.fsm.Clear(context.Background(), senderID(c))
	return b.renderPriorities(c, false)
}

// handlePrioFromSaved lists the user's saved analyses as pickable buttons.
func (b *Bot) handlePrioFromSaved(c tele.Context) error {
	b.clearTransientFSM(c)
	ctx, cancel := context.WithTimeout(context.Background(), listsTimeout)
	defer cancel()

	lists, err := b.store.ListSavedLists(ctx, senderID(c))
	if err != nil {
		return fmt.Errorf("не вдалося прочитати списки: %w", err)
	}
	if len(lists) == 0 {
		return renderOrEdit(c,
			"📂 У тебе ще немає збережених списків. Збережи аналіз програми (💾) або додай посиланням.",
			tele.ModeMarkdown, prioAddKeyboard(false))
	}
	kb := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(lists)+1)
	for _, l := range lists {
		label := l.Name
		if label == "" && l.Program != nil {
			label = l.Program.UniversityName
		}
		rows = append(rows, kb.Row(kb.Data(
			truncateRunes("📂 "+label, 60), btnUniquePrioPickSaved, callback.Encode(strconv.FormatInt(l.ID, 10)))))
	}
	rows = append(rows, kb.Row(kb.Data("⬅️ Назад", btnUniquePriorities)))
	kb.Inline(rows...)
	return renderOrEdit(c, "📂 *Обери збережений список*, щоб додати до пріоритетів:", tele.ModeMarkdown, kb)
}

// handlePrioPickSaved appends the chosen saved list to the priorities.
func (b *Bot) handlePrioPickSaved(c tele.Context) error {
	b.clearTransientFSM(c)
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
	pi := storage.PriorityItem{URL: item.URL}
	if item.Program != nil {
		pi.University = item.Program.UniversityName
		pi.Program = item.Program.ProgramName
	}
	if err := b.appendPriority(ctx, senderID(c), pi); err != nil {
		return err
	}
	return b.renderPriorities(c, false)
}

// handlePrioRemove drops item N from the list.
func (b *Bot) handlePrioRemove(c tele.Context) error {
	b.clearTransientFSM(c)
	idx, ok := callback.From(c).Int(0)
	if !ok {
		return errors.New("втрачено позицію")
	}
	if err := b.mutatePriorities(c, func(ps []storage.PriorityItem) []storage.PriorityItem {
		if idx < 0 || idx >= len(ps) {
			return ps
		}
		return append(ps[:idx], ps[idx+1:]...)
	}); err != nil {
		return err
	}
	return b.renderPriorities(c, false)
}

// handlePrioUp / handlePrioDown reorder the list (priority order is the point).
func (b *Bot) handlePrioUp(c tele.Context) error   { return b.movePriority(c, -1) }
func (b *Bot) handlePrioDown(c tele.Context) error { return b.movePriority(c, +1) }

func (b *Bot) movePriority(c tele.Context, delta int) error {
	b.clearTransientFSM(c)
	idx, ok := callback.From(c).Int(0)
	if !ok {
		return errors.New("втрачено позицію")
	}
	if err := b.mutatePriorities(c, func(ps []storage.PriorityItem) []storage.PriorityItem {
		j := idx + delta
		if idx < 0 || idx >= len(ps) || j < 0 || j >= len(ps) {
			return ps
		}
		ps[idx], ps[j] = ps[j], ps[idx]
		return ps
	}); err != nil {
		return err
	}
	return b.renderPriorities(c, false)
}

// --- storage helpers ------------------------------------------------------

// appendPriority adds one item, ignoring duplicates and respecting the cap.
func (b *Bot) appendPriority(ctx context.Context, uid int64, item storage.PriorityItem) error {
	settings, err := b.store.GetUserSettings(ctx, uid)
	if err != nil {
		return err
	}
	for _, p := range settings.Priorities {
		if p.URL == item.URL {
			return errors.New("ця програма вже у твоєму списку")
		}
	}
	if len(settings.Priorities) >= storage.MaxPriorities {
		return fmt.Errorf("список повний — максимум %d пріоритетів", storage.MaxPriorities)
	}
	settings.Priorities = append(settings.Priorities, item)
	return b.store.SetUserSettings(ctx, uid, settings)
}

// mutatePriorities loads, transforms and persists the user's priority list.
func (b *Bot) mutatePriorities(c tele.Context, fn func([]storage.PriorityItem) []storage.PriorityItem) error {
	uid := senderID(c)
	ctx, cancel := context.WithTimeout(context.Background(), listsTimeout)
	defer cancel()
	settings, err := b.store.GetUserSettings(ctx, uid)
	if err != nil {
		return err
	}
	settings.Priorities = fn(settings.Priorities)
	return b.store.SetUserSettings(ctx, uid, settings)
}
