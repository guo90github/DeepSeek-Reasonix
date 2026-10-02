# TODO — 多智能体集群落地执行清单

> 用途：**落地阶段的唯一执行清单**。每完成一项就在本文件里把 `[ ]` 改成 `[x]`，并在同一行末尾补证据
> （文件路径 / 命令 / 提交号）。条目**只给指针，不复制出口条件**——出口条件一律以
> `docs/agents/AGENT_BUS.md` §11（S1 的实现规格见 §11.1，实现期修正见 §11.3）为准，避免两份真相漂移。
> 规则：本目录（`docs/agents/`）是这批文档的落点；新增文档一律落在这里；推进中发现缺失的节点**追加到本文件**。

## T0 文档收敛（落地前）

- [x] T0-1 契约落盘并迁入本目录 → `docs/agents/AGENT_BUS.md`
- [x] T0-2 两份旧文档加状态块指向契约 → `docs/agents/multi-agent-collaboration-design.md`、`...-development-plan.md`
- [x] T0-3 提交文档基线 → `c414f6e55`（中文信息 + `Documentation-impact: updated - …`；纯文档）
- [ ] T0-4 确认是否需要 `*.zh-CN.md` 英文对照（`docs/COLLAB-SURFACE.md` 是中文单份先例，倾向不需要）

## T1 对抗评审

- [x] T1-1 一轮对抗评审（独立只读子智能体，任务是「挑错」而非补全）→ 结论：**不能直接开工**，须先补 S1 规格 + 消解范围冲突
- [x] T1-2 结论逐条并入契约（改判留痕）→ `docs/agents/AGENT_BUS.md` §11.1（S1 规格）、§11.2（评审留痕）、§10（3 处处置补正）、§跨工作区与 §9.3（3 处锚点修正）、§集群规模（单机边界）
- [x] T1-3 第二轮对抗评审（**对着 S1 代码挑错**）→ 17 条，处置见 `docs/agents/AGENT_BUS.md` **§11.4**：已修 10 条（可推进集合、revert-from-stale、Sweep 显式、no_progress 仅系统、幂等键不含时间、split 重复子 id、require 闭依赖、读锁预算、跨窗口修复、意图冲突拒收）+ 3 条记为边界（挂 T4-8/T5-6/T7-5）

## T2 补规格（**必须在对应阶段开工前完成**）

- [x] T2-1 视图/摘要规格 → 已定，见 `docs/agents/AGENT_BUS.md` **§13.1**（谁生成 / 三种投影 / 行字段集 / ≤8 KiB 且 ≤200 行 / 游标增量不重发 / 超限取前 K 且计数可见 / 头部字节不变）
- [x] T2-2 「就绪」事件发起者 → 已定，见 **§13.2**（主路径 = 让依赖变 `done` 的写入者在同一事务内发；兜底 = 宿主每 tick 扫描幂等；每参与者每 tick 最多一条唤醒）
- [ ] T2-3 预算与既有旋钮对齐：`GoalTokenBudget`（`internal/config/config.go:1301`）/ spend budget / `REASONIX_SKIP_BUDGET` 谁统谁，超限后 Goal 变 `blocked` 还是 `complete`
- [ ] T2-4 **并发槽的持久队列落点**：槽满「排队而非失败」需要一个持久队列，而既有 inbox 是**会话级**且嵌套 fail-fast —— 定死后再做 S5
- [ ] T2-5 **能力的授权链**：缺能力派生「获取能力」子节点时，谁批装依赖/改仓库（`CapabilityGrant` 交集 vs 新授权）

## T3 S1 黑板内核 — **已完成**（`internal/agentbus/board`：6 源文件 + 6 测试文件；提交 `1b40a2f1d`）

规格：`docs/agents/AGENT_BUS.md` §11.1 + §11.3（实现期修正）+ §11.4（第二轮评审处置）。`go test ./internal/agentbus/board/` → `ok`。

