# 推理稽查（Reasoning Audit，思考链质量分析）

Reasonix 用**独立 evaluator 模型**分析一条回复的思考链质量——该模型与会话模型完全隔离。
审计是**用户手动触发**的，绝不在后台自动运行：用户点击 assistant 回复思考链下方的审计按钮
（`components/AuditInlineCard.tsx`）打开 `components/AuditModal.tsx`，结果（评分、各类问题计数、成本）
在弹窗里就地展示——**一次性、看完即弃**，不落盘、不聚合到 Tab。

```toml
[agent]
audit_model = "deepseek-v4-pro"   # 独立评估模型；留空 = 关闭
audit_threshold = 0.6             # 低于此评分标记为"需关注"
audit_effort = "low"              # 审计模型自身的思考深度 (off|low|medium|high)
audit_max_chars = 10000           # 送审思考链的字符上限（按字符计；0 = 默认 10000）

[notifications]
audit_below = true                # 目前只有 schema：没有代码读它（低分在审计弹窗里体现）
```

## 工作原理

- **手动触发**：用户点击该条回复思考链下方的审计按钮；绑定方法 `App.AuditTurn` 收到**该条消息的思考链**
  作为实参，经 `Controller.AuditStream` 跑一次，弹窗依次展示精确请求、evaluator 的实时输出与判定。
- **独立模型**：`audit_model` 由 `internal/boot` 的 `AuditProviderResolver` 解析
  （克隆自 `PromptOptimizeProviderResolver`），生成独立 provider 实例，绝不在会话模型上运行。
- **思考深度**：`audit_effort` 控制审计模型打分时自身的思考深度——`off`/`low`/`medium`/`high`。
  空或 `off` 保持审计确定性强（`EffortOverride: "disabled"`）；显式档位透传给 provider adapter。
- **结果**：返回 content-free 的 `ReasoningAuditTotals`（评分 + 各类计数 + `Issues` +
  `EvalTokens`/`EvalCost`/`ElapsedMs`）并在弹窗中展示；低于 `audit_threshold` 时判定标"需关注"。
  成本经审计模型的 `RateCardForModel` + `billing.BuildQuote` 计算。

evaluator 返回紧凑的 JSON 判定：

```json
{"score":0.4,"contradiction":1,"factual_error":2,"invalid_inference":0,"redundancy":3,"instruction_drift":0,"omission":1}
```

六类质量问题被计数：**contradiction**（逻辑矛盾）、**factual_error**（事实错误）、
**invalid_inference**（无效推理）、**redundancy**（冗余）、**instruction_drift**（偏离指令）、
**omission**（漏说）。（记录类型仍保留 `hallucination`，以便旧的四类输出照样能解码。）
`score` 是 0..1 的综合质量分。

## 架构

- **`internal/event/reasoning_audit.go`** — `ReasoningAuditTotals`（content-free）、
  `ReasoningAuditSink`、`RecordReasoningAudit`。`OptionalSinkCapabilities` 编译期断言所有
  sink 装饰器都透传。
- **`internal/control/analyze_reasoning.go`** — `Controller.AnalyzeReasoning`
  （独立 evaluator 调用，克隆自 `OptimizePrompt` 侧车模式）与
  `Controller.AuditStream`（桌面走的流式形态：推理文本是调用的实参，宿主无需猜是哪条链）。
  `Controller.auditConfig` 把 model/resolver/rate-card/enabled/threshold/effort 收敛为
  一个生命周期。
- **`desktop/reasoning_audit.go`** — `App.AuditTurn`（绑定，用户触发）与
  `audit:request` / `audit:chunk` / `audit:done` 三个流式事件。
- **`desktop/audit_settings_app.go`** — 设置 UI 的 config getter/setter。
- **前端** — `lib/auditStream.ts`（事件订阅）、`components/AuditInlineCard.tsx`（按钮；
  懒加载，使审计代码不进首包）、`components/AuditModal.tsx`（请求 → 实时输出 → 判决），
  以及 `components/SettingsPanel.tsx` 的 `auditModel` 字段（设置 → 模型）。

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

## 整会话审计（多轮）

会话栏（新建会话按钮右侧紧邻位置）另有一个入口，把**当前会话已加载的全部轮次**作为一次运行来审计。
与单轮审计一样：用户手动触发、一次性、不落盘，用 `audit_model` 跑，绝不进入会话历史或
provider 可见前缀。

范围：前端转录当前持有的轮次（桌面端的已加载窗口）。没有思考链的轮次会被跳过；
远端 surface 的轮次不属于本机会话，因此该入口在远端 surface 下保持禁用。

分两趟，因为单次调用必须把每条思考链截断到失去意义：

1. **分段逐轮评分** — 轮次按顺序切成最多 8 段（每段 ≤8 轮）。每段一次
   `audit_segment_prompt.md` 调用，用与单轮审计完全相同的六类与公式给每轮打分
   （因此分数与单轮审计可比），并额外返回该轮的*结论*以及可直接指认的、与更早轮次的冲突。
2. **跨轮会审** — 一次 `audit_session_prompt.md` 调用，输入只有逐轮的结构化结果
   （数字、结论、被引用的原文片段，绝不含思考原文），产出跨轮矛盾、跨轮偏航、重复死路、
   未兑现承诺、错误传播，以及会话总评与趋势。

逐轮保真度受 `agent.audit_max_chars` 与单次调用输入预算（`sessionAuditCallInputChars`）双重约束：
会话越长，每轮可用额度越小，而不是撑爆一次调用；整体调用数上界为
`sessionAuditMaxSegments` + 1。

- **`internal/control/analyze_session_reasoning.go`** — `Controller.AuditSessionReasoning`
  （规划 → 分段调用 → 会审调用 → `SessionAuditTotals`），以及用于步骤进度的 `SessionAuditEvent`。
- **提示词** — `audit_segment_prompt.md` 与 `audit_session_prompt.md`，与单轮提示词一同 embed。
- **`desktop/session_audit.go`** — `App.AuditSession`（绑定），逐步流式发
  `sessionaudit:event`，结束时发 `sessionaudit:done` 携带判定结果。
- **前端** — `components/SessionAuditLauncher.tsx`（会话栏入口）、
  `components/SessionAuditModal.tsx`（进度、会话总评、跨轮问题、逐轮表、以及确切的请求/输出）、
  `lib/sessionAuditTurns.ts`（转录 → 审计负载）。

## 已知缺口

- **boot-level effect test** 尚未补充——REASONIX.md 要求性能特性在 `internal/boot`
  最终边界落 effect test；当前仅以组件边界单测覆盖。
