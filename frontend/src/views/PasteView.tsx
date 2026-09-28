import { useState } from 'react';
import {
  ModuleRegistry, AllCommunityModule, themeQuartz,
  type ColDef, type RowClickedEvent, type SelectionChangedEvent, type CellKeyDownEvent,
} from 'ag-grid-community';
import { AgGridReact } from 'ag-grid-react';
import { smartParseDetailed } from '../lib/smartParse';
import TdsSheet, { type TDS } from '../components/TdsSheet';

ModuleRegistry.registerModules([AllCommunityModule]);

const gridTheme = themeQuartz.withParams({
  rowHeight: 32,
  headerHeight: 36,
  fontSize: 12.5,
  fontFamily: "'IBM Plex Sans', sans-serif",
  backgroundColor: '#FFFFFF',
  foregroundColor: '#0F1923',
  borderColor: '#E5E7EB',
  oddRowBackgroundColor: '#F8F9FA',
  headerBackgroundColor: '#F8F9FA',
  headerTextColor: '#64748B',
  accentColor: '#0055DD',
});

// Quote-builder layout: the grid carries identity only; substitutes and the
// full data sheet live in the right-hand panel for the active row.
interface GridRow {
  pn: string;
  qty: number;
  description: string;
  condition: string;
  category: string;
}

const colDefs: ColDef<GridRow>[] = [
  { field: 'pn', headerName: 'Part Number', width: 150, cellClass: 'mono-cell' },
  { field: 'qty', headerName: 'Qty', width: 60, type: 'rightAligned' },
  { field: 'description', headerName: 'Description', flex: 1, minWidth: 180 },
  { field: 'condition', headerName: 'Condition', width: 90 },
  { field: 'category', headerName: 'Category', width: 140 },
];

// Trigger a browser download robustly: the anchor must be in the document
// and the object URL must outlive the click (WebView2 revokes eagerly).
function downloadBlob(blob: Blob, name: string) {
  const a = document.createElement('a');
  a.href = URL.createObjectURL(blob);
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(a.href), 30_000);
}

