package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/partstableHQ/connector/internal/lookup"
)

// ProductionPartsSearchURL is the production parts search API — the full
// calibrated dataset (187K parts). The Connector's local compendium is the
// offline cache; when online, the typeahead hits this for live results.
const ProductionPartsSearchURL = "https://partstable.com/api/v1/parts/search"

// ProductionTDSURL is the canonical read model TDS endpoint
// (Constitution §4.1: GET /api/public/v1/tds/:pn).
const ProductionTDSURL = "https://partstable.com/api/public/v1/tds"

var catalogClient = &http.Client{Timeout: 10 * time.Second}

// productionRequest builds a GET to the production parts search. The
// browser-style User-Agent is required: Cloudflare's bot protection
// rejects the default Go client UA.
func productionRequest(ctx context.Context, query, limit string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		ProductionPartsSearchURL+"?q="+url.QueryEscape(query)+"&limit="+url.QueryEscape(limit), nil) // #nosec G704 -- fixed URL constant
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) PartsTableConnector/1.0")
	return req, nil
}

type catalogSearchResult struct {
	PartNumber   string  `json:"partNumber"`
	NormalizedPn string  `json:"normalizedPn"`
	Description  string  `json:"description"`
	Manufacturer string  `json:"manufacturer"`
	Category1    *string `json:"category1"`
	Category2    *string `json:"category2"`
	ListPrice    string  `json:"listPrice"`
	LastCost     string  `json:"lastCost"`
}

type catalogSearchResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Query   string                `json:"query"`
		Results []catalogSearchResult `json:"results"`
	} `json:"data"`
}

// productionResult resolves one part against the production catalog and
// maps the hit into the lookup.Result shape the UI already renders.
// Returns nil when the production API is unreachable or has no match —
// callers then fall back to the local compendium.
func productionResult(ctx context.Context, pn string) *lookup.Result {
	req, err := productionRequest(ctx, pn, "1")
	if err != nil {
		return nil
	}
	resp, err := catalogClient.Do(req) // #nosec G704 -- fixed URL constant (ProductionTDSURL)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	var parsed catalogSearchResponse
	if err := json.Unmarshal(body, &parsed); err != nil || !parsed.Success || len(parsed.Data.Results) == 0 {
		return nil
	}
	h := parsed.Data.Results[0]
	// The search is fuzzy — accept the hit only when the normalized identity
	// matches what was asked for (no near-misses passed off as answers).
	if !strings.EqualFold(strings.ReplaceAll(h.PartNumber, "-", ""), strings.ReplaceAll(pn, "-", "")) {
		return nil
	}
	category := strings.TrimSpace(strings.Join(nonNil(h.Category1, h.Category2), " · "))
	return &lookup.Result{
		Query:      pn,
		Normalized: strings.ToUpper(strings.ReplaceAll(pn, "-", "")),
		MatchedBy:  lookup.MatchExact,
		Part: &lookup.Part{
			PN:          h.PartNumber,
			DisplayPN:   h.PartNumber,
			Description: h.Description,
			Category:    category,
			Xrefs:       []lookup.Xref{},
			Holders:     []lookup.Holder{},
		},
	}
}

func nonNil(ss ...*string) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if s != nil && *s != "" {
			out = append(out, *s)
		}
	}
	return out
}

// tdsSubstitutes fetches the production TDS substitutes for one part.
func tdsSubstitutes(ctx context.Context, pn string) []lookup.Xref {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		ProductionTDSURL+"/"+url.PathEscape(pn), nil) // #nosec G704 -- fixed URL constant
	if err != nil {
		return nil
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) PartsTableConnector/1.0")
	resp, err := catalogClient.Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil || resp.StatusCode != http.StatusOK {
		return nil
	}
	var parsed struct {
		Success bool `json:"success"`
		Data    struct {
			Substitutes []struct {
				PartNumber string `json:"partNumber"`
				Source     string `json:"matchSource"`
			} `json:"substitutes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || !parsed.Success {
		return nil
	}
	xrefs := make([]lookup.Xref, 0, len(parsed.Data.Substitutes))
	for _, sub := range parsed.Data.Substitutes {
		xrefs = append(xrefs, lookup.Xref{
			ToPN:   sub.PartNumber,
			Kind:   "substitute",
			Source: sub.Source,
		})
	}
	return xrefs
}

// handlePLookup is the production-enriched single-part lookup: identity
// from the catalog, substitutes from TDS; local compendium as the offline
// fallback. Used by the paste view to enrich every parsed row with REAL data.
func (s *Server) handlePLookup(w http.ResponseWriter, r *http.Request) {
	pn := strings.TrimSpace(r.URL.Query().Get("pn"))
	if pn == "" {
		respond(w, http.StatusBadRequest, map[string]string{"error": "missing pn parameter"})
		return
	}
	if res := productionResult(r.Context(), pn); res != nil {
		if subs := tdsSubstitutes(r.Context(), res.Part.PN); len(subs) > 0 {
			res.Part.Xrefs = subs
		}
		respond(w, http.StatusOK, res)
		return
	}
	res, err := s.svc.Lookup(r.Context(), pn)
	respondResult(w, res, err)
}

// handleTDS proxies the production TDS endpoint: the full technical data
// sheet for one part — substitutes, confidence, lifecycle, broker guidance.
func (s *Server) handleTDS(w http.ResponseWriter, r *http.Request) {
	pn := strings.TrimPrefix(r.URL.Path, "/tds/")
	if pn == "" {
		respond(w, http.StatusBadRequest, map[string]string{"error": "missing pn"})
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet,
		ProductionTDSURL+"/"+url.PathEscape(pn), nil) // #nosec G704 -- fixed URL constant
	if err != nil {
		respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) PartsTableConnector/1.0")
	resp, err := catalogClient.Do(req) // #nosec G704 -- fixed URL constant (ProductionTDSURL)
	if err != nil {
		respond(w, http.StatusBadGateway, map[string]string{"error": "TDS service unreachable"})
		return
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil || resp.StatusCode != http.StatusOK {
		respond(w, resp.StatusCode, map[string]string{"error": fmt.Sprintf("TDS lookup failed (HTTP %d)", resp.StatusCode)})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// style typeahead dropdown is populated with REAL part data. The production
// response shape is passed through (partNumber, description, manufacturer,
// category1/2, listPrice, lastCost) so the UI renders exactly what the
// production system renders.
func (s *Server) handleCatalogSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		respond(w, http.StatusOK, map[string]any{"results": []any{}})
		return
	}
	limit := r.URL.Query().Get("limit")
	if limit == "" {
		limit = "8"
	}
	req, err := productionRequest(r.Context(), q, limit)
	if err != nil {
		respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	resp, err := catalogClient.Do(req) // #nosec G704 -- fixed URL constant via productionRequest
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			_ = resp.Body.Close()
		}
		// Offline: degrade to the local compendium so the typeahead still works.
		local, lerr := s.svc.Search(r.Context(), q, 8)
		if lerr != nil {
			respond(w, http.StatusBadGateway, map[string]string{"error": "production search unreachable and no local compendium"})
			return
		}
		respond(w, http.StatusOK, map[string]any{"results": local, "source": "local"})
		return
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		respond(w, http.StatusBadGateway, map[string]string{"error": "production search response unreadable"})
		return
	}
	var parsed catalogSearchResponse
	if err := json.Unmarshal(body, &parsed); err != nil || !parsed.Success {
		respond(w, http.StatusBadGateway, map[string]string{"error": "production search returned an invalid response"})
		return
	}
	respond(w, http.StatusOK, map[string]any{
		"results": parsed.Data.Results,
		"source":  "production",
	})
}
