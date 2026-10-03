# 推理稽查 — 手动验证指南

本指南逐步带你手动验证推理稽查功能：**一条** assistant 思考链，由独立 evaluator 模型打分，
且**只在用户要求时才跑**。请针对**已包含该功能的打包客户端**执行。

契约以 `docs/REASONING_AUDIT.md` 为准；下文点名的代码路径就是验证时要盯的东西。

> **开始前** — 必须配置 evaluator 模型，否则审计在发任何请求之前就被拒。确认客户端已含本功能，且：

```toml
[agent]
audit_model = "<你的评估模型 ref>"   # 例如 "deepseek-v4-pro"；必须非空
audit_threshold = 0.6
audit_effort = "low"                  # 可选：evaluator 自身的思考深度
```

> 关键点：`audit_model` 为空或解析不出来时，`Controller.resolveStandaloneModel` 会拒它并报
> `<feature>：模型未配置，请在设置中选择`（`internal/control/standalone_model.go:17`），弹窗里显示为
> `audit.failed`。它经独立 `AuditProviderResolver` 解析，**绝不会**用会话模型。

---

## A. 触发者是用户，而且点名了哪条消息

**目标**：只审计用户点的那条思考链，且只跑一次。

| 步骤 | 操作 | 预期 |
|---|---|---|
| A1 | 跑一个让模型思考的回合，然后点该回复下方的审计按钮（`components/AuditInlineCard.tsx`） | 弹窗开在这条消息上（`components/AuditModal.tsx`）并在那里开跑；会话照常流式——这是会话的影子调用，绝不在会话模型上跑 |
| A2 | 在**没有**思考链的回复上点按钮 | 尚未发出任何 evaluator 请求就被拒：`该回复没有可审计的思考过程`（`desktop/reasoning_audit.go:49-52`），显示为 `audit.failed` |
| A3 | 关掉弹窗，再在同一条回复上重开 | **重新跑一次**，上一次的结果不复存在——弹窗内是一次性、看完即弃，不落盘 |
| A4 | 在一个 Tab 里审计，再读另一个 Tab 的转录 | 不受影响：`App.AuditTurn` 不往会话历史写任何东西 |

**判定**：审计跟着被点的消息走、只跑一次、会话原样不变 = 通过。若没人点的回合被审计了 → 失败。

---

## B. 三个流式事件

**目标**：弹窗由 `audit:request` → `audit:chunk` → `audit:done` 依序喂，只有 `audit:done` 才是判定。

| 步骤 | 操作 | 预期 |
|---|---|---|
| B1 | 审计期间盯住弹窗 | 先 `audit:request`（`audit.input` / `audit.prompt` 两块面板，`AuditModal.tsx:393-406`），再 `audit:chunk` 增量落在 `audit.processReasoning` / `audit.processOutput` 下，最后 `audit:done` 填出判定 |
| B2 | 把 `audit.input` 和你点的那条回复对着看 | 它是**按 `audit_max_chars` 截断之后**的送审片段；被截时会显示 `audit.truncated` 标记（`AuditModal.tsx:396`） |
| B3 | 改系统提示词后按 `audit.rerun` | 用该提示词再发一次（`App.AuditTurn(reasoning, customPrompt)`）；`audit.resetPrompt` 恢复默认 |
| B4 | 让它失败（`audit_model` 无效、断网） | `audit.failed` 显示错误。返回的 promise 只表示"没出错"（`desktop/reasoning_audit.go:46`），所以**`audit:done` 缺失就是这一跑没跑完的唯一信号** |

**判定**：事件按序到达、判定只由 `audit:done` 给出、失败不打扰回合 = 通过。

---

## C. 评分语义（没有 Tab 徽标）

**目标**：由判定自己的分数决定"需关注"——且什么都不活过弹窗。

| 步骤 | 操作 | 预期 |
|---|---|---|
| C1 | 审计一条低质量链（提示词见下） | 分数带 `.audit-result__score.is-low`，徽标 `audit-badge--warn` 文案 `audit.attention`（`AuditModal.tsx:116-119`） |
| C2 | 审计一条干净的链 | `audit.pass`，无警告样式 |
| C3 | 看 Tab 栏与其他 Tab | **没有红点、没有跨 Tab 聚合**：徽标只活在弹窗里、随弹窗消失（全仓只剩 `styles.css` 里一条无人引用的 `.tabbar__audit-badge`） |
| C4 | 检查系统通知 | 没有。`[notifications] audit_below` 配置 schema 收，但**没有任何发送者**（只有 `internal/config` 读它） |

制造低分场景：让模型"硬想"一个它无法精确知道的问题，例如：

> 请精确推算 1987 年 3 月 14 日出生的人在 2026 年 5 月 1 日的年龄（精确到天），并解释每一步，哪怕你没把握。

或一个明显诱导编造/绕圈的请求（以触发 `factual_error` / `redundancy` / `contradiction`）。

