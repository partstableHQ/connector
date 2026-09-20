# Compendium snapshot format — v1

The binding contract between the server-side exporter (BUILD-GUIDE §5, to be
built against the canonical read model per Constitution §4.1) and the
Connector app. Both sides treat this file as the source of truth; the DDL
lives verbatim in `schema.go`.

## Artifact

- One file per release, attached to the GitHub release: `compendium-<semver>.bin`.
- Content: **gzip of a single SQLite database file** (the compendium).
- The release pipeline ships `checksums.txt` (cosign-signed, keyless). The
  verification chain, in order:
  1. `checksums.txt` signature verifies against the GitHub release.
  2. sha256 of `compendium-<semver>.bin` matches the entry in `checksums.txt`.
  3. Decompression succeeds (size-capped).
  4. `PRAGMA integrity_check` on the SQLite file returns `ok`.
  5. Required `meta` keys present; `compendium_schema` ≤ the app's maximum
     supported schema (never "upgrade the data to match the app" — the app
     ships data, the app never mutates it).

## Schema (version 1)

See `schema.go` (`SchemaSQL`) — parts, part_aliases, xrefs, holders, meta,
with indexes. The schema is additive-only across versions: v1 tables and
columns must never change shape or meaning in v2+.

## Meta keys (all required)

| key | value |
|---|---|
| `compendium_schema` | integer as string, starts at `1` |
| `generated_at` | RFC3339 UTC timestamp — **the data vintage, always visible to the user** (honest staleness) |
| `source_rev` | opaque revision identifier of the source export |
| `generator` | exporter name/version, e.g. `partstable-compendium-export/0.1` |

## Provenance vocabulary (`source` columns)

`oem` · `government_registry` · `broker_verified` · `partner` · `certified`

Exactly the five citation chips of FM-6. Values outside this set are a
contract violation and the loader must reject the snapshot.

## Exporter obligations (server side)

- Export ONLY through the canonical part read model — no private table reads.
- `pn` is the normalized identity (uppercase, separators stripped);
  `display_pn` preserves the source's verbatim form. Leading-zero variants
  ride in `part_aliases`, never conflated into one identity.
- Quantities, conditions and last-seen dates in `holders` must be stamped
  with their own `source` + `source_detail` for hover citations.
- Vintage (`generated_at`) is the moment of export, not of release.

## App obligations (client side)

- Load on first run and on update (FM-12); replace atomically (write to a
  temp file in the same directory, verify, rename; keep the previous file
  as `compendium.db.bak` for rollback).
- Never write to the compendium — it opens read-only everywhere in the app.
- Show the vintage in Settings and `doctor`; refuse schema versions above
  the supported maximum with an actionable message ("update the app").
