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
// account service: browser opens the form, signs up, the loopback
// callback fires, PKCE exchange completes, and the key lands where the
// app would store it. This is the sign-in experience, minus the human.
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
		ClientID:     "connector-desktop",
	}

	// The fake browser: open the authorize URL, submit the form with the
	// hidden binding plus credentials, then follow the redirect to the
	// app's loopback callback — everything a human does, minus the typing.
	// The recover guard keeps a late callback delivery from failing the
	// test after Pair has already returned and closed its listener.
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

// driveBrowser walks the sign-in like a user: GET the authorize page,
// carry its hidden fields plus credentials through the form, then follow
// the redirect to the app's loopback callback.
func driveBrowser(authorizeURL, email, password string) {
	noFollow := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	resp, err := noFollow.Get(authorizeURL)
	if err != nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	u, err := url.Parse(authorizeURL)
	if err != nil {
		return
	}
	q := u.Query()
	vals := url.Values{
		"state":        {q.Get("state")},
		"challenge":    {q.Get("code_challenge")},
		"redirect_uri": {q.Get("redirect_uri")},
		"client_id":    {q.Get("client_id")},
		"email":        {email},
		"password":     {password},
	}
	resp, err = noFollow.PostForm(serviceFormAction(authorizeURL), vals)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if loc, lerr := resp.Location(); lerr == nil {
		// Follow to the app's loopback callback — this completes the Pair.
		r, err := http.Get(loc.String())
		if err == nil {
			_, _ = io.Copy(io.Discard, r.Body)
			_ = r.Body.Close()
		}
	}
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
