import { useState, useRef, useCallback } from 'react';
import { API } from '../api';

// Production catalog hit — the exact shape from partstable.com/api/v1/parts/search
export interface CatalogHit {
  partNumber: string;
  normalizedPn: string;
  description: string;
  manufacturer: string | null;
  category1: string | null;
  category2: string | null;
  listPrice: string;
  lastCost: string;
}

/** V5 PartSearchInput dropdown spec: rows show PN (mono 600) +
 * manufacturer (bold #6C757D) + description (11px), highlight #F0F4FF. */
function HighlightedPN({ pn, query }: { pn: string; query: string }) {
  const idx = pn.toUpperCase().indexOf(query.toUpperCase());
  if (idx < 0) return <b style={{ fontFamily: 'var(--font-mono)', fontSize: 12.5, fontWeight: 600 }}>{pn}</b>;
  return (
    <b style={{ fontFamily: 'var(--font-mono)', fontSize: 12.5, fontWeight: 600 }}>
      {pn.slice(0, idx)}
      <span style={{ color: 'var(--color-primary)' }}>{pn.slice(idx, idx + query.length)}</span>
      {pn.slice(idx + query.length)}
    </b>
  );
}

export default function LookupView() {
  const [query, setQuery] = useState('');
  const [hits, setHits] = useState<CatalogHit[]>([]);
  const [showPop, setShowPop] = useState(false);
  const [activeIdx, setActiveIdx] = useState(-1);
  const [selected, setSelected] = useState<CatalogHit | null>(null);
  const [status, setStatus] = useState('Type 2+ characters — live search of the PartsTable catalog.');
  const abortRef = useRef<AbortController | null>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const seqRef = useRef(0);
  const boxRef = useRef<HTMLDivElement>(null);

  const doTypeahead = useCallback((q: string) => {
    if (debounceRef.current) clearTimeout(debounceRef.current);
    if (q.trim().length < 2) { setShowPop(false); setStatus('Type 2+ characters — live search of the PartsTable catalog.'); return; }
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
          ? `${results.length} match${results.length === 1 ? '' : 'es'} · ${body.source === 'production' ? 'live catalog' : 'local compendium'}`
          : 'no match in the catalog');
      } catch (e: unknown) {
        if (e instanceof DOMException && e.name === 'AbortError') return;
        if (seq === seqRef.current) setStatus('search hiccup — try again');
      }
    }, 300); // V5 PartSearchInput debounce: 300ms
  }, []);

  const selectHit = (idx: number) => {
    setSelected(hits[idx]);
    setShowPop(false);
    setQuery(hits[idx].partNumber);
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Escape') { setShowPop(false); return; }
    if (!showPop || hits.length === 0) return;
    if (e.key === 'ArrowDown') { e.preventDefault(); setActiveIdx((i) => Math.min(i + 1, hits.length - 1)); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setActiveIdx((i) => Math.max(i - 1, 0)); }
    else if (e.key === 'Enter') { e.preventDefault(); if (activeIdx >= 0) selectHit(activeIdx); }
  };

  return (
    <div className="view-lookup" ref={boxRef}>
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

      {/* V5 PartSearchInput dropdown: absolute panel, rows = PN + mfr + desc */}
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
                <span className="pop-pn"><HighlightedPN pn={h.partNumber} query={query} /></span>
                <span className="pop-meta">
                  {h.manufacturer && <b style={{ color: '#6C757D' }}>{h.manufacturer}</b>}
                  <span className="pop-desc">{h.description}</span>
                </span>
                {h.listPrice !== '0.00' && h.listPrice && (
                  <span className="pop-price mono">${h.listPrice}</span>
                )}
              </div>
            ))
          )}
        </div>
      )}

      <p className="statrow">{status}</p>

      {/* Selected part detail — the production data block */}
      {selected && (
        <div className="card">
          <div className="card-head">
            <h2 className="mono">{selected.partNumber}</h2>
            {selected.category1 && <span className="tag">{selected.category1}{selected.category2 ? ` · ${selected.category2}` : ''}</span>}
            {selected.manufacturer && <span className="tag">{selected.manufacturer}</span>}
          </div>
          <p className="desc">{selected.description}</p>
          <table>
            <tbody>
              <tr><td>List price</td><td className="num">{selected.listPrice}</td></tr>
              <tr><td>Last cost</td><td className="num">{selected.lastCost}</td></tr>
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
