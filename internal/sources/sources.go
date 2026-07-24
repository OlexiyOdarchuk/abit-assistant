// Package sources wires the concrete data-source drivers into the shapes the
// service layer expects. It exists so the several command entrypoints
// (cmd/app, cmd/web, cmd/bot) build the sources identically — including
// optional, env-gated features — without duplicating the assembly.
package sources

import (
	"log/slog"
	"os"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/osvita"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/osvitabrowser"
)

// EnvOsvitaBrowserURL names the environment variable that, when set, points at
// a headless-Chromium DevTools endpoint (e.g. http://chromium:9222) used to
// clear osvita's Turnstile challenge on the applicant API. Unset → no browser
// fallback, and Turnstile-gated programs fail as they did before.
const EnvOsvitaBrowserURL = "OSVITA_BROWSER_URL"

// NewOsvita builds the vstup.osvita.ua source. When OSVITA_BROWSER_URL is set,
// it installs the browser-backed requests fallback so challenged programs still
// resolve; otherwise it returns a plain HTTP source. Extra opts are forwarded
// to osvita.New (kept last so callers can't accidentally clobber the fallback).
func NewOsvita(log *slog.Logger, opts ...osvita.Option) *osvita.Parser {
	if u := os.Getenv(EnvOsvitaBrowserURL); u != "" {
		fb := osvitabrowser.New(u, osvitabrowser.WithLogger(log))
		log.Info("osvita: browser fallback enabled", "browser_url", u)
		opts = append(opts, osvita.WithRequestsFallback(fb))
	}
	return osvita.New(opts...)
}
