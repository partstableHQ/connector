// Package connector hosts the embedded frontend assets of the PartsTable
// Connector desktop app.
package connector

import "embed"

// FrontendFS holds the compiled frontend (the vite build output in
// frontend/dist). It must exist before `go build`:
//
//	npm --prefix frontend ci && npm --prefix frontend run build
//
//go:embed all:frontend/dist
var FrontendFS embed.FS
