-- +goose Up
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

-- +goose Down
DROP TABLE IF EXISTS meta;
DROP TABLE IF EXISTS settings;
