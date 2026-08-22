# Bug Report: 会话多轮后流式输出退化为一次性输出（仅显示"工作中 N 秒"）

> **提交对象**: DeepSeek-Reasonix 作者 / Reasonix upstream maintainers
> **报告日期**: 2026-08-17
> **分支**: `dev`　**HEAD**: `cf5ac0b35`（v1.26.0，docs(release): prepare v1.26.0 bilingual notes）
> **环境**: Windows 桌面端（`desktop/frontend` WebView2 前端），Provider: DeepSeek Anthropic-compatible endpoint，thinking 开启
> **状态**: 仅诊断，未改动任何代码

---

## 1. 摘要 (Summary)

**中文**：同一会话内，前几轮（通常 1–3 轮，但**轮次不固定**）思考过程与回答均能丝滑流式输出；之后某一轮起，UI 全程只显示「(1) 工作中 N 秒」读秒，思考过程与正式回答都不再实时出现，回合结束后一次性整体显示。该行为是 v1.26.0 引入的回归，根源在 agent 层的 `deferredStreamSink` 把 `Text`/`Message` 增量事件也纳入了"等待 reasoning 事件"的缓冲。

**English**：Within one conversation, the first few turns stream thinking and answer smoothly (turn count is NOT fixed); afterwards the UI only shows "working Ns" while the whole response (thinking + answer) appears at once when the turn completes. This is a regression introduced in v1.26.0: the agent's `deferredStreamSink` now buffers `Text`/`Message` delta events until a `Reasoning` event arrives; when the DeepSeek Anthropic endpoint omits thinking on a turn (a documented recurring behavior), the buffer is only released at turn commit, producing one-shot output.

---

## 2. 现象 (Symptom)

1. 新建会话，正常提问，前几轮思考过程逐字流式显示，回答同样流式，体验正常。
2. 继续对话（轮次不固定，可能与历史中首次出现工具调用轮相关），某一轮起：
   - 提交后 UI 仅显示运行状态条「(1) 工作中 10 秒」（读秒持续增加），期间**无任何流式内容**；
   - 思考过程不显示、回答不显示、工具调用卡片也不实时出现；
   - 回合结束瞬间，完整内容一次性全部渲染。
3. 一旦该会话进入此状态，**后续每一轮都保持一次性输出**（直到会话结束）。

**截图证据**：`.reasonix/attachments/clipboard-20260817-113758.613921-000001.png`
OCR 内容：用户消息「为什么宿主每次都乱卡进程？」+ 状态条「(1)工作中10秒」。用户侧观感为"好像卡死了"，实际是输出被缓冲。

---

## 3. 复现步骤 (Reproduction)

1. 使用 DeepSeek Anthropic-compatible endpoint，thinking 开启（默认）。
2. 新会话中先进行几轮纯问答（无工具调用）。
3. 提出需要工具调用（读文件 / 执行命令等）的问题，或继续任意对话直至 DeepSeek 某一轮未返回 thinking。
4. 观察：该轮起 UI 进入「工作中 N 秒」读秒、无流式内容，回合结束一次性输出。

> 备注：由于触发条件依赖 DeepSeek 服务端是否在当轮返回 thinking，复现率与轮次不严格对应；但一旦 fallback 电路（见 §6.5）开启，会话内 100% 复现。

---

## 4. 影响范围 (Impact)

- **前端**：桌面端（`desktop/frontend`）全部受影响——不是前端缺陷，前端正常消费事件流。
- **后端**：`internal/agent` 的流式事件下发路径。
- **Provider**：所有"严格推理回放协议"的 Provider，本报告实测场景为 **DeepSeek Anthropic**（`RequiresToolCallReasoning()==true` 且 `AllowsEmptyReasoningFallback()==false`）。OpenAI-compatible DeepSeek（空字符串回退）与 GLM（no-retry fallback）不受此影响——#8952 已分别为它们保留了实时流出路径。
- **严重度**：高（核心交互体验回归，思考过程展示是 DeepSeek 模型的核心卖点）。

---

## 5. 根因分析 (Root Cause)

### 5.1 流式管线本身正常

Provider 请求恒为流式：`internal/provider/anthropic/anthropic.go:288` `Stream()` + `:456` `Stream: true`；`internal/provider/openai/openai.go:789` `Stream: true`。SSE 逐块解析为 chunk 后，agent 逐块发出增量事件：

- `internal/agent/agent.go:1896` → `event.Reasoning`（思考增量，逐 chunk）
- `internal/agent/agent.go:1904` → `event.Text`（回答增量，逐 chunk）

### 5.2 缓冲机制：`deferredStreamSink`

