# Multi-Agent Collaboration System — Technical Design

> 分支：`feat/decentralized-agent-collab`（基于 `dev-2`）
> 设计原则：**去中心化、peer-to-peer、交流驱动规划**。本方案刻意不采用 `task`/`parallel_tasks`/`fleet` 的单父节点调度范式，而把它们定位为底层执行原语，在其上构建一层自主协作运行时。

## 1. System Overview

### 1.1 目标

在 Reasonix 打包后的运行形态（Electron 桌面端 + Go CLI）中，引入一个**协作作用域（Collaboration Scope）**：多个对等 agent 在该作用域内通过结构化消息自主交流，基于交流结果自发分解任务、认领工作、解决冲突并最终收敛。单一父节点不再制定全局计划；任何 agent 都可以提议、拒绝、认领与发布结论。

### 1.2 核心原则

1. **Peer，不是 Subordinate**：参与方是能力不同的对等体，角色只决定"工具集 + 系统提示词 + 写路径"，不决定"谁指挥谁"。
2. **交流即规划**：任务分解发生在消息流中（propose → bid → accept → commit），不来自预设 DAG。
3. **Referee ≠ Planner**：保留一个只做协议裁判的 Facilitator，负责收敛判定、成本上限与仲裁，不参与任务分解。
4. **全部可持久、可审计**：消息、承诺、任务状态进入现有 JSONL/event log 体系，可重放、可续跑。
5. **有界**：轮次、消息数、token/成本、静默窗口都设上限，防止"无限对话"与成本爆炸。

### 1.3 在现有架构中的位置

```
┌─ Electron shell ──────────────────────────────┐
│  WorkspacePanel.tsx (collab 视图)             │
│        │ desktopBridge (JSON-RPC)             │
└────────┼──────────────────────────────────────┘
         ▼
┌─ control.Controller（transport-agnostic）─────┐
│  CollaborationRuntime（新增，workspace 级）    │
│   ├─ PeerRegistry      (agent 身份/能力/租约) │
│   ├─ MessageBus        (speech-act 消息流)    │
│   ├─ Blackboard        (任务池/承诺账本)      │
│   └─ Facilitator       (quorum/终止/仲裁)     │
│        │ 复用                                   │
│   SubagentScheduler · SubagentStore ·          │
│   SessionLease · write_claims · event.Sink     │
└───────────────────────────────────────────────┘
```

关键点：新能力加在 `control.Controller` 与一个新的 `internal/collab` 包中，**不写进任何前端**，这样 CLI、serve、桌面端自动继承（符合项目分层约定）。`SubagentScheduler` 仍负责每个 peer 的实际模型执行与并发上限。

**依赖方向（分层约束，`tools/repolint/layers.go`）**：`control → collab → agent`。`internal/collab` 不得 import `internal/control`（只有 frontends 与 hosts 可 import control），否则形成环或触发 repolint layering。协作运行时只做编排，不持有 Controller。

**v1 范围**：协作作用域限定在**单进程内**（一个 desktop app 进程，或一个 serve/CLI 进程）。跨进程 peer 通信（多 desktop 实例、多 serve 节点）明确列为 v2，因为现有 session lease 与 SubagentStore 锁都是进程本地语义。

## 2. Agent Architecture

### 2.1 Peer 身份与能力

每个 peer 是一个**持久化的子 agent 会话**，复用现有 `SubagentStore`（`internal/agent/subagent_store.go`）的 transcript 持久化、`continue_from`/`fork_from` 续跑能力，但额外获得：

- `AgentID`：协作作用域内稳定、可寻址的标识。
- `Capability` 声明：模型、工具白名单、`write_paths`、只读/可写——直接复用 `CapabilityGrant` 与 `write_claims`。
- `Mailbox`：独立收件箱，订阅一个或多个 topic。
- `Commitment`：对已认领任务的承诺记录（带 deadline 与撤销语义）。

