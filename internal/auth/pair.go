package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/pkg/browser"
)

// defaultBrowserOpener opens the system browser.
func defaultBrowserOpener(u string) error { return browser.OpenURL(u) }

// PairTimeout bounds one sign-in attempt: long enough for a slow browser
// login, short enough that a stuck flow does not linger.
const PairTimeout = 5 * time.Minute

// ErrNoBrowser is returned when no browser could be opened; the message
// tells the user what to do about it.
var ErrNoBrowser = errors.New("could not open a browser — sign in with `partstable login --api-key` instead")

// Pair runs the sign-in flow: open the sign-in page, wait for the user to
// finish there, collect the authorization code by polling the service
// (device-flow style — the pairing secret never reaches any web page),
// then exchange the code (with the PKCE verifier) for the account key.
// progress, when non-nil, receives short human-readable status lines.
// The wire protocol is specified in FLOW.md.
func Pair(ctx context.Context, cfg Config, openBrowser func(string) error, progress func(string)) (KeyInfo, error) {
	say := func(s string) {
		if progress != nil {
			progress(s)
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, PairTimeout)
	defer cancel()

	verifier, err := randomSeed(64) // 86 base64url chars, within RFC 7636 bounds
	if err != nil {
		return KeyInfo{}, fmt.Errorf("auth: %w", err)
	}
	state, err := randomSeed(16)
	if err != nil {
		return KeyInfo{}, fmt.Errorf("auth: %w", err)
	}
	pairingID, err := randomSeed(32)
	if err != nil {
		return KeyInfo{}, fmt.Errorf("auth: %w", err)
	}
	challenge := pkceChallenge(verifier)

	redirectURI := "http://127.0.0.1/callback"

	authURL := fmt.Sprintf("%s?response_type=code&client_id=%s&redirect_uri=%s&state=%s&code_challenge=%s&code_challenge_method=S256&scope=api&pairing_id=%s",
		cfg.AuthorizeURL,
		url.QueryEscape(cfg.ClientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(state),
		url.QueryEscape(challenge),
		url.QueryEscape(pairingID),
	)
	say("Checking the account service…")
	if err := preflight(ctx, cfg.AuthorizeURL); err != nil {
		return KeyInfo{}, err
	}

	say("Opening the sign-in window — finish signing in there.")
	if openBrowser == nil {
		openBrowser = defaultBrowserOpener
	}
	if err := openBrowser(authURL); err != nil {
		if errors.Is(err, ErrNoBrowser) {
			return KeyInfo{}, err
		}
		return KeyInfo{}, fmt.Errorf("could not open the sign-in page: %w", err)
	}

	say("Waiting for you to finish signing in…")

	// Poll for the pairing completion. The app holds the pairing secret;
	// the web page never sees it, so no browser behavior can break the
	// handoff (the failure mode that made in-app sign-in dead-end).
	var code string
	for {
		select {
		case <-ctx.Done():
			return KeyInfo{}, errors.New("sign-in timed out — nothing was changed; try again when ready")
		case <-time.After(pollInterval):
		}
		got, status, err := pollPairing(ctx, cfg.PollURL, pairingID)
		if err != nil {
			continue // transient poll failures must not kill the attempt
		}
		if status == "complete" {
			code = got
			break
		}
	}

	say("Finishing sign-in…")
	return exchange(ctx, cfg, code, redirectURI, verifier)
}

// pollInterval is the pairing poll cadence.
const pollInterval = 2 * time.Second

// pollPairing queries the service once for a pairing's completion.
func pollPairing(ctx context.Context, pollURL, pairingID string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		pollURL+"?id="+url.QueryEscape(pairingID), nil)
	if err != nil {
		return "", "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Status string `json:"status"`
		Code   string `json:"code"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&body); err != nil {
		return "", "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("poll failed (HTTP %d)", resp.StatusCode)
	}
	return body.Code, body.Status, nil
}

// preflight verifies the account service actually answers before the app
// opens a browser — a dead route must fail in one second with the truth,
// never strand a user on an error page while the app waits five minutes.
func preflight(ctx context.Context, authorizeURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authorizeURL, nil)
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse // any redirect means "someone is home"
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("the PartsTable account service is unreachable — check your connection and try again later")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 500 {
		// A reverse proxy answering 5xx means the route/deployment is not
		// live — e.g. a tunnel pointing at nothing.
		return errors.New("the PartsTable account service isn't live yet — sign-in arrives with the public release")
	}
	return nil
}

// exchange trades the authorization code (plus PKCE verifier) for the
// account key at the token endpoint.
func exchange(ctx context.Context, cfg Config, code, redirectURI, verifier string) (KeyInfo, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"client_id":     {cfg.ClientID},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return KeyInfo{}, fmt.Errorf("auth: token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return KeyInfo{}, errors.New("could not reach the sign-in service — check your connection and try again")
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != http.StatusOK {
		var errBody struct {
			Error string `json:"error"`
		}
		msg := fmt.Sprintf("sign-in failed (HTTP %d)", resp.StatusCode)
		if json.Unmarshal(body, &errBody) == nil && errBody.Error != "" {
			msg = "sign-in failed: " + errBody.Error
		}
		return KeyInfo{}, errors.New(msg)
	}

	var tok struct {
		AccountEmail string `json:"account_email"`
		APIKey       string `json:"api_key"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.APIKey == "" {
		return KeyInfo{}, errors.New("the sign-in service returned an unexpected response — try again")
	}
	return KeyInfo{Email: tok.AccountEmail, APIKey: tok.APIKey}, nil
}

// pkceChallenge derives the S256 challenge for a verifier.
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
