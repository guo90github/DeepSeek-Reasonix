# 结构化思考步骤展示 — 设计文档

> 目标效果：`result.md`（桌面端结构化思考步骤卡片：Step 1..N 标题、状态、每步耗时、单步展开/折叠、进度显示）。
> 对照旧方案：`prd.md`（"强制结构化思维"，本文第 4 章说明为何不采纳）。

---

## 1. 背景与目标

### 1.1 目标

把桌面端当前"一整段平铺的 thinking 文本"升级为**结构化步骤列表**：

- 思考流式中，步骤实时出现，带状态（`pending` / `streaming` / `complete`）与每步耗时；
- 每步可独立展开/折叠，点击查看该步的详细内容；
- 思考结束后显示 `N/N steps completed` 与总耗时；
- 保持现有的折叠摘要、显示模式（hidden/summary/auto/expanded）、费用与 token 展示不变。

### 1.2 非目标（明确不做）

- ❌ 不强制模型按固定模板思考（不做格式校验、不做重新生成重试循环）；
- ❌ 不新增后端事件类型、不改 wire 协议、不改 `control.Controller` 与 system prompt；
- ❌ 不做步骤级语义验证（不校验"理解→分析→规划→执行→验证"的顺序）。

理由见第 4 章方案对比。

---

## 2. 术语

| 术语 | 含义 |
| --- | --- |
| reasoning text | 模型思考的自由文本流，经 `kind: "reasoning"` 事件增量到达前端（`LiveStream.reasoning`） |
| marker | 步骤标记行，如 `Step 1: 理解需求`、`### 2. 分析代码`、`第 3 步：设计方案` |
| step | 由 marker 切分出的一个思考步骤：`{ index, title, content, status, startedAt, completedAt }` |
| 分割器 | 纯函数 `segmentReasoningSteps(text) → Step[]`，对 reasoning text 做增量切分 |

---

## 3. 现状盘点（代码事实）

### 3.1 已具备的能力（无需开发）

| 能力 | 位置 |
| --- | --- |
| reasoning 流式事件（增量、按帧合并） | `desktop/frontend/src/lib/streamDeltaBatch.ts`、`useController.ts` `applyLiveSegments` |
| 平铺 reasoning 文本 + 起止时间戳 | `LiveStream`（`useController.ts:186`）：`reasoningStartedAt / reasoningCompletedAt / reasoningComplete` |
| 思考总耗时展示（"thinking · 12s"） | `liveReasoningDurationMs`（`useController.ts:1047`）+ `AssistantReasoningPanel.tsx` |
| 面板展开/折叠、流式自动跟随、完成后自动收起、用户覆盖 | `AssistantReasoningPanel.tsx` + `useReasoningDisplayMode`（`reasoningDisplayPreference.ts`，模式 hidden/summary/auto/expanded） |
| 折叠后单行摘要 | `ReasoningSummary.tsx` + `reasoningSummary.ts` |
| 费用 / token 展示 | `sessionCost / sessionTokens / turnCost`（`WireEvent`，`types.ts:361`），StatusBar / ContextPanel / ContextWindowRing / UsageStatsPanel |
| 工具调用卡片（dispatch/progress/result + diff + 摘要） | `ToolCard` + `tools.ts`（`subjectOf / diffsFor / summarize`） |
| 步骤式列表 UI 先例（pending/in_progress/completed） | `TodoPanel.tsx`（计划模式 todo 列表） |
| i18n 三语体系 | `locales/en.ts`、`locales/zh.ts`、`locales/zh-TW.ts` |
| reasoning 样式基座 | `styles.css`（`.reasoning__*`，约 4672 行起） |

### 3.2 缺口（本次开发内容）

| 缺口 | 说明 |
| --- | --- |
| 步骤分割 | reasoning 是单个平铺字符串，无任何 step 概念；`EventKind` 26 个事件类型中无 step 类（`types.ts:9`） |
| 每步状态与耗时 | 需由分割器 + 增量时间戳派生 |
| 步骤卡片 UI | 单步展开/折叠、○/●/✓ 状态、进度 `N/N` |
| 折叠已完成步骤 | 文档中 "★ 折叠步骤 1-3" 的交互 |

---

## 4. 方案选型

### 方案 A：前端分割（**采纳**）

在前端把已持有的 `reasoning` 文本增量切分为步骤，纯派生状态渲染。

