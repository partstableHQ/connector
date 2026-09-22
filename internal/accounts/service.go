package accounts

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/url"
)

// Service serves the OAuth endpoints of FLOW.md over an http.Handler.
type Service struct {
	store    *Store
	codes    *CodeStore
	clientID string
}

// New builds the account service. store must be opened; clientID must
// match what the Connector sends (auth.ClientID).
func New(store *Store, clientID string) *Service {
	return &Service{store: store, codes: NewCodeStore(), clientID: clientID}
}

// Handler returns the root handler (mount under /oauth or serve at root —
// the routes below are absolute, so mount the handler at the site root).
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /oauth/authorize", s.handleAuthorize)
	mux.HandleFunc("POST /oauth/authorize", s.handleAuthorizeSubmit)
	mux.HandleFunc("POST /oauth/token", s.handleToken)
	// Health lives under /oauth/* so a site-root mount can never shadow
	// the main website's own routes in production.
	mux.HandleFunc("GET /oauth/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

// authorizeParams is the validated binding of an authorize request.
type authorizeParams struct {
	State       string
	Challenge   string
	RedirectURI string
	ClientID    string
}

// validateAuthorize checks the contract: our client, loopback redirect,
// S256 challenge, non-empty state. Returns a human error for the error page.
func validateAuthorize(q url.Values, clientID string) (authorizeParams, string) {
	bad := func(msg string) (authorizeParams, string) { return authorizeParams{}, msg }
	if q.Get("response_type") != "code" {
		return bad("unsupported response_type")
	}
	if q.Get("client_id") != clientID {
		return bad("unknown client")
	}
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		return bad("PKCE (S256) is required")
	}
	if q.Get("state") == "" {
		return bad("missing state")
	}
	ru, err := url.Parse(q.Get("redirect_uri"))
	if err != nil || ru.Scheme != "http" || ru.Path != "/callback" ||
		(ru.Hostname() != "127.0.0.1" && ru.Hostname() != "localhost") {
		// Loopback only (RFC 8252) — this is also the open-redirect guard.
		return bad("redirect_uri must be a loopback callback")
	}
	return authorizeParams{
		State:       q.Get("state"),
		Challenge:   q.Get("code_challenge"),
		RedirectURI: q.Get("redirect_uri"),
		ClientID:    q.Get("client_id"),
	}, ""
}

func (s *Service) handleAuthorize(w http.ResponseWriter, r *http.Request) {
	params, errMsg := validateAuthorize(r.URL.Query(), s.clientID)
	if errMsg != "" {
		errorPage(w, http.StatusBadRequest, errMsg)
		return
	}
	renderForm(w, formValues{
		State: params.State, Challenge: params.Challenge,
		RedirectURI: params.RedirectURI, ClientID: params.ClientID,
	}, "")
}

// formValues carries the pending authorize binding through the form.
type formValues struct {
	State       string
	Challenge   string
	RedirectURI string
	ClientID    string
	Email       string
}

func (s *Service) handleAuthorizeSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		errorPage(w, http.StatusBadRequest, "malformed sign-in")
		return
	}
	vals := formValues{
		State:       r.PostFormValue("state"),
		Challenge:   r.PostFormValue("challenge"),
		RedirectURI: r.PostFormValue("redirect_uri"),
		ClientID:    r.PostFormValue("client_id"),
		Email:       r.PostFormValue("email"),
	}
	q := url.Values{
		"response_type":         {"code"},
		"client_id":             {vals.ClientID},
		"redirect_uri":          {vals.RedirectURI},
		"state":                 {vals.State},
		"code_challenge":        {vals.Challenge},
		"code_challenge_method": {"S256"},
		"scope":                 {"api"},
	}
	params, errMsg := validateAuthorize(q, s.clientID)
	if errMsg != "" {
		errorPage(w, http.StatusBadRequest, errMsg)
		return
	}

	email, apiKey, created, err := s.store.SignInOrSignUp(r.Context(), vals.Email, r.PostFormValue("password"))
	if err != nil {
		msg := err.Error()
		if errors.Is(err, ErrWrongPassword) || errors.Is(err, ErrInvalidEmail) || errors.Is(err, ErrWeakPassword) {
			renderForm(w, vals, msg)
			return
		}
		errorPage(w, http.StatusInternalServerError, "sign-in is temporarily unavailable — try again shortly")
		return
	}
	_ = created

	// The code binds the account (normalized email + API key) to the
	// verified authorize request; the token exchange releases both.
	code := s.codes.Issue(email, apiKey, params.Challenge, params.RedirectURI, params.ClientID)
	dest := params.RedirectURI + "?code=" + url.QueryEscape(code) + "&state=" + url.QueryEscape(params.State)
	http.Redirect(w, r, dest, http.StatusFound)
}

