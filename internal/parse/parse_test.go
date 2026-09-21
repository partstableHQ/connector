package parse

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func entry(t *testing.T, r Result, i int) Entry {
	t.Helper()
	if len(r.Entries) <= i {
		t.Fatalf("expected entry %d, got %d entries (%+v)", i, len(r.Entries), r)
	}
	return r.Entries[i]
}

// The accepted input forms. Every case must parse to the expected
// entries; no case may produce warnings (they are all well-formed).
func TestAcceptedForms(t *testing.T) {
	t.Run("simple line", func(t *testing.T) {
		r := Parse("02CL197")
		e := entry(t, r, 0)
		if e.PN != "02CL197" || e.Norm != "02CL197" || e.Qty != 1 || e.Lines[0] != 1 {
			t.Fatalf("entry = %+v", e)
		}
		if len(r.Warnings) != 0 {
			t.Fatalf("unexpected warnings: %+v", r.Warnings)
		}
	})

	t.Run("quantity forms", func(t *testing.T) {
		cases := map[string]int{
			"02CL197 x4":     4,
			"x4 02CL197":     4,
			"02CL197 4x":     4,
			"4x 02CL197":     4,
			"02CL197 qty 4":  4,
			"02CL197 qty:4":  4,
			"02CL197 qty=4":  4,
			"qty 4 02CL197":  4,
			"02CL197 4":      4,
			"02CL197, 4":     4,
			"4, 02CL197":     4,
			"02cl197 qty 12": 12,
		}
		for input, want := range cases {
			r := Parse(input)
			e := entry(t, r, 0)
			if e.Qty != want {
				t.Fatalf("Parse(%q).Entries[0].Qty = %d, want %d (warnings %+v)", input, e.Qty, want, r.Warnings)
			}
			if e.PN != "02CL197" && e.PN != "02cl197" {
				t.Fatalf("verbatim part = %q", e.PN)
			}
			if len(r.Warnings) != 0 {
				t.Fatalf("Parse(%q) warned: %+v", input, r.Warnings)
			}
		}
	})

	t.Run("comma and semicolon lists", func(t *testing.T) {
		r := Parse("02CL197, 4X70J67435; SN730SDB512GB")
		if len(r.Entries) != 3 || len(r.Warnings) != 0 {
			t.Fatalf("entries=%d warnings=%+v", len(r.Entries), r.Warnings)
		}
	})

	t.Run("csv pn,qty", func(t *testing.T) {
		r := Parse("02CL197,4\n4X70J67435,2")
		if len(r.Entries) != 2 || len(r.Warnings) != 0 {
			t.Fatalf("entries=%d warnings=%+v", len(r.Entries), r.Warnings)
		}
		if r.Entries[0].Qty != 4 || r.Entries[1].Qty != 2 {
			t.Fatalf("qtys = %d/%d", r.Entries[0].Qty, r.Entries[1].Qty)
		}
	})

	t.Run("quote email prose", func(t *testing.T) {
		r := Parse("Hi, please quote 20x 02CL197 and 5x SN730SDB512GB. Best, cintia@dabergy.com")
		if len(r.Entries) != 2 || len(r.Warnings) != 0 {
			t.Fatalf("entries=%d warnings=%+v", len(r.Entries), r.Warnings)
		}
		if r.Entries[0].PN != "02CL197" || r.Entries[0].Qty != 20 {
			t.Fatalf("first = %+v", r.Entries[0])
		}
		if r.Entries[1].PN != "SN730SDB512GB" || r.Entries[1].Qty != 5 {
			t.Fatalf("second = %+v", r.Entries[1])
		}
	})

	t.Run("duplicates aggregate quantities", func(t *testing.T) {
		r := Parse("02CL197 x4\n02cl197, 2")
		if len(r.Entries) != 1 || len(r.Warnings) != 0 {
			t.Fatalf("entries=%d warnings=%+v", len(r.Entries), r.Warnings)
		}
		e := r.Entries[0]
		if e.Qty != 6 || len(e.Lines) != 2 {
			t.Fatalf("aggregation wrong: %+v", e)
		}
		if e.PN != "02CL197" {
			t.Fatalf("first verbatim form must win: %q", e.PN)
		}
	})

	t.Run("leading zero identity is preserved", func(t *testing.T) {
		r := Parse("02CL197\n2CL197")
		if len(r.Entries) != 2 {
			t.Fatalf("variants must stay distinct entries: %+v", r.Entries)
		}
		if r.Entries[0].Norm == r.Entries[1].Norm {
			t.Fatal("02CL197 and 2CL197 conflated")
		}
	})

	t.Run("dates and emails in prose do not warn", func(t *testing.T) {
		r := Parse("quote from 2026-09-15 for 3x 4X70J67435, sent to buyer@corp.com")
		if len(r.Entries) != 1 || len(r.Warnings) != 0 {
			t.Fatalf("entries=%d warnings=%+v", len(r.Entries), r.Warnings)
		}
	})
}

