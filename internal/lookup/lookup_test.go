package lookup

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/partstableHQ/connector/internal/compendium"
)

// seed builds an open compendium with one part, an alias, one xref and one
// holder — enough to exercise every lookup route.
func seed(t *testing.T) *Service {
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
		`INSERT INTO part_aliases (alias, pn) VALUES ('2CL197', '02CL197')`,
		`INSERT INTO xrefs (from_pn, to_pn, kind, source, source_detail)
		 VALUES ('02CL197', '32GBDDR43200ECC', 'substitute', 'broker_verified', 'broker lot #812')`,
		`INSERT INTO holders (pn, holder, qty, condition, last_seen, source, source_detail)
		 VALUES ('02CL197', 'Test Broker NL', 4, 'refurb', '2026-09-01', 'partner', 'feed sync')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return New(db)
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"02CL197":     "02CL197",
		"  02cl197  ": "02CL197",
		"02-cl.197":   "02CL197",
		"2c_l197":     "2CL197",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

// FM-5: leading-zero variants must resolve without conflating identities —
// the verbatim query and any normalization are visible in the result.
func TestLookupRoutes(t *testing.T) {
	ctx := context.Background()
	svc := seed(t)

	t.Run("exact", func(t *testing.T) {
		res, err := svc.Lookup(ctx, "02cl197")
		if err != nil {
			t.Fatalf("lookup: %v", err)
		}
		if res.MatchedBy != MatchExact || res.Part == nil {
			t.Fatalf("matched_by=%q part=%v", res.MatchedBy, res.Part)
		}
		if res.Part.DisplayPN != "02cl197" || res.Part.Description != "LP ECC UDIMM 32GB DDR4-3200" {
			t.Fatalf("part identity wrong: %+v", res.Part)
		}
		if len(res.Part.Xrefs) != 1 || res.Part.Xrefs[0].Source != "broker_verified" {
			t.Fatalf("xrefs wrong: %+v", res.Part.Xrefs)
		}
		if len(res.Part.Holders) != 1 || res.Part.Holders[0].Qty != 4 {
			t.Fatalf("holders wrong: %+v", res.Part.Holders)
		}
	})

	t.Run("alias", func(t *testing.T) {
		res, err := svc.Lookup(ctx, "2c-l197")
		if err != nil {
			t.Fatalf("lookup: %v", err)
		}
		if res.MatchedBy != MatchAlias {
			t.Fatalf("matched_by=%q, want alias", res.MatchedBy)
		}
		if res.Part == nil || res.Part.PN != "02CL197" {
			t.Fatalf("alias must resolve to the canonical identity: %+v", res.Part)
		}
	})

	t.Run("not found is an answer, not an error", func(t *testing.T) {
		res, err := svc.Lookup(ctx, "NOTAREALPN99")
		if err != nil {
			t.Fatalf("lookup: %v", err)
		}
		if res.MatchedBy != MatchNone || res.Part != nil {
			t.Fatalf("not-found must be empty: %+v", res)
		}
		if res.Query != "NOTAREALPN99" || res.Normalized != "NOTAREALPN99" {
			t.Fatalf("verbatim query must ride along: %+v", res)
		}
	})

	t.Run("no compendium", func(t *testing.T) {
		_, err := New(nil).Lookup(ctx, "02CL197")
		if !errors.Is(err, ErrNoCompendium) {
			t.Fatalf("err = %v, want ErrNoCompendium", err)
		}
	})
}

func TestXrefsEndpoint(t *testing.T) {
	ctx := context.Background()
	svc := seed(t)

	res, err := svc.Xrefs(ctx, "02CL197")
	if err != nil {
		t.Fatalf("xrefs: %v", err)
	}
	if res.MatchedBy != MatchExact || res.Part == nil || len(res.Part.Xrefs) != 1 {
		t.Fatalf("xref result wrong: %+v", res)
	}
}

func TestHealth(t *testing.T) {
	ctx := context.Background()
	svc := seed(t)

	info, err := svc.Health(ctx)
	if err != nil || info == nil {
		t.Fatalf("health: %v, %v", info, err)
	}
	if info.Schema != 1 || info.PartCount != 1 {
		t.Fatalf("info wrong: %+v", info)
	}
	want := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	if !info.Vintage.Equal(want) {
		t.Fatalf("vintage = %v, want %v", info.Vintage, want)
	}

	if h, err := New(nil).Health(ctx); err != nil || h != nil {
		t.Fatalf("health without compendium = %v, %v; want nil, nil", h, err)
	}
}