- [ ] T3-17 去重：`internal/agentbus/jsonl`（新，日志底座：append/sync、torn tail 修复、损坏行计数）与 `internal/agentbus/board/log.go`
      的同类机械目前并存（board 尚未改为委托）。**为什么没顺手改**：board 的 S1 面已绿且有独立测试，改它属于与本次目标无关的重构；
      等下一刀碰 board 日志时一次性委托过去（`go test ./internal/agentbus/...` 是这次改动的守门测试）。
- [x] T3-1 非法迁移拒收（带原因）→ `transition_test.go` `TestIllegalTransitionsAreRejectedWithReason`（26 用例逐条断 reason）+ `TestRejectedOpsLeaveNoTrace`
- [x] T3-2 重放幂等 / 重复 id 不追加 → `TestDuplicateOpIDDoesNotAppendTwice`、`TestFoldCountsDuplicateIDs`
- [x] T3-3 并发折叠 = 串行 → `TestConcurrentApplyFoldsLikeSerial`（8 goroutine × 5 op，seq 1..40 无重号无缺号）
- [x] T3-4 杀掉写者后仍可折出完整状态 → `proc_test.go` `TestKilledWriterLeavesAFoldableLog`（子进程写完即 exit 9；父进程折出 5 条并继续可写）
- [x] T3-5 两个不同进程并发写不丢 op → `TestConcurrentProcessesDoNotLoseOps`（3 子进程 × 6 op）
- [x] T3-6 租约到期回收 + 记 `no_progress`（幂等） → `TestSweepReclaimsExpiredClaimAndRecordsNoProgress`、`TestSweepIsIdempotent`、`TestSweepReclaimsSeveralExpiredClaimsInOnePass`、`TestSweepLeavesLiveClaimsAlone`、`TestHeartbeatExtendsTheLeaseAndSweepSparesIt`
- [x] T3-7 `revert` 后传递下游 `done` ⇒ `stale`（且 stale 可回 open） → `TestRevertMarksDoneDependentStaleAndStaleReturnsToOpen`
- [x] T3-8 无证据 `abandon` / 无证据或自证的 `decide(done)` 被拒 → 同 T3-1 表 + `TestAbandonRequestThenDecideAbandoned`、`TestCapabilityGapClearsTheClaim`
- [x] T3-9 阶段闸门 → `gofmt -l internal/agentbus/` 空；`go vet ./internal/agentbus/...` 空；`go test ./internal/tool/builtin/` ok(21s)；`go test ./internal/boot/` ok(152s)；`make lint` 本包 **新增 0 条**（余 18 条既有红在 `internal/recap`/`internal/serve`/`internal/control`/`internal/boot`/`tools/collabgate`/`internal/agent`，工作区未改动这些文件）；`go run ./tools/repolint` 余 **2 条既有红**（`desktop/frontend/src/components/SettingsPanel.tsx`、`desktop/frontend/src/lib/useController.ts` 超预算，同样未改动）——**未放宽任何 baseline**
- [x] T3-10 结构体按生命周期分组 → 本包**不持有 mutex/atomic**（互斥由 `internal/filelock` 承担）⇒ `struct-state` 不适用，未新增受计结构
- [x] T3-11 `seq` 跨进程单调不重号 → `TestSeqIsAssignedFromTheLog`、`TestConcurrentProcessesDoNotLoseOps`
- [x] T3-12 截断尾行不计入 op；坏行跳过并计数 → `TestTruncatedTailIsNotAnOp`、`TestNextWriterRepairsTheTornTail`、`TestCorruptMidFileLineIsSkippedAndCounted`、`TestRepairRejectsAnUnreachableTornTail`、`proc_test.go` `TestTornTailFromAKilledWriterIsRepaired`
- [x] T3-13 成环的 `split`/`require` 被拒 → `TestSplitAndRequireRejectCycles`
- [x] T3-14 分层自证 → `layering_test.go`（不 import `control`/`agent`/`serve`/`boot`）
- [x] T3-15 实现期修正回写契约 → `docs/agents/AGENT_BUS.md` §11.3（5 条）

**备注**：`internal/boot` 测试串较长（>2 分钟），以 `go test -timeout 20m ./internal/boot/` 在后台跑；结果记在下一次推进里。

