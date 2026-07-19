// Command aa is the AbitAssistant developer CLI: it drives the parsers and
// crypto utilities from a terminal during development.
//
// Usage:
//
//	aa osvita    <program-url>
//	aa abitpoisk <"surname initial initial">
//	aa edbo decrypt <b64> <prsid> <n> [year]
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/abit"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/edbo"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/abitpoisk"
	"github.com/OlexiyOdarchuk/abit-assistant/internal/parser/osvita"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var err error
	switch os.Args[1] {
	case "osvita":
		err = runOsvita(ctx, os.Args[2:])
	case "abitpoisk":
		err = runAbitPoisk(ctx, os.Args[2:])
	case "decode":
		err = runDecode(os.Args[2:])
	case "edbo":
		err = runEdbo(os.Args[2:])
	case "help", "-h", "--help":
		usage(os.Stdout)
		return
	default:
		usage(os.Stderr)
		os.Exit(2)
	}
	if err != nil {
		if errors.Is(err, context.Canceled) {
			os.Exit(130)
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage(w *os.File) {
	fmt.Fprintln(w, `aa — abit-assistant CLI

Commands:
  osvita    <program-url>                  Parse a vstup.osvita.ua program page.
  abitpoisk <"surname initial initial">    Search abit-poisk.org.ua.
  decode    [< program.json]               Decode a Program (stdin) into []Abiturient.
  edbo decrypt <b64> <prsid> <n> [year]    Decrypt a 2025-archive EDBO name blob
                                           (vstup2025.edbo.gov.ua; the 2026
                                           portal is a React app, not this scheme).

Examples:
  aa osvita https://vstup.osvita.ua/y2025/r14/282/1471029/
  aa osvita https://... | aa decode
  aa abitpoisk "Бовкун О В"
  aa edbo decrypt dEZ3eS94ZjVVVWlab2lHd0o4ZHFJZz09 14 1 2025`)
}

func runOsvita(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: aa osvita <program-url>")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	p := osvita.New()
	prog, err := p.Parse(ctx, args[0])
	if err != nil {
		return err
	}
	return emitJSON(prog)
}

func runAbitPoisk(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New(`usage: aa abitpoisk "<surname initial initial>"`)
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// abit-poisk.org.ua serves a broken cert chain; the Python upstream
	// also disables verification. Keep it library-opt-in, enable here.
	slog.Warn("abit-poisk has a broken cert chain; TLS verification is disabled for this request")
	c := abitpoisk.New(abitpoisk.WithInsecureTLS())
	entries, err := c.Search(ctx, args[0])
	if err != nil {
		return err
	}
	return emitJSON(entries)
}

func runDecode(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: aa decode  (reads Program JSON from stdin)")
	}
	var prog abit.Program
	if err := json.NewDecoder(os.Stdin).Decode(&prog); err != nil {
		return fmt.Errorf("decode program: %w", err)
	}
	return emitJSON(abit.Decode(&prog))
}

func runEdbo(args []string) error {
	if len(args) < 1 || args[0] != "decrypt" {
		return errors.New("usage: aa edbo decrypt <b64> <prsid> <n> [year]")
	}
	rest := args[1:]
	if len(rest) < 3 || len(rest) > 4 {
		return errors.New("usage: aa edbo decrypt <b64> <prsid> <n> [year]")
	}
	year := "2025"
	if len(rest) == 4 {
		year = rest[3]
	}
	prsid, err := parseInt(rest[1], "prsid")
	if err != nil {
		return err
	}
	n, err := parseInt(rest[2], "n")
	if err != nil {
		return err
	}
	out, err := edbo.DecryptName(rest[0], prsid, n, year)
	if err != nil {
		return err
	}
	fmt.Println(out)
	return nil
}

func parseInt(s, what string) (int, error) {
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", what, err)
	}
	return v, nil
}

func emitJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
