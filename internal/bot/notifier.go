package bot

import (
	"context"
	"fmt"
	"time"

	tele "gopkg.in/telebot.v3"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
)

// notifyInterval is how often the change-notifier re-checks saved lists.
// osvita re-syncs from EDBO a few times a day (it stamps each page "Дані
// отримані з ЄДЕБО HH:MM"); a 3h sweep catches every refresh within a few
// hours, and the SourceAsOf guard makes sweeps that hit unchanged data cheap.
const notifyInterval = 3 * time.Hour

// startupSweepDelay is how long after boot the first sweep runs. Without it the
// first notification would wait a whole interval — and on ephemeral hosting
// that restarts/redeploys often, the ticker might never reach 3h, so nobody
// ever gets notified. A short delay lets the process settle first.
const startupSweepDelay = 90 * time.Second

// runChangeNotifier periodically re-analyses every user's saved lists and DMs
// them when a program's admission chance changes (e.g. 🟢 High → 🟡 Medium).
// It turns the bot from a one-shot tool into something worth keeping open for
// the whole campaign: the decision about priorities is time-sensitive, and the
// applicant doesn't sit in the lists all day. Runs on a ticker until ctx is
// cancelled.
func (b *Bot) runChangeNotifier(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	// Run an initial sweep shortly after startup, then on the interval. This
	// makes the feature actually deliver on restart-happy hosting and testable
	// without waiting a full interval.
	select {
	case <-ctx.Done():
		return
	case <-time.After(startupSweepDelay):
		b.sweepChanceChanges(ctx)
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.sweepChanceChanges(ctx)
		}
	}
}

// sweepChanceChanges walks every user's saved lists once, comparing the stored
// snapshot's chance to a freshly fetched one and notifying on a change. Fetches
// go through the cache-aware, rate-limited ProgramService, so a sweep that
// touches many lists sharing the same programs stays cheap and polite.
func (b *Bot) sweepChanceChanges(ctx context.Context) {
	uids, err := b.store.Queries.ListUserIDs(ctx)
	if err != nil {
		b.log.WarnContext(ctx, "notifier: list users", "err", err)
		return
	}
	notified := 0
	for _, uid := range uids {
		select {
		case <-ctx.Done():
			return
		default:
		}
		notified += b.notifyUserChanges(ctx, uid)
	}
	if notified > 0 {
		b.log.InfoContext(ctx, "notifier: sweep done", "notifications", notified, "users", len(uids))
	}
}

