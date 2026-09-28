import { useState } from 'react';
import ComponentDiagram from './ComponentDiagram';

export interface Sub {
  partNumber: string;
  description: string;
  relationshipType: string;
  confidence: number;
  matchGrade: string;
  manufacturer: string | null;
  matchSource: string;
  sources: string[];
}

export interface TDS {
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
  confidence: number; // production /tds payload: 0-100 data confidence
  confidenceSources: string[];
  lifecycle: { status: string };
  isHazmat: boolean;
  rohs?: boolean;
  substitutes: Sub[];
  oemLink?: { url: string; label: string };
  brokerGuidance: { tips: string[]; warnings: string[]; mistakes: string[] };
  templateMapping?: { template?: string; modelType?: string };
  svgParams?: Record<string, string | undefined>;
}

export function copyText(t: string) {
  try {
    void navigator.clipboard?.writeText(t);
  } catch { /* clipboard unavailable — silent */ }
}

// Copy-to-clipboard chip placed next to every part number in the sheet.
export function CopyBtn({ value }: { value: string }) {
  const [done, setDone] = useState(false);
  return (
    <button
      type="button"
      className="copy-btn"
      title={`Copy ${value}`}
      onClick={(e) => {
        e.preventDefault();
        e.stopPropagation();
        copyText(value);
        setDone(true);
        setTimeout(() => setDone(false), 1200);
      }}
    >
      {done ? '✓ copied' : 'copy'}
    </button>
  );
}

// Title-case a category/description word for the sheet title, e.g.
// "SSD" → "SSD", "battery module" → "Battery Module"; a plural category
// reads better singular in "{pn} — {word} Specs" ("Batteries" → "Battery").
const titleCase = (s: string | null | undefined): string => {
  const t = (s ?? '').trim();
  if (!t) return 'Part';
  const cased = t.replace(/\w\S*/g, (w) => (w.length > 4 && w === w.toLowerCase() ? w[0].toUpperCase() + w.slice(1) : w));
  return cased.replace(/ies$/i, 'y').replace(/([^s])s$/i, '$1');
};

const specRow = (label: string, value: React.ReactNode) => (
  <div className="spec-row" key={label}>
    <span className="spec-label">{label}</span>
    <span className="spec-value">{value}</span>
  </div>
);

// TdsSheet renders the production /parts/:pn technical data sheet: header,
// broker guidance, overview, where-it-fits, specifications (identity only —
// never pricing), verified substitutes with copy buttons, truth-only
// lifecycle, and the validation footer.
export default function TdsSheet({ tds }: { tds: TDS }) {
  const cat1 = tds.category || tds.partCategory || '';
  const cat2 = tds.category2 || tds.partSubcategory || '';
  return (
    <div className="tds-sheet">
      {/* Header block */}
      <div className="tds-card">
        <div className="tds-crumb">
          PartsTable / <span className="tds-crumb-cat">{(cat1 || 'PART').toUpperCase()}</span> / <span className="tds-crumb-pn">{tds.partNumber}</span>
        </div>
        <h1 className="tds-title">
          {tds.partNumber} — {titleCase(cat2 || cat1 || tds.description)} Specs
          <CopyBtn value={tds.partNumber} />
        </h1>
        <div className="tds-verified">✓ Verified by PartsTable</div>
        <div className="tds-brandline">
          <b>{tds.brand || tds.manufacturer}</b> · {(cat1 || 'PART').toUpperCase()}{cat2 ? ` · ${cat2.toUpperCase()}` : ''}
        </div>
      </div>

      {/* Component diagram — drawn from the production template + svgParams */}
      <ComponentDiagram template={tds.templateMapping?.template} params={tds.svgParams} />

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
      {cat1 && (
        <div className="tds-card">
          <div className="tds-h">Where it fits</div>
          <div className="tds-fits">
            <span className="tds-fits-node">{cat1.toUpperCase()}</span>
            <span className="tds-fits-arrow">→</span>
            {cat2 && (
              <>
                <span className="tds-fits-node">{cat2.toUpperCase()}</span>
                <span className="tds-fits-arrow">→</span>
              </>
            )}
            <span className="tds-fits-pn">{tds.partNumber}</span>
          </div>
        </div>
      )}

      {/* Specifications — identity and data confidence only; the Connector
          never shows pricing (CEO ruling 2026-09-28). */}
      <div className="tds-card">
        <div className="tds-h">Specifications</div>
        {specRow('Part Number', <span className="mono">{tds.partNumber}</span>)}
        {specRow('Manufacturer', tds.manufacturer)}
        {(cat1 || cat2) && specRow('Category', cat2 ? `${cat1} · ${cat2}` : cat1)}
        {typeof tds.confidence === 'number' && (
          <div className="spec-row">
            <span className="spec-label">Data confidence</span>
            <span className="spec-value">
              <span className="tds-conf-bar" aria-hidden="true"><span style={{ width: `${Math.min(100, Math.max(0, tds.confidence))}%` }} /></span>
              {tds.confidence}%
            </span>
          </div>
        )}
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
              <div className="tds-sub-head">
                <a className="tds-sub-pn" href={`https://partstable.com/parts/${s.partNumber}`} target="_blank" rel="noreferrer">{s.partNumber}</a>
                <CopyBtn value={s.partNumber} />
                <span className={`tds-rel ${s.relationshipType === 'primary' ? 'tds-rel-primary' : 'tds-rel-compat'}`}>
                  {s.relationshipType === 'primary' ? 'Primary' : 'Compatible'}
                </span>
              </div>
              {s.description && <div className="tds-sub-desc">{s.description}</div>}
              <div className="tds-sub-grade">
                grade {s.matchGrade} · {Math.round(s.confidence * 100)}% · {s.sources.join(', ')}
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Lifecycle — production payload carries status (+ compliance when
          known); nothing is rendered that the data does not assert. */}
      <div className="tds-card">
        <div className="tds-h">Lifecycle</div>
        {specRow('Status', <span className={tds.lifecycle?.status === 'active' ? 'tds-status-green' : 'tds-status-amber'}>{tds.lifecycle?.status || 'Unknown'}</span>)}
        {(tds as { rohs?: boolean }).rohs === true && (
          <div className="tds-badges"><span className="tds-badge tds-badge-green">RoHS Compliant</span></div>
        )}
        {tds.isHazmat && (
          <div className="tds-badges">
            <span className="tds-badge tds-badge-red">DG Class 9 — Lithium</span>
            <span className="tds-badge tds-badge-red">Restricted Air Freight</span>
          </div>
        )}
      </div>

      {/* Footer validation line */}
      <div className="tds-validated">
        ✓ Validated with multiple authoritative sources, including Government, OEM, and Certified sources.
        <div className="tds-validated-sub">PartsTable · partstable.com</div>
      </div>
    </div>
  );
}
