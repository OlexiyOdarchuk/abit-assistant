// Package pgtest provides a PostgreSQL-backed *storage.Store for tests.
//
// It needs a running Postgres: set TEST_DATABASE_URL to a superuser/owner
// connection URL (e.g. the docker-compose one,
// "postgres://abit:abit@localhost:5432/abit?sslmode=disable"). Each New()
// creates a throwaway database on that server, runs migrations into it, and
// drops it on cleanup — so tests are fully isolated. When TEST_DATABASE_URL
// is unset the test is skipped, keeping `go test ./...` runnable without a DB.
package pgtest

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"sync/atomic"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx"

	"github.com/OlexiyOdarchuk/abit-assistant/internal/storage"
)

var counter atomic.Int64

// New returns a Store bound to a fresh, migrated, throwaway database.
func New(t *testing.T) *storage.Store {
	t.Helper()
	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("pgtest: set TEST_DATABASE_URL to run storage-backed tests")
	}
	ctx := context.Background()

	admin, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatalf("pgtest: open admin: %v", err)
	}
	defer admin.Close()

	name := fmt.Sprintf("aa_test_%d_%d", os.Getpid(), counter.Add(1))
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("pgtest: create database %s: %v", name, err)
	}

	st, err := storage.Open(ctx, withDatabase(t, base, name))
	if err != nil {
		dropDatabase(base, name)
		t.Fatalf("pgtest: storage.Open: %v", err)
	}
	t.Cleanup(func() {
		_ = st.Close()
		dropDatabase(base, name)
	})
	return st
}

// withDatabase returns base with its database (path) replaced by name.
func withDatabase(t *testing.T, base, name string) string {
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("pgtest: parse TEST_DATABASE_URL: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}

// dropDatabase removes a throwaway database, forcing off any leftover
// connections (Postgres 13+).
func dropDatabase(base, name string) {
	admin, err := sql.Open("pgx", base)
	if err != nil {
		return
	}
	defer admin.Close()
	_, _ = admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
}
