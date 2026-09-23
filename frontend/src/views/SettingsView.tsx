import { useState, useEffect } from 'react';
import { API, type Health } from '../api';

export default function SettingsView({ health, onHealth }: {
  health: Health | null;
  onHealth: (h: Health | null) => void;
}) {
  const [updateNote, setUpdateNote] = useState('');
  const [checking, setChecking] = useState(false);

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

  return (
    <div className="view-settings">
      <div className="card">
        <h3>Account</h3>
        <p className="muted">
          {health?.auth?.signed_in ? `Signed in as ${health.auth.email}` : 'Not signed in'}
        </p>
        {health?.auth?.signed_in && (
          <button className="btn-secondary" onClick={() => {
            fetch(`${API}/auth/logout`, { method: 'POST' }).catch(() => {});
            fetch(`${API}/health`).then((r) => r.json()).then(onHealth).catch(() => {});
          }}>Sign out</button>
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
      <p className="more-link">
        There's more — the paid end-to-end system at{' '}
        <a href="https://partstable.com" target="_blank" rel="noreferrer">partstable.com</a>
      </p>
    </div>
  );
}
