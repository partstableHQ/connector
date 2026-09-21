package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// The app database must open cleanly, apply all embedded migrations, and be
// idempotent under re-migration — a client DB is only ever migrated
// forward, at every startup (FM-4).
func TestOpenAndMigrateIdempotent(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "app.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	for i := 0; i < 2; i++ {
		if err := Migrate(ctx, db); err != nil {
			t.Fatalf("migrate pass %d: %v", i+1, err)
		}
	}

	v, err := Version(db)
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if v != int64(MigrationCount()) {
		t.Fatalf("applied version %d, want %d", v, MigrationCount())
	}

	// Settings is the app's key-value surface; prove it actually works.
	if _, err := db.Exec(`INSERT INTO settings (key, value) VALUES ('probe', 'ok')`); err != nil {
		t.Fatalf("settings insert: %v", err)
	}
	var got string
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'probe'`).Scan(&got); err != nil {
		t.Fatalf("settings select: %v", err)
	}
	if got != "ok" {
		t.Fatalf("settings round-trip got %q, want %q", got, "ok")
	}

	integ, err := Integrity(ctx, db)
	if err != nil || integ != "ok" {
		t.Fatalf("integrity = %q, %v", integ, err)
	}
}

// WAL must be the journal mode on the app database — it is the durability
// default in the build guide and the prerequisite for concurrent reads.
func TestWALJournalMode(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	var mode string
	if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}
}

// A database written by a newer app must be refused, never touched — the
// forward-only doctrine cuts both ways: an old app cannot guess at a
// newer schema.
func TestMigrateRefusesDatabaseFromNewerApp(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "app.db")

	db, err := Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = db.Close() }()

	// Bring the DB to the current schema the normal way, then simulate a
	// newer app having applied a migration this build does not know.
	if err := Migrate(ctx, db); err != nil {
		t.Fatalf("initial migrate: %v", err)
	}
	future := MigrationCount() + 5
	if _, err := db.Exec(`INSERT INTO schema_migrations (version, name) VALUES (?, 'future')`, future); err != nil {
		t.Fatalf("seed future row: %v", err)
	}

	err = Migrate(ctx, db)
	if err == nil {
		t.Fatal("migrate of a newer database must fail")
	}
	if !strings.Contains(err.Error(), "update the app") {
		t.Fatalf("error must be actionable, got: %v", err)
	}
}