## T4 S2 写侧接线 + 读侧投影（含跨工作区与子树分片）

- [x] T4-1 多会话可写同一 `board` → board 级跨进程多写者已测；control 级写路径已落（`ApplyAgentBusOp`，拒绝是 typed）；
      control 级两会话同写的用例见 `internal/control/agentbus_multisession_test.go`。身份已从"会话路径的副作用"改成**显式 participant**
      （`SetAgentBus(dir, id)` + boot 的 `AgentBusID`）—— 无路径会话由此也能入列
- [x] T4-2 视图裁剪 → `internal/agentbus/view.go`（owned / waiting / needed 三条规则；≤200 行 且 ≤8 KiB；游标增量；超限计数可见）
- [x] T4-3 跨子树只经边界节点 → 机制已有（`require` 生成边界节点）；**还缺**跨子树可见性的用例
- [x] T4-4 守卫三 → `internal/boot/agentbus_effect_test.go`（真实 provider 边界：只含我的节点、别人的不出现、system 前缀无污染；未接线即零变化）+ `internal/boot/agentbus_wiring_test.go`（Options 宿主接线，pathless 会话）
- [ ] T4-5 读侧复用任务树/事件 → 已提供人读行投影 `AgentBusTasks`（扁平行 + `Deps`，供任务树嵌套，不新增存储）；**面板与事件族归 S6/T8**
- [ ] T4-6 `make frontend-check` 过（等前端面）
- [x] T4-7 视图的增量读（cursor/seq）与「头部稳定、尾部追加」实测命中前缀缓存
      → 证据：`internal/control/agentbus_multisession_test.go`（T4-1）/ `internal/agentbus/subtree_visibility_test.go`（T4-3）/ `internal/boot/agentbus_prefix_test.go`（T4-7：两轮之间 system 前缀与工具清单字节不变、第二轮只带增量）

## T5 S3 交流三档

- [x] T5-1 自由对话 `say` + 话题边界（轮数/预算/静默窗口触顶即收口）→ `internal/agentbus/talk.go`（`ApplyTalk` 只按**已记录状态**判接受；
      `TopicLapsed` 是派生谓词，收口由 `CloseLine` 产出记录、下一个写者追加——重放不依赖读取时刻）+ `talklog.go`（写事务内 `CloseLapsed`）
- [x] T5-2 默认不灌上下文（只投点名与摘要）→ `Digest(topic, participant, cursor)`：只给点名行 + 其余计数；
      turn-tail `<agentbus-talk>`（单轮封顶 50 行 + `truncated=true` + `hidden=`）见 `internal/control/agentbus_talk.go`
- [x] T5-3 有界点对点 `ask`/`answer` + 回执通道（`results/<correlation>.json`）→ 链上记账 hop/token、`ask` 开链设 TTL、过期即 `expired`；
      `results.go` 原子替换 + correlation 必可作文件名 + 状态封闭集；命令面 `AgentBusAsk/Answer` + `Post/ReadAgentBusResult`
- [ ] T5-4 跨进程目标带令牌；超速返回 `rate_limited` → `rate_limited` 已落（`agentbus.RefuseRate`，命令面原样透传 typed）；
      **跨进程带令牌未做**（属宿主/传输层：随 T5-5 的宿主路由一起接）
- [x] T5-5 就绪即事件唤醒（`interval` 只兜底）→ 内核侧已落：`internal/agentbus/wake.go`（`WakeTargets` 从 op 日志 + 话题面派生目标，
      key 由工作集合派生 ⇒ 每 tick 幂等）+ `internal/control/agentbus_wake.go`（`SetAgentBusWaker` 宿主路由、不唤醒自己、失败释放 key 重试）
      + 写路径发起（`ApplyAgentBusOp` 非 replay 时唤醒）；**宿主侧路由（谁安装 waker：桌面多 tab / serve / 远端）未接**，下一刀
- [ ] T5-6 **节点记录 `Requester`**（`require`/`assert`/`split` 写入），使"谁要这个节点"回到折出状态——目前只能从 op 轨迹派生，
      见 `docs/agents/AGENT_BUS.md` §13.3 的缺口说明

