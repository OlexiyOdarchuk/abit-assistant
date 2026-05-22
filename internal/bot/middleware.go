package bot

import (
	"context"
	"time"

	tele "gopkg.in/telebot.v3"
)

// logUpdates is a telebot middleware that logs every update with the
// sender ID, elapsed handler time, and the message text (truncated).
func (b *Bot) logUpdates(next tele.HandlerFunc) tele.HandlerFunc {
	return func(c tele.Context) error {
		started := time.Now()
		err := next(c)
		attrs := []any{
			"user_id", senderID(c),
			"took_ms", time.Since(started).Milliseconds(),
			"text", truncated(c.Text(), 80),
		}
		if err != nil {
			attrs = append(attrs, "err", err)
			b.log.Error("handler failed", attrs...)
		} else {
			b.log.Info("handler ok", attrs...)
		}
		return err
	}
}

// trackUser is middleware that ensures the user row exists and bumps the
// activates counter. Storage failures never break the user flow.
func (b *Bot) trackUser(next tele.HandlerFunc) tele.HandlerFunc {
	return func(c tele.Context) error {
		uid := senderID(c)
		if uid != 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if err := b.store.Queries.IncrementActivates(ctx, uid); err != nil {
				b.log.Warn("user track failed", "err", err, "user_id", uid)
			}
			cancel()
		}
		return next(c)
	}
}

// senderID returns the Telegram user ID, or 0 if unavailable.
func senderID(c tele.Context) int64 {
	if c == nil || c.Sender() == nil {
		return 0
	}
	return c.Sender().ID
}

// truncated cuts a string to at most n bytes, appending "…" if cut.
func truncated(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
