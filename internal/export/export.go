// Package export renders paste results as .xlsx (FM-9) — headers,
// quantities, and citation columns, so the file stands alone once it
// leaves the app. Opens clean in Excel 2016+ (excelize guarantees the
// format; a read-back test pins the shape).
package export

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/partstableHQ/connector/internal/compendium"
	"github.com/partstableHQ/connector/internal/lookup"
	"github.com/partstableHQ/connector/internal/parse"
	"github.com/xuri/excelize/v2"
)

const sheet = "Parts"

// Row is one line of the export: the user's paste entry plus its lookup
// result (nil Result or nil Part when the compendium has no record — the
// row still ships so the export is a complete copy of the paste).
type Row struct {
	Entry parse.Entry
	Res   *lookup.Result
}

// citationLabel maps the compendium source vocabulary to the human label.
func citationLabel(source string) string {
	switch source {
	case compendium.SourceOEM:
		return "OEM"
	case compendium.SourceGovernmentRegistry:
		return "GOVERNMENT REGISTRY"
	case compendium.SourceBrokerVerified:
		return "BROKER-VERIFIED"
	case compendium.SourcePartner:
		return "PARTNER"
	case compendium.SourceCertified:
		return "CERTIFIED ★"
	default:
		return strings.ToUpper(source)
	}
}

// Build renders the rows into .xlsx bytes.
func Build(rows []Row) ([]byte, error) {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	// The fresh workbook's default sheet is the only one; make it ours.
	if err := f.SetSheetName(f.GetSheetName(0), sheet); err != nil {
		return nil, fmt.Errorf("export: rename sheet: %w", err)
	}

	headers := []string{"Your part", "Qty", "Description", "Category", "Cross-references", "Holders", "Source lines"}
	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		return nil, fmt.Errorf("export: header style: %w", err)
	}
	wrap, err := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"}})
	if err != nil {
		return nil, fmt.Errorf("export: wrap style: %w", err)
	}

	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return nil, fmt.Errorf("export: header %s: %w", h, err)
		}
		if err := f.SetCellStyle(sheet, cell, cell, bold); err != nil {
			return nil, fmt.Errorf("export: header style %s: %w", h, err)
		}
	}

	for i, r := range rows {
		rowN := i + 2
		desc, category := "(no record in compendium rev)", ""
		var xrefs, holders []string
		if r.Res != nil && r.Res.Part != nil {
			desc = r.Res.Part.Description
			category = r.Res.Part.Category
			for _, x := range r.Res.Part.Xrefs {
				xrefs = append(xrefs, fmt.Sprintf("%s (%s · %s)", x.ToPN, x.Kind, citationLabel(x.Source)))
			}
			for _, h := range r.Res.Part.Holders {
				cond := h.Condition
				if cond == "" {
					cond = "condition n/a"
				}
				holders = append(holders, fmt.Sprintf("%s — qty %d · %s · seen %s (%s)",
					h.Holder, h.Qty, cond, h.LastSeen, citationLabel(h.Source)))
			}
		}
		if len(xrefs) == 0 {
			xrefs = []string{"—"}
		}
		if len(holders) == 0 {
			holders = []string{"—"}
		}

		lineNos := make([]string, 0, len(r.Entry.Lines))
		for _, n := range r.Entry.Lines {
			lineNos = append(lineNos, fmt.Sprintf("%d", n))
		}

		values := []any{
			r.Entry.PN, r.Entry.Qty, desc, category,
			strings.Join(xrefs, "\n"), strings.Join(holders, "\n"),
			strings.Join(lineNos, ", "),
		}
		for c, v := range values {
			cell, _ := excelize.CoordinatesToCellName(c+1, rowN)
			if err := f.SetCellValue(sheet, cell, v); err != nil {
				return nil, fmt.Errorf("export: row %d: %w", rowN, err)
			}
			if c >= 2 { // description onward is multi-line text
				if err := f.SetCellStyle(sheet, cell, cell, wrap); err != nil {
					return nil, fmt.Errorf("export: row %d style: %w", rowN, err)
				}
			}
		}
	}

	widths := map[string]float64{"A": 20, "B": 7, "C": 46, "D": 14, "E": 52, "F": 56, "G": 12}
	for col, w := range widths {
		if err := f.SetColWidth(sheet, col, col, w); err != nil {
			return nil, fmt.Errorf("export: width %s: %w", col, err)
		}
	}
	if err := f.SetPanes(sheet, &excelize.Panes{
		Freeze:      true,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	}); err != nil {
		return nil, fmt.Errorf("export: freeze header: %w", err)
	}

	var buf bytes.Buffer
	if _, err := f.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("export: write: %w", err)
	}
	return buf.Bytes(), nil
}