`internal/agent/run_loop.go:49-119`：

```go
// deferredStreamSink keeps selected stream events local until the caller
// chooses which provider response to adopt. On an ordinary healthy DeepSeek
// turn, reasoning arrives before tool calls and unlocks live tool-card events.
// On the rare malformed turn with no reasoning, only the speculative partial
// tool cards remain buffered, so retrying does not flash duplicate cards in the
// UI. A recovery attempt buffers everything because it may be discarded.
type deferredStreamSink struct {
	inner               event.Sink
	deferAll            bool
	waitingForReasoning bool
	sawReasoning        bool
	events              []event.Event
}

func newReasoningAwareStreamSink(inner event.Sink) *deferredStreamSink {
	return &deferredStreamSink{inner: inner, waitingForReasoning: true}
}

func newDeferredStreamSink(inner event.Sink) *deferredStreamSink {
	return &deferredStreamSink{inner: inner, deferAll: true}
}

func (s *deferredStreamSink) Emit(e event.Event) {
	...
	if s.waitingForReasoning && !s.sawReasoning {
		switch e.Kind {
		case event.ToolDispatch, event.ToolResult, event.Text, event.Message:
			// Keep every user-visible speculative event private until reasoning
			// proves the turn replayable. Healthy DeepSeek responses emit
			// reasoning first, so their live-streaming fast path is unchanged.
			s.events = append(s.events, e)
			return
		}
	}
	s.inner.Emit(e)
}
```

**关键点（run_loop.go:85-93）**：在收到首个非空 `Reasoning` 事件之前，`ToolDispatch`、`ToolResult`、**`Text`、`Message`** 全部被压入本地缓冲，不转发给 UI。只有两种出口：

1. 收到非空 `Reasoning` 事件 → `flushBuffered()`（run_loop.go:98-106）即时释放；
2. 回合提交时 `streamSink.Flush()`（run_loop.go:461）一次性释放。

### 5.3 为什么 DeepSeek 全量走缓冲路径

`internal/agent/sampling_attempt.go:28-42`：

```go
func (a *Agent) samplingAttemptSinks() (*deferredStreamSink, event.Sink) {
	warnOnMissing := provider.WarnOnMissingToolCallReasoning(a.svc.prov)
	replaySensitive := provider.RequiresToolCallReasoning(a.svc.prov) ||
		provider.RequiresReasoningRoundTrip(a.svc.prov) ||
		warnOnMissing
	if replaySensitive && (!provider.AllowsEmptyReasoningFallback(a.svc.prov) ||
		warnOnMissing) {
		streamSink := newReasoningAwareStreamSink(a.svc.sink)
		return streamSink, streamSink
	}
	return nil, a.svc.sink
}
```

DeepSeek Anthropic 适配器：

- `internal/provider/anthropic/anthropic.go:214-216` `RequiresToolCallReasoning() == deepSeekThinkingEnabled()` → 开启 thinking 时为 true；
- `internal/provider/anthropic/missing_reasoning_fallback.go:23` `AllowsEmptyReasoningFallback() == false`（Anthropic thinking 块不存在空回退）；
- 同时实现 `MissingToolCallReasoningWarningPolicy`。

因此 **DeepSeek thinking 模式下每次采样尝试都使用 `newReasoningAwareStreamSink`**，即：回答增量（Text/Message）与工具事件全部依赖"该轮出现 thinking"才能实时放出。

### 5.4 触发条件：DeepSeek 端间歇性不发 thinking

DeepSeek Anthropic 兼容端存在已知的间歇性行为：部分轮次（尤其请求历史中已包含需回放 thinking 的工具轮时）不返回 thinking 内容。仓库自身提交历史多次记载：

- #8924（`515026e11`）："DeepSeek Anthropic conversations could become unreplayable after server-side web search, interrupted tool turns, or truncated/missing reasoning"；"DeepSeek requires provider-issued thinking content to be replayed, so a missing value is retried once before tools execute"。
- #8952（`6667ea600`）："DeepSeek Anthropic tool turns that omitted required reasoning stopped the run after one exact replay"；#8921 前身提交 `e4bbe4889`："DeepSeek V4 Flash can intermittently return tool calls without replayable reasoning_content"。

当该轮没有 thinking 时：`sawReasoning` 恒为 false → 缓冲永不释放 → **整个回合的 Text/Message 全部压到提交时一次性 flush** → UI 只有「工作中 N 秒」，结束一次性输出。

### 5.5 恶化机制：fallback 电路使会话内永久一次性

`internal/agent/run_loop.go:390-458` + `internal/agent/reasoning_replay.go`：

