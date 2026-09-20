// Package version carries build metadata for the PartsTable Connector.
package version

import (
	"runtime/debug"
	"sync"
)

// Injected at link time by goreleaser:
//
//	-ldflags "-X github.com/partstableHQ/connector/internal/version.version=v0.1.0"
var (
	version string
	commit  string
)

var (
	loadOnce sync.Once
	loaded   versionInfo
)

type versionInfo struct{ version, commit string }

// Version returns the release semver. goreleaser sets it at link time; a
// plain `go build` falls back to the module version from the build info,
// then to "dev".
func Version() string { return load().version }

// Commit returns the source revision the binary was built from, if known.
func Commit() string { return load().commit }

func load() versionInfo {
	loadOnce.Do(func() {
		loaded = versionInfo{version: "dev", commit: "unknown"}
		info, ok := debug.ReadBuildInfo()
		if !ok {
			return
		}
		if v := info.Main.Version; v != "" && v != "(devel)" {
			loaded.version = v
		}
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && s.Value != "" {
				loaded.commit = s.Value
			}
		}
	})
	return loaded
}
