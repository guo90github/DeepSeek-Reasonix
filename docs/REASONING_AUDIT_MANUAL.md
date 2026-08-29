# 推理审计（Reasoning Audit）— 手动触发交互重构设计

## 1. 功能概述

将审计从"后台自动静默运行"改为"**用户手动触发**"。用户对某一条 assistant 回复主动发起"审计本轮推理"，后端用独立 `audit_model` 打分并返回结果，前端就地展示。

**触发模型约束**：
- 审计**只在用户点击按钮/菜单项后**执行一次，绝不自动运行（移除现有 `tabEventSink` 在 `TurnDone` 的自动触发）。
- 后端核心审计逻辑 `Controller.AnalyzeReasoning` **保持不变**；仅调整触发时机（从自动改为手动）与前端展示。满足"仅桌面端、不修改后端核心逻辑"约束。

**前端感知变化**：从"被动红点"变为"主动操作 + 明确结果"，用户每次审计都看得到发生了什么。

---

## 2. 设置页 UI 规范

位置：**设置 → 模型（`ModelsSection`）**，复用现有 `SettingsField`（label + hint + icon）组件与分段控件样式。

| 配置项 | 类型 | 默认值 | UI 形式 | 交互逻辑 |
|---|---|---|---|---|
| **审计模型（Audit model）** | 模型选择 | 空 | `ModelPicker` 下拉 | 为空时"审计本轮推理"入口**禁用**并提示需配置。走独立 `AuditProviderResolver`，不占用会话模型。 |
| **启用审计（Enable audit）** | 开关 | 关 | 开关（Switch） | 关闭时审计入口隐藏/禁用，保证绝不运行。 |
| **审计阈值（Threshold）** | 数字 0–1 | 0.6 | 数字输入框 + 滑块 | 仅用于结果展示时的"达标/需关注"标记；**不参与触发**（因为是手动）。 |
| **通知（audit_below）** | 开关 | 关 | 开关 | 保留可选：审计结果低于阈值时发系统通知；手动触发下是"结果后通知"，非后台。 |

**UI 草图**（在 `ModelsSection` 底部新增一个 `SettingsField` 分组"Reasoning Audit"）：
```
Reasoning Audit
  [审计模型  ▼ deepseek-v4-pro]
  [启用审计  ── ON]
  [审计阈值  0.6 ──────────●──]
  [低于阈值通知  ── OFF]
  hint: 审计由你手动触发，不会自动运行。在任意回复上点击“审计”即可。
```

设置项写入现有 `cfg.Agent.AuditModel / AuditEnabled / AuditThreshold` 与 `cfg.Notifications.AuditBelow`（已有配置字段，直接复用；仅新增设置页 UI 读写入口）。

---

## 3. 主界面交互设计

### 3.1 触发入口

**位置**：assistant 消息底部 meta 操作行（`Message.tsx` 的 `msg-meta__btn` 区，与 `CopyButton` 并列）。每条 assistant 消息右侧一个"审计"按钮（图标 + 文字）。

**样式**：小图标按钮，与 `CopyButton` 同级；hover 高亮。未配置 `audit_model` 或未启用时**禁用**（置灰 + tooltip"需在 设置→模型 配置审计模型"）。

> 备选入口（二选一或并存）：消息**右键菜单**加"审计本轮推理"项。建议主用按钮（更可发现），右键菜单作为补充。

### 3.2 触发后的即时反馈（Loading）

点击后按钮进入 **Loading 态**：图标旋转 + "审计中…"，**禁用重复点击**（防连点多次计费）。evaluator 调用是同步后端请求，可能耗时数秒——必须有明确的进行中反馈，否则用户会以为没反应。

```
[审计中… ◐]   ← 按钮禁用，icon 旋转
```

### 3.3 结果展示区域

审计结果**就地嵌入该消息下方**（`AssistantReasoningPanel` 同层，新增一个 `AuditResultCard`），而非弹窗/侧边栏——与上下文就近、不打断阅读流。

