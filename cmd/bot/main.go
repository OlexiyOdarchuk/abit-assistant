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
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/storage"
	"github.com/OlexiyOdarchuk/abit-assistant/pkg/parser/abitpoisk"
	"github.com/OlexiyOdarchuk/abit-assistant/pkg/parser/osvita"
)

const (
	programCacheTTL   = 10 * time.Minute
	applicantCacheTTL = 24 * time.Hour
	enrichWorkers     = 4
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	log := newLogger(cfg.LogLevel)
	slog.SetDefault(log)
	log.Info("starting abit-assistant bot",
		"database", cfg.DatabasePath,
		"admins", len(cfg.AdminIDs),
		"program_ttl", programCacheTTL,
		"applicant_ttl", applicantCacheTTL,
	)

	rootCtx, cancel := signal.NotifyContext(
		context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	store, err := storage.Open(rootCtx, cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("storage: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Warn("storage close", "err", err)
		}
	}()

	osvitaSrc := osvita.New()
	abitpoiskSrc := abitpoisk.New(abitpoisk.WithInsecureTLS())
	programSvc := service.NewProgramService(osvitaSrc, store, programCacheTTL)
	applicantSvc := service.NewApplicantService(abitpoiskSrc, store, applicantCacheTTL)
	enrichSvc := service.NewEnrichService(applicantSvc, enrichWorkers)

	b, err := bot.New(bot.Deps{
		Config:    cfg,
		Store:     store,
		Program:   programSvc,
		Applicant: applicantSvc,
		Enrich:    enrichSvc,
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
