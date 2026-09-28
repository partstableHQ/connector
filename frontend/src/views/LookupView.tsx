import { useState, useRef, useCallback } from 'react';
import { API } from '../api';
import TdsSheet, { type TDS } from '../components/TdsSheet';

interface CatalogHit {
  partNumber: string;
  description: string;
  manufacturer: string | null;
  category1: string | null;
  category2: string | null;
  listPrice: string;
  lastCost: string;
}

export default function LookupView() {
  const [query, setQuery] = useState('');
  const [hits, setHits] = useState<CatalogHit[]>([]);
  const [showPop, setShowPop] = useState(false);
  const [activeIdx, setActiveIdx] = useState(-1);
  const [selected, setSelected] = useState<CatalogHit | null>(null);
  const [tds, setTds] = useState<TDS | null>(null);
  const [tdsLoading, setTdsLoading] = useState(false);
  const [status, setStatus] = useState('Type 2+ characters — live search of the PartsTable catalog.');
  const abortRef = useRef<AbortController | null>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const seqRef = useRef(0);

  const doTypeahead = useCallback((q: string) => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    if (q.trim().length < 2) { setShowPop(false); setStatus('Type 2+ characters — live catalog search.'); return; }
    debounceRef.current = setTimeout(async () => {
      abortRef.current?.abort();
      abortRef.current = new AbortController();
      const seq = ++seqRef.current;
      setStatus('searching…');
      try {
        const r = await fetch(`${API}/catalog/search?q=${encodeURIComponent(q)}&limit=8`, { signal: abortRef.current.signal });
        if (seq !== seqRef.current) return;
        const body = await r.json();
        if (!r.ok) { setStatus(body.error ?? 'search error'); return; }
        const results = (body.results ?? []) as CatalogHit[];
        setHits(results);
        setShowPop(true);
        setActiveIdx(-1);
        setStatus(results.length > 0
          ? `${results.length} match${results.length === 1 ? '' : 'es'} · live catalog`
          : 'no match in the catalog');
      } catch (e: unknown) {
        if (e instanceof DOMException && e.name === 'AbortError') return;
        if (seq === seqRef.current) setStatus('search hiccup — try again');
      }
    }, 300);
  }, []);

  const selectHit = (idx: number) => {
    const hit = hits[idx];
    setSelected(hit);
    setTds(null);
    setTdsLoading(true);
    setShowPop(false);
    setQuery(hit.partNumber);
    fetch(`${API}/tds/${encodeURIComponent(hit.partNumber)}`)
      .then((r) => r.json())
      .then((body) => {
        if (body.success && body.data) setTds(body.data);
      })
      .catch(() => {})
      .finally(() => setTdsLoading(false));
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') { setShowPop(false); return; }
    if (!showPop || hits.length === 0) return;
    if (e.key === 'ArrowDown') { e.preventDefault(); setActiveIdx((i) => Math.min(i + 1, hits.length - 1)); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setActiveIdx((i) => Math.max(i - 1, 0)); }
    else if (e.key === 'Enter') { e.preventDefault(); if (activeIdx >= 0) selectHit(activeIdx); }
  };

  // One-part Excel export: the looked-up part plus its verified substitutes,
  // straight from the same data the sheet shows.
  const exportXLSX = () => {
    if (!selected) return;
    fetch(`${API}/lookup/export?pn=${encodeURIComponent(selected.partNumber)}`)
      .then((r) => { if (!r.ok) throw new Error('export failed'); return r.blob(); })
      .then((b) => {
        const a = document.createElement('a');
        a.href = URL.createObjectURL(b);
        a.download = `partstable-${selected.partNumber}.xlsx`;
        a.click();
        URL.revokeObjectURL(a.href);
      })
      .catch(() => {});
  };

  return (
    <div className="view-lookup">
      <div className="search-wrap">
        <div className="sbox">
          <input
            id="pn"
            type="text"
            value={query}
            onChange={(e) => { setQuery(e.target.value); doTypeahead(e.target.value); }}
            onKeyDown={onKeyDown}
            placeholder="9TMRF"
            autoComplete="off"
            spellCheck={false}
            role="combobox"
            aria-expanded={showPop}
            aria-autocomplete="list"
            aria-label="Part number"
          />
          <span className="kbd">Ctrl K</span>
        </div>
      </div>

      {showPop && (
        <div className="lookup-pop" role="listbox">
          {hits.length === 0 ? (
            <div className="pop-empty">No parts found</div>
          ) : (
            hits.map((h, i) => (
              <div
                key={h.partNumber + String(i)}
                className="pop-row"
                style={{ background: i === activeIdx ? '#F0F4FF' : undefined }}
                onMouseEnter={() => setActiveIdx(i)}
                onClick={() => selectHit(i)}
                role="option"
                aria-selected={i === activeIdx}
              >
                <span className="pop-pn">{h.partNumber}</span>
                <span className="pop-meta">
                  {h.manufacturer && <b>{h.manufacturer}</b>}
                  <span className="pop-desc">{h.description}</span>
                </span>
              </div>
            ))
          )}
        </div>
      )}

      <p className="statrow">{status}</p>

      {selected && (
        <div className="actions" style={{ marginTop: 'var(--space-2)' }}>
          <button className="btn-secondary" onClick={exportXLSX}>Export to Excel</button>
          <span className="statrow" style={{ margin: 0 }}>part + verified substitutes, .xlsx</span>
        </div>
      )}

      {tdsLoading && selected && (
        <div className="tds-card"><p className="statrow">Loading technical data sheet…</p></div>
      )}

      {tds && <TdsSheet tds={tds} />}

      {/* Fallback when no TDS data: simple card */}
      {selected && !tds && !tdsLoading && (
        <div className="tds-card">
          <h1 className="tds-title" style={{ fontSize: 22 }}>{selected.partNumber}</h1>
          <p className="tds-overview">{selected.description}</p>
        </div>
      )}
    </div>
  );
}
