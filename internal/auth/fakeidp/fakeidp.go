// Package fakeidp provides an in-process fake PartsTable identity service
// implementing the contract in internal/auth/FLOW.md — used by tests and
// by local end-to-end runs (PARTSTABLE_AUTH_BASEURL).
package fakeidp

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
)

// Config carries the fake's endpoint URLs (structurally the fields the
// app's auth.Config needs; defined here to avoid an import cycle).
type Config struct {
	AuthorizeURL string
	TokenURL     string
	ClientID     string
}

// Fake is a running fake identity service. It verifies the PKCE binding
// for real: a token request whose S256(code_verifier) does not match the
// challenge recorded at authorize time is rejected with invalid_grant.
type Fake struct {
	srv *httptest.Server

	mu           sync.Mutex
	challenges   map[string]string // issued code → code_challenge
	FailToken    bool              // token endpoint returns an error
	WrongState   bool              // redirects with a mismatched state
	Email        string
	APIKey       string
	VerifyCalls  int
	InvalidGrant int
}

// New starts a fake identity service.
func New() *Fake {
	f := &Fake{
		challenges: map[string]string{},
		Email:      "demo@partstable.com",
		APIKey:     "pt_test_key",
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handler))
	return f
}

// Close shuts the fake down.
func (f *Fake) Close() { f.srv.Close() }

// URL returns the fake's base URL.
func (f *Fake) URL() string { return f.srv.URL }

// Config returns the fake's endpoint configuration.
func (f *Fake) Config() Config {
	return Config{
		AuthorizeURL: f.srv.URL + "/oauth/authorize",
		TokenURL:     f.srv.URL + "/oauth/token",
		ClientID:     "connector-desktop",
	}
}

func (f *Fake) handler(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/oauth/authorize":
		q := r.URL.Query()
		if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
			http.Error(w, "pkce required", http.StatusBadRequest)
			return
		}
		code := "test-code"
		f.mu.Lock()
		f.challenges[code] = q.Get("code_challenge")
		f.mu.Unlock()

		state := q.Get("state")
		f.mu.Lock()
		if f.WrongState {
			state = "tampered"
		}
		f.mu.Unlock()
		// Redirecting to the caller-supplied loopback redirect_uri is this
		// fake's entire purpose; it is a test/dev-only server.
		http.Redirect(w, r, q.Get("redirect_uri")+"?code="+code+"&state="+state, http.StatusFound) // #nosec G710

	case "/oauth/token":
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"malformed request"}`))
			return
		}
		f.mu.Lock()
		f.VerifyCalls++
		fail := f.FailToken
		challenge := f.challenges[r.PostFormValue("code")]
		f.mu.Unlock()

		sum := sha256.Sum256([]byte(r.PostFormValue("code_verifier")))
		if base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
			f.mu.Lock()
			f.InvalidGrant++
			f.mu.Unlock()
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"authorization code verifier rejected"}`))
			return
		}
		if fail {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"account locked"}`))
			return
		}

		f.mu.Lock()
		email, key := f.Email, f.APIKey
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"account_email": email,
			"api_key":       key,
		})
	default:
		http.NotFound(w, r)
	}
}
