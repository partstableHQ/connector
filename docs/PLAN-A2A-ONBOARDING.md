# PLAN — A2A onboarding: Dabergy and Covenco (management-level)

**Owner lane:** Connector (this desk) · **Approvers:** CEO + each company's management (Dabergy:
Rossi Dagnino per the 2026-09-30 multi-tenant ruling) · **Status:** PLAN — no cutover happens
without management sign-off on each side. **Authority source:** CEO scope amendment 2026-10-04
(`docs/SCOPE-A2A.md`): "The Connector needs to live at the top level, not at the user level for a
company… it's our security key that binds and keeps the customer's information local… once we
switch them over, all the communication needs to be done via A2A protocol. Also with APIs, but the
protocol is A2A."

## 0. What "onboarded" means

A company is onboarded when: (1) the **company Connector** is installed at the management level
(one install per company, not per employee), (2) all PartsTable↔company traffic — lookups, RFQs,
ratifications, order/ERP flows — rides the **A2A protocol** through that install, and (3) the
company's own backend system (ERP/CRM) is reached only through its **bespoke signed connector**
(never shared credentials in a SaaS, never scraped).

## 1. Prerequisites (before any company cutover)

| # | Item | Owner | State |
|---|---|---|---|
| P1 | A2A agent acts on `partstable.rfq.request` + `partstable.part.ratify` | platform lane | WO-2026-10-04-a2a-rfq-ratify-platform.md filed |
| P2 | Company-level installer (per-machine, one install per company) | Connector lane | beta.30 ships the installer variant |
| P3 | Company identity binding: the company Connector pairs ONCE to the company's PartsTable account; every A2A message it sends carries that identity | Connector + platform lanes | design below (§3) |
| P4 | Bespoke connector bundle for the company's ERP/CRM, signed | Connector lane (`mkconnector`, ships beta.30) + per-company build | generator live; per-company builds on demand |
| P5 | Encryption posture documented: A2A runs over TLS today; message-level signing is the FM-25 bundle signature — broker data never leaves the company machine except as A2A payloads | Connector lane | §4 |

## 2. The company install (P2/P3 design)

- One installer per company, run once by management on the machine(s) that matter (the "gateway"
  box or each broker desk — company's choice; the license is per COMPANY, never per seat —
  standing pricing ruling).
- On first run, management signs in ONCE (browser pairing, same OAuth flow as today). The pairing
  key lands in the OS keychain of that machine and **names the COMPANY, not the individual**:
  A2A messages carry `from.company` (verified against the account) — employee names never required.
- **The security-key property:** platform services can require "requests must originate from a
  paired company Connector" (the A2A identity IS the key). No connector → website operator
  features stay locked, per the CEO's model. Enforcement lands platform-side; the Connector side
  is ready (identity on every message since beta.29).

## 3. Cutover runbook (per company; Dabergy first — API permission already granted)

1. **Baseline (read-only week):** company Connector installed; broker desks keep current tools;
   lookups/RFQs exercised manually. Ratification flows accumulate quietly (no news is good news).
2. **Mirror (parallel week):** the company's ERP connector (bespoke bundle) runs in READ mode —
   inventory/orders mirrored A2A into the company's own PartsTable view; nothing written back yet.
   Parity: WO-12 paired-run pattern (row-identical md5) applies to any mirrored dataset.
3. **Write lane (approval gate):** RFQs and order flows go A2A for real, beside the old path.
   Divergences block cutover.
4. **Flip (management sign-off):** old direct-API path switched off; A2A is the ONLY path. For
   Dabergy this retires the standing direct-API permission the CEO referenced ("right now we have
   Dabergy's permission to use their API and everything, but once we switch them over…").
5. **Rollback:** flip back is a config change on the company side (old path kept warm for 30 days).

## 4. Security posture (the sales point)

- Credentials: BYO only, OS keychain, zero-knowledge (FM-25 doctrine) — "your ERP password never
  leaves your machine."
- Transport: A2A over HTTPS/TLS to partstable.com (the open endpoint).
- Message integrity: bespoke connector bundles are **ed25519-signed** (`mkconnector`, beta.30);
  the app refuses unsigned/tampered bundles outside dev mode.
- Data locality: broker part lists live in the company's Connector; PartsTable receives only what
  a loop explicitly sends (RFQ lines, ratification counters) — the ratification layer is counters,
  never list dumps.

## 5. Covenco

Same runbook after Dabergy proves it. Covenco's systems differ (per the network-dashboard lane's
coverage work) — the bespoke bundle is the only company-specific build artifact; everything else
in this plan is company-agnostic.

## 6. What this plan deliberately does NOT do

- No tenant boundary crossing: Dabergy marketing/channels are Rossi's lane; this plan is transport
  and install only, and touches no Dabergy surface copy.
- No auto-cutover: every flip is a management decision with a human signature on each side.
