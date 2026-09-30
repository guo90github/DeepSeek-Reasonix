# 会话回顾 落地计划

> 依据：`docs/sessions/CLOSE_PRD.md`（决策记录 O1–O9、§13 术语表）
> 分支：dev-2 · 本计划只写"怎么做、怎么验"，产品口径一律以 PRD 为准

## 0. 两条硬约束（来自源码门禁，先钉死）

1. **分层**：新包 `internal/recap` 属内核层，**不得 import `internal/control`**（`tools/repolint/layers.go` 的 `violates()`：controller 只对 `frontends` 列表与 host 开放）。⇒ **触发接线只能在 `internal/boot`、`internal/cli`、`desktop/` 边界做**，内核包只暴露"观察者 + 后台运行器"。
2. **通道隔离已有底座**：`internal/boundedllm` 的包注释即"独立无工具调用、与主会话隔离、用量记到自己的 source、绝不进主会话前缀缓存"。⇒ O8 的第 1 条不需要新造，直接复用；缺的只有**并发闸门**（该包实测只有超时/token/输出上限，无并发控制）。

## 1. 分步计划

### P0 · 契约冻结（已完成，本计划不重复）

PRD 的 §4.2/§4.6/§10/§13 已把触发点、幂等键、存放位置、模型链、通道隔离、术语定死。本步余项只有两个命名类动作，随 P1-1 一起落地：

- 新增用量来源常量 `event.UsageSourceSessionRecap = "session-recap"`（照 `UsageSourceTitle` 的位置与命名风格）。
- 新增配置项 `Agent.SessionRecapModel`（TOML `session_recap_model`）。

### P1-1 · 常量与配置（纯增量，零行为改动）

| 动作 | 文件 | 参照 |
| --- | --- | --- |
| 加 `UsageSourceSessionRecap` | `internal/event/event.go`（`UsageSource*` 常量块） | `UsageSourceTitle` |
| 加 `Agent.SessionRecapModel` + `ResolveSessionRecapModel` + `SetSessionRecapModel` | `internal/config/config.go`、`internal/config/independent_session_recap.go`（新文件） | `internal/config/independent_web_search.go` |

验收：`gofmt -l` 干净、`go build ./...`、`go test ./internal/config/ ./internal/event/`。

### P1-2 · `internal/recap` 包骨架（新包，内核层）

- `Record`：候选条目（事实 / 根因—修法 / 否证结论 / 交接与未解坑，每条含正文、出处、建议落点）+ 元数据（模型、提示词版本、内容指纹、生成时间）。2026-09-30 起四要素已被候选条目取代（`recap-v3`）。
- `Key`：会话路径 + 内容指纹（轮次数或 transcript 哈希）——幂等的唯一依据，**不用"关闭次数"**。
- `Store`：可弃投影，落 `cache/session-recap/v1.sqlite`，走 `internal/projectiondb.Open`（WAL / `synchronous=NORMAL` / 私有权限 / 短 busy timeout / 远端回退内存 / quarantine 后重建）。
- `Admissible(path)`：可见性护栏 —— `agent.IsVisibleSession` ∧ 非 `<id>-recovery-<16hex>` ∧ 未被移除（`IsDestroyingSession` 由调用侧传入，内核不依赖 control）。
- `doc.go`：包职责与"为什么不进会话"的理由（≤40 行）。

验收：`go test ./internal/recap/` —— 幂等（同指纹不重复生成、指纹变化才重生成）、护栏（cleanup-pending / recovery 副本 / 空路径一律拒收）。

### P1-3 · 生成器（通道隔离 + 模型链 + 脱敏）

- 调用：`boundedllm.Call`，`UsageSource = event.UsageSourceSessionRecap`，超时 / `MaxTokens` / `MaxOutputBytes` / `EffortOverride = PreferredReasoning(prov, "low")`。
- 模型解析链：`agent.LoadSessionModel(path)` → `config.ResolveSessionRecapModel()` → 都不行则记「跳过（待补）」。
- **并发闸门（必须自建）**：同一时刻只放一个回顾调用，且有会话在飞时排队（可让路）。
- 分段：超长会话复用 `internal/agent/session_extract.go` 的 fragment 思路（头尾优先档：首段定目标、末段定结论与待办）。
- 脱敏：落盘前过 `internal/secrets.Redact*`。
- 失败：一律记「待补」并重试，**不落盘半成品**（不沿用标题的"回退到预览"契约）。

验收：`go test ./internal/recap/` —— 计数 provider 断言①同一时刻并发 ≤ 1、②`usage` 事件的 source 是 `session-recap`（不进会话用量）、③模型不可用时不产出记录且标记待补；守卫测试断言本包不 import `internal/control`。

### P1-4 · 触发接线（boot / CLI / 桌面边界）

- 复用 `agent.SessionPersistObserver` 的扇出（`internal/history/indexed.go` 已支持挂附加观察者）或 `SessionEnd` 提示，投递到后台队列。
- **钩子内零 LLM 调用**；话题归档路径会先 `SetSessionPath("")` 再 `Close()`，故空路径直接跳过。
- 复用既有 reconcile/repair 做启动补偿与重试。

验收：`internal/boot/effect_test.go` 模式的效果测试 —— 关闭会话后回顾在后台出现；**关闭路径同步 LLM 调用次数 = 0**（计数 provider 断言）。

### P1-5 · 存量补录

`reasonix catalogs reindex session-recap`（照 `internal/cli/catalogs_history.go`），可中断可续跑、给出 成功/跳过/失败 三态；范围按 PRD §4.4（多根 + 权威转录 + DAG 只取当前 head）。

验收：`go test ./internal/cli/`；对 1000+ 个存量 jsonl 抽样跑一次，统计三态。

### P2 · 人的入口

- `reasonix history <关键词>`（非交互 + TUI 两态），复用既有 FTS 投影，**不建第二个索引**；回顾只作展示（O4）。
- 桌面「会话回顾」管理页：整窗、复用 `HistoryPanel` 的 page 形态与 `TrashPage` 的接线方式，但**独立入口、不吸收 TrashPage**；副标题写明"只含会话关闭时生成的会话回顾"。

验收：`make frontend-check` + `docs/MANAGEMENT_PAGES.md` 同步（`Documentation-impact`）。

### P3 · 质量与脱敏补全

提示词迭代（四要素）；补内网地址类脱敏模式；抽查 20 条 ≥ 80% 达标。

## 2. 每步都要过的门禁（来自 REASONIX.md 的 pre-push 清单）

```bash
gofmt -w .
go vet ./...
make lint                      # golangci-lint + repolint（新包分层在此处被检查）
go test ./internal/recap/ ./internal/boot/ ./internal/config/ ./internal/cli/
go test ./tools/collabgate/    # 若动了 docs/COLLAB-SURFACE.md 引用的行
make frontend-check            # 仅 P2 起
```

PR 元数据（提交时）：`Cache-impact: none - 不新增 Agent 工具、不改系统提示前缀`、`Cache-guard: go test ./internal/boot/ -run Effect`、`Documentation-impact: ...`；提交信息用中文。

## 3. 明确不做（防跑偏）

- 不新增 Agent 工具；不改工具描述/schema；不动系统提示前缀与记忆块（缓存面前缀字节稳定）。
- 不改权威会话存储布局、sidecar 命名与 resume 行为；回顾不得写入权威文件。
- 不建第二个全文索引；不让回顾正文进 FTS（O4 已定）。
- 不与回收站合并入口；不在关闭路径做同步 LLM 调用。
- 不引入新的抽象层（无插件点、无配置开关矩阵）；一次性需求写内联代码。
