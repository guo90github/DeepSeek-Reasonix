# 推理稽查（Reasoning Audit，思考链质量分析）

Reasonix 用**独立 evaluator 模型**分析一条回复的思考链质量——该模型与会话模型完全隔离。
审计是**用户手动触发**的，绝不在后台自动运行。用户在 assistant 回复上点击"审计"按钮，
审计用独立模型运行，结果以 content-free 结果卡（评分、四类问题计数、成本）就地显示在该消息下方。

```toml
[agent]
audit_model = "deepseek-v4-pro"   # 独立评估模型；留空 = 关闭
audit_enabled = true              # 允许手动审计
audit_threshold = 0.6             # 低于此评分标记为"需关注"
audit_effort = "low"              # 审计模型自身的思考深度 (off|low|medium|high)

[notifications]
audit_below = true                # 低于阈值时的可选系统通知
```

## 工作原理

- **手动触发**：用户在 assistant 消息操作行点击"审计"。调用 `Controller.AuditTurn`，
  审计活动 Tab 的最新 assistant 推理并返回 content-free 结果。
- **独立模型**：`audit_model` 由 `internal/boot` 的 `AuditProviderResolver` 解析
  （克隆自 `PromptOptimizeProviderResolver`），生成独立 provider 实例，绝不在会话模型上运行。
- **思考深度**：`audit_effort` 控制审计模型打分时自身的思考深度——`off`/`low`/`medium`/`high`。
  空或 `off` 保持审计确定性强（`EffortOverride: "disabled"`）；显式档位透传给 provider adapter。
- **结果**：返回 content-free 的 `ReasoningAuditTotals`（评分 + 四类计数 + `EvalTokens`/`EvalCost`）
  并就地显示。成本经审计模型的 `RateCardForModel` + `billing.BuildQuote` 计算。

evaluator 返回紧凑的 JSON 判定：

```json
{"score":0.4,"contradiction":1,"hallucination":2,"redundancy":3,"instruction_drift":0}
```

四类质量问题被计数：**contradiction**（逻辑矛盾）、**hallucination**（幻觉）、
**redundancy**（冗余）、**instruction_drift**（偏离指令）。`score` 是 0..1 的综合质量分。

## 架构

- **`internal/event/reasoning_audit.go`** — `ReasoningAuditTotals`（content-free）、
  `ReasoningAuditSink`、`RecordReasoningAudit`。`OptionalSinkCapabilities` 编译期断言所有
  sink 装饰器都透传。
- **`internal/control/analyze_reasoning.go`** — `Controller.AnalyzeReasoning`
  （独立 evaluator 调用，克隆自 `OptimizePrompt` 侧车模式）与
  `Controller.LatestAssistantReasoning`（手动动作取最新 assistant 推理）。
  `Controller.auditConfig` 把 model/resolver/rate-card/enabled/threshold/effort 收敛为
  一个生命周期。
- **`desktop/reasoning_audit.go`** — `App.AuditTurn`（绑定，用户触发）、`audit:result`
  Wails 事件、`notifyAuditAttention`。
- **`desktop/audit_settings_app.go`** — 设置 UI 的 config getter/setter。
- **前端** — `lib/auditAttention.ts` store、`components/AuditResultCard`（按钮 + 结果卡）、
  `components/ReasoningAuditSettings`（设置 → 模型）。

## 模型隔离与思考深度

evaluator **绝不在会话模型上运行**。它使用独立的 `audit_model` 配置项，由
`AuditProviderResolver` 解析，生成独立的 provider 实例。请求确定性强（temperature 0）；
其自身思考深度由 `audit_effort` 决定（空/`off` = 不思考，`low`/`medium`/`high` 透传）。

## 成本记账

`EvalTokens` 从 evaluator 的 `provider.Usage` 填充；`EvalCost` 经审计模型的
`RateCardForModel` + `billing.BuildQuote` 计算。审计调用**不**并入
`RunBudgetSink`/`costquote`——它是 shadow 实用工具，有意与主回合预算解耦。

## 落地说明

本特性落地采用了预算承载：repolint `-update`（用户授权对"禁止为新特性扩基线"规则的豁免）
以及前端 bundle 门禁 ratchet（`check-bundle-budget.mjs`）。规范偏离的完整清单见
`PR_DESCRIPTION.md`。

## 已知缺口

- **boot-level effect test** 尚未补充——REASONIX.md 要求性能特性在 `internal/boot`
  最终边界落 effect test；当前仅以组件边界单测覆盖。
