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
	PollURL      string
	ClientID     string
}

// Fake is a running fake identity service. It simulates the full flow:
// the browser GET of the authorize page auto-completes the pairing (as if
// the user had filled and submitted the form), and the poll endpoint then
// delivers the code to the app's backchannel.
type Fake struct {
	srv *httptest.Server

	mu           sync.Mutex
	pairings     map[string]string // pairing_id or code → code_challenge
	FailToken    bool              // token endpoint returns an error
	Email        string
	APIKey       string
	VerifyCalls  int
	InvalidGrant int
}

// New starts a fake identity service.
func New() *Fake {
	f := &Fake{
		pairings: map[string]string{},
		Email:    "demo@partstable.com",
		APIKey:   "pt_test_key",
	}
	f.srv = httptest.NewServer(http.HandlerFunc(f.handler))
	return f
}

// Close shuts the fake down.
func (f *Fake) Close() { f.srv.Close() }

// Config returns the fake's endpoint configuration.
func (f *Fake) Config() Config {
	return Config{
		AuthorizeURL: f.srv.URL + "/oauth/authorize",
		TokenURL:     f.srv.URL + "/oauth/token",
		PollURL:      f.srv.URL + "/oauth/poll",
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
		f.pairings[code] = q.Get("code_challenge")
		f.mu.Unlock()

		// Auto-complete the pairing: as if the user filled the form.
		if pid := q.Get("pairing_id"); pid != "" {
			f.mu.Lock()
			f.pairings[pid] = code
			f.mu.Unlock()
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body>signed in</body></html>"))

	case "/oauth/poll":
		id := r.URL.Query().Get("id")
		f.mu.Lock()
		code, ok := f.pairings[id]
		f.mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "pending"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "complete", "code": code})

	case "/oauth/token":
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"malformed request"}`))
			return
		}
		f.mu.Lock()
		f.VerifyCalls++
		fail := f.FailToken
		challenge := f.pairings[r.PostFormValue("code")]
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
