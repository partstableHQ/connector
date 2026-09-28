import { useState } from 'react';
import { ModuleRegistry, AllCommunityModule, themeQuartz, type ColDef } from 'ag-grid-community';
import { AgGridReact } from 'ag-grid-react';
import { smartParseDetailed } from '../lib/smartParse';

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
  condition: string;
  manufacturer: string;
  substitutes: string;
  holders: string;
}

const colDefs: ColDef<GridRow>[] = [
  { field: 'pn', headerName: 'Part Number', width: 160, cellClass: 'mono-cell' },
  { field: 'qty', headerName: 'Qty', width: 60, type: 'rightAligned' },
  { field: 'description', headerName: 'Description', flex: 1, minWidth: 200 },
  { field: 'condition', headerName: 'Condition', width: 85 },
  { field: 'manufacturer', headerName: 'Manufacturer', width: 130 },
  { field: 'substitutes', headerName: 'Substitutes', flex: 1, minWidth: 180 },
  { field: 'holders', headerName: 'Holders', flex: 1, minWidth: 180 },
];

export default function PasteView() {
  const [text, setText] = useState('');
  const [partCount, setPartCount] = useState(0);
  const [loading, setLoading] = useState(false);
  const [warnings, setWarnings] = useState<string[]>([]);
  const [rowData, setRowData] = useState<GridRow[]>([]);

  const parseAndLookup = async () => {
    if (!text.trim()) return;
    setLoading(true);
    setWarnings([]);
    setRowData([]);

    const result = smartParseDetailed(text);
    if (result.lines.length === 0) {
      setWarnings(['No part numbers detected. Paste an RFQ, BOM, email, or part list.']);
      setLoading(false);
      return;
    }

    const rows: GridRow[] = [];
    for (const line of result.lines) {
      try {
        const r = await fetch(`http://127.0.0.1:7878/lookup?pn=${encodeURIComponent(line.pn)}`);
        if (!r.ok) {
          rows.push({
            pn: line.pn, qty: line.qty,
            description: line.desc ?? '(no record)',
            condition: line.condition,
            manufacturer: '—', substitutes: '—', holders: '—',
          });
          continue;
        }
        const data = await r.json();
        const part = data.part;
        rows.push({
          pn: line.pn,
          qty: line.qty,
          description: part?.description ?? line.desc ?? '(no record)',
          condition: line.condition,
          manufacturer: part?.category ?? '—',
          substitutes: part?.xrefs.map((x: { to_pn: string; kind: string }) => `${x.to_pn} ${x.kind}`).join(', ') || '—',
          holders: part?.holders.map((h: { holder: string; qty: number }) => `${h.holder} (${h.qty})`).join(', ') || '—',
        });
      } catch {
        rows.push({
          pn: line.pn, qty: line.qty,
          description: '(lookup failed)', condition: line.condition,
          manufacturer: '—', substitutes: '—', holders: '—',
        });
      }
    }
    setRowData(rows);
    setLoading(false);
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
        rows={8}
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