func (s *Service) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		tokenError(w, http.StatusBadRequest, "malformed request")
		return
	}
	if r.PostFormValue("grant_type") != "authorization_code" {
		tokenError(w, http.StatusBadRequest, "unsupported grant_type")
		return
	}
	entry, err := s.codes.redeem(r.PostFormValue("code"))
	if err != nil {
		tokenError(w, http.StatusBadRequest, "authorization code is invalid or expired")
		return
	}
	if r.PostFormValue("client_id") != entry.clientID ||
		r.PostFormValue("redirect_uri") != entry.redirectURI {
		tokenError(w, http.StatusBadRequest, "request does not match the authorization")
		return
	}
	sum := sha256.Sum256([]byte(r.PostFormValue("code_verifier")))
	if base64.RawURLEncoding.EncodeToString(sum[:]) != entry.challenge {
		tokenError(w, http.StatusBadRequest, "authorization code verifier rejected")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"account_email": entry.email,
		"api_key":       entry.apiKey,
	})
}

func tokenError(w http.ResponseWriter, status int, msg string) {
	// account_email/api_key stay out of error bodies by contract.
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func errorPage(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = errorTmpl.Execute(w, msg)
}

var errorTmpl = template.Must(template.New("error").Parse(
	`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>PartsTable</title></head>
<body style="font-family:system-ui,sans-serif;background:#f1f5f9;color:#0f172a;display:grid;place-items:center;min-height:100vh;margin:0">
<div style="text-align:center"><h1>PartsTable</h1><p>{{.}}</p></div></body></html>`))

type formPage struct {
	Values formValues
	Error  string
}

var formTmpl = template.Must(template.New("authorize").Parse(
	`<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Sign in — PartsTable</title></head>
<body style="font-family:system-ui,sans-serif;background:#f1f5f9;color:#0f172a;display:grid;place-items:center;min-height:100vh;margin:0">
<div style="background:#fff;border:1px solid #e2e8f0;border-radius:12px;padding:2rem 2.5rem;min-width:320px">
<div style="display:flex;align-items:center;gap:.6rem;margin-bottom:1rem">
<span style="display:inline-grid;place-items:center;width:2.2rem;height:2.2rem;border-radius:.5rem;background:#0055dd;color:#fff;font-weight:700;font-size:.9rem">PT</span>
<strong>PartsTable</strong> <span style="color:#64748b">Connector — free account, no card</span>
</div>
{{if .Error}}<p style="color:#b91c1c;background:#fef2f2;border:1px solid #fecaca;border-radius:8px;padding:.5rem .75rem">{{.Error}}</p>{{end}}
<form method="post" action="/oauth/authorize">
<input type="hidden" name="state" value="{{.Values.State}}">
<input type="hidden" name="challenge" value="{{.Values.Challenge}}">
<input type="hidden" name="redirect_uri" value="{{.Values.RedirectURI}}">
<input type="hidden" name="client_id" value="{{.Values.ClientID}}">
<p><input style="width:100%;padding:.6rem;border:1px solid #cbd5e1;border-radius:8px;box-sizing:border-box" type="email" name="email" placeholder="you@company.com" required autofocus value="{{.Values.Email}}"></p>
<p><input style="width:100%;padding:.6rem;border:1px solid #cbd5e1;border-radius:8px;box-sizing:border-box" type="password" name="password" placeholder="Password (8+ characters)" required minlength="8"></p>
<p><button style="width:100%;padding:.6rem;background:#0055dd;color:#fff;border:none;border-radius:8px;font-weight:600;cursor:pointer" type="submit">Sign in or create your free account</button></p>
<p style="color:#64748b;font-size:.8rem">New here? This one form creates your account. PartsTable Connector never sees your password — only this page does.</p>
</form>
</div></body></html>`))

func renderForm(w http.ResponseWriter, vals formValues, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	status := http.StatusOK
	if errMsg != "" {
		status = http.StatusUnauthorized
	}
	w.WriteHeader(status)
	_ = formTmpl.Execute(w, formPage{Values: vals, Error: errMsg})
}
