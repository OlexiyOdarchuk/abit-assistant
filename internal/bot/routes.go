package bot

import (
	tele "gopkg.in/telebot.v3"
)

// registerRoutes binds every command and callback handler to the
// telebot instance and applies the middleware chain.
//
// Middleware order matters. With telebot's Use, the LAST one passed is
// the innermost — so the call ordering for a request is:
//
//	logUpdates → reportErrors → trackUser → recoverPanics → handler
//
// recoverPanics must be the innermost so it catches the panic *before*
// it unwinds past reportErrors — otherwise the user gets no friendly
// toast when a handler panics.
func (b *Bot) registerRoutes() {
	b.tg.Use(b.logUpdates, b.reportErrors, b.trackUser, b.recoverPanics)

	// Commands.
	b.tg.Handle("/start", b.handleStart)
	b.tg.Handle("/menu", b.handleMenu)
	b.tg.Handle("/help", b.handleHelp)
	b.tg.Handle("/about", b.handleAbout)
	b.tg.Handle("/cancel", b.handleCancel)
	b.tg.Handle("/search", b.handleSearch)
	b.tg.Handle("/profile", b.handleProfile)
	b.tg.Handle("/lists", b.handleLists)
	b.tg.Handle("/where", b.handleDiscover)
	b.tg.Handle("/admin", b.handleAdmin)

	// Catch-all text: route to active FSM step or implicit /search.
	b.tg.Handle(tele.OnText, b.handleText)

	// Inline keyboards — every button gets a typed handler keyed on
	// its Unique string (no fragile callback_data parsing).
	for unique, h := range map[string]tele.HandlerFunc{
		btnUniqueMenu:                  b.handleMenuCB,
		btnUniqueSearch:                b.handleSearchCB,
		btnUniqueProfile:               b.handleProfileCB,
		btnUniqueLists:                 b.handleListsCB,
		btnUniqueAbout:                 b.handleAboutCB,
		btnUniquePagePrev:              b.handlePagePrev,
		btnUniquePageNext:              b.handlePageNext,
		btnUniqueApplicant:             b.handleApplicantView,
		btnUniqueApplicantHistory:      b.handleApplicantHistory,
		btnUniqueBackToList:            b.handleBackToList,
		btnUniqueToggleMode:            b.handleToggleMode,
		btnUniqueSummary:               b.handleSummaryCB,
		btnUniqueViewList:              b.handleViewListCB,
		btnUniqueChart:                 b.handleChartCB,
		btnUniqueSaveList:              b.handleSaveListCB,
		btnUniqueRefine:                b.handleRefine,
		btnUniqueToggleUnlikely:        b.handleToggleUnlikely,
		btnUniqueDiscover:              b.handleDiscover,
		btnUniqueDiscoverGaluz:         b.handleDiscoverGaluz,
		btnUniqueDiscoverRegionTog:     b.handleDiscoverRegionTog,
		btnUniqueDiscoverRun:           b.handleDiscoverRun,
		btnUniqueDiscoverPage:          b.handleDiscoverPage,
		btnUniqueDiscoverResult:        b.handleDiscoverResult,
		btnUniqueDiscoverMore:          b.handleDiscoverMore,
		btnUniqueDiscoverSaveSafe:      b.handleDiscoverSaveSafe,
		btnUniqueDiscoverOnlyPassTog:   b.handleDiscoverOnlyPassTog,
		btnUniqueDiscoverBack:          b.handleDiscoverBack,
		btnUniqueDiscoverSpec:          b.handleDiscoverSpec,
		btnUniqueDiscoverBudgetTog:     b.handleDiscoverBudgetTog,
		btnUniqueListManage:            b.handleListManage,
		btnUniqueListView:              b.handleListView,
		btnUniqueListRefresh:           b.handleListRefresh,
		btnUniqueListShare:             b.handleListShare,
		btnUniqueListExport:            b.handleListExport,
		btnUniqueListDelete:            b.handleListDelete,
		btnUniqueListDeleteConfirm:     b.handleListDeleteConfirm,
		btnUniqueListsBack:             b.handleListsBack,
		btnUniqueNotifyToggle:          b.handleNotifyToggle,
		btnUniqueAdmin:                 b.handleAdminCB,
		btnUniqueAdminStats:            b.handleAdminStats,
		btnUniqueAdminBroadcast:        b.handleAdminBroadcast,
		btnUniqueAdminBroadcastConfirm: b.handleAdminBroadcastConfirm,
		btnUniqueAdminBroadcastCancel:  b.handleAdminBroadcastCancel,
		btnUniqueAdminVacuum:           b.handleAdminVacuum,
		btnUniqueProfileBack:           b.handleProfileBack,
		btnUniqueProfileEditNMT:        b.handleProfileEditNMT,
		btnUniqueProfileSubject:        b.handleProfileSubject,
		btnUniqueProfileSubjectEdit:    b.handleProfileSubjectEdit,
		btnUniqueProfileSubjectDelete:  b.handleProfileSubjectDelete,
		btnUniqueProfileQuotas:         b.handleProfileQuotas,
		btnUniqueProfileQuotaToggle:    b.handleProfileQuotaToggle,
		btnUniqueProfileCreative:       b.handleProfileCreative,
	} {
		b.tg.Handle(&tele.Btn{Unique: unique}, h)
	}

	// Indicator-only button (current page count). Just close the spinner.
	b.tg.Handle(&tele.Btn{Unique: btnUniqueNoop}, func(c tele.Context) error {
		return c.Respond()
	})
}
