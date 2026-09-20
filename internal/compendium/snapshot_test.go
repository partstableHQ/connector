package compendium

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// buildArchive produces a valid gzipped compendium snapshot with one part,
// one alias, one xref, one holder — the smallest artifact that exercises
// the whole loader contract.
func buildArchive(t *testing.T, dir string, schema int) (archivePath, sha string) {
	t.Helper()

	dbPath := filepath.Join(dir, "src.db")
	db, err := Create(dbPath)
	if err != nil {
		t.Fatalf("create compendium db: %v", err)
	}
	defer func() { _ = db.Close() }()

	if _, err := db.Exec(`INSERT INTO meta (key, value) VALUES
		('compendium_schema', ?), ('generated_at', '2026-09-20T12:00:00Z'),
		('source_rev', 'tds-2026.09.3'), ('generator', 'test/1')`, strconv.Itoa(schema)); err != nil {
		t.Fatalf("meta: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO parts (pn, display_pn, description, category)
		VALUES ('02CL197', '02cl197', 'LP ECC UDIMM 32GB DDR4-3200', 'memory')`); err != nil {
		t.Fatalf("part: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO part_aliases (alias, pn) VALUES ('2CL197', '02CL197')`); err != nil {
		t.Fatalf("alias: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO xrefs (from_pn, to_pn, kind, source, source_detail)
		VALUES ('02CL197', '32GBDDR43200ECC', 'substitute', 'broker_verified', 'broker lot #812')`); err != nil {
		t.Fatalf("xref: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO holders (pn, holder, qty, condition, last_seen, source, source_detail)
		VALUES ('02CL197', 'Test Broker NL', 4, 'refurb', '2026-09-01', 'partner', 'feed sync')`); err != nil {
		t.Fatalf("holder: %v", err)
	}

	archivePath = filepath.Join(dir, "compendium-0.0.0-test.bin")
	if err := gzipCompress(dbPath, archivePath); err != nil {
		t.Fatalf("gzip: %v", err)
	}

	raw, err := os.ReadFile(archivePath) // #nosec G304 -- test fixture path built by the test
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	sum := sha256.Sum256(raw)
	return archivePath, hex.EncodeToString(sum[:])
}

func gzipCompress(src, dst string) error {
	in, err := os.Open(src) // #nosec G304 -- test fixture path built by the test
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst) // #nosec G304 -- test fixture path built by the test
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(out)
	if _, err := io.Copy(zw, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func TestLoadHappyPath(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	archive, sha := buildArchive(t, dir, 1)
	target := filepath.Join(dir, "compendium.db")

	info, err := Load(ctx, archive, target, sha)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if info.Schema != 1 {
		t.Fatalf("schema = %d, want 1", info.Schema)
	}
	wantVintage := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	if !info.Vintage.Equal(wantVintage) {
		t.Fatalf("vintage = %v, want %v", info.Vintage, wantVintage)
	}
	if info.SourceRev != "tds-2026.09.3" || info.Generator != "test/1" {
		t.Fatalf("source rev/generator = %q/%q", info.SourceRev, info.Generator)
	}
	if info.PartCount != 1 || info.XrefCount != 1 || info.HolderCount != 1 {
		t.Fatalf("counts = %d/%d/%d, want 1/1/1", info.PartCount, info.XrefCount, info.HolderCount)
	}

	// The installed compendium must be queryable read-only, by alias, and
	// must reject writes (app obligations in FORMAT.md).
	ro, err := Open(target)
	if err != nil {
		t.Fatalf("open installed: %v", err)
	}
	defer func() { _ = ro.Close() }()

	var desc string
	err = ro.QueryRow(`SELECT p.description FROM parts p
		JOIN part_aliases a ON a.pn = p.pn WHERE a.alias = '2CL197'`).Scan(&desc)
	if err != nil {
		t.Fatalf("query by alias: %v", err)
	}
	if desc != "LP ECC UDIMM 32GB DDR4-3200" {
		t.Fatalf("description = %q", desc)
	}
	if _, err := ro.Exec(`INSERT INTO parts (pn, display_pn, description) VALUES ('X','X','X')`); err == nil {
		t.Fatal("write to compendium succeeded — read-only enforcement broken")
	}
}

func TestLoadChecksumMismatch(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	archive, sha := buildArchive(t, dir, 1)
	target := filepath.Join(dir, "compendium.db")

	b := []byte(sha)
	b[0] ^= 0xff
	badSha := string(b)
	if _, err := Load(ctx, archive, target, badSha); err == nil {
		t.Fatal("load with tampered checksum succeeded")
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("target was created despite checksum failure")
	}
}

func TestLoadRejectsCorruptArchive(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.bin")
	if err := os.WriteFile(bad, []byte("this is not gzip"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "compendium.db")

	if _, err := Load(ctx, bad, target, ""); err == nil {
		t.Fatal("load of non-gzip archive succeeded")
	}
}

func TestLoadRejectsFutureSchema(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	archive, sha := buildArchive(t, dir, MaxSupportedSchema+999)
	target := filepath.Join(dir, "compendium.db")

	if _, err := Load(ctx, archive, target, sha); err != ErrSchemaTooNew {
		t.Fatalf("err = %v, want ErrSchemaTooNew", err)
	}
}

// Updating the compendium must be atomic: the old file becomes .bak, the
// new one is live, and every installed file stays complete (FM-12).
func TestLoadAtomicReplace(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	archive, sha := buildArchive(t, dir, 1)
	target := filepath.Join(dir, "compendium.db")

	if _, err := Load(ctx, archive, target, sha); err != nil {
		t.Fatalf("first load: %v", err)
	}
	info, err := Load(ctx, archive, target, sha)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if info.PartCount != 1 {
		t.Fatalf("reinstalled compendium wrong: %+v", info)
	}
	if _, err := os.Stat(target + ".bak"); err != nil {
		t.Fatalf("backup missing after replace: %v", err)
	}
	ro, err := Open(target)
	if err != nil {
		t.Fatalf("open after replace: %v", err)
	}
	_ = ro.Close()
}
