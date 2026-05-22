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
	"github.com/OlexiyOdarchuk/abit-assistant/pkg/abit"
)

// Subjects the profile lets the user enter scores for. Order matches
// the keyboard rendering — keep semantically grouped:
// the three required НМТ subjects first, then alternatives.
var profileSubjects = []string{
	"Українська мова",
	"Математика",
	"Історія України",
	"Англійська мова",
	"Українська література",
	"Біологія",
	"Фізика",
	"Хімія",
	"Географія",
	"Інша іноземна",
}

// FSM states owned by the profile flow.
const (
	fsmStateProfileEnterScore    = "profile.enter_score"
	fsmStateProfileEnterCreative = "profile.enter_creative"
)

// FSM data key carrying the subject currently being edited.
const fsmKeyCurrentSubject = "current_subject"

const (
	minScore      = 100.0
	maxScore      = 200.0
	maxCreative   = 200
	profileTTLMSG = 3 * time.Second
)

// --- Command + callback entry points -------------------------------------

func (b *Bot) renderProfile(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), profileTTLMSG)
	defer cancel()

	uid := senderID(c)
	nmt, err := b.store.GetUserNMT(ctx, uid)
	if err != nil {
		return fmt.Errorf("не вдалося прочитати НМТ: %w", err)
	}
	settings, err := b.store.GetUserSettings(ctx, uid)
	if err != nil {
		return fmt.Errorf("не вдалося прочитати налаштування: %w", err)
	}

	text, kb := buildProfileView(nmt, settings)
	return renderOrEdit(c, text, tele.ModeMarkdown, kb)
}

// handleProfileBack is the inline "⬅️ До профілю" button.
func (b *Bot) handleProfileBack(c tele.Context) error {
	if err := b.fsm.Clear(context.Background(), senderID(c)); err != nil {
		b.log.Warn("clear fsm on profile-back", "err", err)
	}
	return b.renderProfile(c)
}

// --- NMT scores ----------------------------------------------------------

func (b *Bot) handleProfileEditNMT(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), profileTTLMSG)
	defer cancel()
	nmt, err := b.store.GetUserNMT(ctx, senderID(c))
	if err != nil {
		return err
	}
	text, kb := buildNMTEditView(nmt)
	return renderOrEdit(c, text, tele.ModeMarkdown, kb)
}

// handleProfileSubject opens a per-subject screen: enter-score prompt
// for a new subject, or actions (edit / delete) for an existing one.
func (b *Bot) handleProfileSubject(c tele.Context) error {
	subj := callback.From(c).String(0)
	if subj == "" || !isKnownSubject(subj) {
		return errors.New("невідомий предмет")
	}

	uid := senderID(c)
	ctx, cancel := context.WithTimeout(context.Background(), profileTTLMSG)
	defer cancel()
	nmt, err := b.store.GetUserNMT(ctx, uid)
	if err != nil {
		return err
	}
	if existing, ok := nmt[subj]; ok {
		return b.renderSubjectActions(c, subj, existing)
	}
	return b.askForScore(c, subj)
}

// handleProfileSubjectDelete removes a subject from the user's НМТ map.
func (b *Bot) handleProfileSubjectDelete(c tele.Context) error {
	subj := callback.From(c).String(0)
	if subj == "" {
		return errors.New("не вказано предмет")
	}
	uid := senderID(c)
	ctx, cancel := context.WithTimeout(context.Background(), profileTTLMSG)
	defer cancel()

	nmt, err := b.store.GetUserNMT(ctx, uid)
	if err != nil {
		return err
	}
	if _, ok := nmt[subj]; !ok {
		// Nothing to delete — just go back.
		return b.handleProfileEditNMT(c)
	}
	delete(nmt, subj)
	if err := b.store.SetUserNMT(ctx, uid, nmt); err != nil {
		return fmt.Errorf("не вдалося зберегти НМТ: %w", err)
	}
	return b.handleProfileEditNMT(c)
}

