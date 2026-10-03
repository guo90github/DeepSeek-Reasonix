# Reasoning Audit — Manual Verification Guide

This guide walks through a complete manual verification of the Reasoning Audit
feature: **one** assistant reasoning chain, scored by a dedicated evaluator model,
and only when the user asks for it. Run it against the packaged client that
contains the feature.

The contract lives in `docs/REASONING_AUDIT.md`; the code paths named below are
the ones a verifier has to watch.

> **Before you start** — the evaluator model must be configured, or the audit is
> refused before any request is made. Ensure the client was built with this
> feature and:

```toml
[agent]
audit_model = "<your evaluator model ref>"   # e.g. "deepseek-v4-pro"; must be non-empty
audit_threshold = 0.6
audit_effort = "low"                          # optional: the evaluator's own thinking depth
```

> Key point: an empty or unresolvable `audit_model` is refused by
> `Controller.resolveStandaloneModel` with `<feature>：模型未配置，请在设置中选择`
> (`internal/control/standalone_model.go:17`) and surfaces as `audit.failed` in
> the modal. It resolves through the independent `AuditProviderResolver`, never
> the session model.

---

## A. The trigger is the user, and it names the message

**Goal**: exactly the chain the user pointed at is audited, once.

| Step | Action | Expected |
|---|---|---|
| A1 | Run a turn that makes the model think, then click the audit button under that reply (`components/AuditInlineCard.tsx`) | The modal opens on that message (`components/AuditModal.tsx`) and the audit starts there; the session keeps streaming normally — the call is a shadow of the session, never on the session model |
| A2 | Click the button on a reply with **no** reasoning chain | Refused before any evaluator request: `该回复没有可审计的思考过程` (`desktop/reasoning_audit.go:49-52`), shown as `audit.failed` |
| A3 | Close the modal, then reopen it on the same reply | A **fresh** run starts and the previous one is gone — the modal's run is view-once and nothing is persisted |
| A4 | Audit in one tab, then read another tab's transcript | Untouched: `App.AuditTurn` writes nothing into session history |

**Pass**: the audit follows the clicked message, runs once, and leaves the
session exactly as it was. If a turn gets audited that nobody clicked → fail.

---

## B. The three streaming events

**Goal**: the modal is fed by `audit:request` → `audit:chunk` → `audit:done`, in
that order, and only `audit:done` is a verdict.

| Step | Action | Expected |
|---|---|---|
| B1 | Watch the modal during a run | `audit:request` first (the `audit.input` / `audit.prompt` panels, `AuditModal.tsx:393-406`), then `audit:chunk` deltas under `audit.processReasoning` / `audit.processOutput`, then `audit:done` fills the verdict |
| B2 | Compare `audit.input` with the reply you clicked | It is the audited excerpt **after** truncation to `audit_max_chars`; when it was cut, the `audit.truncated` marker shows (`AuditModal.tsx:396`) |
| B3 | Edit the system prompt and press `audit.rerun` | A second call with that prompt (`App.AuditTurn(reasoning, customPrompt)`); `audit.resetPrompt` restores the default |
| B4 | Make it fail (invalid `audit_model`, offline) | `audit.failed` shows the error. The resolved promise only means "no error" (`desktop/reasoning_audit.go:46`), so **a missing `audit:done` is the only sign of a run that never finished** |

**Pass**: the events arrive in order, the verdict comes only from `audit:done`,
and a failure never disturbs the turn.

---

## C. Score semantics (no tab badge)

**Goal**: the verdict's own score decides "needs attention" — and nothing
outlives the modal.

| Step | Action | Expected |
|---|---|---|
| C1 | Audit a low-quality chain (prompt below) | Score in `.audit-result__score.is-low` and the badge `audit-badge--warn` labelled `audit.attention` (`AuditModal.tsx:116-119`) |
| C2 | Audit a clean chain | `audit.pass`, no warning styling |
| C3 | Look at the tab strip and at other tabs | **No red dot and no cross-tab aggregation**: the badge lives inside the modal and dies with it (the only leftover is an unreferenced `.tabbar__audit-badge` rule in `styles.css`) |
| C4 | Check system notifications | None. `[notifications] audit_below` is accepted by the config schema but has **no sender** (only `internal/config` reads it) |

To manufacture a low-score scenario, ask the model to "think hard" about
something it cannot know precisely, e.g.:

> Compute the exact age in days of someone born March 14, 1987 as of May 1, 2026,
> and explain every step even if unsure.

