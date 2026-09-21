// Command mkcompendium builds a sample compendium snapshot for development
// and release-pipeline testing. It seeds a data directory the way the app
// consumes (compendium.db) and writes the distributable .bin artifact the
// release pipeline will attach to GitHub releases.
//
// Usage:
//
//	go run ./cmd/mkcompendium -out <dir>
//
// It writes <dir>/compendium-<version>.bin (gzip of the database) and, for
// developer convenience, installs the database directly as
// <dir>/compendium.db. It is a developer tool, not part of the shipped app.
package main

import (
	"compress/gzip"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/partstableHQ/connector/internal/compendium"
	_ "modernc.org/sqlite"
)

func main() {
	out := flag.String("out", ".", "directory to write the snapshot and dev compendium into")
	flag.Parse()
	if err := run(*out); err != nil {
		fmt.Fprintln(os.Stderr, "mkcompendium:", err)
		os.Exit(1)
	}
}

func run(out string) error {
	if err := os.MkdirAll(out, 0o750); err != nil {
		return err
	}
	src := filepath.Join(out, "compendium.db")
	_ = os.Remove(src)
	_ = os.Remove(src + "-wal")
	_ = os.Remove(src + "-shm")

	db, err := compendium.Create(src)
	if err != nil {
		return err
	}
	if err := seed(db); err != nil {
		_ = db.Close()
		return err
	}
	if err := db.Close(); err != nil {
		return err
	}

	bin, err := gzipTo(src, filepath.Join(out, "compendium-0.0.0-dev.bin"))
	if err != nil {
		return err
	}
	fmt.Printf("snapshot: %s\nsha256:   %s\ndev db:   %s\n", bin.path, bin.sha256, src)
	return nil
}

func seed(db *sql.DB) error {
	vintage := time.Now().UTC().Format(time.RFC3339)
	stmts := []string{
		fmt.Sprintf(`INSERT INTO meta (key, value) VALUES
			('compendium_schema', '1'), ('generated_at', '%s'),
			('source_rev', 'dev-sample'), ('generator', 'mkcompendium/1')`, vintage),
		// A few realistic memory and drive records for manual UI testing.
		`INSERT INTO parts (pn, display_pn, description, category) VALUES
			('02CL197', '02cl197', 'Lenovo 32GB DDR4-3200 LP ECC UDIMM', 'memory'),
			('4X70J67435', '4X70J67435', 'Lenovo 16GB DDR4-3200 SO-DIMM', 'memory'),
			('SN730SDB512GB', 'sn730-sdb-512gb', 'WD SN730 512GB NVMe M.2 2280', 'storage')`,
		`INSERT INTO part_aliases (alias, pn) VALUES
			('2CL197', '02CL197'),
			('02-CL-197', '02CL197'),
			('SN730512', 'SN730SDB512GB')`,
		`INSERT INTO xrefs (from_pn, to_pn, kind, source, source_detail) VALUES
			('02CL197', '4X70J67435', 'substitute', 'broker_verified', 'verified in broker lot #812, 2026-08'),
			('02CL197', 'SN730SDB512GB', 'bundle', 'partner', 'bundle sheet 2026-09')`,
		`INSERT INTO holders (pn, holder, qty, condition, last_seen, source, source_detail) VALUES
			('02CL197', 'Sample Broker NL', 14, 'refurb', '2026-09-12', 'partner', 'partner feed sync 2026-09-12'),
			('02CL197', 'Sample Broker DE', 3, 'new-open-box', '2026-09-15', 'broker_verified', 'lot #901'),
			('SN730SDB512GB', 'Sample ITAD US', 42, 'pulled', '2026-09-10', 'broker_verified', 'manifest 2214')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("seed: %w", err)
		}
	}
	return nil
}

type artifact struct {
	path   string
	sha256 string
}

func gzipTo(src, dst string) (artifact, error) {
	in, err := os.Open(src) // #nosec G304 -- path comes from the -out flag
	if err != nil {
		return artifact{}, err
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst) // #nosec G304 -- path comes from the -out flag
	if err != nil {
		return artifact{}, err
	}
	h := sha256.New()
	zw := gzip.NewWriter(io.MultiWriter(out, h))
	if _, err := io.Copy(zw, in); err != nil {
		_ = out.Close()
		return artifact{}, err
	}
	if err := zw.Close(); err != nil {
		_ = out.Close()
		return artifact{}, err
	}
	if err := out.Close(); err != nil {
		return artifact{}, err
	}
	return artifact{path: dst, sha256: hex.EncodeToString(h.Sum(nil))}, nil
}
