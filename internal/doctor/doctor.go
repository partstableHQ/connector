// Package doctor diagnoses a Connector installation — install paths,
// app database, compendium state, and (once built) update channel and
// keychain — with green/red output and actionable messages (FM-16).
package doctor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/partstableHQ/connector/internal/compendium"
	"github.com/partstableHQ/connector/internal/paths"
	"github.com/partstableHQ/connector/internal/store"
	"github.com/partstableHQ/connector/internal/version"
)

// Status is the outcome of a single doctor check.
type Status string

// Check statuses. Skip marks roadmap subsystems that cannot be diagnosed
// yet — a skip is never a failure.
const (
	StatusOK   Status = "ok"
	StatusWarn Status = "warn"
	StatusFail Status = "fail"
	StatusSkip Status = "skip"
)

// Check is one diagnostic line.
type Check struct {
	Name   string
	Status Status
	Detail string
}

// Options selects what to diagnose. DataDir empty means the default
// location (internal/paths); tests pass an explicit directory.
type Options struct {
	DataDir string
}

// Summary aggregates the checks; Fail > 0 means the doctor exits nonzero.
type Summary struct {
	Checks []Check
}

// Count returns how many checks carry the given status.
func (s Summary) Count(status Status) int {
	n := 0
	for _, c := range s.Checks {
		if c.Status == status {
			n++
		}
	}
	return n
}

// Run executes every check, prints a human-readable report to w, and
// returns the summary. It never mutates the databases: a missing app DB or
// compendium is a warning with an actionable message, not an error.
func Run(ctx context.Context, opts Options, w io.Writer) (Summary, error) {
	dataDir := opts.DataDir
	if dataDir == "" {
		var err error
		dataDir, err = paths.DataDir()
		if err != nil {
			return Summary{}, err
		}
	}

	var sum Summary
	add := func(name string, status Status, format string, args ...any) {
		sum.Checks = append(sum.Checks, Check{Name: name, Status: status, Detail: fmt.Sprintf(format, args...)})
	}
	write := func(format string, args ...any) {
		_, _ = fmt.Fprintf(w, format, args...)
	}

	write("partstable doctor %s\n\n", version.Version())

	// 1. Data directory: exists and writable.
	probe := filepath.Join(dataDir, ".doctor-probe")
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		add("data dir", StatusFail, "%s — cannot create: %v", dataDir, err)
	} else if f, err := os.Create(probe); err != nil { // #nosec G304 -- probe path built internally from the data dir
		add("data dir", StatusFail, "%s — not writable: %v", dataDir, err)
	} else {
		_ = f.Close()
		_ = os.Remove(probe)
		add("data dir", StatusOK, "%s (writable)", dataDir)
	}

	// 2. App database: present, integral, fully migrated.
	appDB := filepath.Join(dataDir, "app.db")
	if _, err := os.Stat(appDB); os.IsNotExist(err) {
		add("app db", StatusWarn, "not initialized yet — launch the app once (creates %s)", appDB)
	} else {
		db, err := store.Open(appDB)
		if err != nil {
			add("app db", StatusFail, "%s — cannot open: %v", appDB, err)
		} else {
			checkAppDB(ctx, db, add)
			_ = db.Close()
		}
	}

	// 3. Compendium: loaded, integral, consumable schema, honest vintage.
	compDB := filepath.Join(dataDir, "compendium.db")
	if _, err := os.Stat(compDB); os.IsNotExist(err) {
		add("compendium", StatusWarn, "not loaded — install a release snapshot to enable lookups")
	} else {
		db, err := compendium.Open(compDB)
		if err != nil {
			add("compendium", StatusFail, "%s — cannot open: %v", compDB, err)
		} else {
			checkCompendium(ctx, db, add)
			_ = db.Close()
		}
	}

	// 4-5. Subsystems on the roadmap — reported as skipped, never red.
	add("update channel", StatusSkip, "arrives with self-update (ROADMAP slice 6)")
	add("keychain", StatusSkip, "arrives with account pairing (ROADMAP slice 5)")

	write("\n")
	for _, c := range sum.Checks {
		write("[%s] %-15s %s\n", c.Status, c.Name, c.Detail)
	}
	write("\n%d ok · %d warn · %d fail · %d skip\n",
		sum.Count(StatusOK), sum.Count(StatusWarn), sum.Count(StatusFail), sum.Count(StatusSkip))

	return sum, nil
}

func checkAppDB(ctx context.Context, db *sql.DB, add func(string, Status, string, ...any)) {
	integ, err := store.Integrity(ctx, db)
	if err != nil {
		add("app db", StatusFail, "integrity check error: %v", err)
		return
	}
	if integ != "ok" {
		add("app db", StatusFail, "integrity_check reported %q — restore from a fresh install", integ)
		return
	}
	v, err := store.Version(db)
	if err != nil {
		add("app db", StatusFail, "migration state unreadable: %v", err)
		return
	}
	switch {
	case v < int64(store.MigrationCount):
		add("app db", StatusWarn, "integrity ok — migrations pending (%d/%d applied), launch the app to apply", v, store.MigrationCount)
	default:
		add("app db", StatusOK, "integrity ok, schema migration %d/%d applied", v, store.MigrationCount)
	}
}

func checkCompendium(ctx context.Context, db *sql.DB, add func(string, Status, string, ...any)) {
	info, err := compendium.ReadInfo(ctx, db)
	if errors.Is(err, compendium.ErrSchemaTooNew) {
		add("compendium", StatusFail, "%v", err)
		return
	}
	if err != nil {
		add("compendium", StatusFail, "unreadable (%v) — reinstall the snapshot by updating the app", err)
		return
	}
	add("compendium", StatusOK, "schema v%d · rev %s · %d parts · %d xrefs · %d holders (source rev %s)",
		info.Schema, info.Vintage.Format(time.DateOnly), info.PartCount, info.XrefCount, info.HolderCount, info.SourceRev)
}