### 2.2 角色 = 能力组合，而非层级

| 角色 | 工具集/权限 | 职责（对等，非上下级） |
|---|---|---|
| `researcher` | 只读 + web/search/MCP 读取 | 收集事实，发布 `inform` |
| `implementer` | 写工具 + 声明 `write_paths` | 认领实现任务，发布 `commit`/`done` |
| `reviewer` | 只读 + 测试执行 | 对 `done` 产出发布 `accept`/`reject` |
| `critic` | 只读 + 独立评估 | 质疑方案，发布 `propose` 替代路径 |
| `facilitator` | **无规划权**，仅协议工具 | 收集 quorum、判定终止、执行仲裁与预算上限 |

任何 peer 都可以 `propose`（提议目标/方案），`facilitator` 只在协议层面统计与兜底，不拆分任务、不分配任务。

`facilitator` 是**确定性协议引擎**（Go 代码），不是 LLM peer：它只统计 quorum、检测静默、执行预算上限与写冲突仲裁，没有 blackboard 写权限、没有任务分解能力。这样"无规划权"是可验证的结构性质，而不是提示词约束。

### 2.3 生命周期

1. `spawn`：按 profile 创建 peer（`PrepareFresh`），写入 PeerRegistry，建立 mailbox。
2. `engage`：通过订阅 topic 或接收 addressed 消息进入协作。
3. `commit` / `work`：认领后进入自身 ReAct 循环（仍受 `SchedulerPolicy.MaxSteps` 约束）。
4. `publish`：产出写入 blackboard，发布 `done` + 证据引用。
5. `retire` / `resume`：完成或暂停后 transcript 落盘，可 `continue_from` 续跑。

**执行与唤醒模型**：peer 复用的 `SubagentStore` 要求每个 transcript 都挂在某个 parent session 下（`requireParentSession`）。v1 用一个**协作宿主会话（collab host session）**作为 peer 的 parent 锚点——它是作用域的持久化根，peer 都是它的子 agent。消息到达时，`CollaborationRuntime` 通过 mailbox 触发该 peer 的一轮执行（沿用现有 `run_in_background` 语义：跨 turn 保持工作；进程退出即丢队列，见 §6 缺陷 3）。peer 在自己的 ReAct 循环里用 `poll_collab_mailbox` / `publish_collab_message` 工具收发消息，而不是由父 agent 转发。

## 3. Communication Protocol

### 3.1 传输层

- **进程内**：`MessageBus` 为 workspace 级 pub/sub，支持 unicast（`to`）、topic 广播（`topic`）与 groupcast（`to: [ids]`）。
- **持久化**：消息以 append-only 形式写入 `collab/<scope>/messages.jsonl`（复用会话事件日志模式），每条消息带序号，可重放。
- **订阅语义**：peer 只看到自己 mailbox 或订阅 topic 的消息，避免全量对话进入各自上下文。

### 3.2 消息信封

```json
{
  "id": "m_01J…",
  "scope": "workspace:…",
  "from": "agent:impl-2",
  "to": "agent:rev-1",
  "type": "propose",
  "reply_to": "m_00…",
  "expires_at": "…",
  "payload": { "…": "bounded" }
}
```

### 3.3 言语行为（speech acts）

| type | 语义 |
|---|---|
| `announce` | 发布能力或状态（"我可写 `internal/` 且当前空闲"） |
| `propose` | 提议目标、方案或替代路径 |
| `request` | 请求信息或能力（可被拒） |
| `bid` | 对 blackboard 任务投标 |
| `accept` / `reject` | 对 propose/bid/done 的承诺或否决（须附理由） |
| `inform` | 发布发现/事实，无承诺 |
| `commit` | 承诺承担任务（含 deadline） |
| `done` | 交付 + 证据引用（如 `read_subagent_result` 的 ref） |
| `withdraw` | 撤销提议/承诺 |

