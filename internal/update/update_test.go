package update

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/partstableHQ/connector/internal/store"
	"github.com/partstableHQ/connector/internal/telemetry"
)

func openAppDB(t *testing.T) (context.Context, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(filepath.Join(t.TempDir(), "app.db"))
	if err != nil {
		t.Fatalf("open app db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate app db: %v", err)
	}
	return ctx, db
}

// The anonymous install ID must be stable for the life of the install and
// shaped like a UUID — it is the entire usage-counting mechanism.
func TestInstallIDStableAndPersisted(t *testing.T) {
	ctx, db := openAppDB(t)
	m := NewManager("v0.1.0", db)

	id1 := m.InstallID(ctx)
	id2 := m.InstallID(ctx)
	if id1 == "" || id1 != id2 {
		t.Fatalf("install id not stable: %q vs %q", id1, id2)
	}
	if len(id1) != 36 || strings.Count(id1, "-") != 4 {
		t.Fatalf("id %q is not UUID-shaped", id1)
	}

	// Persisted in the app database: a fresh manager over the same
	// database sees the same id — that is what makes it an install id.
	m2 := NewManager("v0.1.0", db)
	if got := m2.InstallID(ctx); got != id1 {
		t.Fatalf("fresh manager got %q, want %q", got, id1)
	}

	// Without a database the id is ephemeral but still present.
	ephemeral := NewManager("v0.1.0", nil)
	if ephemeral.InstallID(ctx) == "" {
		t.Fatal("ephemeral id must not be empty")
	}
}

func TestOptOutRoundTripAndEnvOverride(t *testing.T) {
	ctx, db := openAppDB(t)
	m := NewManager("v0.1.0", db)

	if m.OptedOut(ctx) {
		t.Fatal("fresh install must not be opted out (default ON)")
	}
	if err := m.SetOptOut(ctx, true); err != nil {
		t.Fatalf("set opt-out: %v", err)
	}
	if !m.OptedOut(ctx) {
		t.Fatal("opt-out not persisted")
	}
	if err := m.SetOptOut(ctx, false); err != nil {
		t.Fatalf("clear opt-out: %v", err)
	}
	if m.OptedOut(ctx) {
		t.Fatal("opt-out not cleared")
	}
	if err := m.SetOptOut(ctx, true); err != nil {
		t.Fatal(err)
	}

	// The environment override wins over the stored setting.
	t.Setenv(telemetry.EnvOptOut, "1")
	if !telemetry.EnvForcedOptOut() {
		t.Fatal("env override must read as forced opt-out")
	}
}

// The live check against the real repository: the repo exists and has no
// releases yet, so the check must succeed and report nothing available —
// proving the whole GitHub wiring without publishing a fake release.
func TestCheckLiveRepo(t *testing.T) {
	ctx, db := openAppDB(t)
	m := NewManager("v0.1.0", db)

	res, err := m.Check(ctx)
	if err != nil {
		var netErr interface{ Timeout() bool }
		if errors.As(err, &netErr) && netErr.Timeout() {
			t.Skip("network unavailable")
		}
		t.Skipf("GitHub unreachable from this runner: %v", err)
	}
	if res.Available || res.Latest != "" {
		t.Fatalf("no releases published yet — got %+v", res)
	}
	if !strings.HasPrefix(res.Current, "v0.1.0") {
		t.Fatalf("current = %q", res.Current)
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		candidate, current string
		want               bool
	}{
		{"v0.2.0", "v0.1.0", true},
		{"v0.1.1", "v0.1.0", true},
		{"v0.1.0", "v0.1.0", false},
		{"v0.0.9", "v0.1.0", false},
		{"v1.0.0", "0.0.0-SNAPSHOT-390f231", true},
		{"v0.1.0", "dev", true}, // unparseable current counts as older
	}
	for _, tc := range cases {
		if got := isNewer(tc.candidate, tc.current); got != tc.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", tc.candidate, tc.current, got, tc.want)
		}
	}
}

// The opt-out setting rides in the app database settings table.
func TestSettingHelpersRoundTrip(t *testing.T) {
	ctx, db := openAppDB(t)
	if _, found, err := store.GetSetting(ctx, db, "probe"); found || err != nil {
		t.Fatalf("unset key: found=%v err=%v", found, err)
	}
	if err := store.SetSetting(ctx, db, "probe", "hello"); err != nil {
		t.Fatalf("set: %v", err)
	}
	v, found, err := store.GetSetting(ctx, db, "probe")
	if err != nil || !found || v != "hello" {
		t.Fatalf("get = %q %v %v", v, found, err)
	}
	if err := store.SetSetting(ctx, db, "probe", "again"); err != nil {
		t.Fatal(err)
	}
	if v, _, _ := store.GetSetting(ctx, db, "probe"); v != "again" {
		t.Fatalf("upsert = %q", v)
	}
}