- **改动面**：仅桌面前端 2-3 个新文件 + `AssistantReasoningPanel` 集成 + 样式 + i18n + 测试。
- **优点**：零协议变更、零后端改动、system prompt 前缀字节稳定（符合 `REASONIX.md` cache-first 约定）；会话恢复后对已存 reasoning 文本重跑一次分割即可重建步骤；对所有 provider 统一生效（DeepSeek `reasoning_content` / Anthropic `thinking` block / Gemini thought signature 到达前端时都是平铺文本）。
- **缺点**：步骤结构与标题取决于模型是否书写 marker；无 marker 时退化为现状（单步）。

### 方案 B：后端发 `reasoning_step` 事件（否决）

在 Go 侧解析 reasoning 并发射新的事件类型。

- 否决理由：后端拿到的同样是自由文本，解析逻辑放 Go 与放前端收益完全一样，却要新增 wire 事件类型、改 `EventKind`、动 controller 事件面，且需要同步修改 serve/SSE 等多个前端的协议消费方。属于协议膨胀，无增益。

### 方案 C：`prd.md` 的"强制结构化"（否决）

系统提示注入 `<thinking-protocol>` XML + Go 侧 `ParseReasoningStructure` 校验 + 格式不符时注入纠正指令重试循环。

- 否决理由：
  1. **缓存违约**：修改 base prompt 前缀会击穿 DeepSeek 自动前缀缓存，违反项目 cache-first 顶层约定（且该改动属 cache-sensitive，需 `System-prompt-review` 审查，`scripts/check-cache-impact.sh` 拦截）。
  2. **成本不可控**：格式校验失败触发重新生成 = 一次完整请求的额外费用与延迟，且无上界（"强制循环"）。
  3. **不可靠**：DeepSeek API 不保证输出 XML 结构（`prd.md` 自己也承认"无法强制"）；把 UI 展示绑死在模型格式上，模型一旦不配合整个功能失效。
  4. **架构方向错误**：思考是模型的私有认知过程，框架不应以校验-重试的方式钳制它；展示层做"尽力而为的呈现"即可。

### 4.1 对比

| 维度 | A 前端分割 | B 后端事件 | C 强制结构化 |
| --- | --- | --- | --- |
| 改动面 | 前端 3 文件级 | Go + wire + 多前端 | Go + prompt + 配置 + 重试 |
| 缓存稳定性 | 保持 | 保持 | **破坏** |
| 模型不配合时 | 优雅降级为现状 | 同 A | 功能失效 / 无限重试 |
| 成本 | 0 | 0 | 每次失败 +1 次完整请求 |
| 会话恢复 | 重跑分割 | 需持久化 step 结构 | 需持久化步骤 + 校验 |

---

## 5. 详细设计

### 5.1 数据模型

```typescript
// desktop/frontend/src/lib/reasoningSteps.ts
export type ReasoningStepStatus = "pending" | "streaming" | "complete";

export interface ReasoningStep {
  index: number;          // 1-based
  title: string;          // marker 行文本（去标记前缀），如 "理解需求"
  content: string;        // 该步全部内容（含尾部未完成行）
  status: ReasoningStepStatus;
  startedAt?: number;     // 该步第一个 delta 到达时间（marker 行完成后）
  completedAt?: number;   // 下一个 marker 行完成时间；流式中为 undefined
}

export function segmentReasoningSteps(text: string): ReasoningStep[];
```

### 5.2 分割算法（marker 检测）

**核心原则：只在"已完成的行"上做切分**——marker 正则只匹配以换行符结束的整行，避免流式 token 半行（`"Step"` → `"Step 1"` → `"Step 1:"`）造成误切或闪烁。未完成行始终属于当前 active step 的尾部内容。

**硬标记（必然切分）**，行首匹配：

```
^(?:Step|步骤|第\s*\d+\s*步|第[一二三四五六七八九十]+步)\s*[:：]?\s*(.*)$
^(#+\s*)\d+[.)、]\s*(.*)$          // markdown 编号标题，如 "### 1. 分析"
```

**软标记（≥2 处出现才启用）**：行首编号列表项 `^\d+[.)、]\s+(.+)$`。
防过分割：要求该行 ≤ 40 字符且后续内容行不全是编号项；soft marker 只在同一次思考中出现 ≥2 次且行间无正文时才计入步骤。软标记不满足条件时整体回退。

**标题提取**：去掉标记前缀后取行文本；为空时回退为 `Step {index}`。

