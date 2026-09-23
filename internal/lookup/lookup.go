// Package lookup answers part questions from the local compendium. It is
// the single read path for both the desktop UI and the localhost API — one
// service, the same verbs, no duplicated logic.
package lookup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/partstableHQ/connector/internal/compendium"
)

// ErrNoCompendium is returned when no snapshot is installed yet.
var ErrNoCompendium = errors.New("no compendium loaded — install a release snapshot to enable lookups")

// Service reads the compendium. A nil *sql.DB is legal: every method then
// reports ErrNoCompendium, so the app runs — honestly empty — before the
// first snapshot lands.
type Service struct{ db *sql.DB }

// New builds a Service over an open (read-only) compendium handle.
func New(db *sql.DB) *Service { return &Service{db: db} }

// Normalize uppercases and strips separators (-, _, ., /, spaces). It
// never decides what the user meant: the verbatim query rides along in
// every response and any normalization is displayed (FM-5).
func Normalize(pn string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(pn)) {
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Xref is a cross-reference with its citation (FM-6: no fact without a
// source).
type Xref struct {
	ToPN         string `json:"to_pn"`
	Kind         string `json:"kind"`
	Source       string `json:"source"`
	SourceDetail string `json:"source_detail,omitempty"`
}

// Holder is who holds a part, with its citation.
type Holder struct {
	Holder       string `json:"holder"`
	Qty          int64  `json:"qty"`
	Condition    string `json:"condition,omitempty"`
	LastSeen     string `json:"last_seen"`
	Source       string `json:"source"`
	SourceDetail string `json:"source_detail,omitempty"`
}

// Part is the compendium record for one part number.
type Part struct {
	PN          string   `json:"pn"`
	DisplayPN   string   `json:"display_pn"`
	Description string   `json:"description"`
	Category    string   `json:"category,omitempty"`
	Xrefs       []Xref   `json:"xrefs"`
	Holders     []Holder `json:"holders"`
}

// Result answers one query: verbatim input preserved, normalization
// visible, match route explicit.
type Result struct {
	Query      string `json:"query"`
	Normalized string `json:"normalized"`
	MatchedBy  string `json:"matched_by"` // "exact", "alias", or "" when not found
	Part       *Part  `json:"part"`       // nil when not found
}

// Match routes.
const (
	MatchExact = "exact"
	MatchAlias = "alias"
	MatchNone  = ""
)

// Lookup resolves one part number against the compendium.
func (s *Service) Lookup(ctx context.Context, pn string) (Result, error) {
	res := Result{Query: pn, Normalized: Normalize(pn), MatchedBy: MatchNone}
	if s.db == nil {
		return res, ErrNoCompendium
	}
	if res.Normalized == "" {
		return res, fmt.Errorf("empty part number")
	}

	canonical, matchedBy, err := s.resolve(ctx, res.Normalized)
	if err != nil {
		return res, err
	}
	if canonical == "" {
		return res, nil
	}
	res.MatchedBy = matchedBy

	p := &Part{Xrefs: []Xref{}, Holders: []Holder{}}
	if err := s.db.QueryRowContext(ctx,
		`SELECT pn, display_pn, description, category FROM parts WHERE pn = ?`, canonical).
		Scan(&p.PN, &p.DisplayPN, &p.Description, &p.Category); err != nil {
		return res, fmt.Errorf("lookup %s: %w", canonical, err)
	}
	xrefs, err := s.xrefsFor(ctx, canonical)
	if err != nil {
		return res, err
	}
	holders, err := s.holdersFor(ctx, canonical)
	if err != nil {
		return res, err
	}
	p.Xrefs = xrefs
	p.Holders = holders
	res.Part = p
	return res, nil
}

// Xrefs answers the cross-reference question alone.
func (s *Service) Xrefs(ctx context.Context, pn string) (Result, error) {
	res := Result{Query: pn, Normalized: Normalize(pn), MatchedBy: MatchNone}
	if s.db == nil {
		return res, ErrNoCompendium
	}
	if res.Normalized == "" {
		return res, fmt.Errorf("empty part number")
	}
	canonical, matchedBy, err := s.resolve(ctx, res.Normalized)
	if err != nil {
		return res, err
	}
	if canonical == "" {
		return res, nil
	}
	res.MatchedBy = matchedBy
	xrefs, err := s.xrefsFor(ctx, canonical)
	if err != nil {
		return res, err
	}
	// Reuse Part as the carrier of matched identity; only xrefs populated.
	res.Part = &Part{PN: canonical, Xrefs: xrefs, Holders: []Holder{}}
	return res, nil
}

// Search returns parts whose pn, display_pn, or description starts with
// or contains the normalized query — the typeahead feed for the UI
// (FM-5: instant feedback as you type). Results are capped; the query
// must be at least 2 characters to avoid a full-table scan.
func (s *Service) Search(ctx context.Context, query string, limit int) ([]Part, error) {
	if s.db == nil {
		return nil, ErrNoCompendium
	}
	norm := Normalize(query)
	if len(norm) < 2 {
		return nil, nil
	}
	if limit <= 0 || limit > 20 {
		limit = 8
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT pn, display_pn, description, category FROM parts
		WHERE pn LIKE ? || '%' OR description LIKE '%' || ? || '%'
		ORDER BY CASE WHEN pn LIKE ? || '%' THEN 0 ELSE 1 END, pn
		LIMIT ?`, norm, strings.ToUpper(norm), norm, limit)
	if err != nil {
		return nil, fmt.Errorf("search %s: %w", norm, err)
	}
	defer func() { _ = rows.Close() }()
	out := []Part{}
	for rows.Next() {
		var p Part
		if err := rows.Scan(&p.PN, &p.DisplayPN, &p.Description, &p.Category); err != nil {
			return nil, fmt.Errorf("search %s: %w", norm, err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Health reports the loaded compendium's identity and vintage, or nil when
// nothing is loaded — the honest-staleness display (FM-10) reads this.
func (s *Service) Health(ctx context.Context) (*compendium.Info, error) {
	if s.db == nil {
		return nil, nil
	}
	info, err := compendium.ReadInfo(ctx, s.db)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

// resolve maps a normalized query to the canonical part number, via exact
// match first, then the alias table. Empty canonical = not found.
func (s *Service) resolve(ctx context.Context, normalized string) (string, string, error) {
	var pn string
	err := s.db.QueryRowContext(ctx, `SELECT pn FROM parts WHERE pn = ?`, normalized).Scan(&pn)
	switch {
	case err == nil:
		return pn, MatchExact, nil
	case !errors.Is(err, sql.ErrNoRows):
		return "", MatchNone, fmt.Errorf("resolve %s: %w", normalized, err)
	}
	err = s.db.QueryRowContext(ctx,
		`SELECT pn FROM part_aliases WHERE alias = ?`, normalized).Scan(&pn)
	switch {
	case err == nil:
		return pn, MatchAlias, nil
	case !errors.Is(err, sql.ErrNoRows):
		return "", MatchNone, fmt.Errorf("resolve %s: %w", normalized, err)
	}
	return "", MatchNone, nil
}

func (s *Service) xrefsFor(ctx context.Context, pn string) ([]Xref, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT to_pn, kind, source, source_detail
		FROM xrefs WHERE from_pn = ? ORDER BY to_pn, kind, source`, pn)
	if err != nil {
		return nil, fmt.Errorf("xrefs %s: %w", pn, err)
	}
	defer func() { _ = rows.Close() }()
	out := []Xref{}
	for rows.Next() {
		var x Xref
		if err := rows.Scan(&x.ToPN, &x.Kind, &x.Source, &x.SourceDetail); err != nil {
			return nil, fmt.Errorf("xrefs %s: %w", pn, err)
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Service) holdersFor(ctx context.Context, pn string) ([]Holder, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT holder, qty, condition, last_seen, source, source_detail
		FROM holders WHERE pn = ? ORDER BY last_seen DESC`, pn)
	if err != nil {
		return nil, fmt.Errorf("holders %s: %w", pn, err)
	}
	defer func() { _ = rows.Close() }()
	out := []Holder{}
	for rows.Next() {
		var h Holder
		if err := rows.Scan(&h.Holder, &h.Qty, &h.Condition, &h.LastSeen, &h.Source, &h.SourceDetail); err != nil {
			return nil, fmt.Errorf("holders %s: %w", pn, err)
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
