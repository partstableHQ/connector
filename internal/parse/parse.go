// Package parse turns pasted text into part-number/quantity entries.
//
// The parser-warns rule (the order-intake lesson): unparseable input is
// collected into warnings with its source line — nothing is ever dropped
// silently. Tolerated forms include newline/comma/semicolon/tab lists,
// quantities as x4 / 4x / qty 4 / qty:4 / qty=4 / qty4, CSV pn,qty and
// qty,pn, and prose like quote emails ("please quote 20x 02CL197").
//
// Identity and display are separate: the verbatim part token is preserved
// and the normalized form (lookup.Normalize) aggregates duplicates — a
// part pasted twice sums its quantities.
package parse

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/partstableHQ/connector/internal/lookup"
)

// Entry is one aggregated part from the paste.
type Entry struct {
	Raw   string `json:"raw"`   // the verbatim source line (first occurrence)
	PN    string `json:"pn"`    // the verbatim part token
	Norm  string `json:"norm"`  // normalized identity
	Qty   int    `json:"qty"`   // summed quantity (≥1)
	Lines []int  `json:"lines"` // source line numbers
}

// Warning is input the parser refused, with its source line.
type Warning struct {
	Line int    `json:"line"`
	Raw  string `json:"raw"`
	Text string `json:"text"`
}

// Result is the full outcome of a parse.
type Result struct {
	Entries  []Entry   `json:"entries"`
	Warnings []Warning `json:"warnings"`
}

// Quantity token forms. Each captures the number; tried in order.
var (
	reQtyWord = regexp.MustCompile(`(?i)\bqty\b[\s:=-]*(\d{1,6})\b`)
	reQtyGlue = regexp.MustCompile(`(?i)\bqty(\d{1,6})\b`)
	reQtyLead = regexp.MustCompile(`(?i)^x\s?(\d{1,6})$`)
	reQtyTail = regexp.MustCompile(`(?i)^(\d{1,6})\s?x$`)

	reTokenOK  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
	reHasDigit = regexp.MustCompile(`[0-9]`)
	rePureNum  = regexp.MustCompile(`^[0-9]+$`)
	rePureApl  = regexp.MustCompile(`^[A-Za-z]+$`)
	reDateLike = regexp.MustCompile(`^\d{1,4}[-/.]\d{1,2}[-/.]\d{1,4}$`)
)

const punctuation = ".,;:!?'\"()[]{}<>|«»“”‘’"

// itemKind classifies one whitespace-separated token of a line.
type itemKind int

const (
	itemPart     itemKind = iota // a part token (may carry its own qty)
	itemQtyToken                 // a standalone quantity (x4, qty 4)
	itemBareInt                  // a short pure number (qty candidate)
	itemGlue                     // prose words, emails, dates — ignored
	itemJunk                     // unrecognizable → warning
)

type item struct {
	kind itemKind
	text string // the original token
	part string // for itemPart: the verbatim part token
	qty  int    // for part/qtyToken items: the extracted quantity
	bare int    // for itemBareInt: the number
}

// Parse converts pasted text into entries and warnings.
func Parse(text string) Result {
	res := Result{Entries: []Entry{}, Warnings: []Warning{}}
	byNorm := map[string]int{}

	for i, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		parseLine(line, i+1, &res, byNorm)
	}
	return res
}

func parseLine(line string, lineNo int, res *Result, byNorm map[string]int) {
	warned := false

	var items []item
	for _, seg := range splitSegments(line) {
		for _, tok := range strings.Fields(seg) {
			it, hadWarn := classify(tok, lineNo, line)
			if hadWarn != nil {
				res.Warnings = append(res.Warnings, *hadWarn)
				warned = true
				continue
			}
			if it != nil {
				items = append(items, *it)
			}
		}
	}

	// Attach each quantity and bare integer to the nearest part without a
	// quantity, preferring the left neighbor — "02CL197 x4" and
	// "qty 4 02CL197" both read naturally.
	parts := []int{} // indices into items
	for idx := range items {
		if items[idx].kind == itemPart {
			parts = append(parts, idx)
		}
	}
	claim := func(from int, n int) bool {
		best, bestDist := -1, len(items)+1
		for _, pi := range parts {
			if items[pi].qty > 0 {
				continue // already claimed
			}
			d := pi - from
			if d < 0 {
				d = -d
			}
			if d < bestDist || (d == bestDist && pi < from) {
				best, bestDist = pi, d
			}
		}
		if best == -1 {
			return false
		}
		items[best].qty = n
		return true
	}

	for idx := range items {
		it := &items[idx]
		switch it.kind {
		case itemQtyToken:
			if !claim(idx, it.qty) {
				res.Warnings = append(res.Warnings, Warning{
					Line: lineNo, Raw: line,
					Text: "quantity " + strconv.Itoa(it.qty) + " has no part number",
				})
				warned = true
			}
		case itemBareInt:
			if !claim(idx, it.bare) {
				res.Warnings = append(res.Warnings, Warning{
					Line: lineNo, Raw: line,
					Text: "bare number " + strconv.Itoa(it.bare) + " — treated as neither quantity nor part",
				})
				warned = true
			}
		}
	}

	folded := 0
	for idx := range items {
		it := &items[idx]
		if it.kind != itemPart {
			continue
		}
		qty := it.qty
		if qty <= 0 {
			qty = 1
		}
		fold(res, byNorm, it.part, qty, lineNo, line)
		folded++
	}
	if folded == 0 && !warned {
		res.Warnings = append(res.Warnings, Warning{
			Line: lineNo, Raw: line, Text: "unrecognized line — no part number found",
		})
	}
}