**降级规则（关键，保证零回归）**：`segmentReasoningSteps` 检出步骤数 < 2 时返回空数组，调用方继续渲染现有平铺视图（`StreamingReasoningText` / `Markdown`）。即"模型写结构 → 步骤卡片；不写 → 现状"，中间无半成品状态。

### 5.3 流式增量与时间戳

不侵入 reducer（`applyDeltaSegments`、`LiveStream` 不动），由 Hook 做派生：

```typescript
// desktop/frontend/src/lib/useReasoningSteps.ts
// 输入：item.reasoning（每帧都在变）、running、reasoningComplete
// 内部：ref 保存上一帧文本，diff 出新增片段；若新增片段跨过了一个已完成 marker 行：
//   - 上一个 active step 置 complete，记录 completedAt = now
//   - 新建 step，startedAt = now
// 返回：ReasoningStep[]（含一个 streaming 状态的 active step）
```

- 每步耗时 = `next.startedAt − this.startedAt`（即该 marker 行完成时刻），与现有 `reasoningDurationMs` 的语义一致（现有总耗时 = `reasoningCompletedAt − reasoningStartedAt`，见 `useController.ts:1047`）。
- 流式中 active step 显示"已进行 Xs"（用 `reasoningStartedAt` 起算的累计时间），与现有 header 的 `[thinking: 12s]` 同源。
- `reasoningComplete` 置位时最后一个 active step 落定，得到 `N/N completed` 与总耗时。

### 5.4 状态机

```
未检测到 marker（<2 步）          → 平铺视图（现状），无步骤 UI
检测到 ≥2 步，流式中              → 步骤列表：✓×k + ●（active）+ 摘要行
reasoningComplete 到达            → 全部置 ✓，显示 N/N completed + 总耗时
```

进度显示遵循诚实原则（见第 7 章）：流式中显示"已检测 N 个步骤"，不虚构总数；思考结束后显示 `N/N`。

### 5.5 UI 组件与集成

**新增 `desktop/frontend/src/components/StructuredReasoningStep.tsx`**（对应 `result.md` 的 `ReasoningStepProps`，但收敛为现有代码风格）：

```tsx
interface ReasoningStepProps {
  step: ReasoningStep;
  isActive: boolean;        // 流式跟随目标
  defaultExpanded: boolean;
}
// 渲染：○/●/✓ 图标 + "Step {index}" + title + [耗时] + chevron
// 流式中 active 步带 pulse 动画（复用现有 .reasoning__head[data-running] 的动画模式）
```

**集成点 `AssistantReasoningPanel.tsx`**：

- 当 `steps.length >= 2` 时，`reasoning__body` 内渲染步骤列表替代平铺文本；否则走现有分支（`StreamingReasoningText` / `Markdown`），零行为变化。
- 面板级展开/折叠、显示模式（auto 跟随、完成后自动收起）、用户覆盖逻辑**全部沿用**，不改动。
- 单步展开/折叠是步骤卡片本地 state；"折叠已完成步骤"（文档的 ★）为面板内二级开关，localStorage 持久化。
- 样式追加进 `styles.css` 的 `.reasoning__*` 区段（约 4672 行起），沿用现有 CSS 变量与主题。

### 5.6 历史恢复

reasoning 按消息文本持久化（provider 重放 `m.ReasoningContent`，见 `internal/provider/anthropic/missing_reasoning_fallback.go`），步骤是纯派生数据、不入库。会话/历史加载时对已存 reasoning 文本执行一次全量 `segmentReasoningSteps` 即可重建步骤列表（此时无流式，全部为 complete）。

### 5.7 可访问性与 i18n

- 步骤 header 为 `<button aria-expanded>`，与现有 `reasoning__head` 一致（已有 `message-reasoning-panel.test.tsx` 断言模式可复用）。
- 新增 i18n key（en/zh/zh-TW 同步）：
  - `reasoning.stepsDetected` — "已检测 {n} 个步骤"
  - `reasoning.stepsComplete` — "{n}/{total} 步完成"
  - `reasoning.stepRunning` / `reasoning.stepDone` — 每步状态后缀
  - `reasoning.collapseCompleted` — "折叠已完成步骤"
- 步骤标题是模型生成文本，直接渲染，不做翻译。

### 5.8 测试策略

