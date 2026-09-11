# Multi-Agent Collaboration System — Development Plan

> 分支：`feat/decentralized-agent-collab`（基于 `dev-2`）
> 关联设计：`docs/agents/multi-agent-collaboration-design.md`（已审查修订，v1 冻结）
> 计划性质：全周期开发任务计划书，覆盖从骨架到打包验收的完整生命周期。

## 1. 目标与范围

### 1.1 目标

在 Reasonix 打包运行形态（Electron 桌面端 + Go CLI）中落地一个**去中心化多智能体协作层**：多个对等 agent 通过结构化消息自主交流、自发规划与认领任务、解决冲突并收敛。v1 交付标准以设计文档 §1–§6 为准。

### 1.2 范围内（v1）

- `internal/collab` 包：`PeerRegistry` / `MessageBus` / `Blackboard` / `Facilitator` / 持久化格式。
- `control.Controller` 的 transport-agnostic 命令与 `reasonix.collab.*` 事件。
- `WorkspacePanel.tsx` 的 collab 视图（Agent roster / Message timeline / Blackboard task pool）。
- 单进程内协作作用域；peer 以 collab host session 为父会话持久化。

### 1.3 范围外（v2 或后续）

- 跨进程 peer 通信与 durable queue（进程退出后恢复在途任务）。
- Python 编排框架 sidecar（AutoGen/LangGraph 实验路径）。
- 顶层会话之间的实时消息总线。

## 2. 里程碑

| 里程碑 | 内容 | 出口条件 |
|---|---|---|
| M1 | 契约冻结 | 设计文档 + 本计划评审通过，消息信封/言语行为 schema 定稿 |
| M2 | 内核 MVP | `internal/collab` 四组件可用，单测全绿，repolint 干净 |
| M3 | Controller 集成 | 命令/事件/唤醒/turn-tail/用量归属全部接通，boot effect test 通过 |
| M4 | 前端可见 | `WorkspacePanel.tsx` collab 视图可交互，桌面 smoke 通过 |
| M5 | 发布候选 | CLI headless e2e + 桌面打包验证通过，文档与 PR 元数据门齐全 |

## 3. 阶段与任务

### P0 — 契约与文档冻结（进行中）

| ID | 任务 | 产出 | 验收标准 | 依赖 |
|---|---|---|---|---|
| P0-1 | 设计文档落盘并审查修订 | `docs/agents/multi-agent-collaboration-design.md` | [c1] 分层方向、peer 锚点、唤醒模型、Facilitator 性质、终止优先级、cache 约束、成本归属、v1 边界 8 项缺陷已修订 | — |
| P0-2 | 本计划书落盘 | `docs/agents/multi-agent-collaboration-development-plan.md` | [c2] 全周期阶段、任务、验收标准、依赖、风险、回滚齐全 | P0-1 |

### P1 — `internal/collab` 骨架（M1→M2）

| ID | 任务 | 产出 | 验收标准 | 依赖 |
|---|---|---|---|---|
| P1-1 | 定义消息信封与 speech acts | `internal/collab/message.go` | [c3] 信封字段与 `type` 枚举覆盖设计 §3.2/§3.3；未知字段拒绝；payload 上限生效 | P0-1 |
| P1-2 | 定义 peer/task/commitment 状态结构 | `internal/collab/state.go` | [c4] 与 `SubagentStore` 的 `SubagentSpec`/`SubagentMeta` 可互转；JSON 往返无损 | P1-1 |
| P1-3 | 持久化格式与原子写 | `internal/collab/persist.go` | [c5] append-only 写入 + 原子替换；损坏行跳过不中断启动 | P1-2 |
| P1-4 | 包级 `doc.go` 与分层验证 | `internal/collab/doc.go` | [c6] `control → collab → agent` 依赖方向成立；`go run ./tools/repolint` 无新增违规 | P1-1 |

### P2 — MessageBus + 持久化（M2）

| ID | 任务 | 产出 | 验收标准 | 依赖 |
|---|---|---|---|---|
| P2-1 | 进程内 pub/sub 总线 | `internal/collab/bus.go` | [c7] unicast/topic/groupcast 正确投递；无订阅者时消息仍落盘 | P1-1 |
| P2-2 | Mailbox 与订阅语义 | `internal/collab/mailbox.go` | [c8] peer 只读取自身 mailbox/订阅 topic；消息不跨 scope 泄漏 | P2-1 |
| P2-3 | 重放与幂等 | `internal/collab/replay.go` | [c9] 按序号重放；重复 id 去重；TTL 过期消息不投递 | P2-2 |
| P2-4 | 消息压力与边界测试 | `internal/collab/bus_test.go` | [c10] 大 payload 拒绝；并发投递无丢消息；订阅者慢不阻塞发布 | P2-3 |

