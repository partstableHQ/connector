# PartsTable Connector

<div align="center">

# PartsTable Connector

A free, open-source desktop app for the secondary-market IT industry.

Look up any part. Paste a whole list. Build the sheet for any machine.
Get the description, the substitutes, what it fits — and the source of every fact.

[![MIT License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)
[![Release](https://img.shields.io/badge/release-v0.1.0-blue.svg)](../../releases)
[![Platforms](https://img.shields.io/badge/platforms-Windows%20%7C%20Linux%20%7C%20macOS%20%7C%20Docker-lightgrey.svg)](#install)

</div>

---

## What it is

When hardware goes end-of-life, the OEM stops supplying parts. The machines
keep running — and this industry keeps them running. PartsTable brings the
industry's scattered part knowledge into one place: OEM data, government part
registries, broker-verified lists — assembled, described, and cited. The app
puts it on your computer.

Measured 2026-09-20 and climbing nightly: **187,147 parts · 312,913
cross-references · 3,777 machine models across 36 brands.**

```bash
$ partstable lookup 02CL197

  02CL197 — Dell PowerEdge R620 riser card
  ├─ CROSS-REFERENCES
  │   ├─ 9TMRF   equivalent  [OEM ★certified]
  │   └─ GJW8F   alternate   [broker-verified]
  ├─ FITS
  │   └─ Dell PowerEdge R620
  ├─ WHO HOLDS IT
  │   └─ 4 vendors · 78 units · best: new pull ×32 · seen 2h ago
  └─ SOURCES: every line above carries its citation.
       OEM               from the manufacturer's own part data
       GOV REGISTRY      from official part registries
       BROKER-VERIFIED   confirmed by working brokers in the trade
       PARTNER           live stock from a connected company's ERP
       COMMUNITY CERTIFIED ★  rated right by the brokers who use it —
       every result asks "did we get it right?" (👍/👎); the star is
       earned from those ratings and can be lost if the votes turn
```

## Quickstart

```bash
# 1 · Install
winget install PartsTable.Connector      # Windows
# ...or brew install partstable / curl -fsSL partstable.com/install.sh
# ...or docker run -d ghcr.io/partstable/connector

# 2 · Sign in  (free account, no credit card)
partstable login

# 3 · Look up — free, from minute one, nothing to connect
partstable lookup 02CL197

# 4 · What does this part fit?
partstable fits 9TMRF

# 5 · Build sheet for a machine — complete parts list, cited, EOL clock
partstable buildsheet hpe dl380-gen10

# 6 · Paste a whole list — descriptions + substitutes, cited → Excel
partstable bulk mylist.txt --excel
```

The app works offline: it keeps the reference on your computer and answers
from it, instantly.

## Build sheets & end-of-life clocks

Pick any covered machine — 3,777 models across 36 brands — and get its
complete parts list: every component with its part number, description,
substitutions, and the machine's end-of-life clock. Ask the reverse question
— "what does this part fit?" — and get every machine it serves, each match
carrying its source and trust tier. Every source carries a trust tier with
its measured precision; we publish the number and its sample size, per claim
type, and never round up.

## How it stays current

There is no subscription for data. The reference ships with the software:
new part numbers, corrections, and citations come out as revisions here on
GitHub, and the app updates itself. You always run the latest reference.

## There's more — at partstable.com

The free app is one part of a complete, end-to-end system for IT brokers and
maintainers — available as paid modules, and as an online version your whole
company uses in the browser:

**Sales** · automatic lead capture · **quotations generated for you** · orders
integrated with your ERP · querying your own ERP · querying other brokers ·
**courier integration — labels and tracking** · customer communication, kept
in one thread.

For that part, skip the README: [partstable.com](https://partstable.com).

## Under the hood

Single Go binary, pure-Go embedded store, localhost REST interface
(`/lookup` `/xref` `/bulk` `/rfq`), OS credential store, signed releases,
one-command self-diagnostic (`partstable doctor`). Revisions are published as
GitHub releases and the app self-updates. Self-hosting is supported, always.

## License

[MIT](LICENSE).

<div align="center">
partstable.com/connector
</div>