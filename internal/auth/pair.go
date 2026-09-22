package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
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

// callbackResult carries the outcome of the loopback redirect.
type callbackResult struct {
	code string
	err  error
}

// Pair runs the sign-in flow: open the system browser at the authorize
// URL, receive the loopback redirect, exchange the code (with the PKCE
// verifier) for the account key. progress, when non-nil, receives short
// human-readable status lines. The protocol is specified in FLOW.md.
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
	challenge := pkceChallenge(verifier)

	// Loopback redirect listener (RFC 8252): a random port on 127.0.0.1,
	// announced to the server in redirect_uri.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return KeyInfo{}, fmt.Errorf("auth: local callback listener: %w", err)
	}
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", listener.Addr().(*net.TCPAddr).Port)

	result := make(chan callbackResult, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if e := q.Get("error"); e != "" {
			msg := "sign-in was not completed"
			switch e {
			case "access_denied":
				msg = "sign-in was canceled"
			case "temporarily_unavailable":
				msg = "the sign-in service is temporarily unavailable — try again shortly"
			}
			http.Error(w, msg, http.StatusBadRequest)
			result <- callbackResult{err: errors.New(msg)}
			return
		}
		if q.Get("state") != state {
			// Never surface the code from a response we did not start.
			http.Error(w, "sign-in could not be verified — please try again", http.StatusBadRequest)
			result <- callbackResult{err: errors.New("sign-in could not be verified (state mismatch) — please try again")}
			return
		}
		code := q.Get("code")
		if code == "" {
			http.Error(w, "sign-in did not complete — please try again", http.StatusBadRequest)
			result <- callbackResult{err: errors.New("sign-in did not complete — please try again")}
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, callbackPage)
		result <- callbackResult{code: code}
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	serving := make(chan struct{})
	go func() {
		close(serving)
		_ = server.Serve(listener)
	}()
	<-serving
	defer func() { _ = server.Close() }()

	say("Checking the account service…")
	if err := preflight(ctx, cfg.AuthorizeURL); err != nil {
		return KeyInfo{}, err
	}

	authURL := fmt.Sprintf("%s?response_type=code&client_id=%s&redirect_uri=%s&state=%s&code_challenge=%s&code_challenge_method=S256&scope=api",
		cfg.AuthorizeURL,
		url.QueryEscape(cfg.ClientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(state),
		url.QueryEscape(challenge),
	)
	say("Opening your browser to sign in — finish there and come back here.")
	if openBrowser == nil {
		openBrowser = defaultBrowserOpener
	}
	if err := openBrowser(authURL); err != nil {
		return KeyInfo{}, ErrNoBrowser
	}

	var code string
	select {
	case cr := <-result:
		if cr.err != nil {
			return KeyInfo{}, cr.err
		}
		code = cr.code
	case <-ctx.Done():
		return KeyInfo{}, errors.New("sign-in timed out — nothing was changed; try again when ready")
	}

	say("Finishing sign-in…")
	info, err := exchange(ctx, cfg, code, redirectURI, verifier)
	if err != nil {
		return KeyInfo{}, err
	}
	say(fmt.Sprintf("Signed in as %s.", info.Email))
	return info, nil
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

const callbackPage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>PartsTable Connector</title></head>
<body style="font-family:system-ui,sans-serif;background:#f1f5f9;color:#0f172a;
display:grid;place-items:center;min-height:100vh;margin:0">
<div style="text-align:center">
<h1>Sign-in complete</h1>
<p>You can close this window and return to the PartsTable Connector.</p>
</div></body></html>`

// pkceChallenge derives the S256 challenge for a verifier.
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
