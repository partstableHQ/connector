package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/partstableHQ/connector/internal/export"
	"github.com/partstableHQ/connector/internal/lookup"
	"github.com/partstableHQ/connector/internal/parse"
)

// ProductionPartsSearchURL is the production parts search API — the full
// calibrated dataset (187K parts). The Connector's local compendium is the
// offline cache; when online, the typeahead hits this for live results.
const ProductionPartsSearchURL = "https://partstable.com/api/v1/parts/search"

// ProductionTDSURL is the canonical read model TDS endpoint
// (Constitution §4.1: GET /api/public/v1/tds/:pn).
const ProductionTDSURL = "https://partstable.com/api/public/v1/tds"

var catalogClient = &http.Client{Timeout: 10 * time.Second}

// prodCache is a small TTL cache in front of the production endpoints.
// Every consumer (typeahead, paste-grid enrichment, TDS panel, Excel
// export) shares it, so an export never re-fights the production edge
// for parts the grid looked up seconds ago — the edge throttles bursts,
// and a throttled export used to ship "(no record)" rows the grid had
// already answered.
type prodCacheEntry struct {
	value     any
	expiresAt time.Time
}

var prodCache = struct {
	sync.Mutex
	entries map[string]prodCacheEntry
}{entries: make(map[string]prodCacheEntry)}

const (
	prodCacheTTL      = 15 * time.Minute
	prodCacheMaxEntri = 4096
)

func prodCacheGet(key string) (any, bool) {
	prodCache.Lock()
	defer prodCache.Unlock()
	e, ok := prodCache.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		return nil, false
	}
	return e.value, true
}

func prodCachePut(key string, value any) {
	prodCache.Lock()
	defer prodCache.Unlock()
	if len(prodCache.entries) >= prodCacheMaxEntri {
		// Bounded cache: drop everything past its TTL; if the map is still
		// full of live entries (pathological), drop the whole map — it is a
		// cache, not a store.
		now := time.Now()
		for k, e := range prodCache.entries {
			if now.After(e.expiresAt) {
				delete(prodCache.entries, k)
			}
		}
		if len(prodCache.entries) >= prodCacheMaxEntri {
			prodCache.entries = make(map[string]prodCacheEntry)
		}
	}
	prodCache.entries[key] = prodCacheEntry{value: value, expiresAt: time.Now().Add(prodCacheTTL)}
}

// cachedProductionResult is productionResult behind the TTL cache, with
// the TDS substitute graph already attached — enrichment happens once at
// cache-fill time, so consumers never mutate the shared cached pointer.
func cachedProductionResult(ctx context.Context, pn string) *lookup.Result {
	norm := strings.ToUpper(strings.ReplaceAll(pn, "-", ""))
	if v, ok := prodCacheGet("result:" + norm); ok {
		if res, ok := v.(*lookup.Result); ok {
			return res
		}
	}
	res := productionResult(ctx, pn)
	if res != nil && res.Part != nil {
		if subs := cachedTdsSubsFull(ctx, res.Part.PN); len(subs) > 0 {
			xrefs := make([]lookup.Xref, 0, len(subs))
			for _, sub := range subs {
				xrefs = append(xrefs, lookup.Xref{ToPN: sub.PartNumber, Kind: "substitute"})
			}
			res.Part.Xrefs = xrefs
		}
		prodCachePut("result:"+norm, res)
	}
	return res
}

// cachedTdsSubsFull is tdsSubsFull behind the TTL cache.
func cachedTdsSubsFull(ctx context.Context, pn string) []tdsSub {
	norm := strings.ToUpper(strings.ReplaceAll(pn, "-", ""))
	if v, ok := prodCacheGet("subs:" + norm); ok {
		if subs, ok := v.([]tdsSub); ok {
			return subs
		}
	}
	subs := tdsSubsFull(ctx, pn)
	if subs != nil {
		prodCachePut("subs:"+norm, subs)
	}
	return subs
}

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

