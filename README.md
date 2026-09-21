# PartsTable Connector

**The parts compendium, on your computer.** Look up any part. Paste a whole
list. Get the description, the substitutes, and the source of every fact.

PartsTable Connector is a free, open-source desktop app for IT hardware
brokers and ITAD teams. The compendium ships with the app as a local
database, so every answer comes from your own machine — instantly, offline.

## What it does

- **Look up any part** — description, cross-references, and who holds it.
  Every displayed fact carries a source chip: OEM, government registry,
  broker-verified, partner, or certified ★.
- **Paste a whole list** — newline/comma lists, quantities (`x4`, `4x`,
  `qty 4`), quote emails, CSV. You get one complete, sortable table — and a
  warnings panel for anything that could not be parsed. Nothing is dropped
  silently.
- **Export to Excel** — one click, valid `.xlsx`, opens clean in Excel 2016+.
- **Works offline** — the compendium is local. Airplane mode is a feature,
  and the data vintage is always visible so staleness is honest.
- **Self-updating** — revisions arrive as signed GitHub releases; the app
  detects, verifies, and applies them on restart.
- **Your machine, your queries** — no part number you look up ever leaves
  your computer.

## Local API

The app serves its own lookup API on `127.0.0.1:7878` (`/health`, `/lookup`,
`/xref`, `/bulk`) — the same verbs the desktop UI uses. It binds to the
loopback interface only and never logs a query. Scripting documentation is
coming with v1.1.

## Install

The first public release (v0.1.0) will be one command on Windows:

```sh
winget install PartsTable.Connector
```

Scoop, `.deb`/`.rpm`, and a Docker image follow. Today you can build from
source — see [CONTRIBUTING.md](CONTRIBUTING.md).

## Privacy

The only telemetry is the update check itself: app version, operating
system, and an anonymous install ID. No part numbers, no queries, no email,
no PII — see [PRIVACY.md](PRIVACY.md). Crash reports are strictly opt-in
(off by default).

## Status

v0.1.0 is in development — the public roadmap lives in
[ROADMAP.md](ROADMAP.md).

## License

[MIT](LICENSE)