func (b *Bot) askForScore(c tele.Context, subj string) error {
	uid := senderID(c)
	if err := b.fsm.Set(context.Background(), uid, fsmStateProfileEnterScore,
		map[string]any{fsmKeyCurrentSubject: subj}); err != nil {
		return fmt.Errorf("не вдалося зберегти стан: %w", err)
	}
	text := fmt.Sprintf("📝 Введи бал з предмету *%s* (число від 100 до 200):",
		mdEscape(subj))
	return renderOrEdit(c, text, tele.ModeMarkdown, backToProfileKeyboard())
}

func (b *Bot) renderSubjectActions(c tele.Context, subj string, score float64) error {
	text := fmt.Sprintf("📝 *%s*: `%g`\n\nЩо зробити?", mdEscape(subj), score)
	kb := &tele.ReplyMarkup{}
	kb.Inline(
		kb.Row(kb.Data("✏️ Змінити бал", btnUniqueProfileSubject, subj)),
		kb.Row(kb.Data("🗑 Видалити", btnUniqueProfileSubjectDelete, subj)),
		kb.Row(kb.Data("⬅️ Назад", btnUniqueProfileEditNMT)),
	)
	// Trick: handleProfileSubject re-asks for score when the subject exists,
	// but we want "Edit" to skip the actions screen and prompt for a value.
	// Clear the current score from the local NMT view by directly asking.
	if c.Callback() != nil && callback.From(c).String(1) == "edit" {
		return b.askForScore(c, subj)
	}
	return renderOrEdit(c, text, tele.ModeMarkdown, kb)
}

// handleProfileEnterScore is invoked from the OnText catch-all when the
// user is in fsmStateProfileEnterScore. Validates 100..200, saves, and
// returns the user to the NMT edit screen.
func (b *Bot) handleProfileEnterScore(c tele.Context, state map[string]any) error {
	subj, _ := state[fsmKeyCurrentSubject].(string)
	if subj == "" {
		_ = b.fsm.Clear(context.Background(), senderID(c))
		return errors.New("втрачено предмет — спробуй ще раз з /profile")
	}

	raw := strings.TrimSpace(c.Text())
	raw = strings.ReplaceAll(raw, ",", ".")
	score, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return errors.New("це не число. Введи бал від 100 до 200")
	}
	if score < minScore || score > maxScore {
		return fmt.Errorf("бал має бути від %.0f до %.0f", minScore, maxScore)
	}

	uid := senderID(c)
	ctx, cancel := context.WithTimeout(context.Background(), profileTTLMSG)
	defer cancel()

	nmt, err := b.store.GetUserNMT(ctx, uid)
	if err != nil {
		return err
	}
	if nmt == nil {
		nmt = storage.UserNMT{}
	}
	nmt[subj] = score
	if err := b.store.SetUserNMT(ctx, uid, nmt); err != nil {
		return fmt.Errorf("не вдалося зберегти НМТ: %w", err)
	}
	if err := b.fsm.Clear(context.Background(), uid); err != nil {
		b.log.Warn("clear fsm after score", "err", err)
	}

	text, kb := buildNMTEditView(nmt)
	confirmation := fmt.Sprintf("✅ Збережено: *%s* — `%g`\n\n", mdEscape(subj), score)
	return c.Send(confirmation+text, tele.ModeMarkdown, kb)
}

// --- Quotas --------------------------------------------------------------

func (b *Bot) handleProfileQuotas(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), profileTTLMSG)
	defer cancel()
	settings, err := b.store.GetUserSettings(ctx, senderID(c))
	if err != nil {
		return err
	}
	text, kb := buildQuotasView(settings.Quotas)
	return renderOrEdit(c, text, tele.ModeMarkdown, kb)
}

func (b *Bot) handleProfileQuotaToggle(c tele.Context) error {
	code := callback.From(c).String(0)
	if !isKnownQuota(code) {
		return errors.New("невідома квота")
	}
	uid := senderID(c)
	ctx, cancel := context.WithTimeout(context.Background(), profileTTLMSG)
	defer cancel()

	settings, err := b.store.GetUserSettings(ctx, uid)
	if err != nil {
		return err
	}
	settings.Quotas = toggle(settings.Quotas, code)
	if err := b.store.SetUserSettings(ctx, uid, settings); err != nil {
		return fmt.Errorf("не вдалося зберегти: %w", err)
	}
	text, kb := buildQuotasView(settings.Quotas)
	return renderOrEdit(c, text, tele.ModeMarkdown, kb)
}

