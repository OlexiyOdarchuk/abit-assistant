// Command osvitacheck is a throwaway validation tool for the desktop pivot: it
// launches a local HEADFUL Chrome and tries to fetch a program's applicant
// list from vstup.osvita.ua through it. Its only job is to answer one question
// before we build the Wails shell: does a chromedp-launched real browser clear
// osvita's Turnstile challenge on a real desktop?
//
// Run it ON YOUR DESKTOP (it needs a display + an installed Chrome/Chromium):
//
//	go run ./cmd/osvitacheck https://vstup.osvita.ua/y2026/r27/41/1612502/
//
// A Chrome window will briefly appear while it solves the challenge. Success =
// it prints a non-zero applicant count. Failure ("turnstile ... curLen=0") =
// even headful-launched Chrome is fingerprinted, and we need stealth tuning or
// a different browser-launch strategy.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"time"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/osvitabrowser"
)

// osvita program URL: /yYYYY/rNN/<uid>/<sid>/
var urlRe = regexp.MustCompile(`/y(\d{4})/[^/]+/(\d+)/(\d+)/?$`)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: osvitacheck <vstup.osvita.ua program URL> [chrome-binary-path]")
		os.Exit(2)
	}
	programURL := os.Args[1]
	m := urlRe.FindStringSubmatch(programURL)
	if m == nil {
		fmt.Fprintf(os.Stderr, "URL doesn't look like a program page: %s\n", programURL)
		os.Exit(2)
	}
	year, uid, sid := m[1], m[2], m[3]

	var opts []osvitabrowser.LocalOption
	opts = append(opts, osvitabrowser.WithLocalLogger(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))))
	if len(os.Args) >= 3 {
		opts = append(opts, osvitabrowser.WithExecPath(os.Args[2]))
	}
	drv := osvitabrowser.NewLocal(opts...)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	fmt.Printf("launching headful Chrome, fetching %s (year=%s sid=%s uid=%s)…\n", programURL, year, sid, uid)
	start := time.Now()
	reqs, subj, err := drv.FetchRequests(ctx, programURL, year, sid, uid)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ FAIL after %s: %v\n", time.Since(start).Round(time.Millisecond), err)
		os.Exit(1)
	}
	fmt.Printf("\n✅ OK in %s: %d applicant requests, %d subject rows — Turnstile passed.\n",
		time.Since(start).Round(time.Millisecond), len(reqs), len(subj))
}