1. `reasoningReplayIssue`（reasoning_replay.go:64-78）判定 `ReasoningReplayMissing`（工具轮 + 无 reasoning）→ 一次精确重试；
2. 重试用 `newDeferredStreamSink`（deferAll，run_loop.go:416）→ 重试全程缓冲，一次性；
3. 重试仍缺失 → `runMissingReasoningFallback`（reasoning_replay.go:12-45）→ `a.sess.missingReasoning.fallbackActive = true`（missing_reasoning_watch.go:225/234）；
4. fallback 激活后，本会话所有后续请求经 `withMissingReasoningFallback` ctx（missing_reasoning_watch.go:11-16）以 `thinking.type=disabled` 发送（`internal/provider/anthropic/missing_reasoning_fallback.go:38-49` `applyDeepSeekThinking` → `recoveryWithoutThinking`）；
5. thinking 被禁用后 DeepSeek **永远不会再发 thinking** → 每轮的 reasoningAware sink 都等不到 Reasoning 事件 → **会话内每一轮都变成一次性输出**（reasoningReplayIssue 在 fallbackActive 时虽不再重试，但缓冲行为依旧）。

### 5.6 跨会话影响：持久化电路

`internal/agent/reasoning_warn_state.go:31` 状态文件 **`tool-call-reasoning-warning.json`**（路径 `<Reasonix home>/state`，`internal/config/paths.go:342-348`），自适应退避 10m/30m/2h/6h/24h（#8952 引入），**跨进程、跨会话**影响同配置（provider/model/protocol 指纹）的所有会话。电路开启期间，新会话的首轮也可能直接进入 fallback（一次性），用户感知为"新会话也这样"。

---

## 6. 为什么"轮次不固定"

触发点**不是轮数**，而是"该轮 DeepSeek 是否返回 thinking"：

- 会话早期：请求历史里没有需要回放的 thinking 块（无工具轮），DeepSeek 基本都会发 thinking → sink 立即 flush → 丝滑流式；
- 会话后期：历史中已出现"含 thinking 回放的工具轮"，或 DeepSeek 偶发不发 thinking → 该轮缓冲；一旦 fallback 电路开启，后续**所有**轮次一次性。

用户观察到的"前 3 轮正常、之后一次性"是典型形态（前 3 轮纯问答、第 4 轮起出现工具轮 / thinking 缺失），并非固定阈值。

---

## 7. 回归来源 (Regression Provenance)

本次拉取的 v1.26.0（`609aeb2f8..cf5ac0b35`）中两个提交引入/放大了该缓冲：

| 提交 | 说明 | 证据 |
|---|---|---|
| **#8924** `515026e11` Fix DeepSeek Anthropic reasoning replay | 将缓冲集合从旧版**仅 `ToolDispatch`** 扩大为 `ToolDispatch, ToolResult, Text, Message` | `git log -S "event.ToolResult, event.Text, event.Message"` 命中 515026e11、6667ea600 |
| **#8952** `6667ea600` Fix provider reasoning replay recovery | 将 sink 选择条件从旧版 `WarnOnMissingToolCallReasoning` 改为 `replaySensitive && (!AllowsEmptyReasoningFallback \|\| warnOnMissing)`，使 DeepSeek 每次尝试都走缓冲 sink | `git log -S "replaySensitive"` 命中 6667ea600 |

旧版（`609aeb2f8`）`deferredStreamSink.Emit` 原文（`git show 609aeb2f8:internal/agent/run_loop.go`）：

```go
if s.waitingForReasoning && !s.sawReasoning && e.Kind == event.ToolDispatch {
	s.events = append(s.events, e)
	return
}
```

即旧版**回答文字与思考过程始终实时流出**，仅工具卡片在推理确认前保留——与 sink 自身文档注释（run_loop.go:49-54："only the speculative partial tool cards remain buffered"）一致。当前实现已偏离自身文档意图。

**结论：这是 v1.26.0（#8924/#8952）引入的回归**，旧版本不存在该现象。

---

## 8. 建议修复方案 (Proposed Fix)

> 本次仅诊断，**未实施**。以下方案供作者评估。

### 方案 A（推荐，恢复丝滑流式）

`internal/agent/run_loop.go:85-93`：缓冲集合只保留工具事件，`Text`/`Message` 恢复实时流出：

```go
case event.ToolDispatch, event.ToolResult:
	s.events = append(s.events, e)
	return
```

