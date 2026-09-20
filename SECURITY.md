# Security Policy

## Reporting a vulnerability

Please use GitHub's **private vulnerability reporting** on this repository
(Security tab → Report a vulnerability). Do not open a public issue for a
security problem.

We aim to acknowledge reports within 3 business days and to ship a fix or a
mitigation for critical issues within 30 days.

## Scope

- The desktop app (`cmd/partstable`, `internal/*`) and its embedded frontend.
- The local REST API — it must bind to `127.0.0.1` only; anything else is a
  finding.
- The update pipeline: release signatures, checksums, and the verification
  the app performs before applying an update.

## Design commitments this project is held to

- The API key and any credential live in the OS keychain (Windows Credential
  Manager / Keychain / libsecret), never in plain files.
- Every release ships `checksums.txt` plus a cosign signature and an SBOM;
  the app verifies before applying.
- Telemetry is limited to the anonymous update check (see PRIVACY.md).
