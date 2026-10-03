# Reasoning Audit (reasoning-quality analysis)

Reasonix can audit a turn's reasoning chain with a **dedicated evaluator model**
that is deliberately independent of the session model. Auditing is **user-
triggered only** — it never runs automatically in the background. The user
clicks an "Audit" button on an assistant reply; the audit runs the dedicated
model, and a result card (score, per-kind issue counts, cost, and the excerpts
behind them) is shown inline under that message.

```toml
[agent]
audit_model = "deepseek-v4-pro"   # standalone evaluator; empty = off
audit_threshold = 0.6             # score below this is flagged "needs attention"
audit_effort = "low"              # audit model's own thinking depth (off|low|medium|high)
audit_max_chars = 10000           # audited reasoning excerpt cap, in characters (0 = default 10000)

[notifications]
audit_below = true                # schema-only today: nothing reads it (a low score surfaces in the audit card)
```

## How it works

- **Manual trigger**: the user clicks the audit button under an assistant
  message's reasoning (`components/AuditInlineCard.tsx`), which opens
  `components/AuditModal.tsx`. The bound `App.AuditTurn` receives that message's
  reasoning chain as its argument, runs it through `Controller.AuditStream`, and
  the modal shows the exact request, the evaluator's live output and the
  verdict. The run is one-shot and view-once: closing the modal discards it and
  nothing is persisted.
- **Independent model**: `audit_model` is resolved by `AuditProviderResolver` in
  `internal/boot` (a clone of `PromptOptimizeProviderResolver`), producing a
  separate provider instance. It never runs on the session model.
- **Thinking depth**: `audit_effort` controls how deeply the audit model itself
  thinks while scoring — `off`/`low`/`medium`/`high`. Empty/`off` keeps the
  audit deterministic (`EffortOverride: "disabled"`); explicit levels pass
  through to the provider adapter.
- **Result**: a `ReasoningAuditTotals` (score + per-kind issue
  counts + `Issues` + `EvalTokens`/`EvalCost`/`ElapsedMs`, plus the reader-facing
  `Explanation`/`Findings` basis) is returned and shown in the modal; scoring
  below `audit_threshold` marks the verdict "needs attention". Cost is derived via
  the audit model's `RateCardForModel` + `billing.BuildQuote`. The audit
  **channel** stays content-free: `RecordReasoningAudit` strips the two
  reader-facing fields before any sink sees them.

The evaluator's verdict is a compact JSON object:

```json
{"score":0.4,"contradiction":1,"factual_error":2,"invalid_inference":0,"redundancy":3,"instruction_drift":0,"omission":1}
```

Six failure classes are counted: **contradiction**, **factual_error**,
**invalid_inference**, **redundancy**, **instruction_drift** and **omission**
(`hallucination` stays in the record type so earlier four-class outputs still
decode). `score` is a 0..1 aggregate quality.

## Architecture

- **`internal/event/reasoning_audit.go`** — `ReasoningAuditTotals` (the counts plus
  the reader-facing basis), `ReasoningAuditSink`, `RecordReasoningAudit` (which
  strips that basis, so the channel stays content-free). `OptionalSinkCapabilities`
  compile-asserts that every sink decorator forwards it.
- **`internal/control/analyze_reasoning.go`** — `Controller.AnalyzeReasoning`
  (the independent evaluator call, cloned from the `OptimizePrompt` sidecar
  pattern) and `Controller.AuditStream` (the streaming form the desktop uses:
  the reasoning text is the call's argument, so the host never has to guess
  which chain was meant). `Controller.auditConfig` groups the
  model/resolver/rate-card/enabled/threshold/effort into one lifetime.
- **`desktop/reasoning_audit.go`** — `App.AuditTurn` (bound, user-triggered) and
  the streaming events `audit:request` / `audit:chunk` / `audit:done`.
- **`desktop/audit_settings_app.go`** — config getters/setters for the
  settings UI.
