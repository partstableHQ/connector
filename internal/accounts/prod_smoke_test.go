//go:build prodsmoke

package accounts

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/partstableHQ/connector/internal/auth"
)

// TestProdSignInSmoke runs a REAL pairing against the PRODUCTION account
// service — the same flow as clicking Sign in in the app. Opt-in only:
//
//	PROD_EMAIL=you@partstable.com PROD_PASSWORD=... \
//	  go test -tags prodsmoke -run TestProdSignInSmoke ./internal/accounts/ -v
//
// Creates the account on first run (sign-in-or-create) and asserts the
// API key contract. Run after any accountd deployment.
func TestProdSignInSmoke(t *testing.T) {
	base := os.Getenv("PROD_BASE")
	if base == "" {
		base = "https://partstable.com"
	}
	email := os.Getenv("PROD_EMAIL")
	password := os.Getenv("PROD_PASSWORD")
	if email == "" || password == "" {
		t.Skip("PROD_EMAIL / PROD_PASSWORD not set — production smoke skipped")
	}

	cfg := auth.Config{
		AuthorizeURL: base + "/oauth/authorize",
		TokenURL:     base + "/oauth/token",
		ClientID:     "connector-desktop",
	}
	openBrowser := func(u string) error {
		go func() {
			defer func() { _ = recover() }()
			driveBrowser(u, email, password)
		}()
		return nil
	}

	info, err := auth.Pair(context.Background(), cfg, openBrowser, nil)
	if err != nil {
		t.Fatalf("prod pair: %v", err)
	}
	if !strings.EqualFold(info.Email, email) {
		t.Fatalf("email = %q, want %q", info.Email, email)
	}
	if !strings.HasPrefix(info.APIKey, "pt_") {
		t.Fatalf("api key = %q", info.APIKey)
	}
	t.Logf("PRODUCTION SIGN-IN OK: %s has key %s…", info.Email, info.APIKey[:8])
}
