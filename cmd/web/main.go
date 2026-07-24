// Command web runs the AbitAssistant website: a JSON API over the same
// service use cases as the bot, plus the embedded Svelte frontend.
//
// Environment:
//
//	HTTP_ADDR     — listen address (default ":8080")
//	DATABASE_PATH — SQLite path (default per config)
//	LOG_LEVEL     — debug | info | warn | error (default: info)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/config"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/abitpoisk"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/sources"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/storage"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/web"
)

const (
	programCacheTTL   = 10 * time.Minute
	applicantCacheTTL = 24 * time.Hour
	discoverWorkers   = 6
	simWorkers        = 4
	simMaxLookups     = 40
	vacuumInterval    = time.Hour
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	_ = config.LoadDotEnv(".env") // best-effort
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	log := newLogger(cfg.LogLevel)
	slog.SetDefault(log)

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8081"
	}

	rootCtx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	store, err := storage.Open(rootCtx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("storage: %w", err)
	}
	defer func() {
		if cerr := store.Close(); cerr != nil {
			log.Warn("storage close", "err", cerr)
		}
	}()

	// Physically evict stale cache rows on a schedule so third-party
	// applicant names don't accumulate forever (TTL alone only gates reads).
	go store.RunVacuum(rootCtx, vacuumInterval, programCacheTTL, applicantCacheTTL, log)

	osvitaSrc := sources.NewOsvita(log)
	abitpoiskSrc := abitpoisk.New(abitpoisk.WithInsecureTLS())
	programSvc := service.NewProgramService(osvitaSrc, store, programCacheTTL)
	applicantSvc := service.NewApplicantService(abitpoiskSrc, store, applicantCacheTTL)
	discoverSvc := service.NewDiscoverService(osvitaSrc, programSvc, discoverWorkers)
	resolverSvc := service.NewResolver(osvitaSrc)
	simSvc := service.NewPrioritySimulator(applicantSvc, resolverSvc, programSvc, simWorkers, simMaxLookups)

	srv := web.New(web.Deps{
		Program:   programSvc,
		Discover:  discoverSvc,
		Simulate:  simSvc,
		Applicant: applicantSvc,
		Logger:    log,
	})

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-rootCtx.Done()
		log.Info("shutdown requested")
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutCancel()
		_ = httpSrv.Shutdown(shutCtx)
	}()

	log.Info("starting web server", "addr", addr, "database", config.RedactDatabaseURL(cfg.DatabaseURL))
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http: %w", err)
	}
	log.Info("web server stopped")
	return nil
}

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