### P3 — Blackboard + Contract Net（M2）

| ID | 任务 | 产出 | 验收标准 | 依赖 |
|---|---|---|---|---|
| P3-1 | 任务池与任务卡片 | `internal/collab/blackboard.go` | [c11] `propose` 生成任务卡片；任务状态机 pending→bid→committed→done/failed 合法迁移 | P1-2 |
| P3-2 | 投标/承诺流程 | `internal/collab/contract.go` | [c12] bid 携带能力与 `write_paths`；`accept` 形成 commit；`reject` 必须带理由 | P3-1 |
| P3-3 | 写冲突预检 | `internal/collab/write_claims.go` | [c13] 复用 `write_claims` 语义：并发写者非重叠 `write_paths` 才可同时 commit；冲突 bid 在 award 前拒绝 | P3-2 |
| P3-4 | 依赖动态生成 | `internal/collab/deps.go` | [c14] 任务依赖由 `propose`/`request` 消息动态建立，而非父节点 DAG | P3-2 |

### P4 — Facilitator（M2）

| ID | 任务 | 产出 | 验收标准 | 依赖 |
|---|---|---|---|---|
| P4-1 | 确定性协议引擎骨架 | `internal/collab/facilitator.go` | [c15] Facilitator 无 blackboard 写权限、无任务分解能力（结构上不可达） | P3-1 |
| P4-2 | quorum 与表决 | `internal/collab/quorum.go` | [c16] 有界 propose/reject 表决；平局按最小权限默认值 | P4-1 |
| P4-3 | 终止判定 | `internal/collab/termination.go` | [c17] 优先级：预算 > 轮次/消息数上限 > quorum > 静默窗口；预算耗尽安全暂停 | P4-2 |
| P4-4 | 协作预算与用量 | `internal/collab/budget.go` | [c18] 协作预算独立于宿主会话；超限暂停协作而非杀死宿主 | P4-3 |

### P5 — Controller 集成（M3）

| ID | 任务 | 产出 | 验收标准 | 依赖 |
|---|---|---|---|---|
| P5-1 | Controller 命令面 | `internal/control/*_collab.go` | [c19] `ListPeers`/`ListMessages`/`PublishMessage`/`ListTasks`/`ClaimTask`/`BidTask`/`CollabStatus` 全 frontend 可用 | P4 |
| P5-2 | 事件流 | `internal/control/collab_events.go` | [c20] 消息/投标/承诺/冲突发出 `reasonix.collab.*` 事件，对齐 `reasonix.subagent.*` 约定 | P5-1 |
| P5-3 | peer 唤醒与后台执行接线 | `internal/control/collab_runtime.go` | [c21] mailbox 触发 peer 执行；复用 `run_in_background`；进程退出丢队列行为与设计一致 | P5-1 |
| P5-4 | turn-tail 注入 | `internal/control/input.go` 扩展 | [c22] 协作上下文只 ride turn tail（memory-update/background-jobs 模式），system prefix 字节不变 | P5-1 |
| P5-5 | 用量归属 | `internal/control/collab_usage.go` | [c23] Usage 事件带 `peer_id`，ledger 按 session/peer 分桶 | P5-2 |
| P5-6 | boot 边界效果测试 | `internal/boot/effect_test.go` 扩展 | [c24] 通过真实 `boot.Build` 断言命令/事件/注入到达 provider request 与 frontend sink | P5-3 |

### P6 — 前端 collab 视图（M4）