export default function PasteView() {
  const [text, setText] = useState('');
  const [partCount, setPartCount] = useState(0);
  const [loading, setLoading] = useState(false);
  const [warnings, setWarnings] = useState<string[]>([]);
  const [rowData, setRowData] = useState<GridRow[]>([]);
  const [activePn, setActivePn] = useState<string | null>(null);
  const [tds, setTds] = useState<TDS | null>(null);
  const [tdsLoading, setTdsLoading] = useState(false);
  const [selectedPns, setSelectedPns] = useState<string[]>([]);
  const [exporting, setExporting] = useState(false);
  const [exportError, setExportError] = useState('');

  const parseAndLookup = async () => {
    if (!text.trim()) return;
    setLoading(true);
    setWarnings([]);
    setRowData([]);
    setActivePn(null);
    setTds(null);
    setSelectedPns([]);

    const result = smartParseDetailed(text);
    if (result.lines.length === 0) {
      setWarnings(['No part numbers detected. Paste an RFQ, BOM, email, or part list.']);
      setLoading(false);
      return;
    }

    // Enrich rows concurrently (bounded) — a 100-line RFQ must not crawl.
    // One retry per row: the production edge occasionally drops a request,
    // and a single flake must not blank a row that has a real record.
    const CONCURRENCY = 6;
    const rows: GridRow[] = new Array(result.lines.length);
    let next = 0;
    const fetchRow = async (pn: string) => {
      for (let attempt = 0; attempt < 2; attempt++) {
        const r = await fetch(`http://127.0.0.1:7878/plookup?pn=${encodeURIComponent(pn)}`);
        if (r.ok) return r.json();
        if (attempt === 0) await new Promise((res) => setTimeout(res, 600));
      }
      return null;
    };
    const enrichOne = async (i: number) => {
      const line = result.lines[i];
      try {
        const data = await fetchRow(line.pn);
        if (!data) {
          rows[i] = {
            pn: line.pn, qty: line.qty,
            description: line.desc ?? '(no record)',
            condition: line.condition,
            category: '—',
          };
          return;
        }
        const part = data.part;
        rows[i] = {
          pn: line.pn,
          qty: line.qty,
          description: part?.description ?? line.desc ?? '(no record)',
          condition: line.condition,
          category: part?.category ?? '—',
        };
      } catch {
        rows[i] = {
          pn: line.pn, qty: line.qty,
          description: '(lookup failed)', condition: line.condition,
          category: '—',
        };
      }
    };
    const workers = Array.from({ length: Math.min(CONCURRENCY, result.lines.length) }, async () => {
      for (;;) {
        const i = next++;
        if (i >= result.lines.length) return;
        await enrichOne(i);
      }
    });
    await Promise.all(workers);
    setRowData(rows);
    setLoading(false);
  };

  // Clicking anywhere on a row opens its technical data sheet in the right
  // panel; the row is also checked into the export selection (multi).
  const onRowClicked = (e: RowClickedEvent<GridRow>) => {
    if (!e.data) return;
    openTds(e.data.pn);
  };

  const openTds = (pn: string) => {
    setActivePn(pn);
    setTds(null);
    setTdsLoading(true);
    fetch(`http://127.0.0.1:7878/tds/${encodeURIComponent(pn)}`)
      .then((r) => r.json())
      .then((b) => { if (b.success && b.data) setTds(b.data); })
      .catch(() => {})
      .finally(() => setTdsLoading(false));
  };

  const onSelectionChanged = (e: SelectionChangedEvent<GridRow>) => {
    setSelectedPns(e.api.getSelectedRows().map((r) => r.pn));
  };

  // PartsTable-style keyboard navigation: ArrowDown / ArrowUp move the
  // active row (selection follows) and open its data sheet; Enter opens it.
  const onCellKeyDown = (e: CellKeyDownEvent<GridRow>) => {
    const kev = e.event as KeyboardEvent | undefined;
    if (!kev) return;
    if (kev.key !== 'ArrowDown' && kev.key !== 'ArrowUp' && kev.key !== 'Enter') return;
    kev.preventDefault();
    kev.stopPropagation();
    const api = e.api;
    const displayed: GridRow[] = [];
    api.forEachNodeAfterFilterAndSort((n) => { if (n.data) displayed.push(n.data); });
    if (displayed.length === 0) return;
    const curIdx = activePn ? displayed.findIndex((r) => r.pn === activePn) : -1;
    let nextIdx: number;
    if (kev.key === 'Enter') {
      if (curIdx < 0) return;
      openTds(displayed[curIdx].pn);
      return;
    }
    if (curIdx < 0) nextIdx = 0;
    else nextIdx = kev.key === 'ArrowDown' ? Math.min(curIdx + 1, displayed.length - 1) : Math.max(curIdx - 1, 0);
    if (nextIdx === curIdx) return;
    api.deselectAll();
    const node = api.getRowNode(displayed[nextIdx].pn);
    if (!node) return;
    node.setSelected(true);
    if (node.rowIndex != null) api.ensureIndexVisible(node.rowIndex, 'middle');
    openTds(displayed[nextIdx].pn);
  };

  // Export the ticked rows; with nothing ticked, the whole list. The server
  // re-enriches from production so the file matches the grid.
  const exportXLSX = async () => {
    if (!text.trim() || exporting) return;
    setExporting(true);
    setExportError('');
    try {
      const r = await fetch('http://127.0.0.1:7878/paste/export', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ paste: text, selected: selectedPns }),
      });
      if (!r.ok) throw new Error(`HTTP ${r.status}`);
      downloadBlob(await r.blob(), 'partstable-list.xlsx');
    } catch {
      setExportError('Export failed — check your connection and try again.');
    } finally {
      setExporting(false);
    }
  };

  return (
    <div className="view-paste">
      <p className="lede">Paste part numbers below, one per line.</p>
      <textarea
        value={text}
        onChange={(e) => {
          setText(e.target.value);
          setPartCount(smartParseDetailed(e.target.value).lines.length);
        }}
        rows={6}
        className="paste-input"
        placeholder={'02CL197 x4\n4X70J67435, 2\nSN730SDB512GB'}
      />
      <div className="actions">
        <span className="part-count">{partCount} part{partCount !== 1 ? 's' : ''}</span>
        <button className="btn-primary" onClick={() => void parseAndLookup()} disabled={loading}>
          {loading ? 'Looking up…' : 'Look up list'}
        </button>
        <button
          className="btn-secondary"
          onClick={() => void exportXLSX()}
          disabled={rowData.length === 0 || exporting}
          title={selectedPns.length > 0 ? `Export the ${selectedPns.length} ticked row(s)` : 'Export all rows'}
        >
          {exporting ? 'Exporting…' : selectedPns.length > 0 ? `Export to Excel (${selectedPns.length})` : 'Export to Excel'}
        </button>
        {exportError && <span className="export-error">{exportError}</span>}
      </div>
      {warnings.length > 0 && (
        <div className="panel-warn">
          <strong>{warnings.length} line{warnings.length === 1 ? '' : 's'} need attention</strong> — nothing dropped:
          <ul>{warnings.map((w, i) => <li key={i}>{w}</li>)}</ul>
        </div>
      )}
      {rowData.length > 0 && (
        <div className="paste-split">
          <div className="paste-grid">
            <AgGridReact
              columnDefs={colDefs}
              rowData={rowData}
              theme={gridTheme}
              getRowId={(p) => String(p.data.pn)}
              rowSelection={{
                mode: 'multiRow',
                checkboxes: true,
                headerCheckbox: true,
                enableClickSelection: true,
              }}
              onRowClicked={onRowClicked}
              onSelectionChanged={onSelectionChanged}
              onCellKeyDown={onCellKeyDown}
              onGridReady={(p) => { (window as unknown as Record<string, unknown>).__ptGrid = p.api; }}
              pagination
              paginationPageSize={50}
            />
          </div>
          <div className="paste-tds">
            {tdsLoading && activePn && <p className="statrow">Loading data sheet for {activePn}…</p>}
            {tds && <TdsSheet tds={tds} />}
            {!tds && !tdsLoading && activePn && (
              <div className="tds-card">
                <h1 className="tds-title" style={{ fontSize: 20 }}>{activePn}</h1>
                <p className="tds-overview">No data sheet available for this part.</p>
              </div>
            )}
            {!activePn && (
              <p className="statrow">Click a row (or use ↑/↓) to open its technical data sheet here.</p>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
