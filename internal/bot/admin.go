package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"
)

const (
	adminTimeout      = 5 * time.Second
	broadcastTimeout  = 15 * time.Minute
	broadcastRateMS   = 50 // ~20 msg/sec — well under Telegram's 30/sec global cap

	fsmStateAdminBroadcast        = "admin.broadcast.text"
	fsmStateAdminBroadcastConfirm = "admin.broadcast.confirm"
	fsmKeyBroadcastText           = "text"
)

// requireAdmin returns nil if the caller is in cfg.AdminIDs. Otherwise
// a user-visible error that the middleware will render — handlers don't
// need to do the check twice.
func (b *Bot) requireAdmin(c tele.Context) error {
	if !b.cfg.IsAdmin(senderID(c)) {
		return errors.New("команда доступна тільки адміністраторам")
	}
	return nil
}

// --- /admin command + main panel -----------------------------------------

func (b *Bot) handleAdmin(c tele.Context) error {
	if err := b.requireAdmin(c); err != nil {
		return err
	}
	return b.renderAdminMenu(c)
}

func (b *Bot) handleAdminCB(c tele.Context) error {
	if err := b.requireAdmin(c); err != nil {
		return err
	}
	return b.renderAdminMenu(c)
}

func (b *Bot) renderAdminMenu(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
	defer cancel()
	users, err := b.store.Queries.CountUsers(ctx)
	if err != nil {
		return fmt.Errorf("не вдалося прочитати кількість користувачів: %w", err)
	}

	text := fmt.Sprintf("🛠 *Адмін-панель*\n\n👥 Користувачів: *%d*", users)

	kb := &tele.ReplyMarkup{}
	kb.Inline(
		kb.Row(kb.Data("📊 Статистика", btnUniqueAdminStats)),
		kb.Row(kb.Data("📣 Розсилка", btnUniqueAdminBroadcast)),
		kb.Row(kb.Data("🧹 Очистити кеш", btnUniqueAdminVacuum)),
		kb.Row(kb.Data("⬅️ Меню", btnUniqueMenu)),
	)
	return renderOrEdit(c, text, tele.ModeMarkdown, kb)
}

// --- statistics -----------------------------------------------------------

func (b *Bot) handleAdminStats(c tele.Context) error {
	if err := b.requireAdmin(c); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
	defer cancel()

	users, err := b.store.Queries.CountUsers(ctx)
	if err != nil {
		return err
	}
	totals, err := b.store.Queries.TotalActivates(ctx)
	if err != nil {
		return err
	}
	top, topErr := b.store.Queries.TopUserByActivates(ctx)

	var sb strings.Builder
	sb.WriteString("📊 *Статистика*\n\n")
	fmt.Fprintf(&sb, "👥 Користувачів: *%d*\n", users)
	fmt.Fprintf(&sb, "🔂 Усього звернень: *%d*\n", totals.TotalActivates)
	fmt.Fprintf(&sb, "✅ Успішних: *%d*\n", totals.TotalRightActivates)
	if topErr == nil && top.TgID != 0 {
		fmt.Fprintf(&sb, "\n🏆 Топ-користувач: `%d` (%d звернень)\n",
			top.TgID, top.Activates)
	}

	kb := &tele.ReplyMarkup{}
	kb.Inline(kb.Row(kb.Data("⬅️ Адмін-меню", btnUniqueAdmin)))
	return renderOrEdit(c, sb.String(), tele.ModeMarkdown, kb)
}

// --- vacuum cache ---------------------------------------------------------

func (b *Bot) handleAdminVacuum(c tele.Context) error {
	if err := b.requireAdmin(c); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// TTL=0 → cutoff is "now", drops everything older. Effectively a
	// nuke of both program and applicant caches.
	if err := b.store.VacuumCaches(ctx, 0, 0); err != nil {
		return fmt.Errorf("vacuum: %w", err)
	}
	_ = c.Respond(&tele.CallbackResponse{
		Text: "🧹 Кеші очищено", ShowAlert: true,
	})
	return b.renderAdminMenu(c)
}

// --- broadcast: prompt → confirm → fire-and-forget ------------------------

func (b *Bot) handleAdminBroadcast(c tele.Context) error {
	if err := b.requireAdmin(c); err != nil {
		return err
	}
	if err := b.fsm.Set(context.Background(), senderID(c),
		fsmStateAdminBroadcast, nil); err != nil {
		return err
	}
	text := `📣 *Розсилка*

Надішли наступним повідомленням текст для розсилки.
Підтримується Markdown.

Або /cancel — щоб скасувати.`
	kb := &tele.ReplyMarkup{}
	kb.Inline(kb.Row(kb.Data("⬅️ Адмін-меню", btnUniqueAdmin)))
	return renderOrEdit(c, text, tele.ModeMarkdown, kb)
}

