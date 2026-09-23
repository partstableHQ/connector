import { useState, useRef, useCallback } from 'react';
import { API, type LookupResult, type SearchHit } from '../api';

function Chip({ source, detail }: { source: string; detail?: string }) {
  const labels: Record<string, string> = {
    oem: 'OEM', government_registry: 'GOV REGISTRY', broker_verified: 'BROKER-VERIFIED',
    partner: 'PARTNER', certified: 'CERTIFIED ★',
  };
  return (
    <span className={`chip chip-${source}`} title={detail || source}>
      {labels[source] ?? source.toUpperCase()}
    </span>
  );
}

export default function LookupView() {
  const [query, setQuery] = useState('');
  const [hits, setHits] = useState<SearchHit[]>([]);
  const [showPop, setShowPop] = useState(false);
  const [activeIdx, setActiveIdx] = useState(-1);
  const [result, setResult] = useState<LookupResult | null>(null);
  const [status, setStatus] = useState('Type 2+ characters to search.');
  const abortRef = useRef<AbortController | null>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const seqRef = useRef(0);

  const doTypeahead = useCallback((q: string) => {
    if (q.length < 2) { setShowPop(false); return; }
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(async () => {
      abortRef.current?.abort();
      abortRef.current = new AbortController();
      const seq = ++seqRef.current;
      setStatus('searching…');
      try {
        const r = await fetch(`${API}/search?q=${encodeURIComponent(q)}`, { signal: abortRef.current.signal });
        if (seq !== seqRef.current) return;
        const body = await r.json();
        setHits(body.results ?? []);
        setShowPop(true);
        setActiveIdx(-1);
        setStatus(body.results?.length ? `${body.results.length} match${body.results.length === 1 ? '' : 'es'}` : '0 results');
      } catch (e: unknown) {
        if (e instanceof DOMException && e.name === 'AbortError') return;
        if (seq === seqRef.current) setStatus('search error');
      }
    }, 150);
  }, []);

  const doLookup = useCallback(async (pn: string) => {
    if (!pn) return;
    setStatus(`Looking up ${pn}…`);
    setShowPop(false);
    try {
      const r = await fetch(`${API}/lookup?pn=${encodeURIComponent(pn)}`);
      if (!r.ok) {
        const err = await r.json().catch(() => ({ error: r.statusText }));
        setStatus(err.error ?? 'error');
        setResult(null);
        return;
      }
      const body: LookupResult = await r.json();
      setResult(body);
      setStatus('');
    } catch { setStatus('lookup failed'); setResult(null); }
  }, []);

  const selectHit = (idx: number) => {
    setQuery(hits[idx].pn);
    setShowPop(false);
    void doLookup(hits[idx].pn);
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (!showPop || hits.length === 0) {
      if (e.key === 'Enter') { e.preventDefault(); void doLookup(query); }
      return;
    }
    if (e.key === 'ArrowDown') { e.preventDefault(); setActiveIdx((i) => Math.min(i + 1, hits.length - 1)); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setActiveIdx((i) => Math.max(i - 1, 0)); }
    else if (e.key === 'Enter') {
      e.preventDefault();
      if (activeIdx >= 0) selectHit(activeIdx);
      else { setShowPop(false); void doLookup(query); }
    } else if (e.key === 'Escape') setShowPop(false);
  };

  return (
    <div className="view-lookup">
      <div className="search-wrap">
        <div className="sbox">
          <input
            ref={(el) => { if (el) (window as any).__pnInput = el; }}
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
        <button className="sbtn" onClick={() => void doLookup(query)}>Look up</button>
      </div>

      {showPop && (
        <div className="lookup-pop" role="listbox">
          {hits.length === 0 ? (
            <div className="pop-empty">No match in the documented set.</div>
          ) : (
            hits.map((h, i) => (
              <div
                key={h.pn}
                className={`pop-row ${i === activeIdx ? 'active' : ''}`}
                onClick={() => selectHit(i)}
                role="option"
                aria-selected={i === activeIdx}
              >
                <span className="pop-pn">{h.display_pn}</span>
                <span className="pop-desc">{h.description}</span>
              </div>
            ))
          )}
        </div>
      )}

      <div className="chip-row">
        <span className="chip-label">Try:</span>
        {['02CL197', '4X70J67435', 'SN730SDB512GB'].map((q) => (
          <button key={q} className="ex-chip mono" onClick={() => { setQuery(q); void doLookup(q); }}>{q}</button>
        ))}
      </div>
      <p className="statrow">{status}</p>
      <div aria-live="polite">
        {result && <ResultCard res={result} />}
      </div>
    </div>
  );
}

function ResultCard({ res }: { res: LookupResult }) {
  const p = res.part;
  if (!p) {
    return (
      <div className="card">
        <h2 className="mono">{res.normalized}</h2>
        <p className="muted">No record. Entered as <span className="mono">{res.query}</span>.</p>
      </div>
    );
  }
  return (
    <div className="card">
      <div className="card-head">
        <h2 className="mono">{p.display_pn}</h2>
        {p.category && <span className="tag">{p.category}</span>}
        {res.matched_by === 'alias' && (
          <span className="match-note">via alias — <span className="mono">{p.pn}</span></span>
        )}
      </div>
      <p className="desc">{p.description}</p>

      <h3>Cross-references</h3>
      {p.xrefs.length > 0 ? (
        <table><thead><tr><th>Part</th><th>Kind</th><th>Source</th></tr></thead>
          <tbody>{p.xrefs.map((x) => (
            <tr key={x.to_pn}>
              <td className="mono">{x.to_pn}</td><td>{x.kind}</td>
              <td><Chip source={x.source} detail={x.source_detail} /></td>
            </tr>))}</tbody></table>
      ) : <p className="muted">None on record.</p>}

      <h3>Who holds it</h3>
      {p.holders.length > 0 ? (
        <table><thead><tr><th>Holder</th><th>Qty</th><th>Condition</th><th>Last seen</th><th>Source</th></tr></thead>
          <tbody>{p.holders.map((h, i) => (
            <tr key={i}>
              <td>{h.holder}</td><td className="num">{h.qty}</td><td>{h.condition}</td>
              <td className="mono">{h.last_seen}</td><td><Chip source={h.source} detail={h.source_detail} /></td>
            </tr>))}</tbody></table>
      ) : <p className="muted">None on record.</p>}
    </div>
  );
}
