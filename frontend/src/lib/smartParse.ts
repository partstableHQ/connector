/**
 * smartParse.ts — FR-S1.1 universal paste parser (Beta 2.0, Dabergy)
 *
 * Pure, deterministic, client-side. No React, no API (NG-5: no LLM parsing).
 * Accepts any clipboard content — Excel spill (TSV + headers), CSV (comma /
 * semicolon / pipe), Word-table paste, email free text — and extracts
 * structured lines: { pn, qty, desc?, condition, conditionAssumed, rawLine }.
 *
 * Canonical condition set (FR-G.1): { NEW, NOB, REF, USED, F/S }.
 * Unknown / missing condition → 'REF' with conditionAssumed: true.
 * Qty default when absent: 1 (FR-G.2).
 */

export type CanonicalCondition = 'NEW' | 'NOB' | 'REF' | 'USED' | 'F/S'

export interface ParsedLine {
  pn: string
  qty: number
  desc?: string
  condition: CanonicalCondition
  conditionAssumed: boolean
  rawLine: string
}

export type ParseMode = 'tabular' | 'freetext'

export interface ParseResult {
  lines: ParsedLine[]
  mode: ParseMode
  headerDetected: boolean
}

export const CANONICAL_CONDITIONS: readonly CanonicalCondition[] = ['NEW', 'NOB', 'REF', 'USED', 'F/S']

// ── Condition aliases (FR-G.1) ───────────────────────────────────────────────

const CONDITION_ALIASES: Readonly<Record<string, CanonicalCondition>> = {
  new: 'NEW',
  newoem: 'NEW',
  'new oem': 'NEW',
  brandnew: 'NEW',
  'brand new': 'NEW',
  nob: 'NOB',
  'n.o.b': 'NOB',
  'new open box': 'NOB',
  ref: 'REF',
  refurb: 'REF',
  refurbished: 'REF',
  used: 'USED',
  preowned: 'USED',
  'pre-owned': 'USED',
  'f/s': 'F/S',
  fs: 'F/S',
  'factory sealed': 'F/S',
}

function normalizeKey(raw: string): string {
  return raw.trim().toLowerCase().replace(/\s+/g, ' ').replace(/\.+$/, '')
}

export function isCanonicalCondition(v: unknown): v is CanonicalCondition {
  return typeof v === 'string' && (CANONICAL_CONDITIONS as readonly string[]).includes(v)
}

/** Map any token to the canonical set. Unknown/missing → REF + assumed (FR-G.1). */
export function normalizeCondition(raw: string): { condition: CanonicalCondition; assumed: boolean } {
  const key = normalizeKey(raw)
  if (!key) return { condition: 'REF', assumed: true }
  const hit = CONDITION_ALIASES[key]
  return hit ? { condition: hit, assumed: false } : { condition: 'REF', assumed: true }
}

// ── PN tokens ────────────────────────────────────────────────────────────────

/** PRD FR-S1.1: /[A-Z0-9][A-Z0-9._-]{2,31}/i with digit+letter mix (covers OEM patterns like 875513-B21, P28586-B21). */
const PN_CANDIDATE_RE = /^[A-Za-z0-9][A-Za-z0-9._-]{2,31}$/
const PN_SCAN_RE = /[A-Za-z0-9][A-Za-z0-9._-]{2,31}/g

/** Spec fragments that are PN-shaped but never part numbers (capacity, speed, rank, wattage, cores, cache). */
const SPEC_TOKEN_RE = /^(\d+(\.\d+)?(GB|TB|MB|G|T|W|K|GHZ|MHZ|WATTS?|GBPS|PIN|C|M)|PC\d.*|\d+RX\d+|DDR\d.*)$/i

/** Quote/order REFERENCE numbers, never part numbers (CEO 2026-08-01: legacy
 *  email subjects + sidebar subjects like "RFQ-2077856-ARGENTINA" polluted
 *  the grid with 11 junk rows). Matches RFQ-2077856-ARGENTINA, RFQ-20778,
 *  RFQ_20607, "RFQ 2068722-PE" style tokens (dash/underscore optional after RFQ). */
