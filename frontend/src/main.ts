import {
  createTable,
  getCoreRowModel,
  getFilteredRowModel,
  getSortedRowModel,
  type ColumnDef,
  type SortingState,
  type VisibilityState,
} from '@tanstack/table-core';
// Geist typography (FM-18), bundled locally — lookups work offline and so
// do the fonts. SIL OFL license, ships with the app.
import '@fontsource/geist-sans/400.css';
import '@fontsource/geist-sans/500.css';
import '@fontsource/geist-sans/600.css';
import '@fontsource/geist-sans/700.css';
import '@fontsource/geist-mono/400.css';
import '@fontsource/geist-mono/500.css';
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

interface AuthStatus {
  signed_in: boolean;
  email?: string;
  signing_in: boolean;
  last_error?: string;
}

interface Health {
  app_version: string;
  compendium: CompendiumInfo | null;
  auth?: AuthStatus | null;
  update?: UpdateSummary | null;
}

interface ParseWarning {
  line: number;
  raw: string;
  text: string;
}

interface PasteEntry {
  raw: string;
  pn: string;
  norm: string;
  qty: number;
  lines: number[];
  result: LookupResult | null;
}

interface PasteResponse {
  entries: PasteEntry[];
  warnings: ParseWarning[];
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
    <span class="account" id="account"></span>
    <span class="vintage" id="vintage">checking local data…</span>
  </header>
  <main class="content">
    <div id="auth" class="hidden"></div>
    <nav class="tabs">
      <button type="button" class="tab active" data-view="lookup">Look up a part</button>
      <button type="button" class="tab" data-view="paste">Paste a list</button>
      <button type="button" class="tab" data-view="settings">Settings</button>
    </nav>

    <section id="view-lookup">
      <form class="search" id="search">
        <input
          id="pn"
          type="text"
          placeholder="Look up a part number — e.g. 02CL197"
          autocomplete="off"
          spellcheck="false"
        />
        <button type="submit">Look up</button>
      </form>
      <p class="hint">Answers come from your own machine. Every fact carries its source.</p>
      <section id="result" aria-live="polite"></section>
    </section>

    <section id="view-paste" class="hidden">
      <p class="lede">Paste a whole list — lines, commas, CSV, quantities like
        <span class="mono">x4</span> / <span class="mono">4x</span> /
        <span class="mono">qty 4</span>, quote emails. Nothing you paste is
        dropped silently: anything we can't read comes back in a warning.</p>
      <textarea id="paste" rows="10" spellcheck="false"
        placeholder="02CL197 x4&#10;4X70J67435, 2&#10;SN730SDB512GB"></textarea>
      <div class="actions">
        <button type="button" id="paste-go" class="primary">Look up list</button>
        <button type="button" id="paste-export" disabled>Export to Excel</button>
      </div>
      <div id="paste-warnings"></div>
      <div id="paste-table"></div>
    </section>

