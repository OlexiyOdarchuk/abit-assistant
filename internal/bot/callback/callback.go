// Package callback gives the bot a typed, bounds-checked view of
// Telegram callback_data. We rely on telebot's Btn.Unique to dispatch
// to handlers (it strips the leading "\f<unique>" marker), and use this
// package to safely pull args out of what remains.
//
// Encoding side is plain: assemble args with a single function and pass
// the result as the last argument to ReplyMarkup.Data().
package callback

import (
	"strconv"
	"strings"

	tele "gopkg.in/telebot.v3"
)

// Separator between args inside the callback data string.
const Sep = "|"

// Encode joins args with the canonical separator. Use this when building
// inline keyboards to keep the encoding consistent everywhere.
//
//	btn := kb.Data("▶️", "page", callback.Encode("competitors", "3"))
func Encode(args ...string) string {
	return strings.Join(args, Sep)
}

// Args wraps the parsed argument slice with safe accessors. The zero
// value behaves like an empty arg list, so callers don't need a nil check.
type Args struct {
	parts []string
}

// From parses the callback data attached to c. Safe to call on a context
// without a callback — returns the zero Args.
func From(c tele.Context) Args {
	if c == nil || c.Callback() == nil {
		return Args{}
	}
	raw := c.Callback().Data
	if raw == "" {
		return Args{}
	}
	return Args{parts: strings.Split(raw, Sep)}
}

// FromString parses a raw string, used in tests and when args come from
// somewhere other than a Telegram update.
func FromString(raw string) Args {
	if raw == "" {
		return Args{}
	}
	return Args{parts: strings.Split(raw, Sep)}
}

// Len returns the number of parsed args.
func (a Args) Len() int { return len(a.parts) }

// String returns arg i, or "" if i is out of range.
func (a Args) String(i int) string {
	if i < 0 || i >= len(a.parts) {
		return ""
	}
	return a.parts[i]
}

// Int returns arg i as int. ok is false when i is out of range OR the
// value can't be parsed — callers are expected to treat both as "bad
// request" and reject gracefully.
func (a Args) Int(i int) (int, bool) {
	s := a.String(i)
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// Int64 is the 64-bit variant of Int, useful for Telegram IDs.
func (a Args) Int64(i int) (int64, bool) {
	s := a.String(i)
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	return n, err == nil
}
