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
audit_enabled = true              # allow manual auditing
audit_threshold = 0.6             # score below this is flagged "needs attention"
audit_effort = "low"              # audit model's own thinking depth (off|low|medium|high)

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

## Known gaps

- **Boot-level effect test** not yet added — REASONIX.md requires performance
  features to land with an effect test at the final `internal/boot` boundary;
  currently covered by component-boundary unit tests.
