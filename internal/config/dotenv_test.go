package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv_BasicFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	body := `
# this is a comment
TELEGRAM_TOKEN=abc123
DATABASE_PATH=./data/test.db
ADMIN_IDS="1,2,3"
LOG_LEVEL='debug'
export EXPORTED_VAR=ok
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	// Make sure none of our keys leak between tests.
	for _, k := range []string{"TELEGRAM_TOKEN", "DATABASE_PATH", "ADMIN_IDS", "LOG_LEVEL", "EXPORTED_VAR"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}

	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}

	cases := map[string]string{
		"TELEGRAM_TOKEN": "abc123",
		"DATABASE_PATH":  "./data/test.db",
		"ADMIN_IDS":      "1,2,3",
		"LOG_LEVEL":      "debug",
		"EXPORTED_VAR":   "ok",
	}
	for k, want := range cases {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestLoadDotEnv_DoesNotOverrideExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("X=from_file"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("X", "from_shell")
	if err := LoadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("X"); got != "from_shell" {
		t.Errorf("shell value should win, got %q", got)
	}
}

func TestLoadDotEnv_MissingFileIsOK(t *testing.T) {
	if err := LoadDotEnv("/path/that/definitely/does/not/exist/.env"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}
