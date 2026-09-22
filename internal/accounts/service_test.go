package accounts

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/partstableHQ/connector/internal/auth"
)

func newTestService(t *testing.T) (*httptest.Server, *Store) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "accounts.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	svc := New(store, auth.ClientID)
	ts := httptest.NewServer(svc.Handler())
	t.Cleanup(ts.Close)
	return ts, store
}

// testVerifier is the PKCE verifier a real client would send;
// testChallenge is its S256 challenge — the way auth.Pair builds them.
const testVerifier = "test-verifier-string"

func testChallenge() string {
	sum := sha256.Sum256([]byte(testVerifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// authorizeURL builds a contract-valid authorize request.
func authorizeURL(base string) string {
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {auth.ClientID},
		"redirect_uri":          {"http://127.0.0.1:61234/callback"},
		"state":                 {"st4te"},
		"code_challenge":        {testChallenge()},
		"code_challenge_method": {"S256"},
		"scope":                 {"api"},
	}
	return base + "/oauth/authorize?" + q.Encode()
}

func TestAuthorizeRendersForm(t *testing.T) {
	ts, _ := newTestService(t)
	resp, err := http.Get(authorizeURL(ts.URL)) // #nosec G107 -- httptest URL
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	for _, want := range []string{"Sign in", `name="state"`, `name="challenge"`, "st4te"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("form missing %q", want)
		}
	}
}

func TestAuthorizeRejectsBadRequests(t *testing.T) {
	ts, _ := newTestService(t)
	good := authorizeURL(ts.URL)

	// External redirect_uri (open-redirect guard).
	r, _ := http.Get(strings.Replace(good,
		"redirect_uri=http%3A%2F%2F127.0.0.1%3A61234%2Fcallback",
		"redirect_uri=https%3A%2F%2Fevil.example%2Fcallback", 1))
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("external redirect must 400, got %d", r.StatusCode)
	}
	_ = r.Body.Close()

	// Missing challenge (PKCE required).
	r, _ = http.Get(strings.Replace(good, "code_challenge=", "code_challenge_x=", 1))
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("missing challenge must 400, got %d", r.StatusCode)
	}
	_ = r.Body.Close()

	// Unknown client.
	r, _ = http.Get(strings.Replace(good, auth.ClientID, "someone-else", 1))
	if r.StatusCode != http.StatusBadRequest {
		t.Errorf("unknown client must 400, got %d", r.StatusCode)
	}
	_ = r.Body.Close()
}

// submit posts credentials and returns the redirect Location (if any).
func submit(t *testing.T, authorizeURL, email, password string) (int, *url.URL, string) {
	t.Helper()
	// Fetch the form to carry its hidden fields through (as a browser would).
	resp, err := http.Get(authorizeURL) // #nosec G107 -- httptest URL
	if err != nil {
		t.Fatal(err)
	}
	form, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	// Extract hidden values.
	vals := url.Values{}
	for _, name := range []string{"state", "challenge", "redirect_uri", "client_id"} {
		marker := `name="` + name + `" value="`
		i := strings.Index(string(form), marker)
		if i < 0 {
			t.Fatalf("form missing hidden field %s", name)
		}
		rest := string(form)[i+len(marker):]
		v := rest[:strings.Index(rest, `"`)]
		decoded, err := url.QueryUnescape(v)
		if err != nil {
			decoded = v
		}
		vals.Set(name, decoded)
	}
	vals.Set("email", email)
	vals.Set("password", password)

	// Stop at the 302: the callback redirect target is the app's loopback
	// listener, which does not exist in the test.
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err = client.PostForm(authorizeURL, vals)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	loc, lerr := resp.Location()
	if lerr != nil {
		loc = nil
	}
	return resp.StatusCode, loc, string(body)
}

func exchange(t *testing.T, tokenURL, code, verifier, redirectURI string) (int, map[string]any) {
	t.Helper()
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {auth.ClientID},
		"code_verifier": {verifier},
	}
	resp, err := http.PostForm(tokenURL, form) // #nosec G107 -- httptest URL
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.StatusCode, body
}

