import './style.css';

// The localhost API (FM-15): the UI speaks the same verbs as any local
// integration. Default port; PARTSTABLE_API_PORT is a dev-only override.
const API_BASE = 'http://127.0.0.1:7878';

interface Xref {
  to_pn: string;
  kind: string;
  source: string;
  source_detail?: string;
}

interface Holder {
  holder: string;
  qty: number;
  condition?: string;
  last_seen: string;
  source: string;
  source_detail?: string;
}

interface Part {
  pn: string;
  display_pn: string;
  description: string;
  category?: string;
  xrefs: Xref[];
  holders: Holder[];
}

interface LookupResult {
  query: string;
  normalized: string;
  matched_by: string;
  part: Part | null;
}

interface CompendiumInfo {
  schema: number;
  vintage: string;
  source_rev: string;
  part_count: number;
}

interface Health {
  app_version: string;
  compendium: CompendiumInfo | null;
}

const CHIP_LABELS: Record<string, string> = {
  oem: 'OEM',
  government_registry: 'GOVERNMENT REGISTRY',
  broker_verified: 'BROKER-VERIFIED',
  partner: 'PARTNER',
  certified: 'CERTIFIED ★',
};

function escapeHTML(s: string): string {
  const map: Record<string, string> = {
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  };
  return s.replace(/[&<>"']/g, (c) => map[c] ?? c);
}

// FM-6: every fact renders with a source chip; hovering shows the full
// source detail.
function chip(source: string, detail?: string): string {
  const label = CHIP_LABELS[source] ?? escapeHTML(source.toUpperCase());
  const title = detail && detail.length > 0 ? detail : CHIP_LABELS[source] ?? source;
  return `<span class="chip chip-${escapeHTML(source)}" title="${escapeHTML(title)}">${label}</span>`;
}

const app = document.querySelector<HTMLDivElement>('#app')!;

app.innerHTML = `
  <header class="topbar">
    <div class="brand">
      <span class="brand-mark">PT</span>
      <span class="brand-name">PartsTable <span class="brand-sub">Connector</span></span>
    </div>
    <span class="vintage" id="vintage">checking local data…</span>
  </header>
  <main class="content">
    <form class="search" id="search">
      <input
        id="pn"
        type="text"
        placeholder="Look up a part number — e.g. 02CL197"
        autocomplete="off"
        spellcheck="false"
        autofocus
      />
      <button type="submit">Look up</button>
    </form>
    <p class="hint">Answers come from your own machine. Every fact carries its source.</p>
    <section id="result" aria-live="polite"></section>
  </main>
`;

const vintageEl = document.querySelector<HTMLSpanElement>('#vintage')!;
const resultEl = document.querySelector<HTMLElement>('#result')!;
const form = document.querySelector<HTMLFormElement>('#search')!;
const input = document.querySelector<HTMLInputElement>('#pn')!;

async function loadHealth(): Promise<void> {
  try {
    const r = await fetch(`${API_BASE}/health`);
    const body: Health = await r.json();
    if (body.compendium) {
      const rev = body.compendium.vintage.slice(0, 10);
      vintageEl.textContent =
        `compendium rev ${rev} · ${body.compendium.part_count.toLocaleString('en-US')} parts`;
      return;
    }
    vintageEl.textContent = 'no compendium loaded yet';
    resultEl.innerHTML = `
      <div class="banner">
        No compendium is installed yet, so there is nothing to look up.
        The data ships with your first release build — nothing is missing
        on your machine.
      </div>`;
  } catch {
    vintageEl.textContent = 'local API unreachable';
    resultEl.innerHTML = `
      <div class="banner">The local lookup service is not answering. Restart the app;
        if the port is busy, set PARTSTABLE_API_PORT.</div>`;
  }
}

form.addEventListener('submit', (e) => {
  e.preventDefault();
  const pn = input.value.trim();
  if (pn.length > 0) void lookup(pn);
});

async function lookup(pn: string): Promise<void> {
  resultEl.innerHTML = `<div class="muted">Looking up ${escapeHTML(pn)}…</div>`;
  try {
    const r = await fetch(`${API_BASE}/lookup?pn=${encodeURIComponent(pn)}`);
    const body = await r.json();
    if (!r.ok) {
      resultEl.innerHTML = `<div class="banner">${escapeHTML(String(body.error ?? 'lookup failed'))}</div>`;
      return;
    }
    render(body as LookupResult);
  } catch {
    resultEl.innerHTML = `<div class="banner">The local lookup service is not answering.</div>`;
  }
}

function render(res: LookupResult): void {
  if (!res.part) {
    resultEl.innerHTML = `
      <div class="card">
        <h2>No record for <span class="mono">${escapeHTML(res.normalized)}</span></h2>
        <p class="muted">Entered as <span class="mono">${escapeHTML(res.query)}</span>.</p>
      </div>`;
    return;
  }
  const p = res.part;
  const matchNote =
    res.matched_by === 'alias'
      ? `<span class="match-note">matched via alias — canonical part <span class="mono">${escapeHTML(p.pn)}</span></span>`
      : '';

  const xrefRows = p.xrefs
    .map(
      (x) => `
      <tr>
        <td class="mono">${escapeHTML(x.to_pn)}</td>
        <td>${escapeHTML(x.kind)}</td>
        <td>${chip(x.source, x.source_detail)}</td>
      </tr>`,
    )
    .join('');

  const holderRows = p.holders
    .map(
      (h) => `
      <tr>
        <td>${escapeHTML(h.holder)}</td>
        <td class="num">${h.qty}</td>
        <td>${escapeHTML(h.condition ?? '')}</td>
        <td class="mono">${escapeHTML(h.last_seen)}</td>
        <td>${chip(h.source, h.source_detail)}</td>
      </tr>`,
    )
    .join('');

  resultEl.innerHTML = `
    <div class="card">
      <div class="card-head">
        <h2 class="mono">${escapeHTML(p.display_pn)}</h2>
        ${p.category ? `<span class="category">${escapeHTML(p.category)}</span>` : ''}
        ${matchNote}
      </div>
      <p class="desc">${escapeHTML(p.description)}</p>
      <h3>Cross-references</h3>
      ${
        xrefRows
          ? `<table><thead><tr><th>Part</th><th>Kind</th><th>Source</th></tr></thead><tbody>${xrefRows}</tbody></table>`
          : '<p class="muted">No cross-references on record.</p>'
      }
      <h3>Who holds it</h3>
      ${
        holderRows
          ? `<table><thead><tr><th>Holder</th><th>Qty</th><th>Condition</th><th>Last seen</th><th>Source</th></tr></thead><tbody>${holderRows}</tbody></table>`
          : '<p class="muted">No holders on record.</p>'
      }
    </div>`;
}

void loadHealth();
