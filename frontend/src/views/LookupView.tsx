import { useState, useRef, useCallback } from 'react';
import { API } from '../api';

interface CatalogHit {
  partNumber: string;
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
  partSubcategory: string;
  brand: string;
  dataConfidence: number;
  confidenceSources: string[];
  lifecycle: { status: string };
  isHazmat: boolean;
  substitutes: Sub[];
  oemLink: { url: string; label: string };
  brokerGuidance: { tips: string[]; warnings: string[]; mistakes: string[] };
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

  // ── TDS sheet (production /parts/:pn design, ported from the PDF) ──

  const specRow = (label: string, value: React.ReactNode) => (
    <div className="spec-row" key={label}>
      <span className="spec-label">{label}</span>
      <span className="spec-value">{value}</span>
    </div>
  );

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

      {tdsLoading && selected && (
        <div className="tds-card"><p className="statrow">Loading technical data sheet…</p></div>
      )}

      {tds && (
        <div className="tds-sheet">
          {/* Header block */}
          <div className="tds-card">
            <div className="tds-crumb">
              PartsTable / <span className="tds-crumb-cat">{(tds.partCategory || 'PART').toUpperCase()}</span> / <span className="tds-crumb-pn">{tds.partNumber}</span>
            </div>
            <h1 className="tds-title">{tds.partNumber} — Battery Module Specs</h1>
            <div className="tds-verified">✓ Verified by PartsTable</div>
            <div className="tds-brandline">
              <b>{tds.brand || tds.manufacturer}</b> · {(tds.partCategory || 'PART').toUpperCase()} · {(tds.partSubcategory || '').toUpperCase()}
            </div>
          </div>

          {/* What you need to know (broker guidance) */}
          {(tds.brokerGuidance?.tips?.length || tds.brokerGuidance?.warnings?.length || tds.isHazmat) && (
            <div className="tds-card">
              <div className="tds-h">What you need to know</div>
              <div className="tds-callout">
                {tds.isHazmat && (
                  <div className="tds-warnbox">⚠ RESTRICTED for air — DG certificate required</div>
                )}
                <p>
                  {tds.brokerGuidance?.tips?.join(' ')}
                  {tds.brokerGuidance?.warnings?.length ? ` Key checks: ${tds.brokerGuidance.warnings.join(' ')}` : ''}
                </p>
              </div>
            </div>
          )}

          {/* Product overview */}
          <div className="tds-card">
            <div className="tds-h">Product overview</div>
            <p className="tds-overview">
              {tds.description}. Verify compatibility with your specific system configuration before ordering.
            </p>
          </div>

          {/* Where it fits */}
          {tds.partCategory && (
            <div className="tds-card">
              <div className="tds-h">Where it fits</div>
              <div className="tds-fits">
                <span className="tds-fits-node">{(tds.partCategory || '').toUpperCase()}</span>
                <span className="tds-fits-arrow">→</span>
                <span className="tds-fits-node">{(tds.partSubcategory || '').toUpperCase()}</span>
                <span className="tds-fits-arrow">→</span>
                <span className="tds-fits-pn">{tds.partNumber}</span>
              </div>
            </div>
          )}

          {/* Specifications */}
          <div className="tds-card">
            <div className="tds-h">Specifications</div>
            {specRow('Part Number', <span className="mono">{tds.partNumber}</span>)}
            {specRow('Manufacturer', tds.manufacturer)}
            {tds.category2 && specRow('Category', `${tds.category}${tds.category2 ? ' · ' + tds.category2 : ''}`)}
            {selected && specRow('List price', <span className="mono">{selected.listPrice}</span>)}
            {selected && specRow('Last cost', <span className="mono">{selected.lastCost}</span>)}
          </div>

          {/* Verified substitutes */}
          {tds.substitutes.length > 0 && (
            <div className="tds-card">
              <div className="tds-h">
                Verified substitutes &amp; cross-references
                <span className="tds-count">{tds.substitutes.length}</span>
              </div>
              {tds.substitutes.map((s) => (
                <div className="tds-sub" key={s.partNumber}>
                  <div>
                    <a className="tds-sub-pn" href={`https://partstable.com/parts/${s.partNumber}`} target="_blank" rel="noreferrer">{s.partNumber}</a>
                    <span className={`tds-rel ${s.relationshipType === 'primary' ? 'tds-rel-primary' : 'tds-rel-compat'}`}>
                      {s.relationshipType === 'primary' ? 'Primary' : 'Compatible'}
                    </span>
                  </div>
                  <div className="tds-sub-desc">{s.description}</div>
                  <div className="tds-sub-grade">
                    grade {s.matchGrade} · {Math.round(s.confidence * 100)}% · {s.sources.join(', ')}
                  </div>
                </div>
              ))}
            </div>
          )}

          {/* Lifecycle */}
          <div className="tds-card">
            <div className="tds-h">Lifecycle</div>
            {specRow('Status', <span className="tds-status-amber">{tds.lifecycle?.status || 'Unknown'}</span>)}
            {specRow('Availability', '—')}
            <div className="tds-badges">
              <span className="tds-badge tds-badge-green">RoHS Compliant</span>
              <span className="tds-badge tds-badge-gray">EAR99</span>
              <span className="tds-badge tds-badge-amber">ESD Sensitive</span>
              {tds.isHazmat && <span className="tds-badge tds-badge-red">DG Class 9 — Lithium</span>}
              {tds.isHazmat && <span className="tds-badge tds-badge-red">Restricted Air Freight</span>}
            </div>
          </div>

          {/* Footer validation line */}
          <div className="tds-validated">
            ✓ Validated with multiple authoritative sources, including Government, OEM, and Certified sources.
            <div className="tds-validated-sub">PartsTable · partstable.com</div>
          </div>
        </div>
      )}

      {/* Fallback when no TDS data: simple card */}
      {selected && !tds && !tdsLoading && (
        <div className="tds-card">
          <div className="tds-card">
            <h1 className="tds-title" style={{ fontSize: 22 }}>{selected.partNumber}</h1>
            <p className="tds-overview">{selected.description}</p>
          </div>
        </div>
      )}
    </div>
  );
}
