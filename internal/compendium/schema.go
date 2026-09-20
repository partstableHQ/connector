// Package compendium loads and verifies the signed compendium snapshot that
// ships with every release (FM-12). The artifact contract is FORMAT.md in
// this directory — it is the interface the server-side exporter must
// produce and the app consumes.
package compendium

// SchemaSQL is the authoritative compendium schema, version 1. The
// server-side exporter executes this verbatim when producing a snapshot;
// the app only consumes. Every fact in the snapshot carries `source` from
// the Source vocabulary below — the UI citation chips (FM-6) render from it.
const SchemaSQL = `
CREATE TABLE meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE parts (
    pn          TEXT PRIMARY KEY,
    display_pn  TEXT NOT NULL,
    description TEXT NOT NULL,
    category    TEXT NOT NULL DEFAULT ''
);

CREATE TABLE part_aliases (
    alias TEXT NOT NULL PRIMARY KEY,
    pn    TEXT NOT NULL REFERENCES parts(pn)
);

CREATE TABLE xrefs (
    from_pn       TEXT NOT NULL REFERENCES parts(pn),
    to_pn         TEXT NOT NULL REFERENCES parts(pn),
    kind          TEXT NOT NULL,
    source        TEXT NOT NULL,
    source_detail TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (from_pn, to_pn, kind, source)
);

CREATE TABLE holders (
    pn            TEXT NOT NULL REFERENCES parts(pn),
    holder        TEXT NOT NULL,
    qty           INTEGER NOT NULL,
    condition     TEXT NOT NULL DEFAULT '',
    last_seen     TEXT NOT NULL,
    source        TEXT NOT NULL,
    source_detail TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_xrefs_from  ON xrefs(from_pn);
CREATE INDEX idx_holders_pn  ON holders(pn);
CREATE INDEX idx_aliases_pn  ON part_aliases(pn);
`

// Citation sources — the exact vocabulary behind FM-6 source chips.
const (
	SourceOEM                = "oem"
	SourceGovernmentRegistry = "government_registry"
	SourceBrokerVerified     = "broker_verified"
	SourcePartner            = "partner"
	SourceCertified          = "certified"
)

// Sources is the full citation vocabulary, in display order.
var Sources = []string{
	SourceOEM,
	SourceGovernmentRegistry,
	SourceBrokerVerified,
	SourcePartner,
	SourceCertified,
}

// Meta keys every snapshot must carry (see FORMAT.md).
const (
	MetaSchema    = "compendium_schema"
	MetaVintage   = "generated_at" // RFC3339 UTC — the visible data vintage
	MetaSourceRev = "source_rev"
	MetaGenerator = "generator"
)
