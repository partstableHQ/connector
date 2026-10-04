# RULING 2026-10-04 — Connector scope amendment: the company-level A2A key

**Filed by:** ZCode desktop agent (GLM, Tony's machine) · **Authority:** CEO session 2026-10-04, verbatim directives
**Amends:** `marketing/2026-09-20-connector-launch/PRD-2026-09-20-connector-app-v1.md` · `FEATURE-MATRIX-connector-v1.md` (FM-25) · `CONNECTOR-GROWTH-STRATEGY-2026-09-21.md` §1
**Binding on all lanes touching the Connector.** Repo mirror: `partstableHQ/connector` `docs/SCOPE-A2A.md` (same content).

## 1. FM-25 connectors are BESPOKE — and everything is A2A encrypted

The installable integration connector is **custom-made for each user/company** so they can connect
locally with PartsTable software. **Everything that leaves our software is A2A encrypted.** That is
the sales point and the technological reason the customer downloads it: **the Connector enables
A2A communication.** CEO words: bespoke, per-user, local, encrypted.

## 2. The parts reference is the ENABLER LANGUAGE, not the product

The part-list lookup is the canonical language every party speaks when requiring something —
"the canonical order that we speak in this app." It is a **mandatory aspect of the platform being
launched**, not the product itself.

## 3. Human-verified custom part numbers (service)

Users may **custom-add part numbers** that are then **looked at by humans as a service**. (The
company is interested in providing this service; build sequenced after the core loop.)

## 4. RFQ inside the Connector (build item)

Customers validate part numbers → get the description → get the substitutes → export — **and by
pressing a button send the RFQ**, filling in **quantities required and condition per item**.
**Multi-part-number selection capability is mandatory** — port over what the V5 dashboard already
does (multi-select with checkboxes). Executor note: multi-select shipped in beta.24; RFQ send built
this round (beta.29) over the live A2A endpoint (`POST /a2a`, `message/send`, verified 2026-09-30).

## 5. Ratification replaces ratings as the primary signal

Every completed loop — obtaining part numbers and executing an end (Excel export, RFQ sent, or
results shown on screen) — **is construed as moving the line: ratifying.** "It has been seen, it
has not been challenged" → the information is ACCEPTED. **No news is good news.** A thumbs-down is
the explicit challenge that triggers action. Optional thumbs-up/down stays. Part-number **usage is
a reinforcement layer** on part history.

## 6. License split acknowledged

Item confirmed by CEO (code MIT · reference data CC BY-SA 4.0 — CEO ruling A, 2026-09-28). One-liner
locked: **"The parts reference for the secondary-market IT industry. Free, open source, every fact
cited."** The retired headline ("the parts reference, on your computer") comes OUT of the app hero.

## 7. The Connector lives at the COMPANY level — the security key

The Connector **must live at the top level, not at the user level for a company.** Even if it only
enables the website to be operated, its presence/absence gates that — **it is our security key that
binds and keeps the customer's information local.** Management-level installation decision, per company.

Dabergy has granted API permission; **once switched over, all PartsTable↔Dabergy communication is
A2A protocol — mandatory.** Same for Covenco. "Also with APIs, but the protocol is A2A."

## Executor actions this round (beta.29)

1. Retired headline removed from the hero; locked one-liner is the hero line.
2. Vocabulary fixes: "compendium" → "reference" in the topbar; "BOM Lookup" → "Build Sheets" in
   the Enterprise teasers; license split cited in Settings.
3. RFQ send: multi-select → per-item quantity + condition → Send RFQ over A2A (`message/send`,
   data part `partstable.rfq.request`), signed-in identity attached when present.
4. Ratification: loop completions (results shown / export / RFQ) ping `partstable.part.ratify`
   over A2A fire-and-forget; 👍/👎 verdict rides the same channel.
5. Company-level installer + bespoke-connector generator + Dabergy/Covenco A2A onboarding:
   ARCHITECTURE NEXT (not in beta.29) — sequenced after CEO reviews the ruling.
