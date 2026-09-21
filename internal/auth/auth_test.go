package auth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/partstableHQ/connector/internal/auth/fakeidp"
	"github.com/zalando/go-keyring"
)

// httpGet drives the fake browser: it follows the authorize redirect to
// the loopback callback exactly like a real browser would.
func httpGet(u string) (*http.Response, error) {
	return http.Get(u) // #nosec G107 -- URL is the local fake IdP's
}

// cfgOf converts the fake's endpoints into the app's config shape.
func cfgOf(f *fakeidp.Fake) Config {
	c := f.Config()
	return Config{AuthorizeURL: c.AuthorizeURL, TokenURL: c.TokenURL, ClientID: c.ClientID}
}

// The happy path: browser opens, authorize redirects with the code, the
// token exchange proves the PKCE binding, and the key comes back.
func TestPairHappyPath(t *testing.T) {
	idp := fakeidp.New()
	defer idp.Close()

	var opened string
	info, err := Pair(context.Background(), cfgOf(idp),
		func(u string) error { opened = u; go func() { _, _ = httpGet(u) }(); return nil },
		func(string) {})
	if err != nil {
		t.Fatalf("pair: %v", err)
	}
	if info.Email != "demo@partstable.com" || info.APIKey != "pt_test_key" {
		t.Fatalf("key info = %+v", info)
	}
	if !strings.Contains(opened, "/oauth/authorize") ||
		!strings.Contains(opened, "code_challenge_method=S256") ||
		!strings.Contains(opened, "client_id=connector-desktop") {
		t.Fatalf("authorize URL wrong: %q", opened)
	}
	if idp.VerifyCalls != 1 {
		t.Fatalf("token verify calls = %d, want 1", idp.VerifyCalls)
	}
	if idp.InvalidGrant != 0 {
		t.Fatalf("token exchange rejected the verifier — PKCE binding broken")
	}
}

func TestPairRejectsTamperedState(t *testing.T) {
	idp := fakeidp.New()
	defer idp.Close()
	idp.WrongState = true

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := Pair(ctx, cfgOf(idp),
		func(u string) error { go func() { _, _ = httpGet(u) }(); return nil }, nil)
	if err == nil {
		t.Fatal("tampered state must fail the flow")
	}
	if !strings.Contains(err.Error(), "could not be verified") {
		t.Fatalf("error must be actionable: %v", err)
	}
	if idp.VerifyCalls != 0 {
		t.Fatal("a tampered response must never reach the token exchange")
	}
}

func TestPairSurfacesTokenError(t *testing.T) {
	idp := fakeidp.New()
	defer idp.Close()
	idp.FailToken = true

	_, err := Pair(context.Background(), cfgOf(idp),
		func(u string) error { go func() { _, _ = httpGet(u) }(); return nil }, nil)
	if err == nil || !strings.Contains(err.Error(), "account locked") {
		t.Fatalf("error must surface the server's human message: %v", err)
	}
}

func TestPairTimesOut(t *testing.T) {
	idp := fakeidp.New()
	defer idp.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Pair(ctx, cfgOf(idp), func(string) error { return nil }, nil)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want timeout", err)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("timeout must not hang past its deadline")
	}
}

// The production store round-trips through the real OS keychain. Skipped
// where no keychain backend exists (headless Linux CI).
func TestKeyringStoreRoundTrip(t *testing.T) {
	if !keychainAvailable() {
		t.Skip("no OS keychain backend on this machine")
	}
	store := KeyringStore{}
	if _, found, _ := store.Load(); found {
		_ = store.Clear() // start clean
	}

	info := KeyInfo{Email: "roundtrip@partstable.com", APIKey: "pt_rt"}
	if err := store.Save(info); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, found, err := store.Load()
	if err != nil || !found {
		t.Fatalf("load: found=%v err=%v", found, err)
	}
	if got != info {
		t.Fatalf("round trip = %+v, want %+v", got, info)
	}
	if err := store.Clear(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, found, _ := store.Load(); found {
		t.Fatal("cleared key must be gone")
	}
}

func keychainAvailable() bool {
	_, err := keyring.Get(KeychainService, "availability-probe")
	if err == nil {
		return true
	}
	return errors.Is(err, keyring.ErrNotFound)
}

// Manager: login completes in the background and status follows.
func TestManagerLifecycle(t *testing.T) {
	idp := fakeidp.New()
	defer idp.Close()
	store := NewMemoryStore()
	m := NewManager(cfgOf(idp), store)
	m.openBrowser = func(u string) error { go func() { _, _ = httpGet(u) }(); return nil }

	if !m.Login() {
		t.Fatal("first login must start")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st := m.Status(); st.SignedIn {
			if st.Email != "demo@partstable.com" {
				t.Fatalf("email = %q", st.Email)
			}
			goto signedIn
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("login never completed")
signedIn:

	if err := m.Logout(); err != nil {
		t.Fatalf("logout: %v", err)
	}
	if st := m.Status(); st.SignedIn {
		t.Fatalf("still signed in after logout: %+v", st)
	}
}
