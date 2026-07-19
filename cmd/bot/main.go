// Command bot runs the AbitAssistant Telegram bot.
//
// Environment:
//
//	TELEGRAM_TOKEN  — required, Bot API token from @BotFather
//	DATABASE_PATH   — defaults to ./data/abit.db
//	ADMIN_IDS       — comma-separated Telegram user IDs
//	LOG_LEVEL       — debug | info | warn | error (default: info)
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/bot"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/config"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/abitpoisk"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/osvita"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/storage"
)

const (
	programCacheTTL   = 10 * time.Minute
	applicantCacheTTL = 24 * time.Hour
	discoverWorkers   = 6
	simWorkers        = 4
	simMaxLookups     = 40
	// vacuumInterval bounds how often stale cache rows (including
	// third-party applicant names) are physically evicted.
	vacuumInterval = time.Hour
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	// Best-effort: load .env if present in CWD. Existing env vars win.
	if err := config.LoadDotEnv(".env"); err != nil {
		return fmt.Errorf(".env: %w", err)
	}
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	log := newLogger(cfg.LogLevel)
	slog.SetDefault(log)

	cwd, _ := os.Getwd()
	log.Info("starting abit-assistant bot",
		"database", config.RedactDatabaseURL(cfg.DatabaseURL),
		"cwd", cwd,
		"admins", len(cfg.AdminIDs),
		"program_ttl", programCacheTTL,
		"applicant_ttl", applicantCacheTTL,
	)

	rootCtx, cancel := signal.NotifyContext(
		context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	store, err := storage.Open(rootCtx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("storage: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Warn("storage close", "err", err)
		}
	}()
	// Read-back: prove we opened a populated DB (or note that it's new).
	if n, err := store.Queries.CountUsers(rootCtx); err == nil {
		log.Info("storage ready", "users", n)
	}
	// Physically evict stale cache rows on a schedule so third-party
	// applicant names don't accumulate forever (TTL alone only gates reads).
	go store.RunVacuum(rootCtx, vacuumInterval, programCacheTTL, applicantCacheTTL, log)

	osvitaSrc := osvita.New()
	abitpoiskSrc := abitpoisk.New(abitpoisk.WithInsecureTLS())
	programSvc := service.NewProgramService(osvitaSrc, store, programCacheTTL)
	applicantSvc := service.NewApplicantService(abitpoiskSrc, store, applicantCacheTTL)
	// osvitaSrc doubles as the program browser (it implements both
	// parser.Source and service.ProgramBrowser).
	discoverSvc := service.NewDiscoverService(osvitaSrc, programSvc, discoverWorkers)
	// Resolver + ProgramService let the simulator predict placements before
	// recommendation waves (fetch & rank a competitor's higher-priority
	// programs); osvitaSrc provides the university directory + /spec/ browse.
	resolverSvc := service.NewResolver(osvitaSrc)
	simSvc := service.NewPrioritySimulator(applicantSvc, resolverSvc, programSvc, simWorkers, simMaxLookups)

	b, err := bot.New(bot.Deps{
		Config:    cfg,
		Store:     store,
		Program:   programSvc,
		Applicant: applicantSvc,
		Discover:  discoverSvc,
		Simulate:  simSvc,
		Logger:    log,
	})
	if err != nil {
		return fmt.Errorf("bot: %w", err)
	}
	return b.Run(rootCtx)
}

// newLogger builds a JSON slog logger at the requested level.
// Unknown LOG_LEVEL falls back to info.
func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}