// notifyUserChanges processes one user's saved lists and returns how many
// change notifications it sent.
func (b *Bot) notifyUserChanges(ctx context.Context, uid int64) int {
	lists, err := b.store.ListSavedLists(ctx, uid)
	if err != nil {
		b.log.WarnContext(ctx, "notifier: list saved", "err", err, "user_id", uid)
		return 0
	}
	if len(lists) == 0 {
		return 0
	}
	settings, _ := b.store.GetUserSettings(ctx, uid)
	if !settings.NotifyOnChange {
		return 0 // user hasn't opted in
	}
	nmt, _ := b.store.GetUserNMT(ctx, uid)
	in := abit.AnalyzeInput{
		UserScore:  0, // filled per program below (rating depends on the program)
		UserQuotas: settings.Quotas,
	}
	rating := abit.RatingInput{
		NMT:           map[string]float64(nmt),
		CreativeScore: float64(settings.CreativeScorePrediction),
	}

	sent := 0
	for _, l := range lists {
		if l.Program == nil || l.URL == "" {
			continue
		}
		in.UserScore = abit.ComputeRating(l.Program, rating)
		oldChance := abit.Analyze(l.Program, abit.Decode(l.Program), in).Chance

		fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		fresh, err := b.programSvc.Fetch(fetchCtx, l.URL)
		cancel()
		if err != nil {
			b.log.WarnContext(ctx, "notifier: fetch", "err", err, "url", l.URL)
			continue
		}
		// NB: we intentionally do NOT gate on osvita's "Дані отримані з ЄДЕБО"
		// stamp. That stamp has minute granularity and osvita can reuse/cache it
		// while the underlying table (enrolments, cutoff, competitor scores)
		// changes — gating on it made real chance changes never fire. Recompute
		// every sweep; the ProgramService cache keeps the fetch cheap.
		in.UserScore = abit.ComputeRating(fresh, rating)
		newA := abit.Analyze(fresh, abit.Decode(fresh), in)

		// Degraded to "Unknown" (osvita dropped the license volume, or the score
		// couldn't be computed): we can't say anything useful, and we must NOT
		// overwrite the last-good snapshot — otherwise a later recovery back to a
		// real verdict wouldn't be detected. Leave the baseline untouched.
		if newA.Chance == abit.ChanceUnknown {
			continue
		}

		if !chanceChanged(oldChance, newA.Chance) {
			// Same verdict — advance the snapshot so /lists and the next baseline
			// track fresh data without re-announcing.
			if err := b.store.UpdateSavedListProgram(ctx, l.ID, fresh); err != nil {
				b.log.WarnContext(ctx, "notifier: refresh snapshot", "err", err, "list_id", l.ID)
			}
			continue
		}
		if _, err := b.tg.Send(&tele.User{ID: uid},
			buildChanceChangeMessage(l.Name, fresh, oldChance, newA),
			tele.ModeMarkdown, tele.NoPreview); err != nil {
			// A blocked/deleted user surfaces here — log and move on, but do
			// NOT advance the snapshot, so we retry the notice next sweep.
			b.log.WarnContext(ctx, "notifier: send", "err", err, "user_id", uid)
			continue
		}
		// Advance the baseline so the same transition isn't re-announced; the
		// next change is measured against what the user was just told.
		if err := b.store.UpdateSavedListProgram(ctx, l.ID, fresh); err != nil {
			b.log.WarnContext(ctx, "notifier: update snapshot", "err", err, "list_id", l.ID)
		}
		sent++
	}
	return sent
}

// chanceChanged reports whether a chance transition is worth telling the user
// about: the level actually changed, the new level is meaningful (not Unknown),
// AND it isn't a cosmetic relabel between two "you pass" levels. The three
// High* levels (general / Quota 1 / Quota 2) all mean "проходиш", so moving
// between them isn't news worth a DM.
func chanceChanged(old, new abit.ChanceLevel) bool {
	if new == abit.ChanceUnknown || old == new {
		return false
	}
	if old.Tier() == abit.TierSafety && new.Tier() == abit.TierSafety {
		return false // High ↔ High(Quota) — same practical verdict
	}
	return true
}

// buildChanceChangeMessage renders the DM sent when a saved program's chance
// changes.
func buildChanceChangeMessage(name string, prog *abit.Program, old abit.ChanceLevel, newA abit.Analysis) string {
	var header string
	switch {
	case newA.Chance.Tier() > old.Tier():
		header = "📈 Твій шанс зріс!"
	case newA.Chance.Tier() < old.Tier():
		header = "📉 Твій шанс змінився"
	default:
		header = "🔔 Оновлення по збереженій програмі"
	}
	msg := fmt.Sprintf("%s\n\n📂 *%s*\n", header, mdEscape(name))
	if prog != nil && prog.UniversityName != "" {
		msg += fmt.Sprintf("🎓 %s", mdEscape(prog.UniversityName))
		if prog.ProgramName != "" {
			msg += fmt.Sprintf(" — %s", mdEscape(prog.ProgramName))
		}
		msg += "\n"
	}
	msg += fmt.Sprintf("\nШанс: %s %s → %s %s\n",
		old.Emoji(), old.Label(), newA.Chance.Emoji(), newA.Chance.Label())
	if newA.Advice != "" {
		msg += fmt.Sprintf("\n💡 %s\n", mdEscape(newA.Advice))
	}
	if prog != nil && prog.SourceAsOf != "" {
		msg += fmt.Sprintf("\n_Дані osvita станом на %s._", mdEscape(prog.SourceAsOf))
	}
	msg += "\nДеталі — у /lists."
	return msg
}