- **依据**：与 sink 自身文档注释及旧版行为一致；回答/思考的实时性不应依赖"该轮是否出现 thinking"。
- **风险**：缺失推理触发重试/回退时，首轮已流出的文字会被替换。**前端已有完备回滚机制兜底**：`desktop/frontend/src/lib/useController.ts:1257-1311` 在 stream_attempt begin 时快照 `baselineLive`（含 text/reasoning），discard 时恢复快照、不拼接（"Restore live to the pre-attempt snapshot so partial text/reasoning is replaced, not concatenated"）。今天 thinking 增量本就在首轮实时流出并同样依赖该回滚，Text 与其并无本质区别。

### 方案 C（fallback 期间旁路缓冲）

`samplingAttemptSinks()`（sampling_attempt.go:28-42）在 `a.sess.missingReasoning.fallbackActive && SupportsMissingReasoningFallback(prov)` 时直接返回裸 `a.svc.sink`：

- 恢复 fallback 期间的回答流式（此时 thinking 已被禁用，本来无思考可显示）。
- 单独使用仅缓解"永久一次性"，单轮 thinking 缺失仍会一次性；建议与 A 组合。

### 方案 A+C（完整修复）

- 任何轮次回答/思考都实时流出；
- fallback 期间无缓冲；
- 仅工具卡片在推理确认前保留（防重试闪卡）。

### 需同步更新的测试

- `internal/agent/reasoning_replay_recovery_test.go:287` 目前断言 `ToolDispatch, ToolResult, Text, Message` 四种事件均被缓冲（断言的是当前回归行为），需按新语义改为仅工具事件缓冲；
- 建议新增回归测试：thinking 缺失 + 无工具调用的纯文本轮，Text/Message 必须实时到达 sink。

---

## 9. 用户侧临时缓解（无需改代码）

- 删除状态文件 **`<Reasonix home>/state/tool-call-reasoning-warning.json`**（Windows 默认 `<%APPDATA%>\reasonix\state\`），并**新开会话**（fallback 是会话内状态，老会话仍一次性）；
- 注意：这只是重置持久化电路，DeepSeek 再次间歇性缺失 thinking 时仍会复发；根治需 §8 的代码修复。

---

## 10. 附录：证据索引 (Evidence Index)

**后端（根因所在）**

| 位置 | 内容 |
|---|---|
| `internal/agent/run_loop.go:49-119` | `deferredStreamSink` 定义：缓冲逻辑（85-93 含 Text/Message） |
| `internal/agent/run_loop.go:416` | 重试用 `newDeferredStreamSink`（deferAll） |
| `internal/agent/run_loop.go:461` | 提交时 `streamSink.Flush()`（一次性释放点） |
| `internal/agent/sampling_attempt.go:28-42` | `samplingAttemptSinks()`：DeepSeek 全量走 reasoningAware sink |
| `internal/agent/agent.go:1896, 1904` | Reasoning/Text 逐 chunk 增量事件 |
| `internal/agent/reasoning_replay.go:12-45` | `runMissingReasoningFallback`（deferAll fallback） |
| `internal/agent/reasoning_replay.go:64-78` | `reasoningReplayIssue` 判定逻辑 |
| `internal/agent/missing_reasoning_watch.go:11-16, 225/234` | fallback 上下文注入与激活 |
| `internal/agent/reasoning_warn_state.go:31` | 状态文件名 `tool-call-reasoning-warning.json` |
| `internal/config/paths.go:342-348` | 状态目录 `<Reasonix home>/state` |
| `internal/provider/anthropic/anthropic.go:214-223` | DeepSeek 严格回放策略（Requires/AllowsEmpty=false） |
| `internal/provider/anthropic/missing_reasoning_fallback.go:38-49` | fallback 时 `thinking.type=disabled` |
| `internal/provider/anthropic/anthropic.go:288, 456` | Provider 侧恒为流式（排除请求层问题） |

**前端（无缺陷，但提供回滚兜底证据）**

| 位置 | 内容 |
|---|---|
| `desktop/frontend/src/lib/useController.ts:1257-1311` | stream_attempt journal：`baselineLive` 快照 + discard 恢复（流式内容可安全回滚） |

**提交历史（回归来源）**

| Commit | 说明 |
|---|---|
| `515026e11` #8924 | 缓冲集合扩大至 Text/Message（回归点） |
| `6667ea600` #8952 | sink 选择条件改为 replaySensitive（放大回归） |
| `e4bbe4889` | 缓冲机制初版（仅 ToolDispatch，旧版行为） |
| `609aeb2f8` | 旧 dev tip（无此现象，对照组） |

**验证命令**

```bash
git log -S "event.ToolResult, event.Text, event.Message" -- internal/agent/   # 定位回归提交
git show 609aeb2f8:internal/agent/run_loop.go                                 # 旧版缓冲行为对照
go test ./internal/agent/...                                                  # 修复后需跑（含 -race）
```
