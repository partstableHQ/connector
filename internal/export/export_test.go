package export

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/partstableHQ/connector/internal/compendium"
	"github.com/partstableHQ/connector/internal/lookup"
	"github.com/partstableHQ/connector/internal/parse"
	"github.com/xuri/excelize/v2"
)

// buildService seeds a compendium and returns a lookup service over it.
func buildService(t *testing.T) *lookup.Service {
	t.Helper()
	db, err := compendium.Create(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatalf("create compendium: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, stmt := range []string{
		`INSERT INTO meta (key, value) VALUES ('compendium_schema','1'),
		 ('generated_at','2026-09-20T12:00:00Z'), ('source_rev','tds-test'), ('generator','test/1')`,
		`INSERT INTO parts (pn, display_pn, description, category)
		 VALUES ('02CL197', '02cl197', 'LP ECC UDIMM 32GB DDR4-3200', 'memory')`,
		`INSERT INTO xrefs (from_pn, to_pn, kind, source, source_detail)
		 VALUES ('02CL197', '4X70J67435', 'substitute', 'broker_verified', 'lot #812')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return lookup.New(db)
}

// headerRowOf returns the index (0-based, as GetRows indexes) of the
// branded table's header row and its cells.
func headerRowOf(t *testing.T, got [][]string, want []string) int {
	t.Helper()
	for i, row := range got {
		if len(row) > 0 && row[0] == want[0] && len(row) >= len(want) {
			ok := true
			for j, h := range want {
				if row[j] != h {
					ok = false
					break
				}
			}
			if ok {
				return i
			}
		}
	}
	t.Fatalf("header row %v not found in %d rows (first row: %v)", want, len(got), got[0])
	return -1
}

// FM-9: one click must yield a valid .xlsx of the current table. The
// format is the minimal broker deliverable (CEO ruling 2026-09-28) in the
// branded form: PN, qty, description, substitute PNs — no sources, grades,
// holders — under a PartsTable brand band.
func TestBuildRoundTrip(t *testing.T) {
	ctx := t.Context()
	svc := buildService(t)

	paste := "02CL197 x4\nMYSTERY99PN"
	pr := parse.Parse(paste)
	if len(pr.Entries) != 2 || len(pr.Warnings) != 0 {
		t.Fatalf("parse wrong: %+v", pr)
	}
	rows := make([]Row, 0, len(pr.Entries))
	for _, e := range pr.Entries {
		res, err := svc.Lookup(ctx, e.Norm)
		if err != nil {
			t.Fatalf("lookup %s: %v", e.Norm, err)
		}
		rows = append(rows, Row{Entry: e, Res: &res})
	}

	xlsx, err := Build(rows)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !bytes.HasPrefix(xlsx, []byte("PK")) {
		t.Fatal("output is not an xlsx (zip) file")
	}

	f, err := excelize.OpenReader(bytes.NewReader(xlsx))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	defer func() { _ = f.Close() }()

	got, err := f.GetRows(sheet)
	if err != nil {
		t.Fatalf("rows: %v", err)
	}
	// Brand band: the workbook opens on the PartsTable identity.
	brandFound := false
	for i, row := range got {
		if i > 3 {
			break
		}
		for _, c := range row {
			if c == "PartsTable Connector" {
				brandFound = true
			}
		}
	}
	if !brandFound {
		t.Fatalf("brand band missing in first rows: %v", got[:min(2, len(got))])
	}
	hdr := headerRowOf(t, got, []string{"Your part", "Qty", "Description", "Substitutes"})
	if len(got) != hdr+3 {
		t.Fatalf("rows = %d, want header at %d + 2", len(got), hdr)
	}
	// Found part: quantity and bare substitute PNs ride along — no source
	// labels, no holders, no provenance columns.
	found := got[hdr+1]
	if found[0] != "02CL197" || found[1] != "4" {
		t.Fatalf("found row = %v", found)
	}
	if !strings.Contains(found[3], "4X70J67435") {
		t.Fatalf("substitute PN missing: %q", found[3])
	}
	if strings.Contains(found[3], "BROKER") || strings.Contains(found[3], "substitute ·") {
		t.Fatalf("source annotation leaked into export: %q", found[3])
	}
	// Missing part: still present, honestly marked — the export is a
	// complete copy of the paste.
	missing := got[hdr+2]
	if missing[0] != "MYSTERY99PN" || !strings.Contains(missing[2], "no record") {
		t.Fatalf("missing row = %v", missing)
	}
}

// The single-part workbook carries a Substitutes sheet limited to PN,
// relationship, description — no grade/confidence/source columns.
func TestBuildSingleSubsSheet(t *testing.T) {
	xlsx, err := BuildSingle(Row{
		Entry: parse.Entry{PN: "02CL197", Norm: "02CL197", Qty: 1},
	}, []SubRow{
		{PartNumber: "00DH517", Relationship: "primary", Description: "IBM FlashSystem battery module"},
		{PartNumber: "00ND094", Relationship: "compatible", Description: ""},
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(xlsx))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	defer func() { _ = f.Close() }()
	got, err := f.GetRows("Substitutes")
	if err != nil {
		t.Fatalf("subs rows: %v", err)
	}
	hdr := headerRowOf(t, got, []string{"Part number", "Relationship", "Description"})
	if len(got) != hdr+3 {
		t.Fatalf("subs rows = %d, want header at %d + 2", len(got), hdr)
	}
	sub := got[hdr+1]
	if sub[0] != "00DH517" || sub[1] != "primary" {
		t.Fatalf("sub row = %v", sub)
	}
}
