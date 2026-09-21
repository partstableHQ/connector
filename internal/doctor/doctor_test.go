package doctor

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/partstableHQ/connector/internal/store"
)

// openMigratedAppDB seeds the app database exactly as the app will at
// startup: open, then apply all embedded migrations.
func openMigratedAppDB(ctx context.Context, path string) (*sql.DB, error) {
	db, err := store.Open(path)
	if err != nil {
		return nil, err
	}
	if err := store.Migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

// Fresh install: everything that can be warn/skip is, nothing fails, and
// the report says what to do next (FM-16: actionable messages).
func TestRunFreshInstall(t *testing.T) {
	var buf bytes.Buffer
	sum, err := Run(context.Background(), Options{
		DataDir:       t.TempDir(),
		KeychainProbe: func() KeychainState { return KeychainState{Reachable: true} },
		UpdateProbe:   func() UpdateChannelState { return UpdateChannelState{Reachable: true} },
	}, &buf)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if sum.Count(StatusFail) != 0 {
		t.Fatalf("fresh install must not fail: %+v", sum.Checks)
	}
	if sum.Count(StatusWarn) < 3 {
		t.Fatalf("expected warns for app db, compendium, and sign-in: %+v", sum.Checks)
	}
	out := buf.String()
	for _, want := range []string{"launch the app once", "install a release snapshot", "partstable login"} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing actionable message %q:\n%s", want, out)
		}
	}
}

// Healthy install: doctor goes fully green on the implemented checks —
// app DB migrated and a compendium snapshot actually loaded.
func TestRunHealthyInstall(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	// Initialize the app DB exactly as the app will at startup.
	db, err := openMigratedAppDB(ctx, filepath.Join(dir, "app.db"))
	if err != nil {
		t.Fatalf("seed app db: %v", err)
	}
	_ = db.Close()

	// Load a minimal but valid compendium snapshot.
	seedCompendium(ctx, t, filepath.Join(dir, "compendium.db"))

	var buf bytes.Buffer
	sum, err := Run(ctx, Options{
		DataDir: dir,
		KeychainProbe: func() KeychainState {
			return KeychainState{Reachable: true, SignedIn: true, Email: "t@example.com"}
		},
		UpdateProbe: func() UpdateChannelState { return UpdateChannelState{Reachable: true} },
	}, &buf)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if sum.Count(StatusFail) != 0 || sum.Count(StatusWarn) != 0 {
		t.Fatalf("healthy install must be green (fails=%d warns=%d):\n%s",
			sum.Count(StatusFail), sum.Count(StatusWarn), buf.String())
	}
	if !strings.Contains(buf.String(), "integrity ok") || !strings.Contains(buf.String(), "schema v1") {
		t.Errorf("report missing green details:\n%s", buf.String())
	}
}
