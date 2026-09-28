import { useState, useEffect } from 'react';
import HomeView from './views/HomeView';
import SettingsView from './views/SettingsView';
import { API, type Health } from './api';

type View = 'home' | 'settings';

export type Theme = 'light' | 'dark';

export default function App() {
  const [view, setView] = useState<View>('home');
  const [health, setHealth] = useState<Health | null>(null);
  const [theme, setTheme] = useState<Theme>(
    () => (localStorage.getItem('pt-theme') as Theme) || 'light',
  );

  useEffect(() => {
    fetch(`${API}/health`)
      .then((r) => r.json())
      .then(setHealth)
      .catch(() => {});
  }, []);

  // Theme is applied at the document root; every surface keys off
  // [data-theme] in index.css.
  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme);
    localStorage.setItem('pt-theme', theme);
  }, [theme]);

  return (
    <>
      <header className="topbar">
        <div className="brand" role="button" tabIndex={0}
          onClick={() => setView('home')}
          onKeyDown={(e) => { if (e.key === 'Enter') setView('home'); }}
          title="PartsTable Connector — home"
        >
          <span className="brand-mark">PT</span>
          <span className="brand-name">
            PartsTable <span className="brand-sub">Connector</span>
          </span>
        </div>
        <span className="account">{health?.auth?.signed_in ? health.auth.email : ''}</span>
        <span className="vintage">
          {health?.compendium
            ? `compendium rev ${health.compendium.vintage.slice(0, 10)} · ${health.compendium.part_count.toLocaleString()} parts`
            : 'live catalog'}
        </span>
        <button
          className="theme-toggle"
          onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
          title="Toggle light/dark"
        >
          {theme === 'dark' ? '☀' : '☾'}
        </button>
        <button
          className={`gear-btn ${view === 'settings' ? 'active' : ''}`}
          onClick={() => setView(view === 'settings' ? 'home' : 'settings')}
          title="Settings"
          aria-label="Settings"
        >
          ⚙
        </button>
      </header>
      <main className="content">
        {view === 'home' && <HomeView />}
        {view === 'settings' && (
          <SettingsView health={health} onHealth={setHealth} theme={theme} onTheme={setTheme} />
        )}
      </main>
    </>
  );
}
