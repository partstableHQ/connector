package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/partstableHQ/connector/internal/auth"
	"github.com/partstableHQ/connector/internal/compendium"
	"github.com/partstableHQ/connector/internal/lookup"
)

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	db, err := compendium.Create(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatalf("create compendium: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, stmt := range []string{
		`INSERT INTO meta (key, value) VALUES ('compendium_schema','1'),
		 ('generated_at','2026-09-20T12:00:00Z'), ('source_rev','tds-test'), ('generator','test/1')`,
		`INSERT INTO parts (pn, display_pn, description, category)
		 VALUES ('02CL197', '02cl197', 'LP ECC UDIMM 32GB DDR4-3200', 'memory')`,
		`INSERT INTO part_aliases (alias, pn) VALUES ('2CL197', '02CL197')`,
		`INSERT INTO xrefs (from_pn, to_pn, kind, source, source_detail)
		 VALUES ('02CL197', '32GBDDR43200ECC', 'substitute', 'broker_verified', 'broker lot #812')`,
		`INSERT INTO holders (pn, holder, qty, condition, last_seen, source, source_detail)
		 VALUES ('02CL197', 'Test Broker NL', 4, 'refurb', '2026-09-01', 'partner', 'feed sync')`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	ts := httptest.NewServer(New(lookup.New(db), "test-version", DefaultPort, auth.NewManager(auth.Config{}, auth.NewMemoryStore())).Handler())
	t.Cleanup(ts.Close)
	return ts
}

func getJSON(t *testing.T, url string) (int, map[string]any) {
	t.Helper()
	r, err := http.Get(url) // #nosec G107 -- httptest URL built by the test
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = r.Body.Close() }()
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return r.StatusCode, body
}

func TestHealthEndpoint(t *testing.T) {
	ts := testServer(t)
	code, body := getJSON(t, ts.URL+"/health")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if body["app_version"] != "test-version" {
		t.Fatalf("app_version = %v", body["app_version"])
	}
	comp, ok := body["compendium"].(map[string]any)
	if !ok || comp["schema"].(float64) != 1 || comp["part_count"].(float64) != 1 {
		t.Fatalf("compendium health wrong: %v", body["compendium"])
	}
}

func TestLookupEndpoint(t *testing.T) {
	ts := testServer(t)

	code, body := getJSON(t, ts.URL+"/lookup?pn=2c-l197")
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if body["matched_by"] != "alias" {
		t.Fatalf("matched_by = %v", body["matched_by"])
	}
	if body["query"] != "2c-l197" || body["normalized"] != "2CL197" {
		t.Fatalf("verbatim/normalized wrong: %v / %v", body["query"], body["normalized"])
	}
	part := body["part"].(map[string]any)
	if part["pn"] != "02CL197" {
		t.Fatalf("part pn = %v", part["pn"])
	}
	if len(part["xrefs"].([]any)) != 1 || len(part["holders"].([]any)) != 1 {
		t.Fatalf("cited facts missing: %v", part)
	}

	code, body = getJSON(t, ts.URL+"/lookup?pn=NOPE123")
	if code != http.StatusOK || body["matched_by"] != "" || body["part"] != nil {
		t.Fatalf("not-found must be a 200 empty answer: %d %v", code, body)
	}

	code, _ = getJSON(t, ts.URL+"/lookup")
	if code != http.StatusBadRequest {
		t.Fatalf("missing pn must 400, got %d", code)
	}
}

func TestXrefEndpoint(t *testing.T) {
	ts := testServer(t)
	code, body := getJSON(t, ts.URL+"/xref?pn=02CL197")
	if code != http.StatusOK || body["matched_by"] != "exact" {
		t.Fatalf("xref endpoint wrong: %d %v", code, body)
	}
	if len(body["part"].(map[string]any)["xrefs"].([]any)) != 1 {
		t.Fatalf("xrefs missing: %v", body)
	}
}

func TestBulkEndpoint(t *testing.T) {
	ts := testServer(t)

	req := bulkRequest{PNs: []string{"02CL197", "2CL197", "MISSING1"}}
	b, _ := json.Marshal(req)
	resp, err := http.Post(ts.URL+"/bulk", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	results := body["results"].([]any)
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	if results[0].(map[string]any)["matched_by"] != "exact" {
		t.Fatalf("first result wrong: %v", results[0])
	}
	if results[2].(map[string]any)["part"] != nil {
		t.Fatalf("missing part must be nil: %v", results[2])
	}

	// Over the cap: refused, never silently truncated.
	huge := bulkRequest{PNs: make([]string, MaxBulkParts+1)}
	hugeBody, _ := json.Marshal(huge)
	resp, err = http.Post(ts.URL+"/bulk", "application/json", bytes.NewReader(hugeBody))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("over-cap bulk must 400, got %d", resp.StatusCode)
	}
}

