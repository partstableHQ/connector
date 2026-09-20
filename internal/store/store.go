// Package store owns the application's local SQLite database: opening with
// the right pragmas and applying embedded forward-only migrations.
//
// The compendium database is NOT this database — it is a data artifact
// loaded from release snapshots (see internal/compendium).
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver (pure Go)
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// MigrationCount is the number of embedded migrations; the applied version
// must always equal this after Migrate.
var MigrationCount = countMigrations()

func countMigrations() int {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			n++
		}
	}
	return n
}

// dsn builds a modernc.org/sqlite DSN with the pragmas that must hold on
// every pooled connection: WAL journaling, foreign keys, sane lock wait.
func dsn(path string) string {
	pragma := func(p string) string { return "_pragma=" + url.QueryEscape(p) }
	return "file:" + filepath.ToSlash(path) + "?" + strings.Join([]string{
		pragma("journal_mode(WAL)"),
		pragma("foreign_keys(1)"),
		pragma("busy_timeout(5000)"),
	}, "&")
}

// Open opens (creating if necessary) the app database. Callers on the
// CLI path should follow with Migrate; the GUI runs both eagerly at
// startup (BUILD-GUIDE §1: run migrations eagerly).
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("store: ping %s: %w", path, err)
	}
	return db, nil
}

// Migrate applies all embedded migrations. Forward-only: there is no
// downgrade path by doctrine — client databases are never down-migrated,
// releases only move forward (with a backup, see compendium rollback).
func Migrate(ctx context.Context, db *sql.DB) error {
	// Explicit dialect: the legacy goose API's driver-name auto-detection
	// misfires for the modernc "sqlite" driver on a fresh database.
	if err := goose.SetDialect("sqlite3"); err != nil {
		return fmt.Errorf("store: dialect: %w", err)
	}
	goose.SetBaseFS(migrationsFS)
	goose.SetLogger(goose.NopLogger())
	defer goose.SetBaseFS(nil)
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		return fmt.Errorf("store: migrate: %w", err)
	}
	return nil
}

// Version reports the applied migration version (-1 when none applied).
func Version(db *sql.DB) (int64, error) {
	return goose.GetDBVersion(db)
}

// Integrity runs SQLite's integrity_check and returns the result string
// ("ok" on a healthy database).
func Integrity(ctx context.Context, db *sql.DB) (string, error) {
	var res string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&res); err != nil {
		return "", fmt.Errorf("store: integrity_check: %w", err)
	}
	return res, nil
}
