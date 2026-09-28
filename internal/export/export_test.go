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

// FM-9: one click must yield a valid .xlsx of the current table. The
// format is the minimal broker deliverable (CEO ruling 2026-09-28):
// PN, qty, description, substitute PNs — no sources, grades, holders.
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
	if len(got) != 3 {
		t.Fatalf("rows = %d, want header + 2", len(got))
	}
	wantHeader := []string{"Your part", "Qty", "Description", "Substitutes"}
	for i, h := range wantHeader {
		if got[0][i] != h {
			t.Fatalf("header %d = %q, want %q", i, got[0][i], h)
		}
	}
	// Found part: quantity and bare substitute PNs ride along — no source
	// labels, no holders, no provenance columns.
	if got[1][0] != "02CL197" || got[1][1] != "4" {
		t.Fatalf("found row = %v", got[1])
	}
	if !strings.Contains(got[1][3], "4X70J67435") {
		t.Fatalf("substitute PN missing: %q", got[1][3])
	}
	if strings.Contains(got[1][3], "BROKER") || strings.Contains(got[1][3], "substitute ·") {
		t.Fatalf("source annotation leaked into export: %q", got[1][3])
	}
	// Missing part: still present, honestly marked — the export is a
	// complete copy of the paste.
	if got[2][0] != "MYSTERY99PN" || !strings.Contains(got[2][2], "no record") {
		t.Fatalf("missing row = %v", got[2])
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
	if len(got) != 3 {
		t.Fatalf("subs rows = %d, want header + 2", len(got))
	}
	wantHeader := []string{"Part number", "Relationship", "Description"}
	for i, h := range wantHeader {
		if got[0][i] != h {
			t.Fatalf("header %d = %q, want %q", i, got[0][i], h)
		}
	}
	if got[1][0] != "00DH517" || got[1][1] != "primary" {
		t.Fatalf("sub row = %v", got[1])
	}
}
