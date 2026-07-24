package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/apidto"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/desktop"
)

// App is the Wails-bound adapter: it owns the app lifecycle (a SQLite cache and
// the desktop Core) and forwards each bound method to the Core using the Wails
// startup context. Every exported method here becomes callable from the Svelte
// frontend as window.go.main.App.<Method>.
type App struct {
	ctx   context.Context
	core  *desktop.Core
	cache *desktop.Cache
	log   *slog.Logger
}

// NewApp builds the adapter; the heavy wiring happens in startup once Wails
// hands us a context.
func NewApp() *App {
	return &App{log: slog.Default()}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	path, err := cacheDBPath()
	if err != nil {
		a.log.Error("desktop: cache path", "err", err)
		path = ":memory:"
	}
	cache, err := desktop.OpenCache(path)
	if err != nil {
		// Never fail startup over the cache — fall back to an ephemeral one so
		// the app still runs, just without cross-launch persistence.
		a.log.Error("desktop: open cache, using in-memory", "path", path, "err", err)
		cache, _ = desktop.OpenCache(":memory:")
	}
	a.cache = cache
	a.core = desktop.NewCore(cache, a.log)
	a.log.Info("desktop: ready", "cache", path)
}

func (a *App) shutdown(_ context.Context) {
	if a.cache != nil {
		_ = a.cache.Close()
	}
}

// cacheDBPath returns a per-user writable path for the SQLite cache
// (e.g. ~/.config/AbitAssistant/cache.db on Linux).
func cacheDBPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	appDir := filepath.Join(dir, "AbitAssistant")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(appDir, "cache.db"), nil
}

// --- bound methods (window.go.main.App.*) — mirror the web JSON API ---

// GetFilters returns the static discover pickers.
func (a *App) GetFilters() (apidto.FiltersResp, error) {
	return a.core.GetFilters(a.ctx)
}

// Analyze scores the user against one program's competitive list.
func (a *App) Analyze(url string, profile apidto.Profile) (apidto.AnalyzeResp, error) {
	return a.core.Analyze(a.ctx, url, profile)
}

// Discover runs the "where can I get in" search.
func (a *App) Discover(req desktop.DiscoverRequest) (apidto.DiscoverResp, error) {
	return a.core.Discover(a.ctx, req)
}

// Simulate runs the priority simulation on a program.
func (a *App) Simulate(url string, profile apidto.Profile, deep bool) (apidto.SimulateResp, error) {
	return a.core.Simulate(a.ctx, url, profile, deep)
}

// Applicant returns an applicant's other applications.
func (a *App) Applicant(name string, score float64) (apidto.ApplicantResp, error) {
	return a.core.Applicant(a.ctx, name, score)
}

// Predict scores the user's ranked application list.
func (a *App) Predict(urls []string, profile apidto.Profile, excludeUnlikely bool) (apidto.PredictResp, error) {
	return a.core.Predict(a.ctx, urls, profile, excludeUnlikely)
}