// classify maps one token to an item. A non-nil warning return is appended
// to the result; a nil item and nil warning means the token is prose glue.
func classify(tok string, lineNo int, raw string) (*item, *Warning) {
	clean := strings.Trim(tok, punctuation)
	if clean == "" {
		return nil, nil
	}

	// Quantity-bearing tokens first: "x4", "4x", "qty 4"-style glue can
	// only ride on a part when written together ("02CL197x4").
	qty, rest, hasQty := extractQty(clean)
	if hasQty && rest == "" {
		return &item{kind: itemQtyToken, text: tok, qty: qty}, nil
	}
	rest = strings.Trim(rest, punctuation)
	if rest != "" && rest != clean && isPartToken(rest) {
		return &item{kind: itemPart, text: tok, part: rest, qty: qty}, nil
	}
	if rest != clean {
		clean = rest
	}

	// Tolerated glue first — dates, emails, prose words — so they can
	// never pose as part numbers (a date passes the part-token shape).
	switch {
	case rePureApl.MatchString(clean), reDateLike.MatchString(clean), strings.Contains(clean, "@"):
		return nil, nil
	case isPartToken(clean):
		return &item{kind: itemPart, text: tok, part: clean, qty: qty}, nil
	case rePureNum.MatchString(clean) && len(clean) <= 6:
		n, err := strconv.Atoi(clean)
		if err != nil || n < 0 {
			return nil, &Warning{Line: lineNo, Raw: raw, Text: "unreadable number " + strconv.Quote(tok)}
		}
		return &item{kind: itemBareInt, text: tok, bare: n}, nil
	default:
		return nil, &Warning{Line: lineNo, Raw: raw, Text: "unrecognized token " + strconv.Quote(tok)}
	}
}

// extractQty finds a quantity form in tok and returns (qty, remainder,
// found). Forms: "qty 4"/"qty:4"/"qty=4"/"qty4", "x4", "4x".
func extractQty(tok string) (int, string, bool) {
	for _, re := range []*regexp.Regexp{reQtyWord, reQtyGlue, reQtyLead, reQtyTail} {
		if m := re.FindStringSubmatchIndex(tok); m != nil {
			n, err := strconv.Atoi(tok[m[2]:m[3]])
			if err != nil || n <= 0 {
				continue
			}
			return n, strings.TrimSpace(tok[:m[0]] + " " + tok[m[1]:]), true
		}
	}
	return 0, tok, false
}

// isPartToken decides whether a token can be a part number: right charset
// and length, containing at least one digit (so prose never poses as a
// part). Long pure-numeric tokens (serial-style SKUs) qualify; short ones
// are quantity candidates handled above.
func isPartToken(tok string) bool {
	if len(tok) < 3 || len(tok) > 40 || !reTokenOK.MatchString(tok) {
		return false
	}
	if !reHasDigit.MatchString(tok) {
		return false
	}
	if rePureNum.MatchString(tok) && len(tok) <= 6 {
		return false
	}
	return true
}

// splitSegments breaks a line on the list separators we accept.
func splitSegments(line string) []string {
	return strings.FieldsFunc(line, func(r rune) bool {
		return r == ',' || r == ';' || r == '\t'
	})
}

// fold folds one part occurrence into the aggregate entries.
func fold(res *Result, byNorm map[string]int, pn string, qty int, lineNo int, rawLine string) {
	norm := lookup.Normalize(pn)
	if idx, ok := byNorm[norm]; ok {
		e := &res.Entries[idx]
		e.Qty += qty
		e.Lines = append(e.Lines, lineNo)
		return
	}
	byNorm[norm] = len(res.Entries)
	res.Entries = append(res.Entries, Entry{
		Raw: rawLine, PN: pn, Norm: norm, Qty: qty, Lines: []int{lineNo},
	})
}