约束：所有 payload 有大小上限；`reject` 必须带理由；消息带 TTL 与幂等 id，防重复与无限对话。

### 3.4 基于交流的任务规划（Contract Net + Blackboard）

1. 任一 peer `propose` 一个目标到 `topic:tasks`，blackboard 生成任务卡片（未认领）。
2. 有能力的 peer 读取任务池并 `bid`（含自身能力、预计路径、写路径声明）。
3. 提议方或 `facilitator` 按简单规则（能力匹配、写路径无冲突、先到先得）`accept` 一个 bid，形成 `commit`。
4. 执行方完成后 `done` + 证据；`reviewer` 发布 `accept`/`reject`。
5. 规划不是预设的：任务依赖在执行中通过 `propose`/`request` 动态生成，而不是父节点声明的 DAG。

### 3.5 冲突解决与收敛

- **写冲突**：直接复用 `write_claims` 与 `<id>.jsonl.lease.lock`；并发写者必须声明非重叠 `write_paths`，冲突在 `bid` 阶段即被拒绝。同一协作宿主会话内的 peer 共享同一个 `workspacelease.Owner`（`TaskTool.WithWorkspaceLease` 已有"共享 owner"要求，独立 owner 会死锁）。
- **意见冲突**：对同一任务出现竞争方案时，`facilitator` 发起一轮有界 `propose/reject` 表决；平局按"最小权限默认值"（保守方案胜出）执行。
- **终止判定**：四类信号共同判定收敛，**优先级为：预算上限 > 轮次/消息数上限 > quorum 计数 > 静默窗口**。预算耗尽立即停止并安全暂停；`facilitator` 只宣布收敛，不做规划。

## 4. Integration Strategy

### 4.1 后端（内核）

- 新增 `internal/collab` 包：`PeerRegistry`、`MessageBus`、`Blackboard`、`Facilitator`、`collab_state.go`（持久化格式）。
- `CollaborationRuntime` 挂接在 workspace 作用域上，peer 的实际执行仍走 `TaskTool`/`SubagentScheduler` 路径，因此**每个 peer 的 transcript、进度事件、成本统计自动复用现有体系**。
- `control.Controller` 暴露一组 transport-agnostic 命令：
  - `ListPeers` / `ListMessages` / `PublishMessage`
  - `ListTasks` / `ClaimTask` / `BidTask`
  - `CollabStatus`（quorum、静默窗口、预算）
- 事件流新增 `reasonix.collab.*` 事件族（对齐现有 `reasonix.subagent.*` 事件约定），消息、投标、承诺、冲突都作为事件发出。
- **用量归属**：每个 peer 的 Usage 事件带 `UsageSource` 与 `peer_id`，进入现有 ledger 按 session/peer 分桶；协作预算独立于宿主会话 budget，超限由 `facilitator` 暂停协作而非杀死宿主会话。

### 4.2 前端（`WorkspacePanel.tsx`）

- 在 `WorkspacePanel.tsx` 增加 **collab 视图**（与现有 workspace tree / 完成摘要并列，按 viewMode 切换）：
  - **Agent roster**：peer 卡片（角色、能力、运行状态、占用写路径）。
  - **Message timeline**：按 scope 渲染消息流，speech-act 类型带不同徽标（propose/bid/commit/done）。
  - **Blackboard task pool**：任务卡片 + 投标/承诺状态 + 冲突高亮。
- 数据流沿用现有模式：`desktopBridge` 调用 `control.Controller` 命令，事件经现有 event stream 推送到 React 状态；**不绕过 controller 直连**。
- 现有未提交的临时探针改动（`WorkspacePanel.tsx:199-208` 附近）与本设计无关，落地时先行隔离或还原，避免混入。

## 5. Recommended Tech Stack

### 5.1 主推荐：在 Reasonix Go 内核内原生实现

**推荐**：新增 `internal/collab`，用 Go 实现协作运行时，**不引入 Python 框架作为打包依赖**。

