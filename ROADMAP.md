# Roadmap — PartsTable Connector

Vertical slices from the v1.0 build plan (PRD 2026-09-20 + BUILD-GUIDE
§10). Each slice is done only with browser/UI or CLI evidence — no
completion claims without verification.

| # | Slice | Features | Status |
|---|---|---|---|
| 1 | Skeleton: repo, Wails v3 shell, CI (3 OSes), goreleaser dry-run | FM-19, FM-20 groundwork | **shipped** (2026-09-20) |
| 2 | Store: embedded SQLite (modernc) + goose migrations + snapshot loader/verifier + `doctor` | FM-4, FM-12, FM-16 | **shipped** (2026-09-20) — snapshot format contract: `internal/compendium/FORMAT.md`; migrations live at `internal/store/migrations/` (Go embed cannot cross package dirs) |
| 3 | Lookup UI + local API (`/lookup` `/xref` `/bulk`, 127.0.0.1 only) | FM-5, FM-6, FM-15 | **shipped** (2026-09-20) — verified end-to-end against a seeded compendium; public API docs deferred to v1.1 per the cut line |
| 4 | Paste-a-list parser + results table + Excel export | FM-7, FM-8, FM-9 | **shipped** (2026-09-21) — verified end-to-end in-browser (sort/filter/column-hide/warnings); export verified by xlsx read-back. Grid runs on @tanstack/table-core pinned to v8 (v9's vanilla API is adapter-oriented) with explicit state slices (8.21 reads unregistered slices unconditionally) |
| 5 | Auth: browser OAuth+PKCE pairing, OS keychain storage | FM-3 | planned |
| 6 | Self-update + anonymous update-check ping + Settings | FM-11, FM-13 | planned |
| 7 | Offline verification pass + branding pass (Geist, tokens) + single "There's more" link | FM-10, FM-17, FM-18 | planned |
| 8 | Release engineering: v0.1.0 — Authenticode signing, notarized DMG, winget/scoop PRs, Docker (ghcr.io), screenshots → demo GIF | FM-1, FM-2, FM-14, FM-20 full pass | planned |

## Deliberate deferrals (v1.1+)

- **FM-2 full matrix** — Homebrew tap, `.deb`/`.rpm` polish, Docker image can
  trail v1.0 with Windows-first install.
- **FM-14 crash reporting** — GlitchTip integration can trail v1.0; the
  opt-in default stays OFF either way.
- **FM-15 public documentation** — the localhost API ships internal-only in
  v1.0 and gets documented publicly at v1.1.
- **Playwright smoke tests on the Wails UI** — added with the first real UI
  surface (slice 3), not at scaffold time.
- **Darwin release assets** — join at slice 8 together with signing and
  notarization; an unsigned mac binary behind Gatekeeper would poison the
  first impression.
