package osvitabrowser

import (
	"context"
	"log/slog"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/chromedp/chromedp"
)

// LocalDriver launches a local Chrome/Chromium on the user's own machine and
// drives it via CDP. Unlike the remote Driver (a headless sidecar, which
// osvita's Turnstile fingerprints and blocks), this runs the browser HEADFUL
// on a real desktop — the same shape that passed the challenge during recon —
// which is the whole reason the desktop build can reach osvita at all.
//
// It reuses the exact in-page collector as the remote driver; only the browser
// allocation differs (chromedp launches a found Chrome instead of attaching to
// a remote one).
type LocalDriver struct {
	log      *slog.Logger
	execPath string   // optional: explicit Chrome/Chromium binary; "" → auto-detect
	extra    []string // optional: extra raw chrome flags
}

// LocalOption configures a LocalDriver.
type LocalOption func(*LocalDriver)

// WithLocalLogger attaches a logger.
func WithLocalLogger(l *slog.Logger) LocalOption { return func(d *LocalDriver) { d.log = l } }

// WithExecPath pins the Chrome/Chromium binary to launch (otherwise chromedp
// auto-detects an installed Chrome/Edge/Chromium).
func WithExecPath(p string) LocalOption { return func(d *LocalDriver) { d.execPath = p } }

// NewLocal builds a LocalDriver.
func NewLocal(opts ...LocalOption) *LocalDriver {
	d := &LocalDriver{log: slog.Default()}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// FetchRequests implements osvita.RequestsFetcher by launching a headful local
// browser and running the collector against programURL.
func (d *LocalDriver) FetchRequests(ctx context.Context, programURL, year, sid, uid string) ([]abit.RawRequest, map[string]abit.ApplicantSubjects, error) {
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, d.allocatorOptions()...)
	defer cancelAlloc()
	return runCollect(allocCtx, d.log, programURL, year, sid, uid)
}

// allocatorOptions builds the chromedp launch flags. It starts from chromedp's
// defaults but forces HEADFUL and strips the automation signals that make a
// launched Chrome look like a bot to Turnstile:
//   - headless=false: a real rendered browser (headless is detected/blocked).
//   - enable-automation off + AutomationControlled disabled: hide navigator.
//     webdriver and the "controlled by automation" markers.
//
// A real, recent Chrome/Chromium must be installed; chromedp auto-detects it
// unless WithExecPath pins one.
func (d *LocalDriver) allocatorOptions() []chromedp.ExecAllocatorOption {
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts,
		chromedp.Flag("headless", false),
		chromedp.Flag("enable-automation", false),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
	)
	if d.execPath != "" {
		opts = append(opts, chromedp.ExecPath(d.execPath))
	}
	for _, f := range d.extra {
		opts = append(opts, chromedp.Flag(f, true))
	}
	return opts
}
