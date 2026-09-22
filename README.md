# PartsTable Connector

<div align="center">

# PartsTable Connector

A free, open-source desktop app for the secondary-market IT industry —
the parts connector that links your ERP, your CRM, and your tools to a
calibrated parts reference.

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

Measured 2026-09-20 and climbing nightly: **187,147 calibrated parts · 312,913
cross-references · 3,777 machine models across 36 brands.**

```bash
$ partstable lookup 95Y4812

  95Y4812 — Lenovo 64GB (1x64GB) 4Rx4 PC4-17000P-L DDR4-2133 LRDIMM
  ├─ CROSS-REFERENCES
  │   └─ 95Y4814    compatible alternate  [every claim carries its source]
  └─ SOURCES: every line above carries its citation.
       OEM/vendor         from the manufacturer's own part data
       GOV REGISTRY      from official part registries
       BROKER-VERIFIED   confirmed by working brokers in the trade
       PARTNER           live stock from a connected company's ERP
       COMMUNITY CERTIFIED ★  rated right by the brokers who use it —
       every result asks "did we get it right?" (👍/👎); the star is
       earned from those ratings and can be lost if the votes turn
```

## Quickstart

```bash
# 1 · Install — one binary, ~30 MB
winget install PartsTable.Connector      # Windows
# Linux and macOS one-liners publish here with the first release
# (exact shipped commands only — never placeholders)

# 2 · Sign in  (free account, no credit card)
partstable login

# 3 · Look up — free, from minute one, nothing to connect
partstable lookup 95Y4812

# 4 · What does this part fit? — every edge carries its source and tier
partstable fits 95Y4812

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
carrying its source and trust tier. Every part number is calibrated: each
source carries a trust tier with its measured precision; we publish the
number and its sample size, per claim type, and never round up.

## The community is in the app

The reference gets sharper every time someone uses it. Every result asks
"did we get it right?" (👍/👎). Brokers leave notes on part numbers —
"this MPN has fake revs in the gray market" — and the live ticker shows
the trade at work: verifications landing, notes added, stock listed.
Accuracy earns verifier rank; rank earns trust, and it can decay. No
streaks, no point games — a broker's rank has to mean something. What the
trade verifies this week is what trends. Your credentials and your
searches stay yours; community features are opt-in, action-based, and
never built from what you look up.

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