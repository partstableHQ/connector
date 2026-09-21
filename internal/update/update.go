// Package update implements the self-update channel (FM-11) and the
// anonymous update-check ping (FM-13). Releases come from GitHub
// (partstableHQ/connector) via go-selfupdate — checksum-verified assets,
// applied with a rename-then-replace so a running app keeps working and a
// restart finishes the update; the previous binary stays as .old for
// rollback. The ping is the app's ONLY telemetry (PRIVACY.md).
package update

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/creativeprojects/go-selfupdate"
	"github.com/partstableHQ/connector/internal/store"
	"github.com/partstableHQ/connector/internal/telemetry"
)

// Release repository. goreleaser publishes here on tags.
const (
	RepoOwner = "partstableHQ"
	RepoName  = "connector"
)

// Settings keys (app database).
const (
	SettingOptOut    = "telemetry_opt_out"
	SettingInstallID = "install_uuid"
)

// ErrUpToDate is returned by Apply when nothing newer exists.
var ErrUpToDate = errors.New("already on the latest version")

// CheckResult is the outcome of one update check.
type CheckResult struct {
	Current     string    `json:"current"`
	Latest      string    `json:"latest,omitempty"`
	Available   bool      `json:"available"`
	URL         string    `json:"url,omitempty"`
	Notes       string    `json:"notes,omitempty"`
	PingSent    bool      `json:"ping_sent"`
	PingSkipped string    `json:"ping_skipped,omitempty"`
	CheckedAt   time.Time `json:"checked_at"`
}

// Summary is the cached state the UI reads without doing network I/O.
type Summary struct {
	Current   string `json:"current"`
	Latest    string `json:"latest,omitempty"`
	Available bool   `json:"available"`
	OptedOut  bool   `json:"telemetry_opt_out"`
	CheckedAt time.Time
}

// Manager owns the update channel state.
type Manager struct {
	mu      sync.Mutex
	current string
	last    *Summary
	db      *sql.DB // app database for install id + settings; nil = ephemeral
	up      *selfupdate.Updater
}

// NewManager builds the update manager over the app database (may be nil —
// the install id is then ephemeral for this process only).
func NewManager(currentVersion string, db *sql.DB) *Manager {
	up, err := selfupdate.NewUpdater(selfupdate.Config{
		Filters: []string{`^partstable-connector_`},
	})
	if err != nil {
		// A bad filter pattern is a programming error; fall back to the
		// default updater rather than shipping without an update channel.
		up = selfupdate.DefaultUpdater()
	}
	return &Manager{current: currentVersion, db: db, up: up}
}

// Check runs one update check: the anonymous ping (unless opted out) plus
// a GitHub release detection. The ping never fails the check.
func (m *Manager) Check(ctx context.Context) (CheckResult, error) {
	res := CheckResult{Current: m.current, CheckedAt: time.Now().UTC()}

	switch {
	case telemetry.EnvForcedOptOut():
		res.PingSkipped = "disabled by " + telemetry.EnvOptOut
	case m.OptedOut(ctx):
		res.PingSkipped = "turned off in settings"
	default:
		if err := telemetry.Ping(ctx, telemetry.CollectorURL(),
			telemetry.NewPayload(m.current, m.InstallID(ctx))); err == nil {
			res.PingSent = true
		}
	}

	rel, found, err := m.up.DetectLatest(ctx, selfupdate.NewRepositorySlug(RepoOwner, RepoName))
	if err != nil {
		return res, fmt.Errorf("update: check failed: %w", err)
	}
	if found {
		res.Latest = rel.Version()
		res.URL = rel.URL
		res.Notes = rel.ReleaseNotes
		res.Available = isNewer(rel.Version(), m.current)
	}

	m.mu.Lock()
	m.last = &Summary{
		Current:   res.Current,
		Latest:    res.Latest,
		Available: res.Available,
		CheckedAt: res.CheckedAt,
		OptedOut:  m.OptedOut(ctx),
	}
	m.mu.Unlock()
	return res, nil
}

// DetectOnly checks the release channel without sending the telemetry
// ping — used by doctor, whose job is diagnostics, not counting.
func (m *Manager) DetectOnly(ctx context.Context) (string, bool, error) {
	rel, found, err := m.up.DetectLatest(ctx, selfupdate.NewRepositorySlug(RepoOwner, RepoName))
	if err != nil || !found {
		return "", found, err
	}
	return rel.Version(), true, nil
}

// Apply downloads the newer release and swaps the running binary in place
// (the old one is kept as .old for rollback). The swap is safe while
// running: a restart finishes the update.
func (m *Manager) Apply(ctx context.Context) (string, error) {
	rel, found, err := m.up.DetectLatest(ctx, selfupdate.NewRepositorySlug(RepoOwner, RepoName))
	if err != nil {
		return "", fmt.Errorf("update: check failed: %w", err)
	}
	if !found || !isNewer(rel.Version(), m.current) {
		return "", ErrUpToDate
	}
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("update: locate running binary: %w", err)
	}
	if err := m.up.UpdateTo(ctx, rel, exe); err != nil {
		return "", fmt.Errorf("update: download failed — the running version is unchanged (%w)", err)
	}
	return rel.Version(), nil
}

// Summary returns the cached state for display; never touches the network.
func (m *Manager) Summary(ctx context.Context) Summary {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.last != nil {
		return *m.last
	}
	return Summary{Current: m.current, OptedOut: m.OptedOut(ctx)}
}

// OptedOut reports the telemetry setting (the environment override is
// checked separately and wins regardless of the stored value).
func (m *Manager) OptedOut(ctx context.Context) bool {
	if m.db == nil {
		return false
	}
	v, found, err := store.GetSetting(ctx, m.db, SettingOptOut)
	return err == nil && found && v == "1"
}

// SetOptOut stores the telemetry toggle.
func (m *Manager) SetOptOut(ctx context.Context, out bool) error {
	if m.db == nil {
		return errors.New("update: settings unavailable (no local database)")
	}
	v := "0"
	if out {
		v = "1"
	}
	return store.SetSetting(ctx, m.db, SettingOptOut, v)
}

// InstallID returns the anonymous install UUID, creating and persisting it
// on first use. With no database it produces an ephemeral id.
func (m *Manager) InstallID(ctx context.Context) string {
	if m.db != nil {
		if id, found, err := store.GetSetting(ctx, m.db, SettingInstallID); err == nil && found && id != "" {
			return id
		}
		id, err := newUUIDv4()
		if err == nil {
			if err := store.SetSetting(ctx, m.db, SettingInstallID, id); err == nil {
				return id
			}
		}
	}
	id, _ := newUUIDv4()
	return id
}

// isNewer compares a candidate release against the running version.
// Unparseable running versions (plain "dev") always count as older — an
// up-to-date claim needs a parseable current version.
func isNewer(candidate, current string) bool {
	cur, err := semver.NewVersion(current)
	if err != nil {
		return true
	}
	rel, err := semver.NewVersion(candidate)
	if err != nil {
		return false
	}
	return rel.GreaterThan(cur)
}

// newUUIDv4 builds an RFC 4122 v4 UUID without a dependency.
func newUUIDv4() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
