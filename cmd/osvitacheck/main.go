// Command osvitacheck validates the full desktop fetch path against live
// osvita, end to end: plain HTTP static GET → on 403, a single headful-browser
// run that clears the Cloudflare/Turnstile challenge and returns the page HTML
// + the applicant requests → parse → validate. It runs the SAME osvita.Parse
// the desktop app uses, so a green run here means the app will work.
//
// Run it ON YOUR DESKTOP (needs a display + an installed Chrome/Chromium):
//
//	go run ./cmd/osvitacheck https://vstup.osvita.ua/y2026/r27/41/1612502/
//
// A Chrome window appears; solve the "я не робот" checkbox if it shows one.
// Success prints the program name, subjects-config size and applicant count —
// if the name and subjects are populated, the app's score/chances/rivals will
// compute correctly (they're derived from exactly these).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/osvita"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/osvitabrowser"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: osvitacheck <vstup.osvita.ua program URL> [chrome-binary-path]")
		os.Exit(2)
	}
	programURL := os.Args[1]

	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	var opts []osvitabrowser.LocalOption
	opts = append(opts, osvitabrowser.WithLocalLogger(log))
	if len(os.Args) >= 3 {
		opts = append(opts, osvitabrowser.WithExecPath(os.Args[2]))
	}
	browser := osvitabrowser.NewLocal(opts...)
	src := osvita.New(
		osvita.WithProgramDataFetcher(browser),
		osvita.WithRequestsFallback(browser),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	fmt.Printf("fetching %s (a Chrome window will open — solve the captcha if shown)…\n", programURL)
	start := time.Now()
	prog, err := src.Parse(ctx, programURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ FAIL after %s: %v\n", time.Since(start).Round(time.Millisecond), err)
		os.Exit(1)
	}
	fmt.Printf("\n✅ OK in %s:\n", time.Since(start).Round(time.Millisecond))
	fmt.Printf("   program:  %q\n", prog.ProgramName)
	fmt.Printf("   spec:     %s\n", prog.SpecCode)
	fmt.Printf("   subjects: %d (config for score) \n", len(prog.Subjects))
	fmt.Printf("   requests: %d applicants\n", len(prog.Requests))
	if prog.ProgramName == "" || len(prog.Subjects) == 0 {
		fmt.Fprintln(os.Stderr, "⚠️  name or subjects EMPTY — analysis would show no score/chances. This is the bug guard talking.")
		os.Exit(1)
	}
}