- **Frontend** — `lib/auditStream.ts` (the event subscriptions),
  `components/AuditInlineCard.tsx` (the button; imported lazily so the audit
  code stays out of the initial bundle), `components/AuditModal.tsx` (request →
  live output → verdict) and the `auditModel` field in
  `components/SettingsPanel.tsx` (设置 → 模型).

There is **no tab badge and no cross-tab aggregation**: a verdict lives in the
modal that produced it and is never persisted.

## Model isolation & thinking depth

The evaluator **never runs on the session model**. It uses a dedicated
`audit_model` entry resolved by `AuditProviderResolver`, producing an
independent provider instance. The request is deterministic (temperature 0);
its own reasoning depth is governed by `audit_effort` (empty/`off` = no
thinking, `low`/`medium`/`high` pass through).

## Cost accounting

`EvalTokens` is filled from the evaluator's `provider.Usage`; `EvalCost` is
derived via the audit model's `RateCardForModel` + `billing.BuildQuote`. The
audit call is **not** folded into `RunBudgetSink`/`costquote` — it is a shadow
utility deliberately decoupled from the main turn budget.

## Landing note

This feature was landed with a budget carry-forward: repolint `-update`
(user-authorized exemption to the new-feature baseline rule) and frontend bundle
gate ratchets in `check-bundle-budget.mjs`. See `PR_DESCRIPTION.md` for the full
accounting of spec deviations.

## Whole-session audit (multi-turn)

The session tab strip carries a second trigger, immediately after the
new-session button, that audits **every loaded turn of the active session as one
run**. It is user-triggered, one-shot, and never persisted; like the per-turn
audit it runs on `audit_model` and never touches the session history or the
provider-visible prefix.

Scope: the turns the transcript currently holds (the desktop's loaded window).
Turns without a reasoning chain are skipped, and a remote surface's turns are
not this host's session, so the trigger stays disabled there.

Two passes, because one call would have to truncate every chain into
uselessness:

1. **Per-segment scoring** — the turns are split into at most 8 consecutive
   groups (≤8 turns each). Each group is one `audit_segment_prompt.md` call that
   scores every turn with the same six classes and formula as the single-turn
   audit (so the scores stay comparable) and additionally returns each turn's
   *conclusion* plus any conflict directly identifiable against an earlier turn.
2. **Cross-turn review** — one `audit_session_prompt.md` call receives only the
   structured per-turn results (numbers, conclusions, quoted excerpts — never the
   reasoning text) and reports cross-turn contradictions, cross-turn drift,
   repeated dead ends, unmet commitments, and error propagation, plus a session
   score and a trend.

Per-turn fidelity is bounded by `agent.audit_max_chars` and by a per-call input
budget (`sessionAuditCallInputChars`): a longer session gets a smaller per-turn
allowance rather than overflowing one call, and the run is capped at
`sessionAuditMaxSegments` + 1 calls.

- **`internal/control/analyze_session_reasoning.go`** —
  `Controller.AuditSessionReasoning` (plan → segment calls → review call →
  `SessionAuditTotals`) with `SessionAuditEvent` for step progress.
- **Prompts** — `audit_segment_prompt.md` and `audit_session_prompt.md`, embedded
  beside the per-turn one.
- **`desktop/session_audit.go`** — `App.AuditSession` (bound), streaming
  `sessionaudit:event` per step and `sessionaudit:done` with the verdict.
- **Frontend** — `components/SessionAuditLauncher.tsx` (the tab-strip trigger),
  `components/SessionAuditModal.tsx` (progress, session verdict, cross-turn
  issues, per-turn table, and the exact requests/outputs), and
  `lib/sessionAuditTurns.ts` (transcript → audit payload).

## Known gaps

- **Boot-level effect test** not yet added — REASONIX.md requires performance
  features to land with an effect test at the final `internal/boot` boundary;
  currently covered by component-boundary unit tests.