| ID | 任务 | 产出 | 验收标准 | 依赖 |
|---|---|---|---|---|
| P6-1 | 数据层 hook | `desktop/frontend/src/lib/useCollab*.ts` | [c25] 经 `desktopBridge` 调用 Controller 命令，不绕过 controller | P5-1 |
| P6-2 | Agent roster | `desktop/frontend/src/components/CollabPeers.tsx` | [c26] peer 卡片显示角色/能力/运行状态/写路径 | P6-1 |
| P6-3 | Message timeline | `desktop/frontend/src/components/CollabTimeline.tsx` | [c27] speech-act 类型徽标；按 scope 过滤；事件驱动增量更新 | P6-1 |
| P6-4 | Blackboard task pool | `desktop/frontend/src/components/CollabTasks.tsx` | [c28] 任务卡片 + 投标/承诺状态 + 冲突高亮 | P6-1 |
| P6-5 | WorkspacePanel 接入 | `desktop/frontend/src/components/WorkspacePanel.tsx` | [c29] 按 viewMode 切换 collab 视图；临时探针改动先行隔离或还原 | P6-2 P6-3 P6-4 |

### P7 — 端到端验收与打包（M5）

| ID | 任务 | 产出 | 验收标准 | 依赖 |
|---|---|---|---|---|
| P7-1 | CLI headless e2e | `internal/cli/*_collab_test.go` 或 e2e 场景 | [c30] 两个以上 peer 完成 propose→bid→commit→done→accept 全链路 | P5 |
| P7-2 | 桌面 smoke | `desktop/...` smoke 用例 | [c31] 打包前桌面进程内协作作用域可启动、可渲染、可结束 | P6 |
| P7-3 | 性能与成本闸门 | 测试报告 | [c32] 并发上限与预算上限生效；无消息风暴；单进程内存增量可接受 | P7-1 |
| P7-4 | 文档与 PR 元数据 | `docs/agents/` 补充 + PR body | [c33] `Cache-impact`/`System-prompt-review`/`Documentation-impact` 字段齐全并通过本地脚本 | P7-2 |
| P7-5 | 打包验证 | 按 `REASONIX.local.md` SOP 构建并 `verify-windows-portable.sh` exit 0 | [c34] 打包产物含 collab 能力且无回归 | P7-3 |

## 4. 验证策略

- **单元/边界**：每个 `internal/collab` 组件独立表驱动测试；状态机非法迁移、TTL/幂等、写冲突预检必须有判别性用例。
- **分层**：`go run ./tools/repolint` 每个阶段末运行，不新增违规；`control → collab → agent` 方向由 repolint layering 强制。
- **效果测试**：性能与集成类改动按 `internal/boot/effect_test.go` 模式在最终边界断言（provider request、frontend sink、事件流）。
- **导入环**：P5 新增 controller/agent 交互前运行 `go test ./internal/control/ ./internal/agent/`，遇 `[setup failed]` 立即处理环。
- **回归基线**：`gofmt -w .`、`go vet ./...`、`make lint`、`go test ./internal/tool/builtin/ ./internal/boot/` 每个阶段末执行。

## 5. 风险登记

| 风险 | 影响 | 缓解 | 触发信号 |
|---|---|---|---|
| peer 自由对话发散、成本爆炸 | 高 | 消息类型/TTL/payload 上限 + 预算优先级最高 + quorum 终止 | 消息数/预算超限 |
| 写冲突死锁 | 高 | 共享 `workspacelease.Owner` + `write_paths` 预检 + bid 阶段拒绝 | 并发写者重叠路径 |
| 进程退出丢队列 | 中 | v1 明确接受；peer transcript 持久化保证可续跑 | 桌面崩溃/关机 |
| prompt cache 失效 | 中 | 协作上下文只 ride turn tail，不动 system prefix | prefix 字节变化 |
| 分层违规/导入环 | 高 | repolint + `go test` 环检测每阶段执行 | CI 失败 |
| 前端视图复杂度回归 | 中 | 先隔离/还原 `WorkspacePanel.tsx` 探针，再增量接入 | 桌面 smoke 失败 |

## 6. 回滚计划

- **分阶段可回滚**：P1–P4 为纯新增 `internal/collab`，不触及既有路径，回滚 = 删除包与对应测试。
- **P5/P6 逐提交可逆**：Controller 命令、事件、注入点、前端视图各自独立提交；任一 gate 失败即 revert 该提交，不牵连整体。
- **打包前必须全绿**：M5 出口以 P7-5 打包验证通过为准；不通过不合并、不发布。

## 7. 交付节奏（建议）

1. P0 → P1 → P2 一个 PR（内核骨架 + 消息总线）。
2. P3 → P4 一个 PR（blackboard + facilitator）。
3. P5 一个 PR（Controller 集成 + effect test，重点 PR 元数据）。
4. P6 一个 PR（前端视图）。
5. P7 收尾 PR（e2e/smoke/文档/打包验证）。
