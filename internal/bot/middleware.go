package bot

import (
	"context"
	"runtime/debug"
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
			b.log.Debug("handler ok", attrs...)
		}
		return err
	}
}

// recoverPanics turns panics into errors so the rest of the middleware
// chain can deal with them. The stack trace goes to the log; the user
// gets a friendly notice (rendered by reportErrors).
func (b *Bot) recoverPanics(next tele.HandlerFunc) tele.HandlerFunc {
	return func(c tele.Context) (err error) {
		defer func() {
			if r := recover(); r != nil {
				b.log.Error("panic in handler",
					"err", r, "user_id", senderID(c),
					"stack", string(debug.Stack()))
				err = errInternal
			}
		}()
		return next(c)
	}
}

// reportErrors renders any error from downstream handlers as a friendly
// message (alert for callbacks, plain text for messages) and returns it
// up so the outer logger can record it. Handlers must NOT send their
// own error messages — keeps the UX consistent.
func (b *Bot) reportErrors(next tele.HandlerFunc) tele.HandlerFunc {
	return func(c tele.Context) error {
		err := next(c)
		if err == nil {
			return nil
		}
		msg := "⚠️ " + err.Error()
		if cb := c.Callback(); cb != nil {
			_ = c.Respond(&tele.CallbackResponse{
				Text:      truncated(err.Error(), 190),
				ShowAlert: true,
			})
		} else {
			_ = c.Send(msg)
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

// errInternal is the sentinel returned by recoverPanics so reportErrors
// can surface a generic message without leaking implementation details.
var errInternal = botError("сталася внутрішня помилка, повідомили адміністраторів")

// botError is a tiny error type with a stable string so handlers can
// return user-facing messages directly.
type botError string

func (e botError) Error() string { return string(e) }

func senderID(c tele.Context) int64 {
	if c == nil || c.Sender() == nil {
		return 0
	}
	return c.Sender().ID
}

// truncated cuts s to at most n runes (NOT bytes — Cyrillic / emoji
// would otherwise be cut mid-codepoint), appending "…" if shortened.
func truncated(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

