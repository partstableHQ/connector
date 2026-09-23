// Package api serves the Connector's local REST API — the same verbs the
// desktop UI uses — bound to 127.0.0.1 only (FM-15). It deliberately logs
// nothing: query content never touches disk or a terminal (PRIVACY.md).
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/partstableHQ/connector/internal/auth"
	"github.com/partstableHQ/connector/internal/compendium"
	"github.com/partstableHQ/connector/internal/export"
	"github.com/partstableHQ/connector/internal/lookup"
	"github.com/partstableHQ/connector/internal/parse"
	"github.com/partstableHQ/connector/internal/telemetry"
	"github.com/partstableHQ/connector/internal/update"
)

// DefaultPort is the loopback port the local API binds.
const DefaultPort = 7878

// EnvPort overrides the port (dev and port-conflict fallback).
const EnvPort = "PARTSTABLE_API_PORT"

// MaxBulkParts caps one bulk request; 500+ PNs must complete fast (FM-7),
// the cap leaves generous headroom while bounding abuse of the loopback.
const MaxBulkParts = 5000

// Port resolves the API port: PARTSTABLE_API_PORT when set and valid,
// otherwise DefaultPort.
func Port() (int, error) {
	raw := os.Getenv(EnvPort)
	if raw == "" {
		return DefaultPort, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 65535 {
		return 0, fmt.Errorf("api: %s=%q is not a valid port", EnvPort, raw)
	}
	return n, nil
}

// MoreURL is the single outbound link the free app carries (FM-17): one
// quiet footer entry — no upsell screens, modals, or feature-gated
// buttons, and the app never nags.
const MoreURL = "https://partstable.com"

// Server is the localhost API.
type Server struct {
	svc     *lookup.Service
	authm   *auth.Manager
	updatem *update.Manager
	openURL func(string) error
	version string
	addr    string
	http    *http.Server
}

// SetURLOpener wires the system-browser opener for outbound links (the
// desktop app provides the WebView's external-browser handler; headless
// serve leaves it unset and /more returns the URL instead).
func (s *Server) SetURLOpener(fn func(string) error) { s.openURL = fn }

// New builds the API server. authm and updatem may be nil (their verbs
// then report unavailable). It does not bind; call Listen + Serve.
func New(svc *lookup.Service, appVersion string, port int, authm *auth.Manager, updatem *update.Manager) *Server {
	s := &Server{
		svc:     svc,
		authm:   authm,
		updatem: updatem,
		version: appVersion,
		addr:    fmt.Sprintf("127.0.0.1:%d", port),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /lookup", s.handleLookup)
	mux.HandleFunc("GET /search", s.handleSearch)
	mux.HandleFunc("GET /xref", s.handleXref)
	mux.HandleFunc("POST /bulk", s.handleBulk)
	mux.HandleFunc("POST /paste", s.handlePaste)
	mux.HandleFunc("POST /paste/export", s.handlePasteExport)
	mux.HandleFunc("POST /auth/login", s.handleAuthLogin)
	mux.HandleFunc("POST /auth/logout", s.handleAuthLogout)
	mux.HandleFunc("POST /update/check", s.handleUpdateCheck)
	mux.HandleFunc("POST /update/apply", s.handleUpdateApply)
	mux.HandleFunc("GET /settings", s.handleGetSettings)
	mux.HandleFunc("POST /settings", s.handlePostSettings)
	mux.HandleFunc("POST /more", s.handleMore)
	s.http = &http.Server{
		Handler:           s.cors(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

// Addr returns the address the server binds (always loopback).
func (s *Server) Addr() string { return s.addr }

// Listen binds the loopback address.
func (s *Server) Listen() (net.Listener, error) {
	return net.Listen("tcp", s.addr)
}

// Serve accepts connections on l until the server is closed.
func (s *Server) Serve(l net.Listener) error { return s.http.Serve(l) }

// Close immediately shuts the server down.
func (s *Server) Close() error { return s.http.Close() }

// Handler exposes the root handler for tests.
func (s *Server) Handler() http.Handler { return s.http.Handler }

// cors adds the headers that let the desktop webview (a different origin)
// call the loopback API. The API is loopback-only and carries no
// credentials, so a permissive origin is the correct posture.
func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type healthResponse struct {
	AppVersion string           `json:"app_version"`
	Compendium *compendium.Info `json:"compendium"`
	Auth       *auth.Status     `json:"auth"`
	Update     *update.Summary  `json:"update"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	info, err := s.svc.Health(r.Context())
	if err != nil {
		respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	resp := healthResponse{AppVersion: s.version, Compendium: info}
	if s.authm != nil {
		st := s.authm.Status()
		resp.Auth = &st
	}
	if s.updatem != nil {
		sm := s.updatem.Summary(r.Context())
		resp.Update = &sm
	}
	respond(w, http.StatusOK, resp)
}

func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if s.updatem == nil {
		respond(w, http.StatusServiceUnavailable, map[string]string{"error": "update manager unavailable"})
		return
	}
	res, err := s.updatem.Check(r.Context())
	if err != nil {
		respond(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	respond(w, http.StatusOK, res)
}

func (s *Server) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if s.updatem == nil {
		respond(w, http.StatusServiceUnavailable, map[string]string{"error": "update manager unavailable"})
		return
	}
	version, err := s.updatem.Apply(r.Context())
	switch {
	case errors.Is(err, update.ErrUpToDate):
		respond(w, http.StatusOK, map[string]any{"applied": false, "reason": "already up to date"})
	case err != nil:
		respond(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
	default:
		respond(w, http.StatusOK, map[string]any{"applied": true, "version": version, "restart": true})
	}
}

type settingsPayload struct {
	TelemetryOptOut *bool `json:"telemetry_opt_out"`
}

type settingsResponse struct {
	TelemetryOptOut    bool `json:"telemetry_opt_out"`
	TelemetryEnvForced bool `json:"telemetry_env_forced"`
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	if s.updatem == nil {
		respond(w, http.StatusServiceUnavailable, map[string]string{"error": "settings unavailable"})
		return
	}
	respond(w, http.StatusOK, settingsResponse{
		TelemetryOptOut:    s.updatem.OptedOut(r.Context()),
		TelemetryEnvForced: telemetry.EnvForcedOptOut(),
	})
}

func (s *Server) handlePostSettings(w http.ResponseWriter, r *http.Request) {
	if s.updatem == nil {
		respond(w, http.StatusServiceUnavailable, map[string]string{"error": "settings unavailable"})
		return
	}
	var body settingsPayload
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&body); err != nil {
		respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body: " + err.Error()})
		return
	}
	if body.TelemetryOptOut != nil {
		if err := s.updatem.SetOptOut(r.Context(), *body.TelemetryOptOut); err != nil {
			respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	respond(w, http.StatusOK, settingsResponse{
		TelemetryOptOut:    s.updatem.OptedOut(r.Context()),
		TelemetryEnvForced: telemetry.EnvForcedOptOut(),
	})
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, _ *http.Request) {
	if s.authm == nil {
		respond(w, http.StatusServiceUnavailable, map[string]string{"error": "account manager unavailable"})
		return
	}
	started := s.authm.Login()
	respond(w, http.StatusAccepted, map[string]bool{"started": started})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, _ *http.Request) {
	if s.authm == nil {
		respond(w, http.StatusServiceUnavailable, map[string]string{"error": "account manager unavailable"})
		return
	}
	if err := s.authm.Logout(); err != nil {
		respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respond(w, http.StatusOK, map[string]bool{"signed_in": false})
}

func (s *Server) handleLookup(w http.ResponseWriter, r *http.Request) {
	pn := strings.TrimSpace(r.URL.Query().Get("pn"))
	if pn == "" {
		respond(w, http.StatusBadRequest, map[string]string{"error": "missing pn parameter"})
		return
	}
	res, err := s.svc.Lookup(r.Context(), pn)
	respondResult(w, res, err)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		respond(w, http.StatusOK, map[string]any{"results": []lookup.Part{}})
		return
	}
	results, err := s.svc.Search(r.Context(), q, 8)
	if errors.Is(err, lookup.ErrNoCompendium) {
		respond(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if err != nil {
		respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respond(w, http.StatusOK, map[string]any{"results": results})
}

func (s *Server) handleXref(w http.ResponseWriter, r *http.Request) {
	pn := strings.TrimSpace(r.URL.Query().Get("pn"))
	if pn == "" {
		respond(w, http.StatusBadRequest, map[string]string{"error": "missing pn parameter"})
		return
	}
	res, err := s.svc.Xrefs(r.Context(), pn)
	respondResult(w, res, err)
}

type bulkRequest struct {
	PNs []string `json:"pns"`
}

func (s *Server) handleBulk(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	var req bulkRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body: " + err.Error()})
		return
	}
	if len(req.PNs) == 0 {
		respond(w, http.StatusBadRequest, map[string]string{"error": "pns must be a non-empty array"})
		return
	}
	if len(req.PNs) > MaxBulkParts {
		respond(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("pns exceeds the %d-part limit", MaxBulkParts)})
		return
	}
	results := make([]lookup.Result, 0, len(req.PNs))
	for _, pn := range req.PNs {
		res, err := s.svc.Lookup(r.Context(), pn)
		if errors.Is(err, lookup.ErrNoCompendium) {
			respond(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
			return
		}
		if err != nil {
			// One bad line must not sink the batch: report it as not-found
			// and let the warnings surface in the caller's UI.
			res = lookup.Result{Query: pn, Normalized: lookup.Normalize(pn), MatchedBy: lookup.MatchNone}
		}
		results = append(results, res)
	}
	respond(w, http.StatusOK, map[string]any{"results": results})
}

func respondResult(w http.ResponseWriter, res lookup.Result, err error) {
	switch {
	case errors.Is(err, lookup.ErrNoCompendium):
		respond(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	case err != nil:
		respond(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	default:
		respond(w, http.StatusOK, res)
	}
}

// maxPasteBytes caps a pasted list; 4 MB is orders of magnitude beyond a
// 500-part quote (FM-7) while bounding abuse of the loopback.
const maxPasteBytes = 4 << 20

// pasteEntry is one parsed line of the paste with its lookup answer.
type pasteEntry struct {
	parse.Entry
	Result *lookup.Result `json:"result"`
}

type pasteResponse struct {
	Entries  []pasteEntry    `json:"entries"`
	Warnings []parse.Warning `json:"warnings"`
}

// pasteRows is the shared path of both paste verbs: parse the raw text,
// look up every aggregated entry. The parser-warns rule means the
// warnings list rides along no matter what.
func (s *Server) pasteRows(ctx context.Context, text string) ([]pasteEntry, []parse.Warning, error) {
	pr := parse.Parse(text)
	entries := make([]pasteEntry, 0, len(pr.Entries))
	for _, e := range pr.Entries {
		res, err := s.svc.Lookup(ctx, e.Norm)
		if err != nil {
			return nil, nil, err
		}
		entries = append(entries, pasteEntry{Entry: e, Result: &res})
	}
	return entries, pr.Warnings, nil
}

func (s *Server) handlePaste(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPasteBytes))
	if err != nil {
		respond(w, http.StatusBadRequest, map[string]string{"error": "paste too large or unreadable"})
		return
	}
	entries, warnings, err := s.pasteRows(r.Context(), string(body))
	if errors.Is(err, lookup.ErrNoCompendium) {
		respond(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if err != nil {
		respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	respond(w, http.StatusOK, pasteResponse{Entries: entries, Warnings: warnings})
}

func (s *Server) handlePasteExport(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxPasteBytes))
	if err != nil {
		respond(w, http.StatusBadRequest, map[string]string{"error": "paste too large or unreadable"})
		return
	}
	entries, _, err := s.pasteRows(r.Context(), string(body))
	if errors.Is(err, lookup.ErrNoCompendium) {
		respond(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if err != nil {
		respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	rows := make([]export.Row, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, export.Row{Entry: e.Entry, Res: e.Result})
	}
	xlsx, err := export.Build(rows)
	if err != nil {
		respond(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="partstable-list.xlsx"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(xlsx)
}

func (s *Server) handleMore(w http.ResponseWriter, _ *http.Request) {
	out := map[string]any{"url": MoreURL, "opened": false}
	if s.openURL != nil {
		if err := s.openURL(MoreURL); err == nil {
			out["opened"] = true
		}
	}
	respond(w, http.StatusOK, out)
}

func respond(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
