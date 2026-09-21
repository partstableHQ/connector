import {
  createTable,
  getCoreRowModel,
  getFilteredRowModel,
  getSortedRowModel,
  type ColumnDef,
  type SortingState,
  type VisibilityState,
} from '@tanstack/table-core';
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
    <span class="vintage" id="vintage">checking local data…</span>
  </header>
  <main class="content">
    <nav class="tabs">
      <button type="button" class="tab active" data-view="lookup">Look up a part</button>
      <button type="button" class="tab" data-view="paste">Paste a list</button>
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
  </main>
`;

const vintageEl = document.querySelector<HTMLSpanElement>('#vintage')!;
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

// ---- tabs --------------------------------------------------------------

const tabs = document.querySelectorAll<HTMLButtonElement>('.tab');
for (const t of tabs) {
  t.addEventListener('click', () => {
    for (const other of tabs) other.classList.toggle('active', other === t);
    const view = t.dataset.view === 'paste' ? 'paste' : 'lookup';
    document.querySelector('#view-lookup')!.classList.toggle('hidden', view !== 'lookup');
    document.querySelector('#view-paste')!.classList.toggle('hidden', view !== 'paste');
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

void loadHealth();
