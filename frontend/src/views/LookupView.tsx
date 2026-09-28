import { useState, useRef, useCallback } from 'react';
import { API } from '../api';

interface CatalogHit {
  partNumber: string;
  normalizedPn: string;
  description: string;
  manufacturer: string | null;
  category1: string | null;
  category2: string | null;
  listPrice: string;
  lastCost: string;
}

interface Sub {
  partNumber: string;
  description: string;
  relationshipType: string;
  confidence: number;
  matchGrade: string;
  manufacturer: string | null;
  matchSource: string;
  sources: string[];
}

interface TDS {
  partNumber: string;
  manufacturer: string;
  description: string;
  shortDescription: string;
  category: string;
  category2: string;
  partCategory: string;
  brand: string;
  dataConfidence: number;
  confidence: number;
  confidenceSources: string[];
  lifecycle: { status: string };
  substitutes: Sub[];
  oemLink: { url: string; label: string };
  brokerGuidance: { tips: string[]; warnings: string[]; mistakes: string[] };
}

function Shield({ confidence }: { confidence: number }) {
  const color = confidence >= 70 ? '#059669' : confidence >= 40 ? '#D97706' : '#DC2626';
  const label = confidence >= 70 ? 'HIGH' : confidence >= 40 ? 'MODERATE' : 'LOW';
  return (
    <span style={{ display: 'inline-flex', alignItems: 'center', gap: 4 }}>
      <svg width="14" height="14" viewBox="0 0 16 16" fill="none">
        <path d="M8 1L2 4V7.5C2 11.1 4.5 14.4 8 15.5C11.5 14.4 14 11.1 14 7.5V4L8 1Z"
          fill={color} stroke={color} strokeWidth={1.3} />
        <path d="M5.5 8.2L7 9.8L10.5 6.2" stroke="#FFFFFF" strokeWidth={1.5}
          strokeLinecap="round" strokeLinejoin="round" />
      </svg>
      <span style={{ fontSize: 10, fontWeight: 600, color, textTransform: 'uppercase' }}>{label}</span>
    </span>
  );
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
        <div className="card">
          <div className="card-head">
            <h2 className="mono">{selected.partNumber}</h2>
            {selected.category1 && <span className="tag">{selected.category1}{selected.category2 ? ` · ${selected.category2}` : ''}</span>}
            {selected.manufacturer && <span className="tag">{selected.manufacturer}</span>}
          </div>
          <p className="desc">{selected.description}</p>
          <table><tbody>
            <tr><td>List price</td><td className="num">{selected.listPrice}</td></tr>
            <tr><td>Last cost</td><td className="num">{selected.lastCost}</td></tr>
          </tbody></table>

          {tdsLoading && <p className="statrow">Loading full data…</p>}
          {tds && (
            <div className="tds-section">
              <h3>Data confidence</h3>
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <Shield confidence={tds.dataConfidence} />
                <span className="muted" style={{ fontSize: 11 }}>
                  {tds.confidenceSources.join(' · ')}
                </span>
              </div>

              {tds.lifecycle?.status && tds.lifecycle.status !== 'Unknown' && (
                <>
                  <h3>Lifecycle</h3>
                  <p>{tds.lifecycle.status}</p>
                </>
              )}

              {tds.substitutes.length > 0 && (
                <>
                  <h3>Substitutes ({tds.substitutes.length})</h3>
                  <table><thead><tr>
                    <th>Part Number</th><th>Relationship</th><th>Grade</th><th>Confidence</th><th>Sources</th>
                  </tr></thead><tbody>
                    {tds.substitutes.map((s) => (
                      <tr key={s.partNumber}>
                        <td className="mono" style={{ fontWeight: 600 }}>{s.partNumber}</td>
                        <td>{s.relationshipType}</td>
                        <td>{s.matchGrade}</td>
                        <td className="num">{Math.round(s.confidence * 100)}%</td>
                        <td>{s.sources.join(', ')}</td>
                      </tr>
                    ))}
                  </tbody></table>
                </>
              )}

              {tds.oemLink?.url && (
                <p style={{ marginTop: 'var(--space-2)' }}>
                  <a href={tds.oemLink.url} target="_blank" rel="noreferrer" className="oem-link">
                    {tds.oemLink.label || 'View on manufacturer site'} →
                  </a>
                </p>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
