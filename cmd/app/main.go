// Command app runs the Telegram bot and the web server together in one
// process, sharing a single database and the same service layer. This is the
// deploy entrypoint for hosts that only allow one Dockerfile/container per
// repo — cmd/bot and cmd/web still exist for running either alone.
//
// Environment:
//
//	DATABASE_URL   — required, PostgreSQL connection URL
//	TELEGRAM_TOKEN — enables the bot; when unset, only the web runs
//	HTTP_ADDR      — web listen address (default ":8080")
//	ADMIN_IDS      — comma-separated Telegram admin user IDs
//	LOG_LEVEL      — debug | info | warn | error (default: info)
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

	"golang.org/x/sync/errgroup"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/bot"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/config"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/abitpoisk"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/osvita"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/service"
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
	_ = config.LoadDotEnv(".env") // best-effort; real env wins
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	log := newLogger(cfg.LogLevel)
	slog.SetDefault(log)

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
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
	if n, err := store.Queries.CountUsers(rootCtx); err == nil {
		log.Info("storage ready", "users", n, "database", config.RedactDatabaseURL(cfg.DatabaseURL))
	}

	go store.RunVacuum(rootCtx, vacuumInterval, programCacheTTL, applicantCacheTTL, log)

	// Services are built once and shared by both surfaces.
	osvitaSrc := osvita.New()
	abitpoiskSrc := abitpoisk.New(abitpoisk.WithInsecureTLS())
	programSvc := service.NewProgramService(osvitaSrc, store, programCacheTTL)
	applicantSvc := service.NewApplicantService(abitpoiskSrc, store, applicantCacheTTL)
	discoverSvc := service.NewDiscoverService(osvitaSrc, programSvc, discoverWorkers)
	resolverSvc := service.NewResolver(osvitaSrc)
	simSvc := service.NewPrioritySimulator(applicantSvc, resolverSvc, programSvc, simWorkers, simMaxLookups)

	// errgroup ties the two goroutines together: if one exits (error or
	// shutdown), gctx is cancelled and the other stops too.
	g, gctx := errgroup.WithContext(rootCtx)

	srv := web.New(web.Deps{
		Program:   programSvc,
		Discover:  discoverSvc,
		Simulate:  simSvc,
		Applicant: applicantSvc,
		Logger:    log,
	})
	g.Go(func() error { return runWeb(gctx, srv, addr, log) })

	if cfg.TelegramToken != "" {
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
		g.Go(func() error { return b.Run(gctx) })
	} else {
		log.Warn("TELEGRAM_TOKEN not set — running web only, bot disabled")
	}

	if err := g.Wait(); err != nil {
		return err
	}
	return nil
}

// runWeb serves the HTTP API/SPA until ctx is cancelled, then drains.
func runWeb(ctx context.Context, srv http.Handler, addr string, log *slog.Logger) error {
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutCancel()
		_ = httpSrv.Shutdown(shutCtx)
	}()
	log.Info("starting web server", "addr", addr)
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