| 层 | 内容 |
| --- | --- |
| 分割器单测（新） | `reasoningSteps.test.ts`：硬标记（中/英/编号标题）、软标记、半行流式不误切、无 marker 返回空、title 提取回退 |
| Hook 测试（新） | `useReasoningSteps`：delta 增量推进时步骤边界与时间戳正确、`reasoningComplete` 落定 |
| 组件测试（新/改） | 步骤卡片渲染、单步展开、折叠已完成；**适配现有断言**：`transcript-virtualization.test.tsx:137` 断言流式 reasoning 渲染为 append-only 平铺文本、无 Markdown 子树——步骤视图需为这些用例提供等价断言（纯文本片段按步渲染） |
| 效果测试（边界） | 参照 `internal/boot/effect_test.go` 模式在最终边界断言：真实事件序列 → 步骤状态机 → DOM |

---

## 6. 架构约束符合性

| `REASONIX.md` 约定 | 符合性 |
| --- | --- |
| cache-first（system prompt 前缀字节稳定） | ✅ 零 prompt 改动 |
| controller 单一后端、前端只做渲染 | ✅ 无 Go 改动 |
| 性能特性在最终边界有效果测试 | ✅ 5.8 节 |
| 一个文件一个职责；≤800 行 | ✅ 分割器/hook/组件分文件 |
| 注释 ≤3 行、默认无注释 | ✅ 代码即真相 |

---

## 7. 限制与诚实边界（与 `result.md` mock 的差异）

1. **步骤标题与数量依赖模型的书写习惯**。DeepSeek 思考是自由文本；只有模型写出 `Step N:` / `步骤 N` / 编号标题等结构时才会出现步骤卡片，否则自动降级为现状。`result.md` 中"理解需求/分析代码结构/…"这类 5 步中文结构是理想化示例，不能保证每次复现。
2. **流式中的 "3/5" 无法预知总数**。思考结束前不存在总步骤数；诚实交互是流式中显示"已检测 N 个步骤"，结束后显示 `N/N`。
3. **每步耗时是近似值**：定义为"该步 marker 行完成时刻 − 上一步 marker 行完成时刻"，token 到达粒度决定精度（与现有总耗时同源）。
4. **可选的 prompt 杠杆**（不默认启用）：在 base prompt 尾部追加"思考时请用 `Step N:` 标注步骤"可显著提高命中率，但会击穿缓存前缀，属 cache-sensitive 变更，需单独评审后决定（`System-prompt-review` gate）。

---

## 8. 实施计划

| 阶段 | 内容 | 产出 | 验收 |
| --- | --- | --- | --- |
| P0 | 分割器纯函数 + 单测 | `reasoningSteps.ts` + test | 全部单测绿；无 marker 返回空 |
| P1 | Hook（增量 + 时间戳）+ 单测 | `useReasoningSteps.ts` + test | 时间戳/边界断言通过 |
| P2 | 步骤卡片组件 + 样式 + 集成 | `StructuredReasoningStep.tsx`、`styles.css`、`AssistantReasoningPanel.tsx` | 手动验证流式步骤出现、展开/折叠、降级路径 |
| P3 | i18n + 折叠已完成 + 效果测试 | 三语 locale、面板二级开关、组件测试 | 既有测试适配后全绿 |

**验收标准（整体）**

- [ ] DeepSeek 思考含步骤标记时，流式中逐步出现 ✓/● 状态卡片，结束时显示 `N/N` 与总耗时
- [ ] 思考无标记时界面与现状完全一致（降级零回归）
- [ ] 每步可展开/折叠；面板级折叠、显示模式、费用/token 展示行为不变
- [ ] 历史会话加载后步骤列表正确重建
- [ ] `npm test`（桌面前端）全绿；无 Go 侧改动，`go test ./...` 不受影响

---

## 9. 附录：与 `prd.md` 的映射

| `prd.md` 内容 | 本文处理 |
| --- | --- |
| 系统提示 XML 强制格式（方案 1） | 否决（缓存违约）；仅作为第 7 节"可选 prompt 杠杆"保留 |
| Go 侧 `reasoning_structure.go` 解析校验（方案 2） | 解析挪到前端（方案 A）；校验整体否决 |
| 格式不符重试循环（方案 3） | 否决（成本不可控、不可靠） |
| Agent Options / TOML 配置（方案 4、5） | 不采纳；本设计零配置即可运行 |
| "思考从涌现变结构" | 明确放弃框架侧强制；展示层尽力呈现，结构由模型自行涌现 |
