package auth

import (
	"context"
	"sync"

	"github.com/pkg/browser"
)

// Status is the account state as shown in the UI and doctor.
type Status struct {
	SignedIn  bool   `json:"signed_in"`
	Email     string `json:"email,omitempty"`
	SigningIn bool   `json:"signing_in"`
	LastError string `json:"last_error,omitempty"`
}

// Manager owns the account state for the running app: status queries,
// browser sign-in launches, and sign-out. It is safe for concurrent use.
type Manager struct {
	mu          sync.Mutex
	signingIn   bool
	lastError   string
	cfg         Config
	store       Store
	openBrowser func(string) error
}

// NewManager builds the account manager. Pass nil for openBrowser to use
// the system browser.
func NewManager(cfg Config, store Store) *Manager {
	return &Manager{
		cfg:         cfg,
		store:       store,
		openBrowser: browser.OpenURL,
	}
}

// Status reports the current account state.
func (m *Manager) Status() Status {
	m.mu.Lock()
	signingIn, lastError := m.signingIn, m.lastError
	m.mu.Unlock()

	st := Status{SigningIn: signingIn, LastError: lastError}
	info, found, err := m.store.Load()
	if err != nil {
		st.LastError = err.Error()
		return st
	}
	if found {
		st.SignedIn = true
		st.Email = info.Email
	}
	return st
}

// Login starts a browser sign-in in the background and reports whether it
// was started (false when one is already in flight).
func (m *Manager) Login() bool {
	m.mu.Lock()
	if m.signingIn {
		m.mu.Unlock()
		return false
	}
	m.signingIn = true
	m.lastError = ""
	cfg, store, opener := m.cfg, m.store, m.openBrowser
	m.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), PairTimeout)
		defer cancel()
		info, err := Pair(ctx, cfg, opener, nil)
		if err == nil {
			err = store.Save(info)
		}
		m.mu.Lock()
		m.signingIn = false
		if err != nil {
			m.lastError = err.Error()
		}
		m.mu.Unlock()
	}()
	return true
}

// Logout removes the stored credential from this machine.
func (m *Manager) Logout() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastError = ""
	return m.store.Clear()
}
