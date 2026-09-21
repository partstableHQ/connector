-- App database v1: settings key-value store and build metadata.
-- Forward-only: there is no down path by doctrine — client databases are
-- never down-migrated, releases only move forward.
CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO meta (key, value)
VALUES ('app_schema_created_at', strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
ON CONFLICT (key) DO NOTHING;
