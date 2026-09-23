package accounts

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/partstableHQ/connector/internal/auth"
)

// TestConnectorPairEndToEnd runs the REAL Connector pairing (auth.Pair —
// the exact code path behind the app's Sign-in button) against the real
// account service: the sign-in window loads the form, the user submits
// credentials, the pairing completes via poll (no loopback anywhere), the
// PKCE exchange runs, and the key lands where the app would store it.
func TestConnectorPairEndToEnd(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "accounts.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ts := httptest.NewServer(New(store, "connector-desktop").Handler())
	t.Cleanup(ts.Close)

	cfg := auth.Config{
		AuthorizeURL: ts.URL + "/oauth/authorize",
		TokenURL:     ts.URL + "/oauth/token",
		PollURL:      ts.URL + "/oauth/poll",
		ClientID:     "connector-desktop",
	}

	// The fake browser: open the authorize URL, submit the form with the
	// hidden binding plus credentials. The pairing completes server-side;
	// Pair's polling picks it up.
	openBrowser := func(u string) error {
		go func() {
			defer func() { _ = recover() }()
			driveBrowser(u, "ceo-beta@partstable.com", "internal-beta-passphrase")
		}()
		return nil
	}

	info, err := auth.Pair(context.Background(), cfg, openBrowser, nil)
	if err != nil {
		t.Fatalf("pair: %v", err)
	}
	if info.Email != "ceo-beta@partstable.com" {
		t.Fatalf("email = %q", info.Email)
	}
	if !strings.HasPrefix(info.APIKey, "pt_") {
		t.Fatalf("api key = %q", info.APIKey)
	}

	// Signing in AGAIN with the same credentials must keep working (the
	// account exists now) and return the SAME api key.
	second, err := auth.Pair(context.Background(), cfg, openBrowser, nil)
	if err != nil {
		t.Fatalf("second pair: %v", err)
	}
	if second.APIKey != info.APIKey {
		t.Fatalf("api key rotated between sign-ins: %q vs %q", second.APIKey, info.APIKey)
	}
}

// driveBrowser walks the sign-in like a user: GET the authorize page to
// get the form with its hidden binding, then submit credentials. The
// pairing completes server-side — no redirect, no loopback.
func driveBrowser(authorizeURL, email, password string) {
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := noFollow.Get(authorizeURL)
	if err != nil {
		return
	}
	form, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	// Carry the hidden fields through.
	vals := url.Values{}
	for _, name := range []string{"state", "challenge", "redirect_uri", "client_id", "pairing_id"} {
		marker := `name="` + name + `" value="`
		i := strings.Index(string(form), marker)
		if i < 0 {
			continue
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

	resp, err = noFollow.PostForm(serviceFormAction(authorizeURL), vals)
	if err != nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}

// serviceFormAction maps an authorize URL to the form's POST action.
func serviceFormAction(authorizeURL string) string {
	u, err := url.Parse(authorizeURL)
	if err != nil {
		return authorizeURL
	}
	u.Path = "/oauth/authorize"
	u.RawQuery = ""
	return u.String()
}
