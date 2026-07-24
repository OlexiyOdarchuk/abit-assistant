// Command AbitAssistant (root package) is the Wails desktop build: a native
// window hosting the same Svelte UI as the web server, backed by the shared
// core (internal/desktop.Core) instead of an HTTP API. The desktop shell exists
// because osvita's applicant API is gated behind Cloudflare Turnstile, which a
// real user browser clears naturally — the core drives a local headful Chrome
// to fetch, so no server-side scraping is needed.
//
// The server variants still live under cmd/{app,bot,web}; this root main is the
// desktop entrypoint (built with `wails build`, not the Dockerfile).
package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "AbitAssistant — шанси на вступ",
		Width:     1100,
		Height:    820,
		MinWidth:  900,
		MinHeight: 640,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []any{
			app,
		},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
