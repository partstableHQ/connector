package compendium

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver (pure Go)
)

// MaxSupportedSchema is the highest compendium schema this build consumes.
// Snapshots above it are rejected with an actionable message.
const MaxSupportedSchema = 1

// decompressedSizeCap guards against pathological archives (the compendium
// is expected to be well under this; the cap is a safety rail, not a quota).
const decompressedSizeCap = 8 << 30 // 8 GiB

// Info describes the loaded compendium.
type Info struct {
	Schema      int
	Vintage     time.Time
	SourceRev   string
	Generator   string
	PartCount   int64
	XrefCount   int64
	HolderCount int64
}

// ErrSchemaTooNew reports a snapshot the app cannot consume yet.
var ErrSchemaTooNew = errors.New("compendium snapshot schema is newer than this app supports; update the app")

// FileSHA256 returns the hex sha256 of a file — the pre-install check
// against the release checksums (verification chain step 2 in FORMAT.md).
func FileSHA256(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- the archive path is the local release artifact chosen by the user or the updater
	if err != nil {
		return "", fmt.Errorf("compendium: open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("compendium: hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Load installs the gzipped snapshot archive at archivePath as the active
// compendium at targetPath. expectedSHA256 (hex, from the release
// checksums) is verified before anything is written; pass "" only in tests
// and developer tooling. The install is atomic: verify → decompress to a
// temp file in the target directory → integrity check → rename, keeping the
// previous compendium as .bak for rollback.
func Load(ctx context.Context, archivePath, targetPath, expectedSHA256 string) (Info, error) {
	if expectedSHA256 != "" {
		got, err := FileSHA256(archivePath)
		if err != nil {
			return Info{}, err
		}
		if got != expectedSHA256 {
			return Info{}, fmt.Errorf("compendium: checksum mismatch: expected %s, got %s", expectedSHA256, got)
		}
	}

	tmp := targetPath + ".tmp"
	if err := decompress(ctx, archivePath, tmp); err != nil {
		_ = os.Remove(tmp)
		return Info{}, err
	}

	db, err := openReadOnly(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return Info{}, err
	}

	res, ierr := integrity(ctx, db)
	if ierr != nil {
		_ = db.Close()
		_ = os.Remove(tmp)
		return Info{}, fmt.Errorf("compendium: integrity check error: %w", ierr)
	}
	if res != "ok" {
		_ = db.Close()
		_ = os.Remove(tmp)
		return Info{}, fmt.Errorf("compendium: integrity check reported %q", res)
	}

	info, rerr := readInfo(ctx, db)
	// Release the file handle before any filesystem operation — Windows
	// refuses to rename or delete open files.
	_ = db.Close()
	if rerr != nil {
		_ = os.Remove(tmp)
		return Info{}, rerr
	}

	bak := targetPath + ".bak"
	_ = os.Remove(bak)
	if err := os.Rename(targetPath, bak); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(tmp)
		return Info{}, fmt.Errorf("compendium: back up previous compendium: %w", err)
	}
	if err := os.Rename(tmp, targetPath); err != nil {
		// Roll the backup forward so a good compendium stays in place.
		_ = os.Rename(bak, targetPath)
		return Info{}, fmt.Errorf("compendium: install snapshot: %w", err)
	}
	return info, nil
}

// Open opens the installed compendium read-only for queries.
func Open(targetPath string) (*sql.DB, error) {
	return openReadOnly(targetPath)
}

// ReadInfo reports the meta of an open compendium database.
func ReadInfo(ctx context.Context, db *sql.DB) (Info, error) {
	return readInfo(ctx, db)
}

func decompress(ctx context.Context, archivePath, targetPath string) error {
	f, err := os.Open(archivePath) // #nosec G304 -- archive path is the local release artifact
	if err != nil {
		return fmt.Errorf("compendium: open archive: %w", err)
	}
	defer func() { _ = f.Close() }()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("compendium: not a gzip snapshot: %w", err)
	}
	defer func() { _ = zr.Close() }()

	out, err := os.Create(targetPath) // #nosec G304 -- same-directory temp file with an internally built name
	if err != nil {
		return fmt.Errorf("compendium: create %s: %w", targetPath, err)
	}

	done := make(chan error, 1)
	go func() {
		n, cerr := io.Copy(out, io.LimitReader(zr, decompressedSizeCap+1))
		if cerr == nil && n > decompressedSizeCap {
			cerr = fmt.Errorf("decompressed snapshot exceeds %d bytes", decompressedSizeCap)
		}
		// A write path's Close error is part of the write's success.
		if clerr := out.Close(); cerr == nil {
			cerr = clerr
		}
		done <- cerr
	}()
	select {
	case err := <-done:
		if err != nil {
			_ = os.Remove(targetPath)
			return fmt.Errorf("compendium: decompress: %w", err)
		}
	case <-ctx.Done():
		_ = out.Close()
		return fmt.Errorf("compendium: decompress canceled: %w", ctx.Err())
	}
	return nil
}

func openReadOnly(path string) (*sql.DB, error) {
	// mode=ro: the compendium is a data artifact; nothing in the app may
	// write to it (FORMAT.md, app obligations).
	dsn := "file:" + filepath.ToSlash(path) + "?mode=ro&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("compendium: open %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("compendium: open %s: %w", path, err)
	}
	return db, nil
}

func integrity(ctx context.Context, db *sql.DB) (string, error) {
	var res string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&res); err != nil {
		return "", err
	}
	return res, nil
}

func readInfo(ctx context.Context, db *sql.DB) (Info, error) {
	meta := map[string]string{}
	rows, err := db.QueryContext(ctx, `SELECT key, value FROM meta`)
	if err != nil {
		return Info{}, fmt.Errorf("compendium: read meta (not a compendium snapshot?): %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return Info{}, fmt.Errorf("compendium: read meta: %w", err)
		}
		meta[k] = v
	}
	if err := rows.Err(); err != nil {
		return Info{}, fmt.Errorf("compendium: read meta: %w", err)
	}

	schemaStr, ok := meta[MetaSchema]
	if !ok {
		return Info{}, fmt.Errorf("compendium: meta key %q missing — not a valid snapshot", MetaSchema)
	}
	schema, err := strconv.Atoi(schemaStr)
	if err != nil {
		return Info{}, fmt.Errorf("compendium: invalid %s %q: %w", MetaSchema, schemaStr, err)
	}
	if schema > MaxSupportedSchema {
		return Info{}, ErrSchemaTooNew
	}
	vintageStr, ok := meta[MetaVintage]
	if !ok {
		return Info{}, fmt.Errorf("compendium: meta key %q missing — not a valid snapshot", MetaVintage)
	}
	vintage, err := time.Parse(time.RFC3339, vintageStr)
	if err != nil {
		return Info{}, fmt.Errorf("compendium: invalid vintage %q: %w", vintageStr, err)
	}

	info := Info{
		Schema:    schema,
		Vintage:   vintage,
		SourceRev: meta[MetaSourceRev],
		Generator: meta[MetaGenerator],
	}
	for _, c := range []struct {
		table string
		dst   *int64
	}{
		{"parts", &info.PartCount},
		{"xrefs", &info.XrefCount},
		{"holders", &info.HolderCount},
	} {
		err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+c.table).Scan(c.dst)
		if err != nil {
			return Info{}, fmt.Errorf("compendium: count %s: %w", c.table, err)
		}
	}
	return info, nil
}
