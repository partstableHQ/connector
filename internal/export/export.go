// Package export renders paste results as .xlsx (FM-9). The format is the
// minimal broker deliverable per the CEO's 2026-09-28 ruling: part numbers,
// quantities, descriptions, and substitute PNs — no sources, grades,
// confidence, or holders — presented in the PartsTable brand form: brand
// band up top, styled header, bordered banded table, print-ready.
package export

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/partstableHQ/connector/internal/lookup"
	"github.com/partstableHQ/connector/internal/parse"
	"github.com/xuri/excelize/v2"
)

const sheet = "Parts"

// Brand palette (the app's design tokens, mirrored in the workbook).
const (
	brandBlue   = "0055DD"
	brandInk    = "0F1923"
	brandMuted  = "64748B"
	brandBorder = "D5DCE8"
	brandBand   = "F4F7FE"
	headerFill  = "0055DD"
)

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
	if err := writePartsSheet(f, rows, time.Now()); err != nil {
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
	now := time.Now()
	if err := writePartsSheet(f, []Row{r}, now); err != nil {
		return nil, err
	}
	if len(subs) > 0 {
		if err := writeSubsSheet(f, subs, now); err != nil {
			return nil, err
		}
	}
	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("export: write: %w", err)
	}
	return buf.Bytes(), nil
}

// excelStyles carries the shared styles of the branded form.
type excelStyles struct {
	brand   int // "PartsTable Connector" title line
	tagline int // muted tagline line
	meta    int // right-aligned generated-at line
	header  int // table header: white on brand blue
	cell    int // bordered, top-aligned, wrapped
	cellBnd int // same, banded fill
	pn      int // part-number cell: monospace
	pnBnd   int // banded
}

func newStyles(f *excelize.File) (excelStyles, error) {
	var s excelStyles
	var err error
	border := []excelize.Border{
		{Type: "left", Color: brandBorder, Style: 1},
		{Type: "right", Color: brandBorder, Style: 1},
		{Type: "top", Color: brandBorder, Style: 1},
		{Type: "bottom", Color: brandBorder, Style: 1},
	}
	wrapTop := &excelize.Alignment{WrapText: true, Vertical: "top"}

	s.brand, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 20, Color: brandBlue},
		Alignment: &excelize.Alignment{Vertical: "center"},
	})
	if err != nil {
		return s, err
	}
	s.tagline, err = f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Italic: true, Size: 10, Color: brandMuted},
	})
	if err != nil {
		return s, err
	}
	s.meta, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 10, Color: brandMuted},
		Alignment: &excelize.Alignment{Horizontal: "right"},
	})
	if err != nil {
		return s, err
	}
	s.header, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Size: 11, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{headerFill}},
		Alignment: &excelize.Alignment{Vertical: "center"},
		Border:    border,
	})
	if err != nil {
		return s, err
	}
	s.cell, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 11, Color: brandInk},
		Alignment: wrapTop, Border: border,
	})
	if err != nil {
		return s, err
	}
	s.cellBnd, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 11, Color: brandInk},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{brandBand}},
		Alignment: wrapTop, Border: border,
	})
	if err != nil {
		return s, err
	}
	s.pn, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 11, Color: brandInk, Family: "Consolas"},
		Alignment: wrapTop, Border: border,
	})
	if err != nil {
		return s, err
	}
	s.pnBnd, err = f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 11, Color: brandInk, Family: "Consolas"},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{brandBand}},
		Alignment: wrapTop, Border: border,
	})
	return s, err
}

// writeBrandBand lays down the logo band: brand line, tagline, and the
// generated-at meta on the right. Returns the row the table header goes on.
func writeBrandBand(f *excelize.File, s excelStyles, sheetName string, lastCol string, parts int, now time.Time) (int, error) {
	stamp := now.Format("2006-01-02 15:04")
	if err := f.SetCellValue(sheetName, "A2", "PartsTable Connector"); err != nil {
		return 0, fmt.Errorf("export: brand line: %w", err)
	}
	if err := f.SetCellStyle(sheetName, "A2", "A2", s.brand); err != nil {
		return 0, fmt.Errorf("export: brand style: %w", err)
	}
	tagline := "The parts reference for the secondary-market IT industry. Free, open source, every fact cited."
	if err := f.SetCellValue(sheetName, "A3", tagline); err != nil {
		return 0, fmt.Errorf("export: tagline: %w", err)
	}
	if err := f.SetCellStyle(sheetName, "A3", "A3", s.tagline); err != nil {
		return 0, fmt.Errorf("export: tagline style: %w", err)
	}
	meta := fmt.Sprintf("Generated %s · %d part%s · partstable.com", stamp, parts, plural(parts))
	if err := f.SetCellValue(sheetName, "E2", meta); err != nil {
		return 0, fmt.Errorf("export: meta line: %w", err)
	}
	if err := f.SetCellStyle(sheetName, "E2", lastCol+"2", s.meta); err != nil {
		return 0, fmt.Errorf("export: meta style: %w", err)
	}
	if err := f.SetRowHeight(sheetName, 2, 28); err != nil {
		return 0, fmt.Errorf("export: brand height: %w", err)
	}
	if err := f.SetRowHeight(sheetName, 4, 8); err != nil {
		return 0, fmt.Errorf("export: spacer height: %w", err)
	}
	return 5, nil
}

