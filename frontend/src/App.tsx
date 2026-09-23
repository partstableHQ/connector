import { useState, useEffect } from 'react';
import LookupView from './views/LookupView';
import PasteView from './views/PasteView';
import SettingsView from './views/SettingsView';
import { API, type Health } from './api';

type View = 'lookup' | 'paste' | 'settings';

export default function App() {
  const [view, setView] = useState<View>('lookup');
  const [health, setHealth] = useState<Health | null>(null);

  useEffect(() => {
    fetch(`${API}/health`)
      .then((r) => r.json())
      .then(setHealth)
      .catch(() => {});
  }, []);

  // Apply theme before render
  useEffect(() => {
    const saved = localStorage.getItem('pt-theme') ?? 'light';
    document.documentElement.setAttribute('data-theme', saved);
  }, []);

  const toggleTheme = () => {
    const cur = document.documentElement.getAttribute('data-theme');
    const next = cur === 'dark' ? 'light' : 'dark';
    document.documentElement.setAttribute('data-theme', next);
    localStorage.setItem('pt-theme', next);
  };

  return (
    <>
      <header className="topbar">
        <div className="brand">
          <span className="brand-mark">PT</span>
          <span className="brand-name">
            PartsTable <span className="brand-sub">Connector</span>
          </span>
        </div>
        <span className="account">{health?.auth?.signed_in ? health.auth.email : ''}</span>
        <span className="vintage">
          {health?.compendium
            ? `compendium rev ${health.compendium.vintage.slice(0, 10)} · ${health.compendium.part_count.toLocaleString()} parts`
            : 'no compendium loaded'}
        </span>
        <button className="theme-toggle" onClick={toggleTheme} title="Toggle light/dark">
          {document.documentElement.getAttribute('data-theme') === 'dark' ? '☀' : '☾'}
        </button>
      </header>
      <main className="content">
        <nav className="tabs">
          {(['lookup', 'paste', 'settings'] as View[]).map((v) => (
            <button key={v} className={`tab ${view === v ? 'active' : ''}`} onClick={() => setView(v)}>
              {v === 'lookup' ? 'Look up a part' : v === 'paste' ? 'Paste a list' : 'Settings'}
            </button>
          ))}
        </nav>
        {view === 'lookup' && <LookupView />}
        {view === 'paste' && <PasteView />}
        {view === 'settings' && <SettingsView health={health} onHealth={setHealth} />}
      </main>
    </>
  );
}
