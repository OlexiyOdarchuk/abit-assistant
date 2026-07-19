// Package config loads runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
)

// Config is the runtime configuration for any abit-assistant entrypoint
// (bot, cli, server, ...). Each entrypoint picks the fields it actually
// needs and validates them via Validate.
type Config struct {
	// TelegramToken is the Bot API token from @BotFather.
	TelegramToken string
	// AdminIDs is the set of Telegram user IDs allowed to use /admin
	// commands.
	AdminIDs []int64
	// DatabaseURL is a PostgreSQL connection URL, e.g.
	// "postgres://user:pass@host:5432/db?sslmode=require". Managed hosts
	// hand this out; set it as DATABASE_URL.
	DatabaseURL string
	// LogLevel is one of "debug", "info", "warn", "error".
	LogLevel string
}

// defaultDatabaseURL is the local-development connection (docker-compose
// brings up this Postgres). Production overrides it via DATABASE_URL.
const defaultDatabaseURL = "postgres://abit:abit@localhost:5432/abit?sslmode=disable"

// Load reads configuration from process environment with sensible defaults.
// It never fails; call Validate before using the bot entrypoint.
func Load() (*Config, error) {
	c := &Config{
		TelegramToken: os.Getenv("TELEGRAM_TOKEN"),
		DatabaseURL:   envOr("DATABASE_URL", defaultDatabaseURL),
		LogLevel:      envOr("LOG_LEVEL", "info"),
	}
	ids, err := parseInt64List(os.Getenv("ADMIN_IDS"))
	if err != nil {
		return nil, fmt.Errorf("ADMIN_IDS: %w", err)
	}
	c.AdminIDs = ids
	return c, nil
}

// Validate checks that fields required for the Telegram bot entrypoint are
// present. CLI / dev entrypoints may skip this and use Config selectively.
func (c *Config) Validate() error {
	if c.TelegramToken == "" {
		return errors.New("config: TELEGRAM_TOKEN is required")
	}
	return nil
}

// IsAdmin reports whether the given Telegram user ID has admin privileges.
func (c *Config) IsAdmin(tgID int64) bool {
	return slices.Contains(c.AdminIDs, tgID)
}

// RedactDatabaseURL masks the password in a connection URL so it's safe to
// log. "postgres://user:secret@host/db" → "postgres://user:***@host/db".
// Falls back to the host portion only if the URL can't be split cleanly.
func RedactDatabaseURL(dsn string) string {
	at := strings.LastIndex(dsn, "@")
	if at < 0 {
		return dsn // no credentials embedded
	}
	scheme := ""
	rest := dsn
	if i := strings.Index(dsn, "://"); i >= 0 {
		scheme = dsn[:i+3]
		rest = dsn[i+3:]
		at = strings.LastIndex(rest, "@")
	}
	creds := rest[:at]
	tail := rest[at:]
	if colon := strings.Index(creds, ":"); colon >= 0 {
		creds = creds[:colon] + ":***"
	}
	return scheme + creds + tail
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseInt64List(s string) ([]int64, error) {
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid id %q: %w", p, err)
		}
		out = append(out, v)
	}
	return out, nil
}