func writePartsSheet(f *excelize.File, rows []Row, now time.Time) error {
	// The fresh workbook's default sheet is the only one; make it ours.
	if err := f.SetSheetName(f.GetSheetName(0), sheet); err != nil {
		return fmt.Errorf("export: rename sheet: %w", err)
	}
	s, err := newStyles(f)
	if err != nil {
		return fmt.Errorf("export: styles: %w", err)
	}
	headerRow, err := writeBrandBand(f, s, sheet, "D", len(rows), now)
	if err != nil {
		return err
	}

	headers := []string{"Your part", "Qty", "Description", "Substitutes"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, headerRow)
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return fmt.Errorf("export: header %s: %w", h, err)
		}
		if err := f.SetCellStyle(sheet, cell, cell, s.header); err != nil {
			return fmt.Errorf("export: header style %s: %w", h, err)
		}
	}
	if err := f.SetRowHeight(sheet, headerRow, 22); err != nil {
		return fmt.Errorf("export: header height: %w", err)
	}

	for i, r := range rows {
		rowN := headerRow + 1 + i
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

		body := s.cell
		pnStyle := s.pn
		if i%2 == 1 {
			body, pnStyle = s.cellBnd, s.pnBnd
		}

		values := []struct {
			col   string
			value any
			style int
		}{
			{"A", r.Entry.PN, pnStyle},
			{"B", r.Entry.Qty, body},
			{"C", desc, body},
			{"D", strings.Join(subs, "\n"), body},
		}
		for _, v := range values {
			if err := f.SetCellValue(sheet, v.col+fmt.Sprint(rowN), v.value); err != nil {
				return fmt.Errorf("export: row %d: %w", rowN, err)
			}
			if err := f.SetCellStyle(sheet, v.col+fmt.Sprint(rowN), v.col+fmt.Sprint(rowN), v.style); err != nil {
				return fmt.Errorf("export: row %d style: %w", rowN, err)
			}
		}
		if err := f.SetRowHeight(sheet, rowN, 30); err != nil {
			return fmt.Errorf("export: row %d height: %w", rowN, err)
		}
	}

	for col, w := range map[string]float64{"A": 20, "B": 7, "C": 54, "D": 30} {
		if err := f.SetColWidth(sheet, col, col, w); err != nil {
			return fmt.Errorf("export: width %s: %w", col, err)
		}
	}
	if err := f.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		YSplit:      headerRow,
		TopLeftCell: "A" + fmt.Sprint(headerRow+1),
		ActivePane:  "bottomLeft",
	}); err != nil {
		return fmt.Errorf("export: freeze header: %w", err)
	}
	return setPrintSetup(f, sheet, "D")
}

// writeSubsSheet appends the Substitutes sheet for the single-part export:
// substitute/primary PNs, relationship, description — nothing else.
func writeSubsSheet(f *excelize.File, subs []SubRow, now time.Time) error {
	const subsSheet = "Substitutes"
	if _, err := f.NewSheet(subsSheet); err != nil {
		return fmt.Errorf("export: substitutes sheet: %w", err)
	}
	s, err := newStyles(f)
	if err != nil {
		return fmt.Errorf("export: substitutes styles: %w", err)
	}
	headerRow, err := writeBrandBand(f, s, subsSheet, "C", len(subs), now)
	if err != nil {
		return err
	}

	headers := []string{"Part number", "Relationship", "Description"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, headerRow)
		if err := f.SetCellValue(subsSheet, cell, h); err != nil {
			return fmt.Errorf("export: substitutes header %s: %w", h, err)
		}
		if err := f.SetCellStyle(subsSheet, cell, cell, s.header); err != nil {
			return fmt.Errorf("export: substitutes header style: %w", err)
		}
	}
	if err := f.SetRowHeight(subsSheet, headerRow, 22); err != nil {
		return fmt.Errorf("export: substitutes header height: %w", err)
	}

	for i, sub := range subs {
		rowN := headerRow + 1 + i
		rel := sub.Relationship
		if rel == "" {
			rel = "substitute"
		}
		desc := sub.Description
		if desc == "" {
			desc = "—"
		}
		body, pnStyle := s.cell, s.pn
		if i%2 == 1 {
			body, pnStyle = s.cellBnd, s.pnBnd
		}
		values := []struct {
			col   string
			value any
			style int
		}{
			{"A", sub.PartNumber, pnStyle},
			{"B", rel, body},
			{"C", desc, body},
		}
		for _, v := range values {
			if err := f.SetCellValue(subsSheet, v.col+fmt.Sprint(rowN), v.value); err != nil {
				return fmt.Errorf("export: substitutes row %d: %w", rowN, err)
			}
			if err := f.SetCellStyle(subsSheet, v.col+fmt.Sprint(rowN), v.col+fmt.Sprint(rowN), v.style); err != nil {
				return fmt.Errorf("export: substitutes row %d style: %w", rowN, err)
			}
		}
		if err := f.SetRowHeight(subsSheet, rowN, 26); err != nil {
			return fmt.Errorf("export: substitutes row %d height: %w", rowN, err)
		}
	}

	for col, w := range map[string]float64{"A": 22, "B": 14, "C": 56} {
		if err := f.SetColWidth(subsSheet, col, col, w); err != nil {
			return fmt.Errorf("export: substitutes width %s: %w", col, err)
		}
	}
	if err := f.SetPanes(subsSheet, &excelize.Panes{
		Freeze:      true,
		YSplit:      headerRow,
		TopLeftCell: "A" + fmt.Sprint(headerRow+1),
		ActivePane:  "bottomLeft",
	}); err != nil {
		return fmt.Errorf("export: substitutes freeze header: %w", err)
	}
	return setPrintSetup(f, subsSheet, "C")
}

// setPrintSetup makes the branded form print one page wide.
func setPrintSetup(f *excelize.File, name, _ string) error {
	fitW, fitH := 1, 0
	portrait := "portrait"
	if err := f.SetPageLayout(name, &excelize.PageLayoutOptions{
		FitToWidth:  &fitW,
		FitToHeight: &fitH,
		Orientation: &portrait,
	}); err != nil {
		return fmt.Errorf("export: page layout: %w", err)
	}
	return nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
