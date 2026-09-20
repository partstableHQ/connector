# Contributing

Thanks for helping make PartsTable Connector better.

## Development setup

Prerequisites: **Go 1.26+**, **Node 24+**, and on Linux
`libgtk-3-dev libwebkit2gtk-4.1-dev pkg-config` (the GUI needs them to
compile; Windows and macOS need nothing extra).

```sh
# build the frontend first — the Go build embeds its output
npm --prefix frontend ci
npm --prefix frontend run build

go vet ./...
go test ./...
go build -o partstable ./cmd/partstable
```

Lint (golangci-lint v2, config in `.golangci.yml`):

```sh
golangci-lint run
```

Release dry-run (no publish; builds the current host target only):

```sh
npm --prefix frontend run build
goreleaser check
goreleaser build --snapshot --clean --single-target
```

## Ground rules

- **Conventional commits** (`feat:`, `fix:`, `docs:`, `chore:`, …). The
  changelog is generated from them by git-cliff at release time.
- One logical change per PR. Tests and lint must pass on all three OSes.
- **No completion claims without evidence**: if a PR says a feature works,
  it includes the test or the screenshot that proves it.
- Never collect anything beyond the anonymous update check documented in
  [PRIVACY.md](PRIVACY.md). A PR that adds data collection will be declined.