**卡片布局**：
```
┌─ Reasoning Audit ─────────────┐
│ 质量分  [0.62]   ● 达标/需关注   │   ← 评分 + 状态徽标
│ 问题：矛盾 1 · 幻觉 0 · 冗余 2 · 偏航 0 │  ← 四类计数
│ 耗时 1.2s · tokens 1,500 · 成本 $0.0015 │  ← EvalTokens/EvalCost
│         [重新审计]              │   ← 可选：对同一轮再跑一次
└──────────────────────────────┘
```

**内容要素**：
- **质量分**：大号数字（0–1），颜色按阈值（≥阈值绿色"达标"，<阈值红色"需关注"）。
- **问题明细**：四类计数（contradiction / hallucination / redundancy / instruction_drift）。
- **成本信息**：`EvalTokens` / `EvalCost`（P1 已实现），让用户知道这次审计花了多少。
- **重新审计**：可选操作，对同一轮重新跑（用于偶发抖动）。

**不做**：不展示原始思考文本（保持 content-free 契约，`REASONIX.md` 硬约束）；如需看思考链，用户在消息上的 `AssistantReasoningPanel` 已可展开原始 reasoning。

---

## 4. 用户体验注意事项

### 4.1 避免误触 / 误计费
- **明确成本提示**：首次点击前 tooltip 提示"审计将调用独立模型并产生 token 费用"。
- **禁用状态清晰**：未配置模型/未启用/该消息无思考链时，按钮禁用或隐藏。
- **防连点**：Loading 期间禁用；每次审计只对"当前展示的思考链"跑一次。

### 4.2 审计失败处理
- **失败态**：后端返回错误（模型无效、超时、断网）→ 卡片显示错误占位"审计失败：[原因]"，**可重试**；不影响消息本身与对话流。
- **无思考链**：该回复没有 thinking → 按钮点击后提示"该回复没有可审计的思考过程"，不发请求（后端 `AnalyzeReasoning` 对空 reasoning 返回 error，前端直接拦截）。

### 4.3 状态一致性
- 手动触发后结果**只绑定该条消息**（按 message/turn 存储），切 Tab / 刷新后仍在（若持久化则存于会话 sidecar；否则至少会话内保留）。
- 撤销原"跨 Tab 自动聚合红点"——因已改手动，`App.reasoningAuditAgg` 的自动聚合与 `audit:attention` 自动事件**不再使用**（或降级为仅手动结果展示的数据源），避免残留后台行为。

### 4.4 与既有 Reasoning 面板的关系
`AssistantReasoningPanel`（展开思考链）与 `AuditResultCard`（质量评分）是**两个独立区块**：前者看"内容"，后者看"质量结论"。二者并列于消息下方，互不覆盖。

---

## 5. 技术实现要点（触发路径）

- **前端**：assistant 消息新增"审计"按钮 → 调现有 bridge 方法（新增 `AuditTurn(turnID)` 绑定，或复用 controller 方法）→ 后端 `Controller.AnalyzeReasoning` 对**该条消息的 `Message.Reasoning`** 打分 → 返回 `ReasoningAuditTotals` → 前端渲染 `AuditResultCard`。
- **移除自动触发**：删除 `tabEventSink.Emit` 中 `TurnDone` 分支的 `app.maybeAuditTabReasoning(...)` 调用（这是"禁止后台偷偷运行"的直接实现）。
- **结果通道**：手动结果可经新 bridge 返回（请求/响应式），不再依赖 `audit:attention` 自动事件；`auditAttention.ts` store 改造为"消息级审计结果"而非"Tab 级注意力"。
- **不改**：`AnalyzeReasoning` 内部逻辑、`event.ReasoningAuditSink` 通道、`auditConfig`、`AuditProviderResolver`。

> 后端核心逻辑零改动；仅删除一处自动触发 + 前端加按钮/卡片/设置 UI。
