// Package auth pairs the desktop app with a free PartsTable account
// (FM-3): an authorization-code + PKCE flow through the system browser,
// with the API key stored in the OS keychain — never in plain files
// (SECURITY.md). The wire protocol is specified in FLOW.md next to this
// file; the server side is implemented against that contract.
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
)

// KeychainService is the OS keychain service name for all stored secrets.
const KeychainService = "partstable-connector"

// The secret lives under one fixed account; the email rides inside the
// encoded payload so Load never needs to guess account names.
const keychainAccount = "api"

// Identity endpoints. PARTSTABLE_AUTH_BASEURL overrides both for
// development and staging.
const (
	DefaultAuthorizeURL = "https://partstable.com/oauth/authorize"
	DefaultTokenURL     = "https://partstable.com/oauth/token" // #nosec G101 -- endpoint URL, not a credential
	ClientID            = "connector-desktop"
	EnvBaseURL          = "PARTSTABLE_AUTH_BASEURL"
)

// KeyInfo is the outcome of a successful pairing.
type KeyInfo struct {
	Email  string `json:"email"`
	APIKey string `json:"api_key"`
}

// Store persists the account key. The production store is the OS keychain
// (Windows Credential Manager / Keychain / libsecret).
type Store interface {
	Save(info KeyInfo) error
	Load() (KeyInfo, bool, error)
	Clear() error
}

// Config carries the identity endpoints.
type Config struct {
	AuthorizeURL string
	TokenURL     string
	ClientID     string
}

// LoadConfig resolves the endpoint configuration: environment override
// first, production defaults otherwise.
func LoadConfig() Config {
	if base := strings.TrimRight(os.Getenv(EnvBaseURL), "/"); base != "" {
		return Config{
			AuthorizeURL: base + "/oauth/authorize",
			TokenURL:     base + "/oauth/token",
			ClientID:     ClientID,
		}
	}
	return Config{
		AuthorizeURL: DefaultAuthorizeURL,
		TokenURL:     DefaultTokenURL,
		ClientID:     ClientID,
	}
}

// KeyringStore is the production Store backed by the OS keychain.
type KeyringStore struct{}

// NewKeyringStore returns the production keychain store.
func NewKeyringStore() Store { return KeyringStore{} }

// Save encodes the key info into the OS keychain. Marshaling the API key
// is the point — its destination is the OS keychain, never a plain file.
func (KeyringStore) Save(info KeyInfo) error {
	b, err := json.Marshal(info) // #nosec G117 -- stored to the OS keychain by design
	if err != nil {
		return fmt.Errorf("auth: encode credential: %w", err)
	}
	if err := keyring.Set(KeychainService, keychainAccount, string(b)); err != nil {
		return fmt.Errorf("auth: keychain write failed: %w", err)
	}
	return nil
}

// Load reads the key info. found is false when nothing is stored.
func (KeyringStore) Load() (KeyInfo, bool, error) {
	s, err := keyring.Get(KeychainService, keychainAccount)
	if errors.Is(err, keyring.ErrNotFound) {
		return KeyInfo{}, false, nil
	}
	if err != nil {
		return KeyInfo{}, false, fmt.Errorf("auth: keychain read failed: %w", err)
	}
	var info KeyInfo
	if err := json.Unmarshal([]byte(s), &info); err != nil || info.APIKey == "" {
		return KeyInfo{}, false, errors.New("auth: stored credential is unreadable — sign in again or run `partstable logout`")
	}
	return info, true, nil
}

// Clear removes the stored credential. Clearing an absent key is success.
func (KeyringStore) Clear() error {
	err := keyring.Delete(KeychainService, keychainAccount)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("auth: keychain delete failed: %w", err)
	}
	return nil
}

// MemoryStore is an in-memory Store for tests and offline development.
type MemoryStore struct {
	mu   sync.Mutex
	info KeyInfo
	has  bool
}

// NewMemoryStore returns an empty in-memory store.
func NewMemoryStore() *MemoryStore { return &MemoryStore{} }

// Save stores the key info in memory.
func (m *MemoryStore) Save(info KeyInfo) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.info, m.has = info, true
	return nil
}

// Load returns the stored key info, if any.
func (m *MemoryStore) Load() (KeyInfo, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.info, m.has, nil
}

// Clear drops the stored key info.
func (m *MemoryStore) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.info, m.has = KeyInfo{}, false
	return nil
}

// randomSeed returns n random bytes as unpadded base64url.
func randomSeed(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
