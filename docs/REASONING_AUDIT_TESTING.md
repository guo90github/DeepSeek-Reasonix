# Reasoning Audit — Manual Verification Guide

This guide walks through a complete manual verification of the Reasoning Audit
feature (independent evaluator + cross-tab aggregation + low-score attention).
Run it against the packaged client that contains the feature.

> **Before you start** — the evaluator model must be configured, or auditing
> silently no-ops. Ensure the client was built with this feature and:

```toml
[agent]
audit_model = "<your evaluator model ref>"   # e.g. "deepseek-v4-pro"; must be non-empty
audit_enabled = true
audit_threshold = 0.6

[notifications]
enabled = true
audit_below = true        # low score triggers a system notification
```

> Key point: `audit_model` must **not** be empty, otherwise `AnalyzeReasoning`
> returns "audit model not configured" and the whole feature is silently off.
> It resolves through the independent `AuditProviderResolver`, never the session
> model.

---

## A. Model isolation (independence)

**Goal**: the audit uses a dedicated model and never disturbs the session model.

| Step | Action | Expected |
|---|---|---|
| A1 | Run a normal turn that makes the model think | Turn proceeds normally, **no added latency or interruption** (audit runs async after TurnDone, off the streaming path) |
| A2 | Watch ~seconds after the turn settles | An audit result appears (red dot / notification), proving the independent evaluator ran in the background |
| A3 | Run several turns | Session thinking config, tool calls, and token accounting are **unaffected**; the session model stays the configured one |

**Pass**: the session stays fluid and the audit lands asynchronously. If the turn
itself slows or is interrupted → fail.

---

## B. Low-score attention (core experience)

**Goal**: low reasoning quality → tab red dot + hover detail + optional system
notification.

To manufacture a low-score scenario, ask the model to "think hard" about
something it cannot know precisely, e.g.:

> Compute the exact age in days of someone born March 14, 1987 as of May 1, 2026,
> and explain every step even if unsure.

or a request that invites fabrication / going in circles (to provoke
`hallucination` / `redundancy` / `contradiction`).

| Step | Action | Expected |
|---|---|---|
| B1 | Run a low-score turn in a tab | After the turn, a **red dot** appears on that tab (`tabbar__audit-badge`) |
| B2 | Hover the red dot | Tooltip shows **score + issue count** (e.g. "Reasoning quality below threshold — score 0.3, 3 issue(s)") |
| B3 | Check system notifications | If `notifications.enabled + audit_below`, a system notification arrives ("Reasoning audit below threshold") |
| B4 | Switch to another tab | **Only the offending tab** has a red dot; healthy tabs do not |

**Pass**: low-score turn → that tab's red dot + detail tooltip. If B3 shows no
notification, check the notification switches.

---

## C. Healthy turns do not false-alarm

**Goal**: clean reasoning does not trigger the red dot.

| Step | Action | Expected |
|---|---|---|
| C1 | Run a clear, confident turn (e.g. "add 42 and 58") | **No red dot** (score ≥ threshold, `attention=false`) |
| C2 | If the tab had a red dot, run a healthy turn | Red dot **disappears** (a healthy audit clears the attention flag) |

**Pass**: healthy turns do not false-alarm and clear a prior red dot.

---

## D. Cost accounting (P1)

**Goal**: the audit's token and cost are truly recorded.

| Step | Action | Expected |
|---|---|---|
| D1 | Run any audited turn | `EvalTokens` in the audit result is **non-zero** (real evaluator spend) |
| D2 | If `audit_model` has a price table | `EvalCost` is a **non-zero cost** (via `RateCardForModel` + `BuildQuote`) |
| D3 | Compare to the session turn's token count | Audit tokens are **separate** (`EvalTokens` in `ReasoningAuditTotals`), **not** folded into the session turn billing |

**Pass**: audit token/cost have real values and stay separate from session billing.

---

## E. Content-free contract

**Goal**: the reasoning text never enters audit telemetry.

| Step | Action | Expected |
|---|---|---|
| E1 | After any audit, inspect the audit data/logs | Contains **only** counts / score / duration / tokens — **no** raw reasoning text |
| E2 | If you observe the `audit:attention` event | Payload is only `tabID / score / issues / attention / elapsedMs`, **no** reasoning text |

**Pass**: the audit event is content-free (a hard `REASONIX.md` contract).

---

## F. Fault tolerance (shadow semantics)

**Goal**: an audit failure never affects the turn.

| Step | Action | Expected |
|---|---|---|
| F1 | Set `audit_model` to an **invalid/nonexistent** model | Turn completes normally, **no red dot, no error, no stall** (audit fails silently) |
| F2 | Run a turn while offline | Turn is normal; audit failure does not disturb the main flow |
| F3 | Leave `audit_model` empty | Turn is normal, no audit at all (disabled branch) |

**Pass**: any audit failure never blocks or rolls back the turn.

---

## G. Multi-tab concurrency (aggregation isolation)

**Goal**: every active tab is audited, independently.

| Step | Action | Expected |
|---|---|---|
| G1 | Open 2-3 tabs | Each tab independently shows its own red-dot state |
| G2 | Low-score in tab A, healthy in tab B | Tab A has a red dot, tab B does not (independent) |
| G3 | Close tab B | Tab A's red dot is unaffected; a closed tab's audit state does not interfere |

**Pass**: each tab's audit state is independent and the shared aggregator is correct.

---

## Quick pass checklist

A complete verification should at least observe:
1. Audit lands **asynchronously** after the turn (no main-flow latency).
2. Low score → that tab's red dot + hover detail (score/issues).
3. Healthy → no red dot, and it clears an old one.
4. Audit has real `EvalTokens` (and `EvalCost` when a price table is configured).
5. Audit events are content-free (no reasoning text).
6. Audit failure / invalid model does not affect the turn.

---

## If no red dot appears

With correct config but no red dot, check in order:
1. `audit_model` is non-empty and valid (most common cause).
2. The turn actually produced a **reasoning chain** (`Message.Reasoning` non-empty) — a turn without thinking is not audited (`turnReasoning` empty returns early).
3. The score is truly below `audit_threshold` (a healthy turn rightly shows no red dot).
4. `audit_enabled` is `true`.
