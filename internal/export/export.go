// Package export renders paste results as .xlsx (FM-9). The format is the
// minimal broker deliverable per the CEO's 2026-09-28 ruling: part numbers,
// quantities, descriptions, and substitute PNs — no sources, grades,
// confidence, or holders. Opens clean in Excel 2016+ (excelize guarantees
// the format; a read-back test pins the shape).
package export

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/partstableHQ/connector/internal/lookup"
	"github.com/partstableHQ/connector/internal/parse"
	"github.com/xuri/excelize/v2"
)

const sheet = "Parts"

// Row is one line of the export: the user's paste entry plus its lookup
// result (nil Result or nil Part when no record exists — the row still
// ships so the export is a complete copy of the paste).
type Row struct {
	Entry parse.Entry
	Res   *lookup.Result
}

// SubRow is one substitute in the single-part export.
type SubRow struct {
	PartNumber   string
	Relationship string
	Description  string
}

// Build renders the rows into .xlsx bytes.
func Build(rows []Row) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	if err := writePartsSheet(f, rows); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("export: write: %w", err)
	}
	return buf.Bytes(), nil
}

// BuildSingle renders one looked-up part plus its substitutes into a
// two-sheet workbook — the deliverable behind the Lookup view's
// Export to Excel button.
func BuildSingle(r Row, subs []SubRow) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()
	if err := writePartsSheet(f, []Row{r}); err != nil {
		return nil, err
	}
	if len(subs) > 0 {
		if err := writeSubsSheet(f, subs); err != nil {
			return nil, err
		}
	}
	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("export: write: %w", err)
	}
	return buf.Bytes(), nil
}

// writePartsSheet fills the workbook's first sheet: your part / qty /
// description / substitutes (bare PNs, one per line). Nothing else.
func writePartsSheet(f *excelize.File, rows []Row) error {
	// The fresh workbook's default sheet is the only one; make it ours.
	if err := f.SetSheetName(f.GetSheetName(0), sheet); err != nil {
		return fmt.Errorf("export: rename sheet: %w", err)
	}

	headers := []string{"Your part", "Qty", "Description", "Substitutes"}
	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return fmt.Errorf("export: header style: %w", err)
	}
	wrap, err := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"}})
	if err != nil {
		return fmt.Errorf("export: wrap style: %w", err)
	}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return fmt.Errorf("export: header %s: %w", h, err)
		}
		if err := f.SetCellStyle(sheet, cell, cell, bold); err != nil {
			return fmt.Errorf("export: header style %s: %w", h, err)
		}
	}

	for i, r := range rows {
		rowN := i + 2
		desc := "(no record)"
		var subs []string
		if r.Res != nil && r.Res.Part != nil {
			if r.Res.Part.Description != "" {
				desc = r.Res.Part.Description
			}
			for _, x := range r.Res.Part.Xrefs {
				subs = append(subs, x.ToPN)
			}
		}
		if len(subs) == 0 {
			subs = []string{"—"}
		}

		values := []any{r.Entry.PN, r.Entry.Qty, desc, strings.Join(subs, "\n")}
		for c, v := range values {
			cell, _ := excelize.CoordinatesToCellName(c+1, rowN)
			if err := f.SetCellValue(sheet, cell, v); err != nil {
				return fmt.Errorf("export: row %d: %w", rowN, err)
			}
			if c >= 2 { // description onward is multi-line text
				if err := f.SetCellStyle(sheet, cell, cell, wrap); err != nil {
					return fmt.Errorf("export: row %d style: %w", rowN, err)
				}
			}
		}
	}

	widths := map[string]float64{"A": 20, "B": 7, "C": 52, "D": 34}
	for col, w := range widths {
		if err := f.SetColWidth(sheet, col, col, w); err != nil {
			return fmt.Errorf("export: width %s: %w", col, err)
		}
	}
	if err := f.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	}); err != nil {
		return fmt.Errorf("export: freeze header: %w", err)
	}
	return nil
}

// writeSubsSheet appends the Substitutes sheet for the single-part export:
// substitute/primary PNs, relationship, description — nothing else.
func writeSubsSheet(f *excelize.File, subs []SubRow) error {
	const subsSheet = "Substitutes"
	if _, err := f.NewSheet(subsSheet); err != nil {
		return fmt.Errorf("export: substitutes sheet: %w", err)
	}
	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return fmt.Errorf("export: substitutes header style: %w", err)
	}
	headers := []string{"Part number", "Relationship", "Description"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(subsSheet, cell, h); err != nil {
			return fmt.Errorf("export: substitutes header %s: %w", h, err)
		}
		if err := f.SetCellStyle(subsSheet, cell, cell, bold); err != nil {
			return fmt.Errorf("export: substitutes header style: %w", err)
		}
	}
	for i, s := range subs {
		rowN := i + 2
		rel := s.Relationship
		if rel == "" {
			rel = "substitute"
		}
		desc := s.Description
		if desc == "" {
			desc = "—"
		}
		values := []any{s.PartNumber, rel, desc}
		for c, v := range values {
			cell, _ := excelize.CoordinatesToCellName(c+1, rowN)
			if err := f.SetCellValue(subsSheet, cell, v); err != nil {
				return fmt.Errorf("export: substitutes row %d: %w", rowN, err)
			}
		}
	}
	for col, w := range map[string]float64{"A": 22, "B": 14, "C": 52} {
		if err := f.SetColWidth(subsSheet, col, col, w); err != nil {
			return fmt.Errorf("export: substitutes width %s: %w", col, err)
		}
	}
	if err := f.SetPanes(subsSheet, &excelize.Panes{
		Freeze:      true,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	}); err != nil {
		return fmt.Errorf("export: substitutes freeze header: %w", err)
	}
	return nil
}