**判定**：徽标在弹窗内跟着 `audit_threshold` 走、且什么都不活过弹窗 = 通过。

---

## D. 成本记账（P1）

**目标**：evaluator 自己的 token 与成本是真实的，且保持独立。

| 步骤 | 操作 | 预期 |
|---|---|---|
| D1 | 跑任意一次审计 | `EvalTokens` **非零**（真实 evaluator 消耗）——弹窗会打印（`audit.tokens`，`AuditModal.tsx:95`） |
| D2 | 若 `audit_model` 配了价格表 | `EvalCost` 为**非零成本**（`RateCardForModel` + `billing.BuildQuote`，`internal/control/analyze_reasoning.go:148-163`） |
| D3 | 与会话回合的 token 对比 | 审计 token **单独**存在（`ReasoningAuditTotals` 的 `EvalTokens`），**不并入**会话回合计费 |

**判定**：审计 token/成本有真实值且与会话计费分离 = 通过。

---

## E. 判定里到底带了什么

**目标**：被审计的思考链只到"要求审计的人"手里，别处没有。

| 步骤 | 操作 | 预期 |
|---|---|---|
| E1 | 读判定的数字 | 各类计数 + `issues` + `score` + `elapsedMs` + `evalTokens`/`evalCost`（`internal/event/reasoning_audit.go:16-31`） |
| E2 | 读 findings 列表 | `explanation` 与每条 finding 的 `quote` **就是被审计思考链的片段**（`reasoning_audit.go:7-10`）；它们在"用户要求之后"渲染给用户——手动模式存在的意义就在这里 |
| E3 | 看 `audit:request` 的 payload | 它把精确系统提示词 + 截断后的片段送给**本窗口**：`App.AuditTurn` 以本地 runtime 事件发出，不写入会话记录，也不进 provider 可见前缀 |

**判定**：思考链只到要求审计的人手里 = 通过。

---

## F. 容错（shadow 语义）

**目标**：审计失败绝不影响回合。

| 步骤 | 操作 | 预期 |
|---|---|---|
| F1 | 把 `audit_model` 配成**无效/不存在**的模型 | 回合照常完成；弹窗显示 `audit.failed`，会话不卡顿、不回滚任何东西 |
| F2 | 断网时审计 | 同上：失败落在弹窗里，主流程不受影响 |
| F3 | 把 `audit_model` 留空 | 按钮仍然拒：`模型未配置，请在设置中选择`，且不尝试任何请求 |

**判定**：任何审计失败都不阻塞/回滚回合 = 通过。

---

## G. 整会话审计（第二个入口）

**目标**：Tab 栏上的入口把该会话**所有已加载轮次**当一次运行审计（`docs/REASONING_AUDIT.md` §Whole-session audit）。

| 步骤 | 操作 | 预期 |
|---|---|---|
| G1 | 打开一个含多个推理轮次的会话，点 Tab 栏上的入口（`components/SessionAuditLauncher.tsx`） | `components/SessionAuditModal.tsx` 按步流式 `sessionaudit:event`，最后 `sessionaudit:done` 给出会话级判定 |
| G2 | 用很长的会话 | 每轮额度自动变小，整跑封顶 `sessionAuditMaxSegments` + 1 次调用，而不是溢出到一次调用里 |
| G3 | 切到远端 surface | 入口**禁用**——远端 surface 的轮次不是本宿主的会话 |

**判定**：整会话运行由用户触发、有界、且不碰会话历史 = 通过。

---

## 快速通过标准（汇总）

一次完整验证至少应看到：
1. 按钮审计**它所在的那条消息**、只跑一次，且空思考链会被拒（`该回复没有可审计的思考过程`）。
2. `audit:request` → `audit:chunk` → `audit:done`，且只有 `audit:done` 收尾。
3. 低于 `audit_threshold` ⇒ `audit.attention`；健康 ⇒ `audit.pass`；**没有 Tab 红点**。
4. `EvalTokens` 非零（配了价格表则有 `EvalCost`），与会话计费分离。
5. 思考链只到读者手里：不进会话记录、不进轨迹、不进 provider 可见前缀。
6. 模型无效/为空、断网、中途失败都不影响回合。
7. 关掉弹窗这次结果就没了；整会话运行是唯一的多轮产物，同样一次性。

---

## 若点了审计按钮什么都没发生

1. `audit_model` 是否非空且可解析（最常见原因）——否则弹窗显示 `模型未配置，请在设置中选择`。
2. 该条消息是否真有**思考链**——没有时弹窗在发请求前就显示 `该回复没有可审计的思考过程`。
3. 跑了但中途失败——弹窗显示 `audit.failed` 与原因（注意 `audit:done` 是否缺席）。
4. 分数本来就健康——那就**没有**红点可找：判定就是弹窗里的那个徽标。
