package bot

import (
	tele "gopkg.in/telebot.v3"
)

// Texts kept here so every handler renders the same canonical phrasing.
const (
	welcomeText = `🎓 *AbitAssistant*

Я допоможу побачити, хто реально конкурує з тобою на бюджет, а хто просто заповнює список. Дані тягну з vstup.osvita.ua і abit-poisk.

Обери дію 👇`

	helpText = `*Команди*

` + "`/menu`" + ` — головне меню
` + "`/search <url>`" + ` — швидкий аналіз програми з vstup.osvita.ua
` + "`/where`" + ` — куди я вступлю: підбір програм за галуззю і регіоном
` + "`/profile`" + ` — твої НМТ і налаштування
` + "`/lists`" + ` — збережені аналізи
` + "`/admin`" + ` — адмін-панель (доступно тільки адміністраторам)
` + "`/about`" + ` — про бот
` + "`/cancel`" + ` — вийти з поточного діалогу
` + "`/help`" + ` — це повідомлення

Можна також просто скинути боту посилання на vstup.osvita.ua — він зрозуміє.`

	aboutText = `*AbitAssistant 3.0*

Open-source бот для абітурієнтів України. Тягне конкурсні списки з vstup.osvita.ua, шукає «реальних» конкурентів і рахує твої шанси.

👨‍💻 Автор: [Олексій Одарчук](https://t.me/NeShawyha)
🛠 Код: [GitHub](https://github.com/OlexiyOdarchuk/abit-assistant)
📄 Ліцензія: GPLv3
💸 [Підтримати на Monobank](https://send.monobank.ua/jar/23E3WYNesG)`
)

// Unique strings for inline-keyboard buttons. Each handler registers
// against these so dispatch is type-safe — no magic-string callback data.
const (
	btnUniqueMenu             = "menu"
	btnUniqueSearch           = "search"
	btnUniqueProfile          = "profile"
	btnUniqueLists            = "lists"
	btnUniqueAbout            = "about"
	btnUniquePagePrev         = "page_prev"
	btnUniquePageNext         = "page_next"
	btnUniqueApplicant        = "applicant_view"
	btnUniqueApplicantHistory = "applicant_history"
	btnUniqueToggleThreat     = "toggle_threat"
	btnUniqueBackToList       = "back_to_list"
	btnUniqueToggleMode       = "toggle_mode"
	btnUniqueSummary          = "summary"
	btnUniqueViewList         = "view_list"
	btnUniqueChart            = "chart"
	btnUniqueSaveList         = "save_list"

	// Discover ("where can I get in") flow.
	btnUniqueDiscover       = "disc"
	btnUniqueDiscoverGaluz  = "disc_g"
	btnUniqueDiscoverRegion = "disc_r"
	btnUniqueDiscoverPage   = "disc_p"
	btnUniqueDiscoverResult = "disc_res"

	// Saved lists.
	btnUniqueListManage        = "l_mng"
	btnUniqueListView          = "l_view"
	btnUniqueListRefresh       = "l_refresh"
	btnUniqueListShare         = "l_share"
	btnUniqueListExport        = "l_export"
	btnUniqueListDelete        = "l_del"
	btnUniqueListDeleteConfirm = "l_del_yes"
	btnUniqueListsBack         = "l_back"

	btnUniqueNoop = "noop"

	// Admin panel.
	btnUniqueAdmin                 = "admin"
	btnUniqueAdminStats            = "admin_stats"
	btnUniqueAdminBroadcast        = "admin_bc"
	btnUniqueAdminBroadcastConfirm = "admin_bc_yes"
	btnUniqueAdminBroadcastCancel  = "admin_bc_no"
	btnUniqueAdminVacuum           = "admin_vacuum"

	// Profile flow.
	btnUniqueProfileEditNMT       = "p_edit_nmt"
	btnUniqueProfileSubject       = "p_subj"
	btnUniqueProfileSubjectEdit   = "p_subj_edit"
	btnUniqueProfileSubjectDelete = "p_subj_del"
	btnUniqueProfileQuotas        = "p_quotas"
	btnUniqueProfileQuotaToggle   = "p_quota_t"
	btnUniqueProfileRegion        = "p_region"
	btnUniqueProfileRegionToggle  = "p_region_t"
	btnUniqueProfileCreative      = "p_creative"
	btnUniqueProfileBack          = "p_back"
)

// mainMenuKeyboard builds the inline keyboard shown on /start and /menu.
func mainMenuKeyboard() *tele.ReplyMarkup {
	kb := &tele.ReplyMarkup{}
	kb.Inline(
		kb.Row(kb.Data("📊 Аналіз спеціальності", btnUniqueSearch)),
		kb.Row(kb.Data("🧭 Куди я вступлю", btnUniqueDiscover)),
		kb.Row(
			kb.Data("👤 Профіль", btnUniqueProfile),
			kb.Data("📂 Списки", btnUniqueLists),
		),
		kb.Row(
			kb.Data("ℹ️ Про бот", btnUniqueAbout),
			kb.URL("💸 Підтримати", "https://send.monobank.ua/jar/23E3WYNesG"),
		),
	)
	return kb
}

// backToMenuKeyboard is the single-button keyboard for sub-screens.
func backToMenuKeyboard() *tele.ReplyMarkup {
	kb := &tele.ReplyMarkup{}
	kb.Inline(kb.Row(kb.Data("⬅️ Меню", btnUniqueMenu)))
	return kb
}

// backToProfileKeyboard for screens nested inside /profile.
func backToProfileKeyboard() *tele.ReplyMarkup {
	kb := &tele.ReplyMarkup{}
	kb.Inline(kb.Row(kb.Data("⬅️ До профілю", btnUniqueProfileBack)))
	return kb
}

// renderOrEdit sends a new message OR edits the current one when the
// trigger was an inline button (callback). This is what keeps the chat
// from flickering — Python's delete+answer is replaced with a single
// in-place update.
func renderOrEdit(c tele.Context, text string, opts ...any) error {
	if c.Callback() != nil {
		err := c.Edit(text, opts...)
		// "message is not modified" happens when the user double-taps a
		// button that points to the screen they're already on — silent.
		if err != nil && !isNotModified(err) {
			return err
		}
		return c.Respond()
	}
	return c.Send(text, opts...)
}

func isNotModified(err error) bool {
	return err != nil && (containsCI(err.Error(), "not modified") ||
		containsCI(err.Error(), "message is not modified"))
}

// containsCI is a tiny case-insensitive substring check we want to avoid
// pulling strings.ToLower allocations into hot paths.
func containsCI(haystack, needle string) bool {
	// Telegram errors are ASCII; a lowercased compare is fine.
	if len(haystack) < len(needle) {
		return false
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			a, b := haystack[i+j], needle[j]
			if a >= 'A' && a <= 'Z' {
				a += 'a' - 'A'
			}
			if b >= 'A' && b <= 'Z' {
				b += 'a' - 'A'
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
