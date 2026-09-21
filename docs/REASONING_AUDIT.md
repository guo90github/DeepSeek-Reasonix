# Reasoning Audit (reasoning-quality analysis)

Reasonix can audit a turn's reasoning chain with a **dedicated evaluator model**
that is deliberately independent of the session model. Auditing is **user-
triggered only** — it never runs automatically in the background. The user
clicks an "Audit" button on an assistant reply; the audit runs the dedicated
model, and a content-free result card (score, per-kind issue counts, cost) is
shown inline under that message.

```toml
[agent]
audit_model = "deepseek-v4-pro"   # standalone evaluator; empty = off
audit_threshold = 0.6             # score below this is flagged "needs attention"
audit_effort = "low"              # audit model's own thinking depth (off|low|medium|high)
audit_max_chars = 10000           # audited reasoning excerpt cap, in characters (0 = default 10000)

[notifications]
audit_below = true                # optional system notification when below threshold
```

## How it works

- **Manual trigger**: the user clicks "Audit" in the assistant message's action
  row. This calls `Controller.AuditTurn`, which audits the active tab's latest
  assistant reasoning and returns a content-free result.
- **Independent model**: `audit_model` is resolved by `AuditProviderResolver` in
  `internal/boot` (a clone of `PromptOptimizeProviderResolver`), producing a
  separate provider instance. It never runs on the session model.
- **Thinking depth**: `audit_effort` controls how deeply the audit model itself
  thinks while scoring — `off`/`low`/`medium`/`high`. Empty/`off` keeps the
  audit deterministic (`EffortOverride: "disabled"`); explicit levels pass
  through to the provider adapter.
- **Result**: a content-free `ReasoningAuditTotals` (score + four issue counts +
  `EvalTokens`/`EvalCost`) is returned and shown inline. Cost is derived via the
  audit model's `RateCardForModel` + `billing.BuildQuote`.

The evaluator's verdict is a compact JSON object:

```json
{"score":0.4,"contradiction":1,"hallucination":2,"redundancy":3,"instruction_drift":0}
```

Four failure classes are counted: **contradiction**, **hallucination**,
**redundancy**, and **instruction_drift**. `score` is a 0..1 aggregate quality.

## Architecture

- **`internal/event/reasoning_audit.go`** — `ReasoningAuditTotals` (content-free),
  `ReasoningAuditSink`, `RecordReasoningAudit`. `OptionalSinkCapabilities`
  compile-asserts that every sink decorator forwards it.
- **`internal/control/analyze_reasoning.go`** — `Controller.AnalyzeReasoning`
  (the independent evaluator call, cloned from the `OptimizePrompt` sidecar
  pattern) and `Controller.LatestAssistantReasoning` (last assistant reasoning
  for the manual action). `Controller.auditConfig` groups the
  model/resolver/rate-card/enabled/threshold/effort into one lifetime.
- **`desktop/reasoning_audit.go`** — `App.AuditTurn` (bound, user-triggered),
  the `audit:result` Wails event, and `notifyAuditAttention`.
- **`desktop/audit_settings_app.go`** — config getters/setters for the
  settings UI.
- **Frontend** — `lib/auditAttention.ts` store, `components/AuditResultCard`
  (the button + result card), `components/ReasoningAuditSettings` (设置 → 模型).

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
