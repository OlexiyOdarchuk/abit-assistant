package bot

import (
	tele "gopkg.in/telebot.v3"
)

// registerRoutes binds every command and callback handler to the
// telebot instance. Middleware is applied here so it's visible in one
// place; handlers themselves remain pure formatting + service calls.
func (b *Bot) registerRoutes() {
	// Global middleware — runs on every update.
	b.tg.Use(b.logUpdates, b.trackUser)

	b.tg.Handle("/start", b.handleStart)
	b.tg.Handle("/help", b.handleHelp)
	b.tg.Handle("/search", b.handleSearch)

	// Anything that isn't a known command and looks like a URL → /search shortcut.
	b.tg.Handle(tele.OnText, b.handleText)

	// Inline-keyboard callbacks for pagination.
	b.tg.Handle(&btnPagePrev, b.handlePagePrev)
	b.tg.Handle(&btnPageNext, b.handlePageNext)
}