const REFERENCE_TOKEN_RE = /^(RFQ|PO|SO|QT|QUOTE|ORDER|INV)[-_\s]?\d/i

export function isPnToken(raw: string): boolean {
  const tok = raw.trim().replace(/^[._-]+|[._-]+$/g, '')
  if (tok.length < 3 || tok.length > 32) return false
  if (!PN_CANDIDATE_RE.test(tok)) return false
  if (!/\d/.test(tok)) return false // must contain a digit
  // Digit+letter mix (875513-B21), OR pure-digit OEM pattern of plausible PN
  // length (EMC 005049568 — CEO doctrine 2026-08-01: the DB validates, not the
  // regex; pure-digit candidates surface as ⚠ unverified when not compendium-known).
  if (!/[A-Za-z]/.test(tok)) {
    if (/^\d{5,15}$/.test(tok)) return true
    const digits = tok.replace(/\D/g, '')
    const digitSepOem = /^\d[\d._-]*\d$/.test(tok)
      && digits.length >= 5 && digits.length <= 15
      // CEO 2026-08-21: the ≥5-digit-group rule rejected valid OEM PNs like
      // 106-838-040-03 and 019-078-046. We now keep any digit-separator token
      // with 5-15 digits and exclude only the common non-PN patterns that
      // triggered the original rule: phone numbers and ISO dates.
      && !/^\d{3}[._-]\d{4}$/.test(tok)               // 555-1212
      && !/^\d{3}[._-]\d{3}[._-]\d{4}$/.test(tok)     // 305-555-1212
      && !/^\d{4}[._-]\d{2}[._-]\d{2}$/.test(tok)    // 2026-08-17
    if (!digitSepOem) return false
  }
  if (CONDITION_ALIASES[normalizeKey(tok)]) return false
  if (SPEC_TOKEN_RE.test(tok)) return false
  if (REFERENCE_TOKEN_RE.test(tok)) return false
  return true
}

/** Uppercase, strip disallowed chars and leading/trailing separators. */
export function normalizePn(raw: string): string {
  return raw
    .toUpperCase()
    .replace(/[^A-Z0-9._-]/g, '')
    .replace(/^[._-]+|[._-]+$/g, '')
    .slice(0, 32)
}

// ── Quantities ───────────────────────────────────────────────────────────────

const QTY_MIN = 1
const QTY_MAX = 100000

/** Strict cell qty: integer in [1, 100000]; tolerates "4.0" and "1,000". */
export function parseQtyValue(raw: string): number | null {
  const t = raw.trim().replace(/,/g, '')
  if (!/^[+-]?\d+(\.\d+)?$/.test(t)) return null
  const n = Number(t)
  if (!Number.isFinite(n) || !Number.isInteger(n)) return null
  if (n < QTY_MIN || n > QTY_MAX) return null
  return n
}

// ── Header detection ─────────────────────────────────────────────────────────

const HEADER_TOKENS: ReadonlySet<string> = new Set([
  'part', 'pn', 'part number', 'partnumber', 'part no', 'partno', 'p/n', 'part#', 'part name',
  'qty', 'quantity', "q'ty",
  'desc', 'description',
  'condition', 'cond',
  'price', 'uom', 'mfg', 'manufacturer',
  'sku', 'item', 'line',
])

type ColRole = 'pn' | 'qty' | 'desc' | 'condition' | null

