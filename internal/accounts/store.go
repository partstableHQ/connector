// Package accounts implements the PartsTable account service described in
// internal/auth/FLOW.md: free email+password accounts (no card), signing
// the Connector's OAuth+PKCE pairing, and issuing the API key the app
// stores in its OS keychain. Runs behind TLS (Cloudflare + Caddy in
// production); this service itself binds loopback and speaks plain HTTP.
package accounts

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver (pure Go)
)

// Password and identifier rules.
const (
	MinPasswordLen = 8
	APIKeyPrefix   = "pt_"
)

// ErrWrongPassword reports an existing account with a mismatched password.
var ErrWrongPassword = errors.New("wrong password for this email")

// ErrInvalidEmail reports an email that failed basic validation.
var ErrInvalidEmail = errors.New("that doesn't look like an email address")

// ErrWeakPassword reports a password below the minimum length.
var ErrWeakPassword = errors.New("password must be at least 8 characters")

// Store owns the accounts database.
type Store struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS accounts (
	email      TEXT PRIMARY KEY,
	password   TEXT NOT NULL,
	api_key    TEXT NOT NULL UNIQUE,
	created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now'))
);`

// Open opens (creating if needed) the accounts database at path.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("accounts: open %s: %w", path, err)
	}
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("accounts: schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// SignInOrSignUp authenticates an existing account or creates a new one —
// the single free-account flow: one form, no card. Returns the normalized
// account email, the account's API key, and whether the account was just
// created.
func (s *Store) SignInOrSignUp(ctx context.Context, email, password string) (normalizedEmail, apiKey string, created bool, err error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !validEmail(email) {
		return "", "", false, ErrInvalidEmail
	}
	if len(password) < MinPasswordLen {
		return "", "", false, ErrWeakPassword
	}

	var hash string
	err = s.db.QueryRowContext(ctx,
		`SELECT password, api_key FROM accounts WHERE email = ?`, email).Scan(&hash, &apiKey)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		apiKey, err = newAPIKey()
		if err != nil {
			return "", "", false, err
		}
		hashBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return "", "", false, fmt.Errorf("accounts: hash: %w", err)
		}
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO accounts (email, password, api_key) VALUES (?, ?, ?)`,
			email, string(hashBytes), apiKey); err != nil {
			return "", "", false, fmt.Errorf("accounts: create: %w", err)
		}
		return email, apiKey, true, nil
	case err != nil:
		return "", "", false, fmt.Errorf("accounts: lookup: %w", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", "", false, ErrWrongPassword
	}
	return email, apiKey, false, nil
}

func validEmail(email string) bool {
	if _, err := mail.ParseAddress(email); err != nil {
		return false
	}
	return strings.Contains(email, "@") && !strings.ContainsAny(email, " \t")
}

// newAPIKey returns a fresh key: pt_ + 32 random bytes, base64url.
func newAPIKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("accounts: key: %w", err)
	}
	return APIKeyPrefix + base64.RawURLEncoding.EncodeToString(b), nil
}

// CodeStore holds pending authorization codes: single-use, short-lived.
// In-memory by design — a restart invalidates pending pairings, which is
// the safe failure direction for an identity flow.
type CodeStore struct {
	mu    sync.Mutex
	codes map[string]codeEntry
}

type codeEntry struct {
	email       string
	apiKey      string
	challenge   string
	redirectURI string
	clientID    string
	expires     time.Time
}

// CodeTTL is how long an issued authorization code remains valid.
const CodeTTL = 10 * time.Minute

// NewCodeStore returns an empty in-memory code store.
func NewCodeStore() *CodeStore {
	return &CodeStore{codes: map[string]codeEntry{}}
}

// Issue mints a single-use authorization code bound to the authenticated
// account and the verified request parameters.
func (c *CodeStore) Issue(email, apiKey, challenge, redirectURI, clientID string) string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Crypto failure must never yield an empty/guessable code.
		panic(fmt.Sprintf("accounts: code entropy: %v", err))
	}
	code := base64.RawURLEncoding.EncodeToString(b)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.codes[code] = codeEntry{
		email: email, apiKey: apiKey, challenge: challenge,
		redirectURI: redirectURI, clientID: clientID,
		expires: time.Now().Add(CodeTTL),
	}
	return code
}

// ErrInvalidCode reports an unknown, expired, or already-used code.
var ErrInvalidCode = errors.New("invalid or expired authorization code")

// redeem consumes a code (single-use) and returns its binding.
func (c *CodeStore) redeem(code string) (codeEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.codes[code]
	if !ok {
		return codeEntry{}, ErrInvalidCode
	}
	delete(c.codes, code)
	if time.Now().After(entry.expires) {
		return codeEntry{}, ErrInvalidCode
	}
	return entry, nil
}