理由：

1. **打包形态**：桌面端是 Electron + Go 子进程（`reasonix-desktop`）。引入 LangChain/AutoGen/CrewAI 意味着要在打包产物里塞 Python runtime + 依赖，安装包体积、进程隔离、崩溃恢复复杂度都显著上升。
2. **复用现有原语**：`SubagentScheduler`、`SubagentStore`、session lease、`write_claims`、event stream、cost/usage 记账全部已在 Go 内核中，协作层只需编排它们；用外部框架等于重造一半内核。
3. **缓存与性能**：协作消息与状态必须像 memory-update / background-jobs 一样**只 ride turn tail**（`internal/control/input.go:183-204` 的既有模式），绝不进入 cache-stable system prefix；内核内实现可直接复用 `Controller.Compose` 的注入点，保持 prompt cache 字节稳定。
4. **前端一致性**：`control.Controller` 是 transport-agnostic 的，内核实现让 CLI/serve/desktop 三端同时获得能力。

### 5.2 借协议思想，不借运行时

| 框架/模式 | 借鉴点 | 不采纳的原因 |
|---|---|---|
| **AutoGen / AG2** | conversable agent、speech acts、group chat 收敛语义 | Python 运行时；自由对话易发散、成本难控 |
| **LangGraph** | 显式状态机 + checkpointing（作为"交流状态机"的建模参考） | 图编排仍偏向集中式；引入 Python 生态 |
| **CrewAI** | 角色/任务抽象 | 与 AutoGen 同类问题，且规划仍是中心化 |
| **Contract Net Protocol** | announce/bid/award 任务分配 | 直接采用其协议，不需要框架 |
| **Blackboard** | 共享任务池 + 订阅 | 直接采用其模式 |

### 5.3 可选的实验路径（不进入 v1）

若后续希望复用现成 Python 多 agent 生态做原型验证，可将 AutoGen/LangGraph 作为**外部 sidecar 进程**，通过 JSON-RPC 或 MCP 与 `internal/collab` 对接，并用 feature flag 隔离；正式打包版仍走 Go 内核路径。该路径仅用于实验，不作为桌面/CLI 产品的默认实现。

### 5.4 持久化选型

- 消息流：append-only JSONL（复用现有 session/event log 模式）。
- 任务/承诺/peer 状态：轻量 JSON checkpoint + 原子写（`internal/checkpoint` 模式）；如索引需求增大，再引入桌面端已有的 SQLite 做 collab 索引，不新引入数据库。

## 6. Design Review & Amendments

初版审查发现并修正以下缺陷：

1. **分层依赖方向未声明** → 明确 `control → collab → agent`；`internal/collab` 不得 import `internal/control`（`tools/repolint/layers.go` 只允许 frontends/hosts import control）。
2. **peer 的持久化父会话锚点缺失** → 引入 collab host session；满足 `SubagentStore.requireParentSession` 约束，peer transcript 全部挂在该宿主会话下。
3. **消息唤醒模型缺失** → mailbox 触发 + 复用 `run_in_background` 语义；同时明确 v1 限制：进程退出即丢在途队列（与现有 fleet/background jobs 一致，durable queue 列入 v2）。
4. **Facilitator 性质模糊** → 改为确定性协议引擎（Go 代码），无 blackboard 写权限，"无规划权"成为结构性质而非提示词约束。
5. **收敛判定优先级未定义** → 明确优先级：预算上限 > 轮次/消息数上限 > quorum 计数 > 静默窗口。
6. **prompt cache 影响表述不精确** → 明确 collab 上下文只 ride turn tail（`internal/control/input.go` 的 memory-update/background-jobs 模式），不进 system prefix。
7. **每 peer 成本归属缺失** → Usage 事件带 `peer_id`，独立预算与 ledger 分桶。
8. **v1 跨进程边界未声明** → 单进程 v1；跨进程 peer 通信列为 v2。
