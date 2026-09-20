package compendium

import (
	"database/sql"
	"fmt"
	"path/filepath"
)

// Create initializes an empty compendium database at path with the v1
// schema. It is the single source of the schema in executable form: the
// server-side export tooling and the tests both go through this, so the
// contract in FORMAT.md can never drift from what the app reads. The
// caller inserts rows and meta, then closes the handle.
func Create(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		return nil, fmt.Errorf("compendium: create %s: %w", path, err)
	}
	if _, err := db.Exec(SchemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("compendium: apply schema to %s: %w", path, err)
	}
	return db, nil
}