// handlePLookup is the production-enriched single-part lookup: identity
// from the catalog, substitutes from TDS; local compendium as the offline
// fallback. Used by the paste view to enrich every parsed row with REAL data.
func (s *Server) handlePLookup(w http.ResponseWriter, r *http.Request) {
	pn := strings.TrimSpace(r.URL.Query().Get("pn"))
	if pn == "" {
		respond(w, http.StatusBadRequest, map[string]string{"error": "missing pn parameter"})
		return
	}
	if res := productionResultWithSubs(r.Context(), pn); res != nil {
		respond(w, http.StatusOK, res)
		return
	}
	res, err := s.svc.Lookup(r.Context(), pn)
	respondResult(w, res, err)
}

// productionResultWithSubs is the enriched production lookup (identity +
// TDS substitutes) behind the TTL cache — what /plookup and paste rows
// serve.
func productionResultWithSubs(ctx context.Context, pn string) *lookup.Result {
	return cachedProductionResult(ctx, pn)
}

// tdsSub is one production substitute with the fields the export renders.
type tdsSub struct {
	PartNumber       string   `json:"partNumber"`
	Description      string   `json:"description"`
	RelationshipType string   `json:"relationshipType"`
	MatchGrade       string   `json:"matchGrade"`
	Confidence       float64  `json:"confidence"`
	Sources          []string `json:"sources"`
}

// tdsSubsFull fetches the production TDS substitutes with grade and
// confidence — the richer shape the Excel export renders.
func tdsSubsFull(ctx context.Context, pn string) []tdsSub {
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
			Substitutes []tdsSub `json:"substitutes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || !parsed.Success {
		return nil
	}
	return parsed.Data.Substitutes
}

// handleLookupExport renders one looked-up part plus its verified
// substitutes as an .xlsx — the single-part counterpart of /paste/export,
// so even a one-part lookup yields the deliverable file with no re-typing.
func (s *Server) handleLookupExport(w http.ResponseWriter, r *http.Request) {
	pn := strings.TrimSpace(r.URL.Query().Get("pn"))
	if pn == "" {
		respond(w, http.StatusBadRequest, map[string]string{"error": "missing pn parameter"})
		return
	}
	res := cachedProductionResult(r.Context(), pn)
	if res == nil || res.Part == nil {
		if lr, err := s.svc.Lookup(r.Context(), pn); err == nil {
			res = &lr
		}
	}
	identity := pn
	if res != nil && res.Part != nil && res.Part.PN != "" {
		identity = res.Part.PN
	}
	if res == nil {
		res = &lookup.Result{Query: pn, Normalized: lookup.Normalize(pn), MatchedBy: lookup.MatchNone}
	}
	// One cached TDS fetch feeds both the Parts sheet substitutes and the
	// Substitutes sheet — a second call to the same endpoint is a flake
	// window, not a different answer.
	subs := cachedTdsSubsFull(r.Context(), identity)
	if res.Part != nil && len(subs) > 0 {
		xrefs := make([]lookup.Xref, 0, len(subs))
		for _, sub := range subs {
			xrefs = append(xrefs, lookup.Xref{ToPN: sub.PartNumber, Kind: "substitute"})
		}
		res.Part.Xrefs = xrefs
	}
	subRows := make([]export.SubRow, 0, len(subs))
	for _, sub := range subs {
		// Production relationship vocabulary is internal ("same_fsc", …);
		// the deliverable carries the two broker-facing words only.
		rel := "substitute"
		if strings.EqualFold(sub.RelationshipType, "primary") {
			rel = "primary"
		}
		subRows = append(subRows, export.SubRow{
			PartNumber:   sub.PartNumber,
			Relationship: rel,
			Description:  sub.Description,
		})
	}
	xlsx, err := export.BuildSingle(export.Row{
		Entry: parse.Entry{PN: identity, Norm: res.Normalized, Qty: 1},
		Res:   res,
	}, subRows)
	if err != nil {
		respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=partstable-%s.xlsx", url.PathEscape(identity)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(xlsx)
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
