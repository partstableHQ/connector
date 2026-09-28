import { useState, useEffect } from 'react';
import { API, type Health } from '../api';
import type { Theme } from '../App';

// Enterprise lineup teasers (ssot 2026-09-20 / connector-settings v1).
// The Connector is the free reference; these are the paid end-to-end
// system's surfaces the CEO named for the waitlist teaser.
const ENTERPRISE_APPS = [
  {
    icon: '📄',
    name: 'BOM Lookup',
    blurb: 'Paste an entire bill of materials — every line identified, matched, and substituted.',
  },
  {
    icon: '📉',
    name: 'EOL Intelligence',
    blurb: 'Lifecycle and end-of-service intelligence across the catalog, before you quote.',
  },
  {
    icon: '🤝',
    name: 'Quotations & Orders',
    blurb: 'The sales desk for secondary IT: quotes, orders, and customers in one place.',
  },
  {
    icon: '📦',
    name: 'Warehouse Shipping & Receiving',
    blurb: 'Receive, bin, pick, and ship secondary IT parts — built for the way brokers actually work.',
  },
];

export default function SettingsView({ health, onHealth, theme, onTheme }: {
  health: Health | null;
  onHealth: (h: Health | null) => void;
  theme: Theme;
  onTheme: (t: Theme) => void;
}) {
  const [updateNote, setUpdateNote] = useState('');
  const [checking, setChecking] = useState(false);
  const [pwOld, setPwOld] = useState('');
  const [pwNew, setPwNew] = useState('');
  const [pwConfirm, setPwConfirm] = useState('');
  const [pwNote, setPwNote] = useState('');
  const [pwBusy, setPwBusy] = useState(false);
  const [pwOpen, setPwOpen] = useState(false);

  useEffect(() => {
    fetch(`${API}/health`).then((r) => r.json()).then(onHealth).catch(() => {});
    fetch(`${API}/settings`).then((r) => r.json()).then((s: { telemetry_opt_out: boolean }) => {
      const cb = document.querySelector<HTMLInputElement>('#settings-telemetry');
      if (cb) cb.checked = !s.telemetry_opt_out;
    }).catch(() => {});
  }, []);

  const checkUpdates = async () => {
    setChecking(true);
    setUpdateNote('Checking…');
    try {
      const r = await fetch(`${API}/update/check`, { method: 'POST' });
      const body = await r.json();
      if (!r.ok) { setUpdateNote(body.error ?? 'check failed'); return; }
      setUpdateNote(body.available
        ? `${body.latest} available — click Download to update`
        : 'You are up to date.');
    } catch { setUpdateNote('check failed'); } finally { setChecking(false); }
  };

  const downloadUpdate = async () => {
    setUpdateNote('Downloading…');
    try {
      const r = await fetch(`${API}/update/apply`, { method: 'POST' });
      const body = await r.json();
      setUpdateNote(body.applied
        ? `Downloaded ${body.version} — restart to finish.`
        : body.reason ?? 'No update.');
    } catch { setUpdateNote('download failed'); }
  };

  const changePassword = async () => {
    if (pwBusy) return;
    if (!pwOld || !pwNew) { setPwNote('Fill in both fields.'); return; }
    if (pwNew !== pwConfirm) { setPwNote('New passwords do not match.'); return; }
    setPwBusy(true);
    setPwNote('');
    try {
      const r = await fetch(`${API}/auth/password`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ old_password: pwOld, new_password: pwNew }),
      });
      const body = await r.json().catch(() => ({}));
      if (r.ok) {
        setPwNote('Password changed ✓');
        setPwOld('');
        setPwNew('');
        setPwConfirm('');
      } else {
        setPwNote(body.error ?? `Could not change the password (${r.status}).`);
      }
    } catch {
      setPwNote('Could not reach the account service — check your connection.');
    } finally {
      setPwBusy(false);
    }
  };

  return (
    <div className="view-settings">
      <div className="card">
        <h3>Appearance</h3>
        <div className="theme-picker" role="radiogroup" aria-label="Desktop theme">
          {(['light', 'dark'] as Theme[]).map((t) => (
            <button
              key={t}
              className={`theme-option ${theme === t ? 'active' : ''}`}
              role="radio"
              aria-checked={theme === t}
              onClick={() => onTheme(t)}
            >
              <span className="theme-swatch" data-swatch={t} aria-hidden="true" />
              {t === 'light' ? '☀ Light' : '☾ Dark'}
            </button>
          ))}
        </div>
        <p className="muted">More themes on the way.</p>
      </div>

      <div className="card">
        <h3>Account</h3>
        <p className="muted">
          {health?.auth?.signed_in ? `Signed in as ${health.auth.email}` : 'Not signed in'}
        </p>
        <div className="actions">
          {health?.auth?.signed_in && (
            <button className="btn-secondary" onClick={() => setPwOpen(!pwOpen)}>
              Change password
            </button>
          )}
          {health?.auth?.signed_in && (
            <button className="btn-secondary" onClick={() => {
              fetch(`${API}/auth/logout`, { method: 'POST' }).catch(() => {});
              fetch(`${API}/health`).then((r) => r.json()).then(onHealth).catch(() => {});
            }}>Sign out</button>
          )}
        </div>
        {pwOpen && health?.auth?.signed_in && (
          <div className="pw-form">
            <input
              type="password"
              placeholder="Current password"
              value={pwOld}
              onChange={(e) => setPwOld(e.target.value)}
              autoComplete="current-password"
            />
            <input
              type="password"
              placeholder="New password (8+ characters)"
              value={pwNew}
              onChange={(e) => setPwNew(e.target.value)}
              autoComplete="new-password"
            />
            <input
              type="password"
              placeholder="Repeat new password"
              value={pwConfirm}
              onChange={(e) => setPwConfirm(e.target.value)}
              autoComplete="new-password"
            />
            <div className="actions">
              <button className="btn-primary" onClick={() => void changePassword()} disabled={pwBusy}>
                {pwBusy ? 'Saving…' : 'Save new password'}
              </button>
              {pwNote && <span className={`pw-note ${pwNote.endsWith('✓') ? 'ok' : ''}`}>{pwNote}</span>}
            </div>
          </div>
        )}
      </div>

      <div className="card">
        <h3>Updates</h3>
        <p className="muted">{health?.update?.current ?? health?.app_version ?? 'checking…'}</p>
        <div className="actions">
          <button className="btn-primary" onClick={() => void checkUpdates()} disabled={checking}>
            Check for updates
          </button>
          {updateNote.includes('available —') && (
            <button className="btn-primary" onClick={() => void downloadUpdate()}>
              Download update
            </button>
          )}
        </div>
        {updateNote && <p className="muted">{updateNote}</p>}
      </div>

      <div className="card">
        <h3>Privacy</h3>
        <label className="toggle">
          <input
            type="checkbox"
            id="settings-telemetry"
            defaultChecked
            onChange={(e) => {
              fetch(`${API}/settings`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ telemetry_opt_out: !e.target.checked }),
              }).catch(() => {});
            }}
          />
          <span>
            <strong>Send anonymous update checks</strong>
            <br />
            <span className="muted">
              The only telemetry: app version, OS, random install ID.
              No part numbers, no queries, no email.
            </span>
          </span>
        </label>
      </div>

      <div className="card">
        <h3>Partstable for Enterprise</h3>
        <p className="muted">
          The Connector is the free reference. The end-to-end system for the secondary IT parts
          industry runs on the same data — join the waitlist at{' '}
          <a href="https://partstable.com" target="_blank" rel="noreferrer">partstable.com</a>.
        </p>
        <div className="apps-grid">
          {ENTERPRISE_APPS.map((app) => (
            <div className="app-card" key={app.name}>
              <div className="app-icon" aria-hidden="true">{app.icon}</div>
              <div className="app-body">
                <div className="app-name">
                  {app.name}
                  <span className="app-badge">Enterprise</span>
                </div>
                <div className="app-blurb">{app.blurb}</div>
              </div>
            </div>
          ))}
        </div>
      </div>

      <div className="card story-card">
        <h3>Our story</h3>
        <p>
          PartsTable is built by IT brokers. We buy and sell secondary-market servers, storage,
          and networking gear every day — and we got tired of guessing. Every part number in this
          app is a part number we have had to look up ourselves, on a deadline, for a real deal.
        </p>
        <p className="muted">
          The parts reference for the secondary-market IT industry. Free, open source, every fact cited.
        </p>
        <p className="muted story-contact">
          PartsTable Design LLC · <a href="mailto:tony@partstable.com">tony@partstable.com</a>
        </p>
      </div>
    </div>
  );
}