// --- Regional coefficient ------------------------------------------------

func (b *Bot) handleProfileRegion(c tele.Context) error {
	ctx, cancel := context.WithTimeout(context.Background(), profileTTLMSG)
	defer cancel()
	settings, err := b.store.GetUserSettings(ctx, senderID(c))
	if err != nil {
		return err
	}
	text, kb := buildRegionView(settings.RegionCoef)
	return renderOrEdit(c, text, tele.ModeMarkdown, kb)
}

func (b *Bot) handleProfileRegionToggle(c tele.Context) error {
	uid := senderID(c)
	ctx, cancel := context.WithTimeout(context.Background(), profileTTLMSG)
	defer cancel()

	settings, err := b.store.GetUserSettings(ctx, uid)
	if err != nil {
		return err
	}
	settings.RegionCoef = !settings.RegionCoef
	if err := b.store.SetUserSettings(ctx, uid, settings); err != nil {
		return fmt.Errorf("не вдалося зберегти: %w", err)
	}
	text, kb := buildRegionView(settings.RegionCoef)
	return renderOrEdit(c, text, tele.ModeMarkdown, kb)
}

// --- Creative score ------------------------------------------------------

func (b *Bot) handleProfileCreative(c tele.Context) error {
	uid := senderID(c)
	ctx, cancel := context.WithTimeout(context.Background(), profileTTLMSG)
	defer cancel()
	settings, err := b.store.GetUserSettings(ctx, uid)
	if err != nil {
		return err
	}

	if err := b.fsm.Set(context.Background(), uid, fsmStateProfileEnterCreative, nil); err != nil {
		return fmt.Errorf("не вдалося зберегти стан: %w", err)
	}

	curMsg := "_не задано_"
	if settings.CreativeScorePrediction > 0 {
		curMsg = fmt.Sprintf("`%d`", settings.CreativeScorePrediction)
	}
	text := fmt.Sprintf(`🎨 *Творчий конкурс*

Поточний бал: %s

Введи прогнозований бал за творчий конкурс (число від 100 до 200), або /cancel.`, curMsg)
	return renderOrEdit(c, text, tele.ModeMarkdown, backToProfileKeyboard())
}

func (b *Bot) handleProfileEnterCreative(c tele.Context) error {
	raw := strings.TrimSpace(c.Text())
	raw = strings.ReplaceAll(raw, ",", ".")
	score, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return errors.New("це не число. Введи бал від 100 до 200")
	}
	if score < minScore || score > maxScore {
		return fmt.Errorf("бал має бути від %.0f до %.0f", minScore, maxScore)
	}

	uid := senderID(c)
	ctx, cancel := context.WithTimeout(context.Background(), profileTTLMSG)
	defer cancel()

	settings, err := b.store.GetUserSettings(ctx, uid)
	if err != nil {
		return err
	}
	settings.CreativeScorePrediction = int(score)
	if err := b.store.SetUserSettings(ctx, uid, settings); err != nil {
		return fmt.Errorf("не вдалося зберегти: %w", err)
	}
	if err := b.fsm.Clear(context.Background(), uid); err != nil {
		b.log.Warn("clear fsm after creative", "err", err)
	}
	return c.Send(fmt.Sprintf("✅ Творчий бал збережено: `%d`", settings.CreativeScorePrediction),
		tele.ModeMarkdown, backToProfileKeyboard())
}

// --- View builders -------------------------------------------------------

