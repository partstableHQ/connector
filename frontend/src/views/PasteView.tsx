import { useState } from 'react';
import { ModuleRegistry, AllCommunityModule, themeQuartz, type ColDef } from 'ag-grid-community';
import { AgGridReact } from 'ag-grid-react';
import { API, type PasteResponse } from '../api';

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

interface GridRow {
  pn: string;
  qty: number;
  description: string;
  substitutes: string;
  holders: string;
}

const colDefs: ColDef<GridRow>[] = [
  { field: 'pn', headerName: 'Part Number', width: 150, cellClass: 'mono-cell' },
  { field: 'qty', headerName: 'Qty', width: 60, type: 'rightAligned' },
  { field: 'description', headerName: 'Description', flex: 1, minWidth: 200 },
  { field: 'substitutes', headerName: 'Substitutes', flex: 1, minWidth: 180 },
  { field: 'holders', headerName: 'Holders', flex: 1, minWidth: 180 },
];

export default function PasteView() {
  const [text, setText] = useState('');
  const [warnings, setWarnings] = useState<PasteResponse['warnings']>([]);
  const [loading, setLoading] = useState(false);
  const [rowData, setRowData] = useState<GridRow[]>([]);

  const parseAndLookup = async () => {
    if (!text.trim()) return;
    setLoading(true);
    try {
      const r = await fetch(`${API}/paste`, {
        method: 'POST',
        headers: { 'Content-Type': 'text/plain' },
        body: text,
      });
      const body: PasteResponse = await r.json();
      if (!r.ok) return;
      setWarnings(body.warnings ?? []);
      setRowData(body.entries.map((e) => ({
        pn: e.pn,
        qty: e.qty,
        description: e.result?.part?.description ?? '(no record)',
        substitutes: e.result?.part?.xrefs.map((x) => `${x.to_pn} ${x.kind} [${x.source}]`).join(', ') ?? '—',
        holders: e.result?.part?.holders.map((h) => `${h.holder} (${h.qty})`).join(', ') ?? '—',
      })));
    } catch { /* API unreachable */ } finally { setLoading(false); }
  };

  const exportXLSX = async () => {
    if (!text.trim()) return;
    try {
      const r = await fetch(`${API}/paste/export`, {
        method: 'POST',
        headers: { 'Content-Type': 'text/plain' },
        body: text,
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
      <p className="lede">
        Paste a whole list — lines, commas, CSV, quantities, quote emails.
        Nothing is dropped silently: unparseable lines come back as warnings.
      </p>
      <textarea
        value={text}
        onChange={(e) => setText(e.target.value)}
        rows={8}
        className="paste-input"
        placeholder={'02CL197 x4\n4X70J67435, 2\nSN730SDB512GB'}
      />
      <div className="actions">
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
          <ul>{warnings.map((w, i) => (
            <li key={i}>line {w.line}: <span className="mono">{w.raw}</span> — {w.text}</li>
          ))}</ul>
        </div>
      )}
      {rowData.length > 0 && (
        <div style={{ height: 400, width: '100%' }}>
          <AgGridReact
            columnDefs={colDefs}
            rowData={rowData}
            theme={gridTheme}
            pagination
            paginationPageSize={50}
          />
        </div>
      )}
    </div>
  );
}
