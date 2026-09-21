# Release runbook — PartsTable Connector

Everything about shipping a release: what is automated, and the exact
go-live steps. The pipeline was proven end-to-end on a throwaway tag
(draft release, all assets, checksums, SBOM, cosign signatures, ghcr
image) — see "Pipeline proof" below.

## Signing policy (CEO ruling 2026-09-21)

**The app ships UNSIGNED. No certificate purchases.** Both paid paths
(Windows Authenticode via Certum/Azure, Apple Developer notarization) are
rejected by CEO decision; this is a recorded ruling, not an open question.

What that means in practice, honestly:

- **Windows**: first launch shows the SmartScreen "Windows protected your
  PC" screen — users click "More info" → "Run anyway". This friction fades
  as download volume builds reputation. `winget install` itself works
  regardless (winget carries the SHA256 we publish). The binary's own
  integrity is still verifiable: `checksums.txt` + cosign signature ship
  with every release, and the self-updater verifies them before applying.
- **macOS**: unsigned binaries trip Gatekeeper on first open — users
  right-click → "Open" once, or run
  `xattr -d com.apple.quarantine partstable`. Because that friction is
  worse than Windows, macOS ships **best-effort** (archives on the
  release, no installer, not advertised) and stays Windows-first per the
  FM cut line.
- **Docker + winget**: unaffected.

Do not add paid-signing steps back without a new CEO decision. If that
ever changes, the wiring notes are in the git history of this file
(cosign pins, mac job layout, and the sign-then-rechecksum ordering).

## Go-live checklist (v0.1.0 — unsigned, ready)

1. Flip `draft: false` in `.goreleaser.yml` and `.goreleaser-darwin.yml`.
2. Tag and push: `git tag v0.1.0 && git push origin v0.1.0`.
3. Verify the published release: assets present, `checksums.txt.sig`
   verifies (`cosign verify-blob`), Docker image pulls.
4. **winget submission** (FM-1): submit the manifests in
   `packaging/winget/` (fill `__VERSION__` and `__SHA256__` from the
   released archive) as a PR to `microsoft/winget-pkgs` under
   `w/Partstable/Partstable.Connector`.
5. Release notes state plainly that the build is unsigned, link the
   SmartScreen/"Run anyway" and Gatekeeper instructions, and point at the
   cosign verification for anyone who wants cryptographic integrity.

## Pipeline proof (2026-09-21, tag v0.0.0-dev.3)

The full pipeline was validated three times on throwaway tags; the first
two runs caught real bugs (kept out of a real release exactly as designed):

1. cosign v3 changed `sign-blob`'s bundle format and broke goreleaser's
   classic args → **cosign is pinned to v2.4.1** on both jobs.
2. goreleaser cannot find a **draft** release by tag, so the mac job
   minted a second untagged draft → the mac job now only builds
   (`--skip=publish,validate,sign`) and uploads with
   `gh release upload` against the ubuntu job's draft.

Run 3 produced a single draft with the complete matrix: windows zip + SBOM,
linux tar.gz + SBOM, darwin amd64+arm64 tar.gz, checksums.txt +
cosign signature/certificate, checksums-darwin.txt + signature/certificate,
and the ghcr.io image (pushed with digest for both `:version` and
`:latest`). Draft and tag were deleted afterwards. Re-run the same way
after any pipeline change, before a real release.

## Asset matrix (automated on tag)

| Asset | Where it's built | Integrity |
|---|---|---|
| `partstable-connector_<v>_windows_amd64.zip` | ubuntu (CGO off) | checksums.txt + cosign signature |
| `partstable-connector_<v>_linux_amd64.tar.gz` | ubuntu (CGO on, GTK4/WebKit) | checksums.txt + cosign signature |
| `partstable-connector_<v>_darwin_{amd64,arm64}.tar.gz` | macos job | checksums-darwin.txt + cosign signature |
| SBOM per archive (syft) | ubuntu | — |
| `ghcr.io/partstablehq/connector:<v>` + `:latest` | ubuntu (Dockerfile) | digest logged in the run |

Binary code signatures: none — see the signing policy above.