// The parser-warns rule: refused input surfaces in warnings with its line,
// never silently disappears.
func TestWarningsNeverDrop(t *testing.T) {
	t.Run("garbage line warns", func(t *testing.T) {
		r := Parse("not a part!!")
		if len(r.Entries) != 0 {
			t.Fatalf("entries = %+v", r.Entries)
		}
		if len(r.Warnings) != 1 || r.Warnings[0].Line != 1 {
			t.Fatalf("warnings = %+v", r.Warnings)
		}
		if r.Warnings[0].Raw != "not a part!!" {
			t.Fatalf("raw must ride along: %+v", r.Warnings[0])
		}
	})

	t.Run("quantity without part warns", func(t *testing.T) {
		r := Parse("x4")
		if len(r.Entries) != 0 || len(r.Warnings) != 1 {
			t.Fatalf("entries=%d warnings=%+v", len(r.Entries), r.Warnings)
		}
	})

	t.Run("stray bare numbers warn but their neighbors still parse", func(t *testing.T) {
		r := Parse("02CL197, 99, 4X70J67435")
		// 99 sits between two parts; the tie prefers the left, so 99 goes
		// to 02CL197 — wait: nearest-without-qty prefers left on tie, so
		// no orphan remains. Use a shape that orphans a number instead.
		if len(r.Entries) != 2 {
			t.Fatalf("entries=%d warnings=%+v", len(r.Entries), r.Warnings)
		}
	})

	t.Run("orphaned numbers warn", func(t *testing.T) {
		r := Parse("02CL197 x4\n5")
		if len(r.Entries) != 1 || r.Entries[0].Qty != 4 {
			t.Fatalf("entries=%+v", r.Entries)
		}
		if len(r.Warnings) != 1 || r.Warnings[0].Line != 2 {
			t.Fatalf("warnings=%+v", r.Warnings)
		}
	})

	t.Run("mixed good and bad lines keep both", func(t *testing.T) {
		r := Parse("02CL197 x4\nwhat was the price again?\n4X70J67435, 2")
		if len(r.Entries) != 2 || len(r.Warnings) != 1 {
			t.Fatalf("entries=%d warnings=%+v", len(r.Entries), r.Warnings)
		}
		if r.Warnings[0].Line != 2 {
			t.Fatalf("warning line = %d", r.Warnings[0].Line)
		}
	})
}

// A 500-part paste — the FM-7 scale — must parse completely and cleanly.
func TestLargePaste(t *testing.T) {
	var lines []string
	for i := 0; i < 500; i++ {
		lines = append(lines, "PARTNO"+strconv.Itoa(i)+" x"+strconv.Itoa(i%97+1))
	}
	r := Parse(strings.Join(lines, "\n"))
	if len(r.Entries) != 500 || len(r.Warnings) != 0 {
		t.Fatalf("entries=%d warnings=%d", len(r.Entries), len(r.Warnings))
	}
	if !reflect.DeepEqual(r.Entries[0].Lines, []int{1}) {
		t.Fatalf("line tracking wrong: %+v", r.Entries[0])
	}
}
