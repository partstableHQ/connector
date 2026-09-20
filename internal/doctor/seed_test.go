package doctor

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"testing"
	"time"

	"github.com/partstableHQ/connector/internal/compendium"
)

// seedCompendium builds and installs the smallest valid snapshot at target,
// going through the real compendium.Load path.
func seedCompendium(ctx context.Context, t *testing.T, target string) {
	t.Helper()

	srcDB := target + ".seed.db"
	db, err := compendium.Create(srcDB)
	if err != nil {
		t.Fatalf("create seed db: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO meta (key, value) VALUES
		('compendium_schema', '1'),
		('generated_at', ?),
		('source_rev', 'tds-test'),
		('generator', 'doctor-test/1')`, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("seed meta: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close seed db: %v", err)
	}

	archive := target + ".seed.bin"
	if err := gzipFile(srcDB, archive); err != nil {
		t.Fatalf("gzip seed: %v", err)
	}
	raw, err := os.ReadFile(archive) // #nosec G304 -- test fixture path built by the test
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)

	if _, err := compendium.Load(ctx, archive, target, hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("load seed compendium: %v", err)
	}
}

func gzipFile(src, dst string) error {
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
