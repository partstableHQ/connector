# WO-2026-10-04 — A2A agent must ACT on Connector signals (rfq.request + part.ratify)

**Filed by:** ZCode desktop agent (GLM, Tony's machine) · **Lane:** partsiq-core platform (the A2A agent)
**Authority:** CEO scope amendment 2026-10-04 (`RULING-2026-10-04-connector-a2a-scope-ceo.md` · Connector repo `docs/SCOPE-A2A.md`)
**Status:** CONTRACT — requirements, not suggestions. The Connector ships the sender side (beta.29, verified live); this work order is the receiver side.

## Verified current state (2026-10-04)

The Connector (Windows desktop, beta.29) sends every RFQ and every ratification over the open A2A
endpoint (`POST /a2a`, JSON-RPC 2.0 `message/send`) with **two parts per message**:
1. a **text part** (your agent already parses this — tasks complete today), and
2. a **data part**: `{kind: "data", data: {type: "<contract type>", data: {...}}}`.

Today your agent completes the tasks but does not act on the structured types. Acting on them is
this work order.

## Contract type 1 — `partstable.rfq.request`

```json
{ "type": "partstable.rfq.request",
  "data": { "lines": [ { "pn": "875488-B21", "qty": 3, "condition": "REF" },
                        { "pn": "718292-001", "qty": 5, "condition": "NEW" } ],
            "from": "jaime@dabergy.com" } }
```

- `condition` ∈ `NEW | REF | AS-IS` (REF = refurbished; the trade's default).
- `from` present when the broker is signed in; absent = anonymous (still accept — spam policy is
  the receiving lane's, but do not reject anonymous RFQs at the door).
- **Required behavior:** create an RFQ record per message (one RFQ, N lines; lines match the
  connector's quote-line doctrine: pn, qty, condition; immutable timestamp). Store `from` when
  present. Surface in the RFQ receiving queue (marketing campaign §2, fifth beat: "Send a Request
  for Quotation to whoever holds the part"). Completion message should say the RFQ was recorded
  (task state `completed` with a text confirmation).
- The text part of the same message carries the same RFQ in broker shorthand
  (`RFQ: 3x 875488-B21 (REF); 5x 718292-001 (NEW);`) — use it for logging/fallback parsing only;
  the data part is authoritative.

## Contract type 2 — `partstable.part.ratify`

```json
{ "type": "partstable.part.ratify",
  "data": { "pns": ["875488-B21", "718292-001"],
            "action": "viewed | exported | rfq | verdict",
            "verdict": "" | "up" | "down",
            "from": "jaime@dabergy.com" } }
```

- **Semantics (CEO ruling, verbatim intent):** every completed loop ratifies the part data —
  "seen, not challenged, accepted." **No news is good news.** `verdict: "down"` is the ONLY
  challenge and must trigger human review.
- **Required behavior:** increment per-PN usage/ratification counters keyed by action
  (viewed < exported < rfq — weight rising); `verdict: "up"` counts as explicit positive;
  `verdict: "down"` opens a review task on the part (review queue, not auto-mutation — humans
  decide what changes). This is the **reinforcement layer on part history** (usage = history).
- Volume note: `viewed` fires once per list per app-loop (not per keystroke) — a 6-part list = one
  `viewed` with 6 PNs. Rate is broker-scale, not telemetry-firehose scale.

## Acceptance criteria

1. `message/send` with either data type returns task state `completed` AND a completion text that
   names the outcome ("RFQ recorded: 2 lines" / "Ratified 2 parts" / "Review opened on X").
2. An RFQ sent from the Connector is visible in the receiving queue with lines intact.
3. A `verdict: "down"` ratification produces a review task; `viewed`/`exported`/`rfq` never do.
4. Both types are recognized regardless of part order, casing, and duplicate PNs (dedupe per message).
5. No auth is required on the endpoint (unchanged) — abuse policy documented by the receiving lane.

## Out of scope here

Connector-side changes (none needed — the sender is live and forward-compatible), the company-level
install model, and tenant onboarding (Dabergy/Covenco) — those live in the Connector repo handoff
and `PLAN-2026-10-04-a2a-onboarding` (management-level).
