const API = 'http://127.0.0.1:7878';

export interface Part {
  pn: string;
  display_pn: string;
  description: string;
  category?: string;
  xrefs: Xref[];
  holders: Holder[];
}

export interface Xref {
  to_pn: string;
  kind: string;
  source: string;
  source_detail?: string;
}

export interface Holder {
  holder: string;
  qty: number;
  condition?: string;
  last_seen: string;
  source: string;
  source_detail?: string;
}

export interface LookupResult {
  query: string;
  normalized: string;
  matched_by: string;
  part: Part | null;
}

export interface SearchHit {
  pn: string;
  display_pn: string;
  description: string;
  category?: string;
}

export interface ParseWarning {
  line: number;
  raw: string;
  text: string;
}

export interface PasteEntry {
  raw: string;
  pn: string;
  norm: string;
  qty: number;
  lines: number[];
  result: LookupResult | null;
}

export interface PasteResponse {
  entries: PasteEntry[];
  warnings: ParseWarning[];
}

export interface Health {
  app_version: string;
  compendium: { schema: number; vintage: string; part_count: number } | null;
  auth?: { signed_in: boolean; email?: string; signing_in: boolean; last_error?: string } | null;
  update?: { current: string; latest?: string; available: boolean; telemetry_opt_out: boolean } | null;
}

export async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const r = await fetch(`${API}${path}`, init);
  if (!r.ok) {
    const body = await r.json().catch(() => ({ error: r.statusText }));
    throw new Error(body.error ?? `HTTP ${r.status}`);
  }
  return r.json();
}

export async function apiRaw(path: string, init?: RequestInit): Promise<Response> {
  return fetch(`${API}${path}`, init);
}

export { API };