function isHeaderToken(cell: string): boolean {
  return HEADER_TOKENS.has(normalizeKey(cell).replace(/[._#/\-]+$/, ''))
}

function roleForHeader(cell: string): ColRole {
  const k = normalizeKey(cell).replace(/[._#/\-]+$/, '')
  if (['part', 'pn', 'part number', 'partnumber', 'part no', 'partno', 'p/n', 'part#', 'part name', 'sku', 'item'].includes(k)) return 'pn'
  if (['qty', 'quantity', "q'ty"].includes(k)) return 'qty'
  if (['desc', 'description'].includes(k)) return 'desc'
  if (['condition', 'cond'].includes(k)) return 'condition'
  return null // price / uom / mfg / line: recognized as header, no output role
}

/** First row with ≥2 header-token hits, or 1 hit that maps to a column role. */
function isHeaderRow(cells: string[]): boolean {
  const hits = cells.filter(isHeaderToken).length
  if (hits >= 2) return true
  if (hits === 1) return cells.some((c) => roleForHeader(c) !== null)
  return false
}

// ── Delimiter detection ──────────────────────────────────────────────────────

/** Priority order on tie: tab first (Excel spill is the primary use case). */
const DELIMITERS = ['\t', '|', ';', ','] as const

interface DelimPick {
  delim: string
  fieldCount: number
  rowMask: boolean[] // true = line matches the modal field count
}

function pickDelimiter(lines: string[]): DelimPick | null {
  let best: { delim: string; score: number; modalCount: number; mask: boolean[] } | null = null
  for (const d of DELIMITERS) {
    const counts = lines.map((l) => l.split(d).length)
    const withDelim = counts.filter((c) => c >= 2)
    if (withDelim.length < 2) continue
    const freq = new Map<number, number>()
    for (const c of withDelim) freq.set(c, (freq.get(c) ?? 0) + 1)
    let modalCount = 0
    let modalFreq = 0
    for (const [c, f] of freq) {
      if (f > modalFreq || (f === modalFreq && c > modalCount)) {
        modalCount = c
        modalFreq = f
      }
    }
    const consistency = modalFreq / withDelim.length
    const coverage = modalFreq / lines.length
    if (consistency < 0.6) continue
    const score = consistency * coverage
    if (!best || score > best.score) {
      best = { delim: d, score, modalCount, mask: counts.map((c) => c === modalCount) }
    }
  }
  return best ? { delim: best.delim, fieldCount: best.modalCount, rowMask: best.mask } : null
}

// ── Column-role inference (no headers) ───────────────────────────────────────

interface ColLayout {
  pnCol: number
  qtyCol: number
  condCol: number
  descCol: number
}

function mean(nums: number[]): number {
  return nums.length === 0 ? 0 : nums.reduce((a, b) => a + b, 0) / nums.length
}

function inferRoles(rows: string[][], fieldCount: number): ColLayout {
  const colValues: string[][] = []
  for (let c = 0; c < fieldCount; c++) colValues.push(rows.map((r) => r[c] ?? ''))

  const pnRate = colValues.map((col) => mean(col.map((v) => (isPnToken(v) ? 1 : 0))))
  const qtyRate = colValues.map((col) => mean(col.map((v) => (parseQtyValue(v) !== null ? 1 : 0))))
  const condRate = colValues.map((col) => mean(col.map((v) => (CONDITION_ALIASES[normalizeKey(v)] ? 1 : 0))))
  const avgWidth = colValues.map((col) => mean(col.map((v) => v.length)))

  // PN column: highest PN-shaped rate, must dominate.
  let pnCol = -1
  for (let c = 0; c < fieldCount; c++) {
    if (pnRate[c] >= 0.5 && (pnCol === -1 || pnRate[c] > pnRate[pnCol])) pnCol = c
  }
  if (pnCol === -1) return { pnCol: -1, qtyCol: -1, condCol: -1, descCol: -1 }

  // Qty column: numeric [1,100000]; prefer adjacent to PN (PRD), else best overall.
  let qtyCol = -1
  for (let c = 0; c < fieldCount; c++) {
    if (c === pnCol || qtyRate[c] < 0.5) continue
    if (Math.abs(c - pnCol) === 1 && (qtyCol === -1 || qtyRate[c] > qtyRate[qtyCol])) qtyCol = c
  }
  if (qtyCol === -1) {
    for (let c = 0; c < fieldCount; c++) {
      if (c === pnCol || qtyRate[c] < 0.6) continue
      if (qtyCol === -1 || qtyRate[c] > qtyRate[qtyCol]) qtyCol = c
    }
  }

  // Condition column: best alias-hit rate among unassigned columns.
  let condCol = -1
  for (let c = 0; c < fieldCount; c++) {
    if (c === pnCol || c === qtyCol || condRate[c] < 0.5) continue
    if (condCol === -1 || condRate[c] > condRate[condCol]) condCol = c
  }

  // Description: widest remaining text column (not PN-shaped, not numeric).
  let descCol = -1
  for (let c = 0; c < fieldCount; c++) {
    if (c === pnCol || c === qtyCol || c === condCol) continue
    if (avgWidth[c] < 4 || pnRate[c] >= 0.5 || qtyRate[c] >= 0.5) continue
    if (descCol === -1 || avgWidth[c] > avgWidth[descCol]) descCol = c
  }

  return { pnCol, qtyCol, condCol, descCol }
}

function layoutFromHeaderRoles(roles: ColRole[]): ColLayout {
  return {
    pnCol: roles.indexOf('pn'),
    qtyCol: roles.indexOf('qty'),
    condCol: roles.indexOf('condition'),
    descCol: roles.indexOf('desc'),
  }
}

function rowToParsed(cells: string[], layout: ColLayout, rawLine: string): ParsedLine | null {
  const pn = normalizePn(cells[layout.pnCol] ?? '')
  if (!pn || !isPnToken(pn)) return null
  const qty = layout.qtyCol >= 0 ? parseQtyValue(cells[layout.qtyCol] ?? '') ?? 1 : 1
  const cond = layout.condCol >= 0 ? normalizeCondition(cells[layout.condCol] ?? '') : { condition: 'REF' as CanonicalCondition, assumed: true }
  const desc = layout.descCol >= 0 ? (cells[layout.descCol] ?? '').trim() : ''
  const line: ParsedLine = { pn, qty, condition: cond.condition, conditionAssumed: cond.assumed, rawLine }
  if (desc) line.desc = desc
  return line
}

// ── Free-text mode ───────────────────────────────────────────────────────────

interface Span {
  index: number
  length: number
}

const QTY_PATTERNS: readonly RegExp[] = [
  /\bqty\b\s*[:=]?\s*(\d{1,6})/i, // qty 5 / qty: 5 / qty=5
  /\b(\d{1,6})\s*x\b/i, // 5x
  /\bx\s*(\d{1,6})\b/i, // x5
  /\b(\d{1,6})\s*(?:pcs?|ea|each|units?)\b/i, // 5 pcs / 5 pcs. / 5 ea
]

function extractQty(line: string): { qty: number; span: Span } | null {
  for (const re of QTY_PATTERNS) {
    const m = re.exec(line)
    if (!m) continue
    const qty = parseQtyValue(m[1])
    if (qty === null) continue
    return { qty, span: { index: m.index, length: m[0].length } }
  }
  return null
}

/**
 * Last-resort qty: a bare in-range integer immediately adjacent to the PN
 * (separator or space only), e.g. a lone "P28586-B21,4,NEW" line. Mirrors the
 * tabular "qty column adjacent to PN" rule; only used when no explicit cue hit.
 */
function adjacentQty(line: string, pnSpan: Span): { qty: number; span: Span } | null {
  const afterStart = pnSpan.index + pnSpan.length
  const am = /^[\s,;:|]*(\d{1,6})\b/.exec(line.slice(afterStart))
  if (am) {
    const qty = parseQtyValue(am[1])
    if (qty !== null) {
      return { qty, span: { index: afterStart + am[0].indexOf(am[1]), length: am[1].length } }
    }
  }
  const bm = /\b(\d{1,6})[\s,;:|]*$/.exec(line.slice(0, pnSpan.index))
  if (bm) {
    const qty = parseQtyValue(bm[1])
    if (qty !== null) return { qty, span: { index: bm.index, length: bm[1].length } }
  }
  return null
}

/** Free-text condition cues. Prose-ambiguous words (new/used) must sit near a PN. */
const FREETEXT_COND: ReadonlyArray<{ re: RegExp; cond: CanonicalCondition; nearPn: boolean }> = [
  { re: /factory\s+sealed/i, cond: 'F/S', nearPn: false },
  { re: /\bf\/s\b/i, cond: 'F/S', nearPn: false },
  { re: /\bFS\b/, cond: 'F/S', nearPn: false }, // case-sensitive on purpose
  { re: /new\s+open\s+box/i, cond: 'NOB', nearPn: false },
  { re: /\bn\.?o\.?b\.?\b/i, cond: 'NOB', nearPn: false },
  { re: /refurbished/i, cond: 'REF', nearPn: false },
  { re: /\brefurb\b/i, cond: 'REF', nearPn: false },
  { re: /\bref\b/i, cond: 'REF', nearPn: false },
  { re: /pre-?owned/i, cond: 'USED', nearPn: false },
  { re: /new\s*oem/i, cond: 'NEW', nearPn: false },
  { re: /\bnew\b/i, cond: 'NEW', nearPn: true },
  { re: /\bused\b/i, cond: 'USED', nearPn: true },
]

const COND_PROXIMITY = 16

function extractCondition(line: string, pnSpans: Span[]): { condition: CanonicalCondition; assumed: boolean; span: Span | null } {
  for (const { re, cond, nearPn } of FREETEXT_COND) {
    const m = re.exec(line)
    if (!m) continue
    if (nearPn) {
      const mEnd = m.index + m[0].length
      const near = pnSpans.some(
        (s) => Math.abs(m.index - (s.index + s.length)) <= COND_PROXIMITY || Math.abs(s.index - mEnd) <= COND_PROXIMITY,
      )
      if (!near) continue
    }
    return { condition: cond, assumed: false, span: { index: m.index, length: m[0].length } }
  }
  return { condition: 'REF', assumed: true, span: null }
}

const DESC_EDGE_RE = /^[\s,;:.\-–—|&/\\]+|[\s,;:.\-–—|&/\\]+$/g

/** Common prose/request words; a "description" made only of these is email boilerplate, not a spec. */
const PROSE_WORDS: ReadonlySet<string> = new Set([
  'a', 'an', 'the', 'and', 'or', 'of', 'to', 'for', 'with', 'in', 'on', 'at', 'by', 'from', 'via',
  'i', 'we', 'you', 'me', 'us', 'our', 'your', 'this', 'that', 'these', 'those', 'it', 'its',
  'is', 'are', 'was', 'were', 'be', 'been', 'have', 'has', 'had', 'do', 'does', 'did',
  'will', 'would', 'can', 'could', 'should', 'may', 'might', 'must', 'not', 'no', 'yes', 'ok',
  'hi', 'hello', 'dear', 'team', 'thanks', 'thank', 'regards', 'best', 'sir', 'madam',
  'please', 'pls', 'plz', 'quote', 'quotation', 'rfq', 'request', 'requesting', 'need', 'needs',
  'want', 'looking', 'look', 'also', 'each', 'per', 'qty', 'quantity', 'pcs', 'pc', 'ea', 'unit', 'units',
  'price', 'pricing', 'cost', 'stock', 'availability', 'available', 'asap', 'rush', 'urgent',
  'let', 'know', 'send', 'sent', 'email', 'mail', 'list', 'bom', 'below', 'following', 'item', 'items',
  'fwd', 'fw', 're', 'subject', 'cc', 'bcc', 'get', 'back', 'today', 'tomorrow',
])

function isProseOnly(s: string): boolean {
  const words = s.toLowerCase().replace(/[^a-z0-9\s]/g, ' ').split(/\s+/).filter(Boolean)
  return words.length > 0 && words.every((w) => PROSE_WORDS.has(w) || /^\d+$/.test(w))
}

function cleanDescRegion(region: string): string {
  return region.replace(DESC_EDGE_RE, '').replace(/\s+/g, ' ').trim()
}

function validDesc(s: string): boolean {
  return s.length >= 3 && /[A-Za-z]/.test(s) && !isProseOnly(s)
}

/**
 * Description candidate for a single-PN line: text after the PN (specs follow
 * PNs in broker emails), falling back to text before it. Qty/condition cue
 * spans are blanked first; pure-prose remainders are rejected.
 */
function extractDesc(line: string, pnSpan: Span, otherSpans: Span[]): string | undefined {
  const blank = (region: string, regionOffset: number): string => {
    const chars = region.split('')
    for (const s of otherSpans) {
      const start = s.index - regionOffset
      for (let i = Math.max(0, start); i < Math.min(chars.length, start + s.length); i++) chars[i] = ' '
    }
    return chars.join('')
  }
  const afterStart = pnSpan.index + pnSpan.length
  const after = cleanDescRegion(blank(line.slice(afterStart), afterStart))
  if (validDesc(after)) return after.slice(0, 200)
  const before = cleanDescRegion(blank(line.slice(0, pnSpan.index), 0))
  if (validDesc(before)) return before.slice(0, 200)
  return undefined
}

/** Connective/cue words allowed BETWEEN PNs in a true PN list
 *  ("875513-B21 and P28586-B21, 5 pcs each" / "…qty 2 used and …"). */
const PN_LIST_CONNECTIVES: ReadonlySet<string> = new Set([
  'and', 'or', 'x', 'qty', 'pc', 'pcs', 'ea', 'each', 'unit', 'units',
  'new', 'used', 'ref', 'refurb', 'refurbished', 'nob', 'fs', 'f', 's',
  'factory', 'sealed', 'oem', 'nob.',
])

/** True when every inter-PN gap holds only separators, numbers, qty/condition
 *  cues, or connectives — i.e. the line really is a list of PNs. A gap with
 *  description words ("49Y4230 Tarj ethernet I340-T2 dual port IBM") means
 *  PN + description: one row, first PN wins (CEO 2026-08-01: one input line =
 *  one quote row; description tokens like I340-T2 / E-2124 / DDR4-2666V must
 *  not spawn their own rows). */
function isPnListLine(line: string, found: Array<{ pn: string; span: Span }>): boolean {
  if (found.length < 2) return false
  for (let i = 0; i < found.length - 1; i++) {
    const gap = line.slice(found[i].span.index + found[i].span.length, found[i + 1].span.index)
    const words = gap.toLowerCase().replace(/[^a-z0-9\s/&+.-]/g, ' ').split(/[\s/&+]+/).filter(Boolean)
    for (const w of words) {
      if (/^\d+(\.\d+)?$/.test(w)) continue
      if (!PN_LIST_CONNECTIVES.has(w.replace(/[.-]+$/, ''))) return false
    }
  }
  return true
}

export function parseFreeTextLine(line: string): ParsedLine[] {
  const seen = new Set<string>()
  const found: Array<{ pn: string; span: Span }> = []
  for (const m of line.matchAll(PN_SCAN_RE)) {
    const pn = normalizePn(m[0])
    if (!pn || !isPnToken(pn) || seen.has(pn)) continue
    seen.add(pn)
    found.push({ pn, span: { index: m.index, length: m[0].length } })
  }
  if (found.length === 0) return []

  // PN + description line (multi-PN-shaped tokens, but not a PN list):
  // first PN is the part; the rest of the line stays as its description.
  if (found.length > 1 && !isPnListLine(line, found)) {
    const first = found[0]
    const qtyHit = extractQty(line) ?? adjacentQty(line, first.span)
    const condHit = extractCondition(line, [first.span])
    const out: ParsedLine = {
      pn: first.pn,
      qty: qtyHit?.qty ?? 1,
      condition: condHit.condition,
      conditionAssumed: condHit.assumed,
      rawLine: line,
    }
    // Qty/condition cue spans are blanked; other PN-shaped tokens REMAIN in the
    // description (I340-T2, E-2124, DDR4-2666V are spec content, not rows).
    const desc = extractDesc(line, first.span, [
      ...(qtyHit ? [qtyHit.span] : []),
      ...(condHit.span ? [condHit.span] : []),
    ])
    if (desc) out.desc = desc
    return [out]
  }

  // Global cues act as fallback; each PN's own segment (up to the next PN) wins.
  const globalQty = extractQty(line)
  const globalCond = extractCondition(line, found.map((f) => f.span))

  return found.map(({ pn }, i) => {
    // Each PN owns the text from itself up to the next PN (plus leading text for the first).
    const segStart = i === 0 ? 0 : found[i].span.index
    const segEnd = i === found.length - 1 ? line.length : found[i + 1].span.index
    const segment = line.slice(segStart, segEnd)
    const pnSpanInSeg = { index: found[i].span.index - segStart, length: found[i].span.length }
    const qtyHit = extractQty(segment) ?? adjacentQty(segment, pnSpanInSeg) ?? globalQty
    const condHit = (() => {
      const local = extractCondition(segment, [pnSpanInSeg])
      return local.assumed && !globalCond.assumed ? globalCond : local
    })()
    const out: ParsedLine = {
      pn,
      qty: qtyHit?.qty ?? 1,
      condition: condHit.condition,
      conditionAssumed: condHit.assumed,
      rawLine: line,
    }
    // Description only for single-PN lines — multi-PN lines leave connective prose.
    if (found.length === 1) {
      const desc = extractDesc(line, found[0].span, [
        ...(qtyHit && qtyHit.span ? [qtyHit.span] : []),
        ...(condHit.span ? [condHit.span] : []),
      ])
      if (desc) out.desc = desc
    }
    return out
  })
}

function parseFreeText(lines: string[]): ParsedLine[] {
  const out: ParsedLine[] = []
  for (const l of lines) out.push(...parseFreeTextLine(l))
  return out
}

// ── Tabular mode ─────────────────────────────────────────────────────────────

interface TabularOutcome {
  parsed: ParsedLine[]
  headerDetected: boolean
  pnResolved: boolean
}

function parseTabular(lines: string[], pick: DelimPick): TabularOutcome {
  const modalIdx = lines.map((_, i) => (pick.rowMask[i] ? i : -1)).filter((i) => i >= 0)
  const modalCells = modalIdx.map((i) => lines[i].split(pick.delim).map((c) => c.trim()))

  let headerDetected = false
  let layout: ColLayout | null = null
  if (modalCells.length > 0 && isHeaderRow(modalCells[0])) {
    headerDetected = true
    const fromHeader = layoutFromHeaderRoles(modalCells[0].map(roleForHeader))
    if (fromHeader.pnCol >= 0) layout = fromHeader
  }
  if (!layout) {
    const dataRows = headerDetected ? modalCells.slice(1) : modalCells
    layout = inferRoles(dataRows, pick.fieldCount)
  }
  if (layout.pnCol < 0) return { parsed: [], headerDetected, pnResolved: false }

  const headerLineIdx = headerDetected ? modalIdx[0] : -1
  const out: ParsedLine[] = []
  lines.forEach((l, i) => {
    if (i === headerLineIdx) return
    if (!pick.rowMask[i]) {
      out.push(...parseFreeTextLine(l)) // junk/prose lines still get a PN scan
      return
    }
    const cells = l.split(pick.delim).map((c) => c.trim())
    const pl = rowToParsed(cells, layout as ColLayout, l)
    if (pl) out.push(pl)
  })
  return { parsed: out, headerDetected, pnResolved: true }
}

// ── Public API ───────────────────────────────────────────────────────────────

export function smartParseDetailed(text: string): ParseResult {
  const lines = String(text ?? '')
    .split(/\r\n|\r|\n/)
    .map((l) => l.trim())
    .filter((l) => l.length > 0)
  if (lines.length === 0) return { lines: [], mode: 'freetext', headerDetected: false }

  const pick = pickDelimiter(lines)
  if (!pick) return { lines: parseFreeText(lines), mode: 'freetext', headerDetected: false }

  const outcome = parseTabular(lines, pick)
  if (!outcome.pnResolved) {
    // Tabular-looking text without a PN column (e.g. prose with commas) → free text.
    return { lines: parseFreeText(lines), mode: 'freetext', headerDetected: outcome.headerDetected }
  }
  return { lines: outcome.parsed, mode: 'tabular', headerDetected: outcome.headerDetected }
}

/** FR-S1.1 entry point: (text: string) => ParsedLine[] */
export function smartParse(text: string): ParsedLine[] {
  return smartParseDetailed(text).lines
}
