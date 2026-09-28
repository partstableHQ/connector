import { useState } from 'react';
import {
  ModuleRegistry, AllCommunityModule, themeQuartz,
  type ColDef, type SelectionChangedEvent,
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
// full data sheet live in the right-hand panel for the selected row.
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

export default function PasteView() {
  const [text, setText] = useState('');
  const [partCount, setPartCount] = useState(0);
  const [loading, setLoading] = useState(false);
  const [warnings, setWarnings] = useState<string[]>([]);
  const [rowData, setRowData] = useState<GridRow[]>([]);
  const [selectedPn, setSelectedPn] = useState<string | null>(null);
  const [tds, setTds] = useState<TDS | null>(null);
  const [tdsLoading, setTdsLoading] = useState(false);

  const parseAndLookup = async () => {
    if (!text.trim()) return;
    setLoading(true);
    setWarnings([]);
    setRowData([]);
    setSelectedPn(null);
    setTds(null);

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

  // Selecting a row opens its technical data sheet in the right panel —
  // the quote-builder flow: rows on the left, full TDS on the right.
  const onSelectionChanged = (e: SelectionChangedEvent<GridRow>) => {
    const row = e.api.getSelectedRows()[0];
    if (!row) return;
    setSelectedPn(row.pn);
    setTds(null);
    setTdsLoading(true);
    fetch(`http://127.0.0.1:7878/tds/${encodeURIComponent(row.pn)}`)
      .then((r) => r.json())
      .then((b) => { if (b.success && b.data) setTds(b.data); })
      .catch(() => {})
      .finally(() => setTdsLoading(false));
  };

  const exportXLSX = async () => {
    if (!text.trim()) return;
    try {
      const r = await fetch('http://127.0.0.1:7878/paste/export', {
        method: 'POST', headers: { 'Content-Type': 'text/plain' }, body: text,
      });
      if (!r.ok) return;
      const blob = await r.blob();
      const a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = 'partstable-list.xlsx';
      a.click();
      URL.revokeObjectURL(a.href);
    } catch { /* silent */ }
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
        <button className="btn-secondary" onClick={() => void exportXLSX()} disabled={rowData.length === 0}>
          Export to Excel
        </button>
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
              rowSelection={{ mode: 'singleRow' }}
              onSelectionChanged={onSelectionChanged}
              onGridReady={(p) => { (window as unknown as Record<string, unknown>).__ptGrid = p.api; }}
              pagination
              paginationPageSize={50}
            />
          </div>
          <div className="paste-tds">
            {tdsLoading && selectedPn && <p className="statrow">Loading data sheet for {selectedPn}…</p>}
            {tds && <TdsSheet tds={tds} />}
            {!tds && !tdsLoading && selectedPn && (
              <div className="tds-card">
                <h1 className="tds-title" style={{ fontSize: 20 }}>{selectedPn}</h1>
                <p className="tds-overview">No data sheet available for this part.</p>
              </div>
            )}
            {!selectedPn && (
              <p className="statrow">Select a row to open its technical data sheet here.</p>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
