// Package paths resolves the on-disk locations of the app's data: the app
// database and the loaded compendium. The directory is created on demand.
//
// Default base (via adrg/xdg): %LOCALAPPDATA% on Windows, XDG data home on
// Linux/macOS. Override with PARTSTABLE_DATA_DIR — used by tests, portable
// installs, and doctor.
package paths

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/adrg/xdg"
)

// EnvDataDir is the environment override for the data directory.
const EnvDataDir = "PARTSTABLE_DATA_DIR"

// DataDir returns the data directory, creating it if missing.
func DataDir() (string, error) {
	dir := os.Getenv(EnvDataDir)
	if dir == "" {
		d, err := xdg.DataFile(filepath.Join("partstable-connector"))
		if err != nil {
			return "", fmt.Errorf("paths: resolve data dir: %w", err)
		}
		dir = d
	}
	// #nosec G703 -- the dir comes from the documented PARTSTABLE_DATA_DIR
	// override (or the XDG resolver); honoring it is the feature.
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("paths: create data dir %s: %w", dir, err)
	}
	return dir, nil
}

// AppDB returns the path of the application database (app.db).
func AppDB() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "app.db"), nil
}

// CompendiumDB returns the path of the loaded compendium database.
func CompendiumDB() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "compendium.db"), nil
}