func buildProfileView(nmt storage.UserNMT, settings storage.UserSettings) (string, *tele.ReplyMarkup) {
	var sb strings.Builder
	sb.WriteString("👤 *Профіль*\n\n")

	sb.WriteString("📚 *Бали НМТ:*\n")
	if len(nmt) == 0 {
		sb.WriteString("   _не заповнено_\n")
	} else {
		for _, subj := range sortedKeys(map[string]float64(nmt)) {
			fmt.Fprintf(&sb, "   • %s: `%g`\n", mdEscape(subj), nmt[subj])
		}
	}

	sb.WriteString("\n⚙️ *Налаштування:*\n")
	if len(settings.Quotas) == 0 {
		sb.WriteString("   🏷 Квоти: _жодних_\n")
	} else {
		fmt.Fprintf(&sb, "   🏷 Квоти: %s\n", strings.Join(settings.Quotas, ", "))
	}
	fmt.Fprintf(&sb, "   🌍 Регіональний коеф.: %s\n", onOff(settings.RegionCoef))
	if settings.CreativeScorePrediction > 0 {
		fmt.Fprintf(&sb, "   🎨 Творчий бал: `%d`\n", settings.CreativeScorePrediction)
	}

	kb := &tele.ReplyMarkup{}
	kb.Inline(
		kb.Row(kb.Data("📝 Бали НМТ", btnUniqueProfileEditNMT)),
		kb.Row(
			kb.Data("🏷 Квоти", btnUniqueProfileQuotas),
			kb.Data("🌍 РК", btnUniqueProfileRegion),
		),
		kb.Row(kb.Data("🎨 Творчий конкурс", btnUniqueProfileCreative)),
		kb.Row(kb.Data("⬅️ Меню", btnUniqueMenu)),
	)
	return sb.String(), kb
}

func buildNMTEditView(nmt storage.UserNMT) (string, *tele.ReplyMarkup) {
	const intro = `📝 *Бали НМТ*

Натисни на предмет, щоб додати або змінити бал.
` + "`✅`" + ` — вже введений.`

	kb := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0)
	row := make([]tele.Btn, 0, 2)
	for _, subj := range profileSubjects {
		label := subj
		if _, ok := nmt[subj]; ok {
			label = "✅ " + subj
		}
		row = append(row, kb.Data(label, btnUniqueProfileSubject, subj))
		if len(row) == 2 {
			rows = append(rows, kb.Row(row...))
			row = row[:0]
		}
	}
	if len(row) > 0 {
		rows = append(rows, kb.Row(row...))
	}
	rows = append(rows, kb.Row(kb.Data("⬅️ До профілю", btnUniqueProfileBack)))
	kb.Inline(rows...)
	return intro, kb
}

func buildQuotasView(active []string) (string, *tele.ReplyMarkup) {
	const intro = `🏷 *Квоти*

Натисни, щоб увімкнути або вимкнути квоту. Активні квоти будуть враховані при фільтрації конкурентів.`

	kb := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(abit.AllQuotas)+1)
	for _, q := range abit.AllQuotas {
		label := q
		if contains(active, q) {
			label = "✅ " + q
		}
		rows = append(rows, kb.Row(kb.Data(label, btnUniqueProfileQuotaToggle, q)))
	}
	rows = append(rows, kb.Row(kb.Data("⬅️ До профілю", btnUniqueProfileBack)))
	kb.Inline(rows...)
	return intro, kb
}

func buildRegionView(active bool) (string, *tele.ReplyMarkup) {
	intro := fmt.Sprintf(`🌍 *Регіональний коефіцієнт*

Якщо твій ВНЗ дає РК (село / певний регіон), коефіцієнт буде застосовано при розрахунку шансів.

Поточний стан: *%s*`, onOff(active))

	label := "Увімкнути"
	if active {
		label = "Вимкнути"
	}

	kb := &tele.ReplyMarkup{}
	kb.Inline(
		kb.Row(kb.Data(label, btnUniqueProfileRegionToggle)),
		kb.Row(kb.Data("⬅️ До профілю", btnUniqueProfileBack)),
	)
	return intro, kb
}

// --- helpers -------------------------------------------------------------

func onOff(b bool) string {
	if b {
		return "✅ увімкнено"
	}
	return "❌ вимкнено"
}

func isKnownSubject(s string) bool {
	for _, x := range profileSubjects {
		if x == s {
			return true
		}
	}
	return false
}

func isKnownQuota(s string) bool {
	for _, x := range abit.AllQuotas {
		if x == s {
			return true
		}
	}
	return false
}

func contains(haystack []string, needle string) bool {
	for _, x := range haystack {
		if x == needle {
			return true
		}
	}
	return false
}

// toggle returns a NEW slice with v removed if present, otherwise appended.
func toggle(list []string, v string) []string {
	for i, x := range list {
		if x == v {
			out := make([]string, 0, len(list)-1)
			out = append(out, list[:i]...)
			out = append(out, list[i+1:]...)
			return out
		}
	}
	return append(append([]string{}, list...), v)
}