    <section id="view-settings" class="hidden">
      <div class="card settings-card">
        <h3>Account</h3>
        <p id="settings-account" class="muted">Checking…</p>
        <button type="button" id="settings-signout" class="secondary hidden">Sign out</button>
      </div>
      <div class="card settings-card">
        <h3>Updates</h3>
        <p id="settings-update" class="muted">Current version — checking…</p>
        <div class="actions">
          <button type="button" id="settings-check">Check for updates</button>
          <button type="button" id="settings-apply" class="hidden primary">Download update</button>
        </div>
        <p id="settings-update-note" class="muted"></p>
      </div>
      <div class="card settings-card">
        <h3>Privacy</h3>
        <label class="toggle">
          <input type="checkbox" id="settings-telemetry" />
          <span>
            <strong>Send anonymous update checks</strong><br />
            <span class="muted">The only telemetry: app version, operating system, and a
            random install ID. No part numbers, no queries, no email — see
            PRIVACY.md. Force-off also works with
            <span class="mono">PARTSTABLE_NO_TELEMETRY=1</span>.</span>
          </span>
        </label>
      </div>
      <p class="more-link">
        There's more — the paid end-to-end system at
        <a href="https://partstable.com" id="more-link">partstable.com</a>
      </p>
    </section>
  </main>
`;

const vintageEl = document.querySelector<HTMLSpanElement>('#vintage')!;
const accountEl = document.querySelector<HTMLSpanElement>('#account')!;
const authEl = document.querySelector<HTMLElement>('#auth')!;
const resultEl = document.querySelector<HTMLElement>('#result')!;
const searchForm = document.querySelector<HTMLFormElement>('#search')!;
const pnInput = document.querySelector<HTMLInputElement>('#pn')!;

// ---- health (the honest vintage, FM-10) -------------------------------

async function loadHealth(): Promise<void> {
  try {
    const r = await fetch(`${API_BASE}/health`);
    const body: Health = await r.json();
    if (body.compendium) {
      const rev = body.compendium.vintage.slice(0, 10);
      vintageEl.textContent =
        `compendium rev ${rev} · ${body.compendium.part_count.toLocaleString('en-US')} parts`;
    } else {
      vintageEl.textContent = 'no compendium loaded yet';
      if (!resultEl.innerHTML.includes('banner')) {
        resultEl.innerHTML = `
          <div class="banner">
            No compendium is installed yet, so there is nothing to look up.
            The data ships with your first release build — nothing is missing
            on your machine.
          </div>`;
      }
    }
    renderAuth(body.auth ?? null);
  } catch {
    vintageEl.textContent = 'local API unreachable';
    resultEl.innerHTML = `
      <div class="banner">The local lookup service is not answering. Restart the app;
        if the port is busy, set PARTSTABLE_API_PORT.</div>`;
  }
}

// ---- account (FM-3) ----------------------------------------------------

function renderAuth(auth: AuthStatus | null): void {
  accountEl.textContent = auth?.signed_in && auth.email ? auth.email : '';

  // Signed in (or state unknown): no banner — the free app never nags.
  if (!auth || auth.signed_in) {
    authEl.classList.add('hidden');
    authEl.innerHTML = '';
    return;
  }

  if (auth.signing_in) {
    authEl.classList.remove('hidden');
    authEl.innerHTML = `
      <div class="banner auth">
        Waiting for your browser… finish the sign-in there and this window
        will update by itself.
      </div>`;
    return;
  }

  const error = auth.last_error
    ? `<div class="auth-error">${escapeHTML(auth.last_error)}</div>`
    : '';
  authEl.classList.remove('hidden');
  authEl.innerHTML = `
    <div class="banner auth">
      <div>
        <strong>Optional: sign in to your free account</strong> — no card,
        lookups work without it. Your key stays in this machine's keychain.
      </div>
      <button type="button" id="auth-login">Sign in — opens your browser</button>
      ${error}
    </div>`;
  document.querySelector<HTMLButtonElement>('#auth-login')?.addEventListener('click', () => {
    void startSignIn();
  });
}

async function startSignIn(): Promise<void> {
  try {
    await fetch(`${API_BASE}/auth/login`, { method: 'POST' });
  } catch {
    authEl.innerHTML = '<div class="banner auth">The local service is not answering.</div>';
    return;
  }
  const deadline = Date.now() + 5 * 60_000;
  while (Date.now() < deadline) {
    await new Promise((r) => setTimeout(r, 2000));
    try {
      const r = await fetch(`${API_BASE}/health`);
      const body: Health = await r.json();
      renderAuth(body.auth ?? null);
      if (!body.auth?.signing_in) break;
    } catch {
      break;
    }
  }
}

// ---- tabs --------------------------------------------------------------

const tabs = document.querySelectorAll<HTMLButtonElement>('.tab');
for (const t of tabs) {
  t.addEventListener('click', () => {
    for (const other of tabs) other.classList.toggle('active', other === t);
    const view = t.dataset.view === 'paste' ? 'paste' : t.dataset.view === 'settings' ? 'settings' : 'lookup';
    document.querySelector('#view-lookup')!.classList.toggle('hidden', view !== 'lookup');
    document.querySelector('#view-paste')!.classList.toggle('hidden', view !== 'paste');
    document.querySelector('#view-settings')!.classList.toggle('hidden', view !== 'settings');
    if (view === 'settings') refreshSettings();
  });
}

// ---- single lookup ------------------------------------------------------

searchForm.addEventListener('submit', (e) => {
  e.preventDefault();
  const pn = pnInput.value.trim();
  if (pn.length > 0) void lookupOne(pn);
});

async function lookupOne(pn: string): Promise<void> {
  resultEl.innerHTML = `<div class="muted">Looking up ${escapeHTML(pn)}…</div>`;
  try {
    const r = await fetch(`${API_BASE}/lookup?pn=${encodeURIComponent(pn)}`);
    const body = await r.json();
    if (!r.ok) {
      resultEl.innerHTML = `<div class="banner">${escapeHTML(String(body.error ?? 'lookup failed'))}</div>`;
      return;
    }
    renderLookup(body as LookupResult);
  } catch {
    resultEl.innerHTML = `<div class="banner">The local lookup service is not answering.</div>`;
  }
}

function renderLookup(res: LookupResult): void {
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

// ---- paste a list (FM-7/8/9) -------------------------------------------

const pasteEl = document.querySelector<HTMLTextAreaElement>('#paste')!;
const pasteGo = document.querySelector<HTMLButtonElement>('#paste-go')!;
const pasteExport = document.querySelector<HTMLButtonElement>('#paste-export')!;
const warningsEl = document.querySelector<HTMLElement>('#paste-warnings')!;
const tableEl = document.querySelector<HTMLElement>('#paste-table')!;

interface GridRow {
  raw: string;
  qty: number;
  description: string;
  substitutesHTML: string;
  substitutesText: string;
  holdersHTML: string;
  holdersText: string;
  lines: string;
}

let gridData: GridRow[] = [];
let sorting: SortingState = [];
let columnVisibility: VisibilityState = { lines: false };
let globalFilter = '';
// table-core 8.21's feature code reads state slices unconditionally even
// when their feature is not registered (getHeaderGroups reads pinning,
// visibility toggles touch pagination), so we supply every slice.
const columnPinning = { left: [] as string[], right: [] as string[] };
const pagination = { pageIndex: 0, pageSize: 1_000_000 };
const fullState = () => ({
  sorting,
  columnVisibility,
  globalFilter,
  columnPinning,
  pagination,
  columnFilters: [] as never[],
  columnOrder: [] as string[],
  columnSizing: {} as Record<string, number>,
  grouping: [] as string[],
  expanded: {} as Record<string, boolean>,
  rowSelection: {} as Record<string, boolean>,
  rowPinning: {} as Record<string, boolean>,
});

const columns: ColumnDef<GridRow>[] = [
  { accessorKey: 'raw', header: 'Your part' },
  { accessorKey: 'qty', header: 'Qty' },
  { accessorKey: 'description', header: 'Description' },
  { accessorKey: 'substitutesText', header: 'Substitutes' },
  { accessorKey: 'holdersText', header: 'Holders' },
  { accessorKey: 'lines', header: 'Line' },
];

const table = createTable<GridRow>({
  data: gridData,
  columns,
  state: fullState(),
  // Sorting and visibility toggles route here: apply the updater to our
  // state variables, then re-render.
  onStateChange: (updater) => {
    const next = typeof updater === 'function' ? updater(table.getState()) : updater;
    if (next.sorting) sorting = next.sorting;
    if (next.columnVisibility) columnVisibility = next.columnVisibility;
    if (next.globalFilter !== undefined) globalFilter = next.globalFilter;
    renderGrid();
  },
  getCoreRowModel: getCoreRowModel(),
  getSortedRowModel: getSortedRowModel(),
  getFilteredRowModel: getFilteredRowModel(),
  renderFallbackValue: null,
});

function renderGrid(): void {
  table.setOptions((prev) => ({
    ...prev,
    data: gridData,
    state: fullState(),
  }));

  const head = table
    .getFlatHeaders()
    .map((h) => {
      const col = h.column;
      const sorted = col.getIsSorted();
      const arrow = sorted === 'asc' ? ' ▲' : sorted === 'desc' ? ' ▼' : '';
      return `<th data-col="${escapeHTML(col.id)}" title="Click to sort">${escapeHTML(
        String(col.columnDef.header),
      )}${arrow}</th>`;
    })
    .join('');

  const rows = table.getRowModel().rows;
  const body = rows
    .map((r) => {
      const o = r.original;
      return `<tr>
        <td class="mono">${escapeHTML(o.raw)}</td>
        <td class="num">${o.qty}</td>
        <td>${escapeHTML(o.description)}</td>
        <td>${o.substitutesHTML}</td>
        <td>${o.holdersHTML}</td>
        <td class="mono">${escapeHTML(o.lines)}</td>
      </tr>`;
    })
    .join('');

  tableEl.innerHTML = `
    <div class="grid-tools">
      <input id="grid-filter" type="text" placeholder="Filter rows…"
        value="${escapeHTML(globalFilter)}" />
      <details class="cols">
        <summary>Columns</summary>
        <div>
          ${table
            .getAllLeafColumns()
            .map(
              (c) => `<label><input type="checkbox" data-col="${escapeHTML(c.id)}"
                ${c.getIsVisible() ? 'checked' : ''}/> ${escapeHTML(String(c.columnDef.header))}</label>`,
            )
            .join('')}
        </div>
      </details>
      <span class="rowcount">${rows.length} of ${gridData.length} parts</span>
    </div>
    <table class="grid">
      <thead><tr>${head}</tr></thead>
      <tbody>${body || '<tr><td colspan="6" class="muted">Nothing to show — paste a list above.</td></tr>'}</tbody>
    </table>`;

  // Sorting: click a header.
  for (const th of tableEl.querySelectorAll<HTMLTableCellElement>('th[data-col]')) {
    th.addEventListener('click', () => {
      const col = table.getColumn(th.dataset.col!);
      col?.getToggleSortingHandler()?.(new MouseEvent('click'));
    });
  }
  // Global filter.
  const filter = tableEl.querySelector<HTMLInputElement>('#grid-filter')!;
  filter.addEventListener('input', () => {
    globalFilter = filter.value;
    renderGrid();
  });
  // Column show/hide: toggleVisibility routes through onStateChange.
  for (const box of tableEl.querySelectorAll<HTMLInputElement>('.cols input[type=checkbox]')) {
    box.addEventListener('change', () => {
      table.getColumn(box.dataset.col!)?.toggleVisibility(box.checked);
    });
  }
}

pasteGo.addEventListener('click', () => void runPaste(false));
pasteExport.addEventListener('click', () => void runPaste(true));

async function runPaste(wantExport: boolean): Promise<void> {
  const text = pasteEl.value;
  if (text.trim().length === 0) {
    warningsEl.innerHTML = '<div class="panel-warn">Paste a list first.</div>';
    return;
  }
  if (wantExport) {
    await exportXLSX(text);
    return;
  }
  pasteGo.disabled = true;
  let body: PasteResponse;
  try {
    const r = await fetch(`${API_BASE}/paste`, {
      method: 'POST',
      headers: { 'Content-Type': 'text/plain' },
      body: text,
    });
    if (!r.ok) {
      const err = await r.json().catch(() => ({}) as { error?: string });
      warningsEl.innerHTML = `<div class="panel-warn">${escapeHTML(err.error ?? 'lookup failed')}</div>`;
      return;
    }
    body = (await r.json()) as PasteResponse;
  } catch {
    warningsEl.innerHTML = '<div class="panel-warn">The local lookup service is not answering.</div>';
    return;
  } finally {
    pasteGo.disabled = false;
  }
  // Outside the network guard: a rendering bug must surface as a real
  // error, not masquerade as an offline message.
  renderPaste(body);
}

async function exportXLSX(text: string): Promise<void> {
  try {
    const r = await fetch(`${API_BASE}/paste/export`, {
      method: 'POST',
      headers: { 'Content-Type': 'text/plain' },
      body: text,
    });
    if (!r.ok) {
      const body = await r.json().catch(() => ({}) as { error?: string });
      warningsEl.innerHTML = `<div class="panel-warn">${escapeHTML(body.error ?? 'export failed')}</div>`;
      return;
    }
    const blob = await r.blob();
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = 'partstable-list.xlsx';
    a.click();
    URL.revokeObjectURL(a.href);
  } catch {
    warningsEl.innerHTML = '<div class="panel-warn">The local lookup service is not answering.</div>';
  }
}

function renderPaste(res: PasteResponse): void {
  // FM-7: the warnings panel — unparseable lines are never dropped.
  warningsEl.innerHTML =
    res.warnings.length === 0
      ? ''
      : `<div class="panel-warn">
          <strong>${res.warnings.length} line${res.warnings.length === 1 ? ' needs' : 's need'} your attention</strong>
          — nothing was dropped silently:
          <ul>${res.warnings
            .map(
              (w) =>
                `<li>line ${w.line}: <span class="mono">${escapeHTML(w.raw)}</span> — ${escapeHTML(w.text)}</li>`,
            )
            .join('')}</ul>
        </div>`;

  gridData = res.entries.map((e) => {
    const part = e.result?.part ?? null;
    const substitutes = part ? part.xrefs : [];
    const holders = part ? part.holders : [];
    return {
      raw: e.pn,
      qty: e.qty,
      description: part ? part.description : '(no record in compendium rev)',
      substitutesHTML:
        substitutes.map((x) => `${escapeHTML(x.to_pn)} ${chip(x.source, x.source_detail)}`).join('<br>') || '—',
      substitutesText: substitutes.map((x) => `${x.to_pn} ${x.kind}`).join(' '),
      holdersHTML:
        holders.map((h) => `${escapeHTML(h.holder)} (${h.qty}) ${chip(h.source, h.source_detail)}`).join('<br>') ||
        '—',
      holdersText: holders.map((h) => `${h.holder} ${h.qty}`).join(' '),
      lines: e.lines.join(', '),
    };
  });

  pasteExport.disabled = gridData.length === 0;
  renderGrid();
}

// ---- settings (FM-11 updates, FM-13 privacy toggle) ---------------------

const settingsAccount = document.querySelector<HTMLElement>('#settings-account')!;
const settingsSignOut = document.querySelector<HTMLButtonElement>('#settings-signout')!;
const settingsUpdate = document.querySelector<HTMLElement>('#settings-update')!;
const settingsUpdateNote = document.querySelector<HTMLElement>('#settings-update-note')!;
const settingsCheck = document.querySelector<HTMLButtonElement>('#settings-check')!;
const settingsApply = document.querySelector<HTMLButtonElement>('#settings-apply')!;
const settingsTelemetry = document.querySelector<HTMLInputElement>('#settings-telemetry')!;

interface UpdateSummary {
  current: string;
  latest?: string;
  available: boolean;
  telemetry_opt_out: boolean;
}

function refreshSettings(): void {
  void (async () => {
    try {
      const [hRes, sRes] = await Promise.all([
        fetch(`${API_BASE}/health`),
        fetch(`${API_BASE}/settings`),
      ]);
      const health: Health = await hRes.json();
      const settings = (await sRes.json()) as { telemetry_opt_out: boolean; telemetry_env_forced: boolean };

      settingsAccount.textContent = health.auth?.signed_in
        ? `Signed in as ${health.auth.email}`
        : 'Not signed in — optional. The banner on the other tabs starts sign-in.';
      settingsSignOut.classList.toggle('hidden', !health.auth?.signed_in);
      settingsTelemetry.checked = !settings.telemetry_opt_out;
      if (settings.telemetry_env_forced) {
        settingsTelemetry.disabled = true;
        settingsTelemetry.checked = false;
      } else {
        settingsTelemetry.disabled = false;
      }

      const u = health.update;
      if (u) {
        settingsUpdate.textContent = u.latest
          ? `Current ${u.current} · latest release ${u.latest}${u.available ? ' — update available' : ''}`
          : `Current ${u.current} — no release published yet`;
      } else {
        settingsUpdate.textContent = `Current ${health.app_version}`;
      }
    } catch {
      settingsAccount.textContent = 'The local service is not answering.';
    }
  })();
}

settingsCheck.addEventListener('click', () => {
  settingsCheck.disabled = true;
  settingsUpdateNote.textContent = 'Checking… (this sends the anonymous update ping unless turned off)';
  void (async () => {
    try {
      const r = await fetch(`${API_BASE}/update/check`, { method: 'POST' });
      const body = (await r.json()) as UpdateSummary & { error?: string };
      if (!r.ok) {
        settingsUpdateNote.textContent = body.error ?? 'check failed';
        return;
      }
      if (body.available) {
        settingsUpdate.textContent = `Current ${body.current} · latest release ${body.latest} — update available`;
        settingsUpdateNote.textContent = 'The download swaps the app in place; restart afterwards to finish.';
        settingsApply.classList.remove('hidden');
      } else {
        settingsUpdate.textContent = `Current ${body.current} — you are up to date`;
        settingsUpdateNote.textContent = '';
      }
    } catch {
      settingsUpdateNote.textContent = 'The local service is not answering.';
    } finally {
      settingsCheck.disabled = false;
    }
  })();
});

settingsApply.addEventListener('click', () => {
  settingsApply.disabled = true;
  settingsUpdateNote.textContent = 'Downloading…';
  void (async () => {
    try {
      const r = await fetch(`${API_BASE}/update/apply`, { method: 'POST' });
      const body = (await r.json()) as { applied?: boolean; version?: string; reason?: string; error?: string };
      if (!r.ok) {
        settingsUpdateNote.textContent = body.error ?? 'download failed';
        return;
      }
      if (body.applied) {
        settingsUpdateNote.textContent = `Downloaded ${body.version} — close and reopen the app to finish the update.`;
        settingsApply.classList.add('hidden');
      } else {
        settingsUpdateNote.textContent = body.reason ?? 'No update to apply.';
        settingsApply.classList.add('hidden');
      }
    } catch {
      settingsUpdateNote.textContent = 'The local service is not answering.';
    } finally {
      settingsApply.disabled = false;
    }
  })();
});

settingsTelemetry.addEventListener('change', () => {
  void (async () => {
    try {
      await fetch(`${API_BASE}/settings`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ telemetry_opt_out: !settingsTelemetry.checked }),
      });
    } catch {
      settingsUpdateNote.textContent = 'Could not save the setting — the local service is not answering.';
    }
  })();
});

settingsSignOut.addEventListener('click', () => {
  void (async () => {
    try {
      await fetch(`${API_BASE}/auth/logout`, { method: 'POST' });
      refreshSettings();
    } catch {
      /* status refresh below will show the failure */
    }
  })();
});

// The single "There's more" link (FM-17): opens in the system browser,
// never inside the app window.
const moreLink = document.querySelector<HTMLAnchorElement>('#more-link')!;
moreLink.addEventListener('click', (e) => {
  e.preventDefault();
  void (async () => {
    try {
      const r = await fetch(`${API_BASE}/more`, { method: 'POST' });
      const body = (await r.json()) as { opened: boolean; url: string };
      if (!body.opened) window.open(body.url, '_blank');
    } catch {
      window.open('https://partstable.com', '_blank');
    }
  })();
});

void loadHealth();
