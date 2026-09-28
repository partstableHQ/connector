// ComponentDiagram renders the production TDS template diagram locally:
// production's /tds payload carries templateMapping (drive-sff, psu,
// storage-hdd, generic-component) and svgParams — the diagram draws those
// real fields and nothing else (no invented data). Brand-styled, works in
// light and dark themes via CSS variables.

interface SvgParams {
  partNumber?: string;
  manufacturer?: string;
  description?: string;
  capacity?: string;
  speed?: string;
  formFactor?: string;
  wattage?: string;
  efficiency?: string;
  [k: string]: string | undefined;
}

interface TemplateMapping {
  template?: string;
  modelType?: string;
}

const INK = 'var(--text-primary, #0F1923)';
const MUTED = 'var(--text-muted, #64748B)';
const LINE = 'var(--border-default, #E5E7EB)';
const PANEL = 'var(--bg-secondary, #F8F9FA)';
const BLUE = '#0055DD';

const MonoText = ({ x, y, size = 11, fill = MUTED, anchor = 'start', children }: {
  x: number; y: number; size?: number; fill?: string; anchor?: 'start' | 'middle' | 'end'; children?: React.ReactNode;
}) => (
  <text x={x} y={y} fontSize={size} fill={fill} textAnchor={anchor}
    style={{ fontFamily: "'JetBrains Mono', Consolas, monospace", fontWeight: 600 }}>
    {children}
  </text>
);

function DriveDiagram({ p, large }: { p: SvgParams; large: boolean }) {
  // 2.5" SFF drive when SFF/absent, wider 3.5" LFF shape when LFF.
  const lff = /lff/i.test(p.formFactor ?? '');
  const w = lff ? 340 : 300;
  const h = lff ? 190 : 170;
  const cx = w / 2;
  return (
    <svg viewBox={`0 0 ${w} ${h}`} className="diagram-svg" role="img"
      aria-label={`${p.manufacturer ?? ''} ${lff ? '3.5 inch' : '2.5 inch'} drive diagram`.trim()}>
      {/* chassis */}
      <rect x={14} y={16} width={w - 28} height={h - 32} rx={10}
        fill={PANEL} stroke={LINE} strokeWidth={1.5} />
      {/* top label strip */}
      <line x1={14} y1={52} x2={w - 14} y2={52} stroke={LINE} strokeWidth={1} />
      <MonoText x={26} y={40} size={10}>{(p.manufacturer ?? '').toUpperCase()}</MonoText>
      <MonoText x={w - 26} y={40} size={10} anchor="end">{p.formFactor ?? (large ? 'SFF 2.5"' : '')}</MonoText>
      {/* connector block */}
      <g>
        <rect x={26} y={70} width={54} height={h - 96} rx={5}
          fill="var(--bg-primary, #FFFFFF)" stroke={LINE} strokeWidth={1.2} />
        {[0, 1, 2, 3, 4, 5].map((i) => (
          <rect key={i} x={32} y={76 + i * 13} width={42} height={7} rx={2} fill={LINE} />
        ))}
      </g>
      {/* capacity hero */}
      <text x={cx + 28} y={h / 2 + 2} textAnchor="middle" fontSize={30} fontWeight={700} fill={INK}
        style={{ fontFamily: "'JetBrains Mono', Consolas, monospace" }}>
        {p.capacity ?? '—'}
      </text>
      {p.speed && <MonoText x={cx + 28} y={h / 2 + 24} anchor="middle">{p.speed}</MonoText>}
      <MonoText x={w - 26} y={h - 28} size={10} anchor="end">{p.partNumber}</MonoText>
    </svg>
  );
}

function PsuDiagram({ p }: { p: SvgParams }) {
  return (
    <svg viewBox="0 0 320 170" className="diagram-svg" role="img"
      aria-label={`${p.manufacturer ?? ''} power supply diagram`.trim()}>
      {/* chassis */}
      <rect x={14} y={16} width={292} height={138} rx={10}
        fill={PANEL} stroke={LINE} strokeWidth={1.5} />
      {/* fan */}
      <circle cx={78} cy={85} r={42} fill="var(--bg-primary, #FFFFFF)" stroke={LINE} strokeWidth={1.4} />
      <circle cx={78} cy={85} r={12} fill="none" stroke={LINE} strokeWidth={1.2} />
      {[0, 60, 120, 180, 240, 300].map((a) => (
        <line key={a}
          x1={78 + 14 * Math.cos((a * Math.PI) / 180)} y1={85 + 14 * Math.sin((a * Math.PI) / 180)}
          x2={78 + 38 * Math.cos((a * Math.PI) / 180)} y2={85 + 38 * Math.sin((a * Math.PI) / 180)}
          stroke={LINE} strokeWidth={5} strokeLinecap="round" />
      ))}
      {/* ratings */}
      <text x={250} y={80} textAnchor="middle" fontSize={32} fontWeight={700} fill={INK}
        style={{ fontFamily: "'JetBrains Mono', Consolas, monospace" }}>
        {p.wattage ?? '—'}
      </text>
      {p.efficiency && (
        <g>
          <rect x={196} y={96} width={108} height={22} rx={11} fill="none" stroke={BLUE} strokeWidth={1.4} />
          <text x={250} y={111} textAnchor="middle" fontSize={11} fontWeight={700} fill={BLUE}>
            {p.efficiency.toUpperCase()}
          </text>
        </g>
      )}
      <MonoText x={26} y={40} size={10}>{(p.manufacturer ?? '').toUpperCase()}</MonoText>
      <MonoText x={294} y={144} size={10} anchor="end">{p.partNumber}</MonoText>
    </svg>
  );
}

function GenericDiagram({ p }: { p: SvgParams }) {
  return (
    <svg viewBox="0 0 320 170" className="diagram-svg" role="img"
      aria-label="Component diagram">
      {/* package */}
      <rect x={95} y={38} width={130} height={94} rx={10}
        fill={PANEL} stroke={LINE} strokeWidth={1.5} />
      {/* pins on both sides */}
      {[0, 1, 2, 3, 4].map((i) => (
        <g key={i}>
          <rect x={75} y={50 + i * 17} width={20} height={8} rx={2} fill={LINE} />
          <rect x={225} y={50 + i * 17} width={20} height={8} rx={2} fill={LINE} />
        </g>
      ))}
      <text x={160} y={90} textAnchor="middle" fontSize={16} fontWeight={700} fill={INK}
        style={{ fontFamily: "'JetBrains Mono', Consolas, monospace" }}>
        {p.partNumber}
      </text>
      <MonoText x={160} y={110} size={9} anchor="middle">{(p.manufacturer ?? '').toUpperCase()}</MonoText>
      <MonoText x={26} y={40} size={10}>{'COMPONENT'}</MonoText>
    </svg>
  );
}

export default function ComponentDiagram({ template, params }: {
  template?: string;
  params?: SvgParams;
}) {
  if (!params || !params.partNumber) return null;
  let body: React.ReactElement;
  if (template === 'psu') body = <PsuDiagram p={params} />;
  else if (template === 'drive-sff' || template === 'storage-hdd') {
    body = <DriveDiagram p={params} large={template === 'storage-hdd'} />;
  } else body = <GenericDiagram p={params} />;
  return (
    <div className="tds-card">
      <div className="tds-h">Component diagram</div>
      <div className="diagram-wrap">{body}</div>
      {params.description && (
        <div className="diagram-caption">{params.description}</div>
      )}
    </div>
  );
}

export type { TemplateMapping };
