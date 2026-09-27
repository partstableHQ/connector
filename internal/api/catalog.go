package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ProductionPartsSearchURL is the production parts search API — the full
// calibrated dataset (187K parts). The Connector's local compendium is the
// offline cache; when online, the typeahead hits this for live results.
const ProductionPartsSearchURL = "https://partstable.com/api/v1/parts/search"

var catalogClient = &http.Client{Timeout: 10 * time.Second}

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

// handleCatalogSearch proxies the production parts search: the IQ-Reseller-
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
	// #nosec G704 -- the destination is the fixed constant
	// ProductionPartsSearchURL; only query parameters are caller-controlled,
	// and both are escaped. This is the intended outbound proxy.
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet,
		ProductionPartsSearchURL+"?q="+url.QueryEscape(q)+"&limit="+url.QueryEscape(limit), nil)
	if err != nil {
		respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	req.Header.Set("Accept", "application/json")
	resp, err := catalogClient.Do(req) // #nosec G704 -- fixed URL constant, see above
	if err != nil {
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
	if err != nil || resp.StatusCode != http.StatusOK {
		respond(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("production search failed (HTTP %d)", resp.StatusCode)})
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