## T6 S4 审议与裁决

- [ ] T6-1 审议状态（参与者/轮次/必答/权重/冷却）
- [ ] T6-2 一次 `refute` 改变结论（可回放：同一 op log 折叠出不同结局）
- [ ] T6-3 等重升级给人；超升级配额自动降级 `undecided-by-rule` 并记账
- [ ] T6-4 弃答以 `no_answer` 可见
- [ ] T6-5 无可核对证据时 `done` 被拒（S1 已实现 `decide(done)` 的证据 + 非产出者复跑门槛，S4 复用）
- [ ] T6-6 票数不改变权重（consensus ≠ evidence）

## T7 S5 集群调度与预算

- [ ] T7-1 **本机 host 级**并发槽 + 排队（槽满排队而非失败；跨机不在 v1）
- [ ] T7-2 四级预算（board → 子树 → 节点 → 回合）+ 触顶即暂停、现场保留
- [ ] T7-3 调度四则：关键路径优先 / 批量领取 / 同子树亲和 / 最难优先
- [ ] T7-4 预算只被验收节点消耗（做工不消耗总预算）

## T8 S6 观测聚合（人读）

- [ ] T8-1 按子树折叠、只显异常/争议/停滞/孤儿，可下钻
- [ ] T8-2 首屏不画 >N 张卡片（阈值可配）；孤儿与停滞必现

## T9 S7 e2e + 无人值守贯通 + 百级压测

- [ ] T9-1 杀掉编排者，未完成节点仍被认领推进
- [ ] T9-2 一次 `refute` 改变结果
- [ ] T9-3 运行时依赖：节点/边数由参与者新增
- [ ] T9-4 无人值守贯通：杀掉桌面进程 → 既有看门狗拉起 → 黑板继续被推进（`docs/UNATTENDED.md` §11 自认此链未真机验收，**第一次必须端到端**）
- [ ] T9-5 任务级落地：验收节点全 `done` + 无未决矛盾才宣告完成
- [ ] T9-6 N≈100：任意杀掉 20% 后仍收敛；不超预算；无 429 风暴
- [ ] T9-7 存在一次真实的「缺能力 → 派生获取能力节点 → 完成」链路

## T10 S8 PR 元数据门 + 打包

- [ ] T10-1 `Cache-impact` / `Cache-guard` / `System-prompt-review` / `Documentation-impact` 齐全
- [ ] T10-2 按本机 SOP 打包并 `verify-windows-portable.sh` exit 0

## 已知边界（第二轮评审认定；挂在对应阶段，不在 S1 修）

- [ ] **T4-8**（S2）写路径每次全量读 + 全量 fold（O(n)/写，总 O(n²)）：改成增量 fold/游标 + 内存态缓存，并带实测预算（评审指出现状与 §5「百级下必须增量读」的精神背离）
- [ ] **T5-6**（S3/S4）`evidence` 目前只有「`Ref` 非空」一条约束：形状、唯一性、去重与去重窗口要在证据权重设计里定死（S1 的 `hasEvidence` 是占位）
- [ ] **T7-5**（S5）`Sweep` 单次上限 256（按 node id 排序，故无永久饥饿）：把上限与语义写进文档，并在调度侧决定 tick 频率

## 输入材料（本目录内，非清单项）

- `docs/agents/AGENT_BUS.md` — 契约（真相源，含 §11.1 S1 规格、§11.2 评审留痕、§11.3 实现期修正）
- `docs/agents/multi-agent-collaboration-design.md` / `...-development-plan.md` — 被取代但保留继承项的旧稿
- `docs/agents/agent-evaluation-whitepaper-01-overview.md` — Agent 评测体系（外部方法论蒸馏），供「可核验」纪律对齐语言

## 不做（明确排除，防止漂移）

- 跨机协作（v1 单机；协议保留可换目标）
- 自由对话承载真相（对话是一等能力，但结论只在黑板上）
- 新守护进程 / 第四个前端（唤醒与常驻复用既有机制）
