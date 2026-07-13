// Package bot wires the Telegram presentation layer over the application
// services in internal/service. Handlers are kept thin — they format
// service results into Telegram messages and back.
package bot

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	tele "gopkg.in/telebot.v3"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/bot/fsm"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/config"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/storage"
)

// Bot is the running Telegram bot, with all its dependencies wired up.
type Bot struct {
	tg           *tele.Bot
	cfg          *config.Config
	store        *storage.Store
	fsm          *fsm.Manager
	programSvc   *service.ProgramService
	applicantSvc *service.ApplicantService
	discoverSvc  *service.DiscoverService
	simSvc       *service.PrioritySimulator
	activates    *activateTracker
	log          *slog.Logger
	// rootCtx is set by Run; long-running goroutines (broadcast) derive
	// their context from it so SIGTERM stops them gracefully.
	rootCtx context.Context
}

// Deps bundles everything Bot needs from the outside. cmd/bot is the
// only place that should construct it.
type Deps struct {
	Config     *config.Config
	Store      *storage.Store
	Program    *service.ProgramService
	Applicant  *service.ApplicantService
	Discover   *service.DiscoverService
	Simulate   *service.PrioritySimulator
	Logger     *slog.Logger
	PollerWait time.Duration // long-poll timeout; 10s is the safe default
}

// New constructs a Bot from Deps. It returns an error if the Telegram
// API rejects the token.
func New(deps Deps) (*Bot, error) {
	if deps.Config == nil || deps.Config.TelegramToken == "" {
		return nil, fmt.Errorf("bot: TELEGRAM_TOKEN required")
	}
	if deps.PollerWait == 0 {
		deps.PollerWait = 10 * time.Second
	}
	log := deps.Logger
	if log == nil {
		log = slog.Default()
	}
	tg, err := tele.NewBot(tele.Settings{
		Token:   deps.Config.TelegramToken,
		Poller:  &tele.LongPoller{Timeout: deps.PollerWait},
		OnError: onTelegramError(log),
	})
	if err != nil {
		return nil, fmt.Errorf("bot: telegram init: %w", err)
	}
	b := &Bot{
		tg:           tg,
		cfg:          deps.Config,
		store:        deps.Store,
		fsm:          fsm.New(deps.Store),
		programSvc:   deps.Program,
		applicantSvc: deps.Applicant,
		discoverSvc:  deps.Discover,
		simSvc:       deps.Simulate,
		log:          log.With("component", "bot"),
	}
	b.activates = newActivateTracker(deps.Store, b.log, 30*time.Second)
	b.registerRoutes()
	return b, nil
}

// Run blocks until ctx is cancelled, at which point the bot is gracefully
// stopped. Each Telegram poll cycle observes the context internally.
// ctx is also stashed on the Bot so detached operations (broadcasts)
// can derive shutdown-aware contexts from it.
func (b *Bot) Run(ctx context.Context) error {
	b.rootCtx = ctx
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		b.log.Info("starting telegram poller")
		b.tg.Start()
	}()
	// Flush buffered activation counters off the hot path; run() performs a
	// final flush when ctx is cancelled.
	go b.activates.run(ctx)
	<-ctx.Done()
	b.log.Info("shutdown requested")
	b.tg.Stop()
	<-stopped
	b.log.Info("telegram poller stopped")
	return nil
}

// onTelegramError builds a handler invoked by telebot for unexpected
// errors during update processing. Every handler error already flows
// through the logUpdates middleware (the outermost in the chain), which
// logs it at Error level with richer context (elapsed time, message
// text). Logging again here would double every error line — so this
// stays at Debug, present only as a backstop for errors that somehow
// bypass the middleware chain.
func onTelegramError(log *slog.Logger) func(error, tele.Context) {
	return func(err error, c tele.Context) {
		uid := int64(0)
		if c != nil && c.Sender() != nil {
			uid = c.Sender().ID
		}
		log.Debug("telegram OnError backstop", "err", err, "user_id", uid)
	}
}
