# Release runbook — PartsTable Connector

Everything about shipping a release: what is automated, what secrets gate
the remaining manual pieces, and the exact go-live checklist. The pipeline
was proven end-to-end on a throwaway tag (draft release, all assets,
checksums, SBOM, cosign signatures, ghcr image) — see "Pipeline proof"
below.

## What a tag ships (automated today)

Pushing a tag `v*` triggers `.github/workflows/release.yml`:

| Asset | Where it's built | Signed |
|---|---|---|
| `partstable-connector_<v>_windows_amd64.zip` | ubuntu (CGO off) | checksums via keyless cosign |
| `partstable-connector_<v>_linux_amd64.tar.gz` | ubuntu (CGO on, GTK4/WebKit) | checksums via keyless cosign |
| `checksums.txt` | ubuntu | cosign (keyless, Sigstore) |
| SBOM per archive (syft) | ubuntu | — |
| `partstable-connector_<v>_darwin_{amd64,arm64}.tar.gz` | macos job | checksums-darwin.txt via cosign |
| `ghcr.io/partstablehq/connector:<v>` + `:latest` | ubuntu (Dockerfile) | image signed? — not yet, see go-live |
| Release page with changelog (git-cliff style from conventional commits) | ubuntu | — |

Releases are created **draft** today. Flip `draft: false` in both
`.goreleaser.yml` and `.goreleaser-darwin.yml` at go-live.

## Go-live checklist (v0.1.0)

1. **Windows Authenticode signing** (kills SmartScreen warnings) — needs a
   certificate:
   - Option A: **Certum Open Source Code Signing** (~€25–69/yr, OSS price).
   - Option B: **Azure Trusted Signing** ($9.99/mo, US identity validation).
   - Once issued: add the signing step to the `goreleaser` job between the
     build and the checksums (sign each `.exe` inside the windows archive,
     then regenerate `checksums.txt` so the cosign signature covers the
     signed artifacts). Suggested action: `azure/trusted-signing-action` or
     `signtool` with the PFX in a secret.
2. **macOS signing + notarization** — needs the Apple Developer Program
   ($99/yr): `DEVELOPER_ID_APPLICATION` certificate (secrets:
   `MACOS_CERT_P12`, `MACOS_CERT_PASSWORD`) plus `APPLE_ID`,
   `APPLE_PASSWORD` (app-specific), `APPLE_TEAM_ID` for notarytool. Wire
   `codesign --deep --options runtime` + `notarytool submit` + `stapler`
   into the `release-macos` job before archiving.
3. **Flip to public**: `draft: false` in both goreleaser configs.
4. **Tag and push**: `git tag v0.1.0 && git push origin v0.1.0`.
5. **Verify** the published release: assets present, `checksums.txt.sig`
   verifies (`cosign verify-blob`), Docker image pulls.
6. **winget submission** (FM-1): with the release public, submit the
   manifest in `packaging/winget/` (fill `__VERSION__` and `__SHA256__`
   from the released installer) as a PR to `microsoft/winget-pkgs` under
   `w/Partstable/Partstable.Connector`. One-time; later versions flow
   through the same PR process or the winget automation.

## Pipeline proof (2026-09-21)

The full pipeline was validated with a throwaway tag `v0.0.0-dev.1`: both
jobs ran green, the draft release carried every asset (windows/linux
archives + checksums + SBOMs + cosign signature + darwin archives +
ghcr image), and the draft + tag were deleted afterwards. Re-run the same
way after any pipeline change, before a real release.

## Known gaps (honest)

- ghcr image is not cosign-signed yet (add `cosign sign` on the digest at
  go-live).
- Windows Authenticode + macOS notarization are wired-in-waiting; they
  activate only with the credentials above.
- winget/scoop/Chocolatey submissions happen after the first public
  release exists.