or a request that invites fabrication / going in circles (to provoke
`factual_error` / `redundancy` / `contradiction`).

**Pass**: the badge follows `audit_threshold` inside the modal, and nothing
outlives the modal.

---

## D. Cost accounting (P1)

**Goal**: the evaluator's own token and cost are real, and stay separate.

| Step | Action | Expected |
|---|---|---|
| D1 | Run any audit | `EvalTokens` is **non-zero** (real evaluator spend) — the modal prints it (`audit.tokens`, `AuditModal.tsx:95`) |
| D2 | If `audit_model` has a price table | `EvalCost` is a **non-zero cost** (`RateCardForModel` + `billing.BuildQuote`, `internal/control/analyze_reasoning.go:148-163`) |
| D3 | Compare with the session turn's token count | Audit tokens are **separate** (`EvalTokens` in `ReasoningAuditTotals`), **not** folded into the session turn's billing |

**Pass**: audit token/cost have real values and stay separate from session billing.

---

## E. What the verdict carries

**Goal**: the audited chain reaches the reader who asked — and nothing else.

| Step | Action | Expected |
|---|---|---|
| E1 | Read the verdict's numbers | Counts per kind + `issues` + `score` + `elapsedMs` + `evalTokens`/`evalCost` (`internal/event/reasoning_audit.go:16-31`) |
| E2 | Read the findings list | `explanation` and each finding's `quote` **are excerpts of the audited chain** (`reasoning_audit.go:7-10`); they render for the reader after the reader asked — that is what the manual mode is for |
| E3 | Inspect the `audit:request` payload | It carries the exact system prompt + the truncated excerpt to **this window only**: `App.AuditTurn` emits it as a local runtime event and writes nothing into the session record or the provider-visible prefix |

**Pass**: the chain only ever reaches the person who asked for the audit.

---

## F. Fault tolerance (shadow semantics)

**Goal**: an audit failure never affects the turn.

| Step | Action | Expected |
|---|---|---|
| F1 | Set `audit_model` to an **invalid/nonexistent** model | The turn completes normally; the modal shows `audit.failed`, the session does not stall and nothing is rolled back |
| F2 | Audit while offline | Same: the failure lands in the modal, the main flow is untouched |
| F3 | Leave `audit_model` empty | The button still refuses — `模型未配置，请在设置中选择` — and no request is attempted |

**Pass**: any audit failure never blocks or rolls back the turn.

---

## G. Whole-session audit (the second trigger)

**Goal**: the tab-strip trigger audits every loaded turn of the session as one
run (`docs/REASONING_AUDIT.md` §Whole-session audit).

| Step | Action | Expected |
|---|---|---|
| G1 | Open a session with several reasoning turns and click the tab-strip trigger (`components/SessionAuditLauncher.tsx`) | `components/SessionAuditModal.tsx` streams `sessionaudit:event` per step, then `sessionaudit:done` with the session verdict |
| G2 | Use a long session | The per-turn allowance shrinks and the run is capped at `sessionAuditMaxSegments` + 1 calls instead of overflowing one call |
| G3 | Switch to a remote surface | The trigger is **disabled** — a remote surface's turns are not this host's session |

**Pass**: the whole-session run is user-triggered, bounded, and never touches the
session history.

---

## Quick pass checklist

A complete verification should at least observe:
1. The button audits **the message it sits under**, once, and refuses an empty chain (`该回复没有可审计的思考过程`).
2. `audit:request` → `audit:chunk` → `audit:done`, and only `audit:done` closes the run.
3. Score below `audit_threshold` ⇒ `audit.attention`; healthy ⇒ `audit.pass`; **no tab red dot**.
4. `EvalTokens` non-zero (and `EvalCost` when a price table is configured), separate from session billing.
5. The chain reaches only the reader: no session record, no trajectory, no provider-visible prefix.
6. An invalid/empty model, an offline run or a mid-run failure never affects the turn.
7. Closing the modal loses the run; the whole-session run is the only multi-turn artifact, and it is one-shot too.

---

## If nothing happens when you click the audit button

1. `audit_model` non-empty and resolvable (most common cause) — otherwise the modal shows `模型未配置，请在设置中选择`.
2. The message has a reasoning chain — otherwise the modal shows `该回复没有可审计的思考过程` before any request.
3. The run failed mid-call — the modal shows `audit.failed` with the reason (watch for a missing `audit:done`).
4. The score is healthy — then there is **no** red dot to look for: the verdict is the badge inside the modal.
