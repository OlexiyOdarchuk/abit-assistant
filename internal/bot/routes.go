package bot

import (
	tele "gopkg.in/telebot.v3"
)

// registerRoutes binds every command and callback handler to the
// telebot instance and applies the middleware chain.
//
// Middleware order matters: recoverPanics is outermost so a downstream
// panic is converted to an error; logUpdates wraps it so even panicked
// handlers get a log line; reportErrors comes next so the user gets the
// friendly message; trackUser is closest to the handler.
func (b *Bot) registerRoutes() {
	b.tg.Use(b.recoverPanics, b.logUpdates, b.reportErrors, b.trackUser)

	// Commands.
	b.tg.Handle("/start", b.handleStart)
	b.tg.Handle("/menu", b.handleMenu)
	b.tg.Handle("/help", b.handleHelp)
	b.tg.Handle("/about", b.handleAbout)
	b.tg.Handle("/cancel", b.handleCancel)
	b.tg.Handle("/search", b.handleSearch)
	b.tg.Handle("/profile", b.handleProfile)
	b.tg.Handle("/lists", b.handleLists)

	// Catch-all text: route to active FSM step or implicit /search.
	b.tg.Handle(tele.OnText, b.handleText)

	// Inline keyboards — every button gets a typed handler keyed on
	// its Unique string (no fragile callback_data parsing).
	for unique, h := range map[string]tele.HandlerFunc{
		btnUniqueMenu:                 b.handleMenuCB,
		btnUniqueSearch:               b.handleSearchCB,
		btnUniqueProfile:              b.handleProfileCB,
		btnUniqueLists:                b.handleListsCB,
		btnUniqueAbout:                b.handleAboutCB,
		btnUniquePagePrev:             b.handlePagePrev,
		btnUniquePageNext:             b.handlePageNext,
		btnUniqueApplicant:            b.handleApplicantView,
		btnUniqueApplicantHistory:     b.handleApplicantHistory,
		btnUniqueBackToList:           b.handleBackToList,
		btnUniqueToggleMode:           b.handleToggleMode,
		btnUniqueSummary:              b.handleSummaryCB,
		btnUniqueViewList:             b.handleViewListCB,
		btnUniqueSaveList:             b.handleSaveListCB,
		btnUniqueListManage:           b.handleListManage,
		btnUniqueListView:             b.handleListView,
		btnUniqueListDelete:           b.handleListDelete,
		btnUniqueListsBack:            b.handleListsBack,
		btnUniqueProfileBack:          b.handleProfileBack,
		btnUniqueProfileEditNMT:       b.handleProfileEditNMT,
		btnUniqueProfileSubject:       b.handleProfileSubject,
		btnUniqueProfileSubjectDelete: b.handleProfileSubjectDelete,
		btnUniqueProfileQuotas:        b.handleProfileQuotas,
		btnUniqueProfileQuotaToggle:   b.handleProfileQuotaToggle,
		btnUniqueProfileRegion:        b.handleProfileRegion,
		btnUniqueProfileRegionToggle:  b.handleProfileRegionToggle,
		btnUniqueProfileCreative:      b.handleProfileCreative,
	} {
		b.tg.Handle(&tele.Btn{Unique: unique}, h)
	}

	// Indicator-only button (current page count). Just close the spinner.
	b.tg.Handle(&tele.Btn{Unique: btnUniqueNoop}, func(c tele.Context) error {
		return c.Respond()
	})
}