// The full pairing against the real service implementation: sign-up on
// the form, redirect with code+state, PKCE exchange, key returned.
func TestFullFlowSignUpAndExchange(t *testing.T) {
	ts, _ := newTestService(t)
	authorize := authorizeURL(ts.URL)

	status, loc, body := submit(t, authorize, "broker@company.com", "hunter22boogaloo")
	if status != http.StatusFound {
		t.Fatalf("submit status %d body %s", status, body)
	}
	if loc == nil || !strings.HasPrefix(loc.String(), "http://127.0.0.1:61234/callback") {
		t.Fatalf("location = %v", loc)
	}
	q := loc.Query()
	if q.Get("state") != "st4te" {
		t.Fatalf("state = %q", q.Get("state"))
	}
	code := q.Get("code")
	if code == "" {
		t.Fatal("no code in redirect")
	}

	codeStatus, tok := exchange(t, ts.URL+"/oauth/token", code, testVerifier, "http://127.0.0.1:61234/callback")
	if codeStatus != http.StatusOK {
		t.Fatalf("token status %d: %v", codeStatus, tok)
	}
	if tok["account_email"] != "broker@company.com" {
		t.Fatalf("email = %v", tok["account_email"])
	}
	key, _ := tok["api_key"].(string)
	if !strings.HasPrefix(key, "pt_") {
		t.Fatalf("api_key = %v", tok["api_key"])
	}

	// Single-use: the same code must now be rejected.
	codeStatus, _ = exchange(t, ts.URL+"/oauth/token", code, testVerifier, "http://127.0.0.1:61234/callback")
	if codeStatus != http.StatusBadRequest {
		t.Fatalf("reused code status %d, want 400", codeStatus)
	}
}

func TestSignInWrongPasswordThenRight(t *testing.T) {
	ts, _ := newTestService(t)
	authorize := authorizeURL(ts.URL)

	// Create the account.
	if status, _, _ := submit(t, authorize, "trader@corp.io", "longenough1"); status != http.StatusFound {
		t.Fatalf("signup status %d", status)
	}

	// Wrong password: re-rendered form with the message, no redirect.
	status, loc, body := submit(t, authorize, "trader@corp.io", "wrongpassword")
	if status != http.StatusUnauthorized || loc != nil {
		t.Fatalf("wrong password = %d %v", status, loc)
	}
	if !strings.Contains(body, "wrong password") {
		t.Fatalf("body missing wrong-password message: %s", body)
	}

	// Right password: redirects.
	if status, _, _ = submit(t, authorize, "trader@corp.io", "longenough1"); status != http.StatusFound {
		t.Fatalf("sign-in status %d", status)
	}
}

func TestTokenRejectsBadVerifier(t *testing.T) {
	ts, _ := newTestService(t)
	authorize := authorizeURL(ts.URL)
	if status, _, _ := submit(t, authorize, "v@corp.io", "longenough1"); status != http.StatusFound {
		t.Fatalf("submit %d", status)
	}
	// Recover the code by signing in again and reading the Location.
	status, loc, _ := submit(t, authorize, "v@corp.io", "longenough1")
	if status != http.StatusFound || loc == nil {
		t.Fatal("no redirect")
	}
	code := loc.Query().Get("code")

	codeStatus, _ := exchange(t, ts.URL+"/oauth/token", code, "wrong-verifier", "http://127.0.0.1:61234/callback")
	if codeStatus != http.StatusBadRequest {
		t.Fatalf("bad verifier status %d, want 400", codeStatus)
	}
}

func TestTokenRejectsMismatchedRedirect(t *testing.T) {
	ts, _ := newTestService(t)
	authorize := authorizeURL(ts.URL)
	if status, _, _ := submit(t, authorize, "m@corp.io", "longenough1"); status != http.StatusFound {
		t.Fatalf("submit %d", status)
	}
	status, loc, _ := submit(t, authorize, "m@corp.io", "longenough1")
	if status != http.StatusFound || loc == nil {
		t.Fatal("no redirect")
	}
	code := loc.Query().Get("code")
	verifier := testVerifier

	codeStatus, _ := exchange(t, ts.URL+"/oauth/token", code, verifier, "http://127.0.0.1:99999/callback")
	if codeStatus != http.StatusBadRequest {
		t.Fatalf("mismatched redirect status %d, want 400", codeStatus)
	}
}
