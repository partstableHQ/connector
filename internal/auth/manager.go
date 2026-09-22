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
	attempt     int
	lastError   string
	cancel      context.CancelFunc
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

// Login starts a browser sign-in and reports whether a fresh attempt is
// now running. If an attempt is already waiting — say the password was
// mistyped and the tab got closed — it is cancelled and a new one starts
// immediately: clicking Sign in must always act, never dead-end.
func (m *Manager) Login() bool {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel() // stop any in-flight attempt
		m.cancel = nil
	}
	m.signingIn = true
	m.lastError = ""
	m.attempt++
	attempt := m.attempt
	cfg, store, opener := m.cfg, m.store, m.openBrowser
	ctx, cancel := context.WithTimeout(context.Background(), PairTimeout)
	m.cancel = cancel
	m.mu.Unlock()

	go func() {
		info, err := Pair(ctx, cfg, opener, nil)
		if err == nil {
			err = store.Save(info)
		}
		cancel()
		m.mu.Lock()
		// Only the CURRENT attempt may report: a cancelled older goroutine
		// must not overwrite a newer attempt's state.
		if attempt == m.attempt {
			m.signingIn = false
			if err != nil {
				m.lastError = err.Error()
			}
		}
		m.mu.Unlock()
	}()
	return true
}

// Logout removes the stored credential from this machine and cancels any
// in-flight sign-in.
func (m *Manager) Logout() error {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.lastError = ""
	m.mu.Unlock()
	return m.store.Clear()
}