// handleAdminBroadcastText is invoked from the OnText catch-all when
// the admin is in fsmStateAdminBroadcast. Stashes the text in FSM and
// shows a confirmation screen with the audience size baked in.
func (b *Bot) handleAdminBroadcastText(c tele.Context, text string) error {
	if !b.cfg.IsAdmin(senderID(c)) {
		// Defensive: should be unreachable since only the admin can
		// be in this FSM state, but never trust client state.
		_ = b.fsm.Clear(context.Background(), senderID(c))
		return errors.New("forbidden")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("повідомлення порожнє")
	}

	ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
	defer cancel()
	users, err := b.store.Queries.CountUsers(ctx)
	if err != nil {
		return err
	}

	if err := b.fsm.Set(context.Background(), senderID(c),
		fsmStateAdminBroadcastConfirm,
		map[string]any{fsmKeyBroadcastText: text}); err != nil {
		return err
	}

	preview := fmt.Sprintf(
		"📣 *Розсилка готова*\n\n"+
			"👥 Отримувачів: *%d*\n\n"+
			"📨 *Повідомлення:*\n\n%s\n\nВідправити?",
		users, text)

	kb := &tele.ReplyMarkup{}
	kb.Inline(
		kb.Row(kb.Data("✅ Відправити", btnUniqueAdminBroadcastConfirm)),
		kb.Row(kb.Data("❌ Скасувати", btnUniqueAdminBroadcastCancel)),
	)
	return c.Send(preview, tele.ModeMarkdown, kb)
}

func (b *Bot) handleAdminBroadcastConfirm(c tele.Context) error {
	if err := b.requireAdmin(c); err != nil {
		return err
	}
	uid := senderID(c)

	ctx, cancel := context.WithTimeout(context.Background(), adminTimeout)
	state, err := b.fsm.Get(ctx, uid)
	cancel()
	if err != nil {
		return err
	}
	text, _ := state.Data[fsmKeyBroadcastText].(string)
	if text == "" {
		return errors.New("текст розсилки втрачено — почни ще раз")
	}

	// Clear FSM immediately so accidental re-taps can't fire again.
	if err := b.fsm.Clear(context.Background(), uid); err != nil {
		b.log.Warn("clear fsm after broadcast confirm", "err", err)
	}

	_ = c.Respond(&tele.CallbackResponse{Text: "📣 Запускаю розсилку…"})

	// Fire-and-forget: a broadcast to 1000+ users at 20/sec needs
	// ~50 seconds — much longer than Telegram's 10s update-handler
	// budget. Detach into a goroutine and report back via DM.
	go b.runBroadcast(uid, text)

	kb := &tele.ReplyMarkup{}
	kb.Inline(kb.Row(kb.Data("⬅️ Адмін-меню", btnUniqueAdmin)))
	return c.Edit(
		"📣 *Розсилка стартувала*\n\nЯ напишу окремим повідомленням, коли закінчу.",
		tele.ModeMarkdown, kb)
}

func (b *Bot) handleAdminBroadcastCancel(c tele.Context) error {
	if err := b.requireAdmin(c); err != nil {
		return err
	}
	if err := b.fsm.Clear(context.Background(), senderID(c)); err != nil {
		b.log.Warn("clear fsm on broadcast cancel", "err", err)
	}
	_ = c.Respond(&tele.CallbackResponse{Text: "❌ Скасовано"})
	return b.renderAdminMenu(c)
}

// runBroadcast sends `text` to every user ID in storage, rate-limited
// under Telegram's bulk-message ceiling. Reports the delivered/failed
// counts to the initiating admin when done.
func (b *Bot) runBroadcast(adminID int64, text string) {
	ctx, cancel := context.WithTimeout(context.Background(), broadcastTimeout)
	defer cancel()

	ids, err := b.store.Queries.ListUserIDs(ctx)
	if err != nil {
		b.log.Error("broadcast: list users failed", "err", err)
		b.notifyAdmin(adminID, "📣 Розсилка не стартувала: помилка читання користувачів.")
		return
	}

	var delivered, failed int
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		chat := &tele.Chat{ID: id}
		if _, err := b.tg.Send(chat, text, tele.ModeMarkdown); err != nil {
			failed++
			b.log.Warn("broadcast send failed",
				"user_id", id, "err", err)
		} else {
			delivered++
		}
		time.Sleep(broadcastRateMS * time.Millisecond)
	}

	report := fmt.Sprintf(
		"📣 *Розсилка завершена*\n\n"+
			"✅ Доставлено: *%d*\n"+
			"⚠️ Невдало: *%d*\n"+
			"👥 Усього: *%d*",
		delivered, failed, len(ids))
	b.notifyAdmin(adminID, report)
}

// notifyAdmin sends a DM to the admin who initiated the operation.
// Errors are logged but swallowed — we don't want a failed status
// message to mask the operation's actual outcome.
func (b *Bot) notifyAdmin(uid int64, text string) {
	if _, err := b.tg.Send(&tele.Chat{ID: uid}, text, tele.ModeMarkdown); err != nil {
		b.log.Warn("notify admin failed", "user_id", uid, "err", err)
	}
}
