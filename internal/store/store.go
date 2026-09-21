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
	"io/fs"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver (pure Go)
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migration is one embedded, forward-only schema step. The file name's
// numeric prefix is its version: 001_init.sql is version 1.
type migration struct {
	version int
	name    string
	sql     string
}

var (
	loadOnce sync.Once
	loaded   []migration
	loadErr  error
)

func all() ([]migration, error) {
	loadOnce.Do(func() {
		loaded, loadErr = loadMigrations()
	})
	return loaded, loadErr
}

// loadMigrations parses migrations/*.sql in version order. Duplicate
// versions are a build-time data error.
func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, fmt.Errorf("store: read embedded migrations: %w", err)
	}
	var ms []migration
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		prefix, rest, _ := strings.Cut(e.Name(), "_")
		v, err := strconv.Atoi(prefix)
		if err != nil {
			return nil, fmt.Errorf("store: migration %s: numeric version prefix required", e.Name())
		}
		if v <= 0 {
			return nil, fmt.Errorf("store: migration %s: version must be positive", e.Name())
		}
		body, err := fs.ReadFile(migrationsFS, "migrations/"+e.Name())
		if err != nil {
			return nil, fmt.Errorf("store: read %s: %w", e.Name(), err)
		}
		ms = append(ms, migration{version: v, name: rest, sql: string(body)})
	}
	slices.SortFunc(ms, func(a, b migration) int { return a.version - b.version })
	for i := 1; i < len(ms); i++ {
		if ms[i].version == ms[i-1].version {
			return nil, fmt.Errorf("store: duplicate migration version %d", ms[i].version)
		}
	}
	return ms, nil
}

// MigrationCount is the number of embedded migrations; after Migrate the
// highest applied version equals this count (versions are 1..N).
func MigrationCount() int {
	ms, _ := all()
	return len(ms)
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
// startup path should follow with Migrate — migrations run eagerly at
// every app start (BUILD-GUIDE §1).
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

// migrationsTable tracks applied versions. One row per migration, appended
// in order — nothing here is ever deleted or rewritten.
const migrationsTable = `
CREATE TABLE IF NOT EXISTS schema_migrations (
	version    INTEGER PRIMARY KEY,
	name       TEXT NOT NULL,
	applied_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
)`

// Migrate applies all embedded migrations that the database has not yet
// applied, each inside its own transaction. Forward-only by doctrine:
// there is no downgrade path — client databases are never down-migrated,
// releases only move forward (with backups, see compendium rollback).
//
// A database written by a NEWER app (versions above this build's embedded
// set) is refused with an actionable error rather than touched.
func Migrate(ctx context.Context, db *sql.DB) error {
	ms, err := all()
	if err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, migrationsTable); err != nil {
		return fmt.Errorf("store: migrations table: %w", err)
	}

	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return err
	}
	if n := len(applied); n > len(ms) {
		return fmt.Errorf("store: database schema v%d is newer than this app supports (v%d) — update the app",
			applied[len(ms)], len(ms))
	}
	for i, m := range ms {
		if i < len(applied) {
			if applied[i] != m.version {
				return fmt.Errorf("store: migration history diverged at position %d (applied v%d, embedded v%d)",
					i, applied[i], m.version)
			}
			continue
		}
		if err := apply(ctx, db, m); err != nil {
			return err
		}
	}
	return nil
}

func appliedVersions(ctx context.Context, db *sql.DB) ([]int, error) {
	rows, err := db.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("store: read migration state: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []int
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			return nil, fmt.Errorf("store: read migration state: %w", err)
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func apply(ctx context.Context, db *sql.DB, m migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin migration %d: %w", m.version, err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return fmt.Errorf("store: apply migration %d (%s): %w", m.version, m.name, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations (version, name) VALUES (?, ?)`, m.version, m.name); err != nil {
		return fmt.Errorf("store: record migration %d: %w", m.version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit migration %d: %w", m.version, err)
	}
	return nil
}

// Version reports the highest applied migration version (0 when none).
func Version(db *sql.DB) (int64, error) {
	var v int64
	if err := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&v); err != nil {
		return 0, fmt.Errorf("store: read schema version: %w", err)
	}
	return v, nil
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