func TestCORSPreflight(t *testing.T) {
	ts := testServer(t)
	req, _ := http.NewRequest(http.MethodOptions, ts.URL+"/lookup", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight must 204, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got == "" {
		t.Fatal("CORS origin header missing — the desktop webview could not call the API")
	}
}

func TestPasteEndpoint(t *testing.T) {
	ts := testServer(t)

	paste := "02CL197 x4\ngarbage line!!\n02cl197, 2"
	resp, err := http.Post(ts.URL+"/paste", "text/plain", strings.NewReader(paste))
	if err != nil {
		t.Fatal(err)
	}
	var body pasteResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}

	// Identical identities aggregate into one entry of qty 6; the alias
	// form 2CL197 would stay a distinct identity by design.
	if len(body.Entries) != 1 || body.Entries[0].Qty != 6 {
		t.Fatalf("entries = %+v", body.Entries)
	}
	if body.Entries[0].PN != "02CL197" || body.Entries[0].Norm != "02CL197" {
		t.Fatalf("verbatim identity wrong: %+v", body.Entries[0].Entry)
	}
	if body.Entries[0].Result == nil || body.Entries[0].Result.Part == nil {
		t.Fatalf("lookup result missing: %+v", body.Entries[0].Result)
	}
	if len(body.Entries[0].Result.Part.Xrefs) != 1 {
		t.Fatalf("cited xrefs missing")
	}

	// The garbage line must surface as a warning, never vanish.
	if len(body.Warnings) != 1 || body.Warnings[0].Line != 2 || body.Warnings[0].Raw != "garbage line!!" {
		t.Fatalf("warnings = %+v", body.Warnings)
	}
}

func TestPasteExportEndpoint(t *testing.T) {
	ts := testServer(t)

	resp, err := http.Post(ts.URL+"/paste/export", "text/plain", strings.NewReader("02CL197 x4"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "spreadsheetml") {
		t.Fatalf("content type = %q", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Fatalf("disposition = %q", cd)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, resp.Body); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("PK")) || buf.Len() < 1000 {
		t.Fatalf("body is not a plausible xlsx (%d bytes)", buf.Len())
	}
}

func TestPasteWithoutCompendium(t *testing.T) {
	ts := httptest.NewServer(New(lookup.New(nil), "test-version", DefaultPort, auth.NewManager(auth.Config{}, auth.NewMemoryStore())).Handler())
	t.Cleanup(ts.Close)

	resp, err := http.Post(ts.URL+"/paste", "text/plain", strings.NewReader("02CL197"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", resp.StatusCode)
	}
}

func TestAuthEndpoints(t *testing.T) {
	mgr := auth.NewManager(auth.Config{}, auth.NewMemoryStore())
	ts := httptest.NewServer(New(lookup.New(nil), "test-version", DefaultPort, mgr).Handler())
	t.Cleanup(ts.Close)

	// Unsigned by default; the health payload carries the auth state.
	code, body := getJSON(t, ts.URL+"/health")
	if code != http.StatusOK {
		t.Fatalf("health status %d", code)
	}
	authState, ok := body["auth"].(map[string]any)
	if !ok || authState["signed_in"] != false {
		t.Fatalf("health auth wrong: %v", body["auth"])
	}

	// Sign out without a stored key: a clean no-op.
	resp, err := http.Post(ts.URL+"/auth/logout", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout status %d", resp.StatusCode)
	}

	// Login starts the background pairing flow; the endpoint's contract is
	// the 202 + started, not the flow's completion.
	resp, err = http.Post(ts.URL+"/auth/login", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	var started map[string]bool
	if err := json.NewDecoder(resp.Body).Decode(&started); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted || !started["started"] {
		t.Fatalf("login start = %d %v", resp.StatusCode, started)
	}
}

func TestPortResolution(t *testing.T) {
	t.Setenv(EnvPort, "")
	p, err := Port()
	if err != nil || p != DefaultPort {
		t.Fatalf("default port = %d, %v", p, err)
	}
	t.Setenv(EnvPort, "8099")
	p, err = Port()
	if err != nil || p != 8099 {
		t.Fatalf("override port = %d, %v", p, err)
	}
	t.Setenv(EnvPort, "not-a-port")
	if _, err := Port(); err == nil || !strings.Contains(err.Error(), "not a valid port") {
		t.Fatalf("invalid port must error clearly, got %v", err)
	}
	_ = os.Unsetenv(EnvPort)
}
