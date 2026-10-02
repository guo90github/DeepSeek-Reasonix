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
- [x] T2-4 **并发槽的持久队列落点** → 已定：队列落 `<board>/queue.jsonl`（与 board/messages/hearings 同层，复用 `internal/agentbus/jsonl` + `filelock`），
      槽位为**本机 host 级**、队列为**板级**；领取复用 S1 租约语义，不新增并发原语。见 `docs/agents/AGENT_BUS.md` §13.4
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
      + 写路径发起（`ApplyAgentBusOp` 非 replay 时唤醒）。**宿主侧路由未接**，但缝已定死：桌面 `tabs map[string]*WorkspaceTab` + `tab.Ctrl`，
      路由 = 找到 `Ctrl.AgentBusParticipant() == target.Participant` 的那个 tab，再 `TryEnqueueFollowup(control.InboxRequest{Submit:…, Source: "agentbus",
      Idempotency: target.Key})`（`TryEnqueueFollowup` 在会话空闲时会触发派发 ⇒ 唤醒真的会起回合）；`AgentBusParticipant()` 本轮已可从内核读出
- [ ] T5-6 **节点记录 `Requester`**（`require`/`assert`/`split` 写入），使"谁要这个节点"回到折出状态——目前只能从 op 轨迹派生，
      见 `docs/agents/AGENT_BUS.md` §13.3 的缺口说明

## T6 S4 审议与裁决

- [x] T6-1 审议状态（参与者/轮次/必答/权重/冷却）→ `internal/agentbus/hearing.go`：`Required` = 节点 owner ∪ 全部 refuter；
      `Round()` 与 `AnsweredThisRound()` **由记录派生**（每名必答者各答一次 = 一轮）；`Cooldown` 用记录自己的时间戳比较（可重放）；
      限制全在 `HearingLimits`（零值 = 不设上限，内核不替使用者发明天花板）
- [x] T6-2 一次 `refute` 改变结论（可回放：同一 op log 折叠出不同结局）→ `internal/control/agentbus_hearing_test.go`
      `TestHearingRefutesAnOutweighedAssertionAndBlocksTheNode`：弱断言 + 更强反证 ⇒ 判 `refuted` ⇒ 节点被 `decide(blocked)`（`Ready` 随之假）；
      换个新会话从同一批文件读到同一判决与同一 `Reason`，且同批记录再折一次结果相同。
      **精确边界**：本用例走的是"审议判决改变结局"，字面 `refute` op 的语义由 S1 覆盖（T3-7）；两者组合的单用例仍缺
- [x] T6-3 等重升级给人；超升级配额自动降级 `undecided-by-rule` 并记账 → `WeighResponse`（等重 → `escalate`；配额用尽 → `undecided-by-rule`）+ `HearingState.Escalations` 台账；
      用例 `TestWeighResponseClosesTheLogOnceTheQuotaIsSpent`（第一次升级、第二次按规则收口、台账 =1）
- [x] T6-4 弃答以 `no_answer` 可见 → `HearingSilent(h, now, lim)`：静默者可见（不删、不静默忽略），用例 `TestHearingSilenceIsVisibleNotDeleted`
- [x] T6-5 无可核对证据时 `done` 被拒 → S1 已落（`decide(done)` 的证据 + 非产出者复跑门槛，见 T3-1/T3-8）；S4 本刀未改这条门槛
- [x] T6-6 票数不改变权重（consensus ≠ evidence）→ `VerifiableWeight` 只数**可核对**证据（`Ref` 非空）；
      用例「五条无证据意见 < 一条 test」判 `refuted`
- [x] T6-7 审议接线 → control 命令面 `OpenAgentBusHearing`/`AnswerAgentBusHearing`/`SettleAgentBusHearing`/`AgentBusHearings`
      + `SetAgentBusHearingLimits`；**未答者纳入唤醒目标**（`WakeTarget.Owes`，用例 `TestASilentParticipantIsWokenToAnswer`）；
      判决经 `decide` 落节点：`refuted ⇒ decide(blocked)`，`stands` 交由产出者自己 `decide(done)`——**审议不得伪造那道门槛要的证据**（T6-5）

## T7 S5 集群调度与预算

- [x] T7-1 **本机 host 级**并发槽 + 排队（槽满排队而非失败；跨机不在 v1）→ `internal/agentbus/queue.go`（记录/折出/FIFO `Next`/深度背压）
      + `queuelog.go`（`queue.jsonl`，复用 jsonl + filelock；`Enqueue` 幂等、`Claim`、`Drop`）+ `schedule.go` 的 `Take`
      + `Ledger.AcquireSlot`（本机 host 级、每参与者一格、`slots_exhausted`）；用例 `TestTakeStopsAtTheHostCeilingAndLeavesTheRestParked`
      （槽满时第二人**一件都拿不到**且工作**仍在队列里**）、`TestParkingNeverTouchesTheBoard`（入队/领取**不动黑板现场**）、
      `TestQueueServesArrivalOrderNotPriority`（按到达顺序，不按优先级）、`TestQueueAppliesBackpressureAtItsDepth`（深度触顶 = `queue_full`）
- [x] T7-2 四级预算（board → 子树 → 节点 → 回合）+ 触顶即暂停、现场保留 → `internal/agentbus/budget.go`：`Charge` 走 node+turn、
      `Settle` 走 board+subtree；拒绝**命名到具体层级**（`budget_board`/`budget_subtree`/`budget_node`/`budget_turn`）且不改账；
      `Remaining` 取最紧一级；拒绝语义 = "现在不行"（停手保留现场，不做放弃路径）
- [x] T7-3 调度四则：关键路径优先 / 批量领取 / 同子树亲和 / 最难优先 → `internal/agentbus/rank.go`：比较链「同子树亲和 → 关键路径（传递依赖数）
      → 最难（声明步数）→ 到达顺序」，末级用稳定排序回落到达序 ⇒ **建议只重排，集合不增不减**；`TakeRanked` 用建议选批次而
      `Next`/队列文件的可见顺序仍是到达序。用例：`TestRankPrefersTheMostUnblockingWork`、`TestRankPrefersAffinityOverCriticality`、
      `TestRankPrefersTheHardestWhenNothingElseSeparates`、`TestRankNeverLosesOrInventsWork`、
      `TestRankFallsBackToArrivalWithoutABoard`、`TestTakeRankedChoosesByAdviceWithoutMovingTheQueue`（最后到达的 root 被先取，队列剩下的仍是到达序）
- [x] T7-4 预算只被验收节点消耗（做工不消耗总预算）→ `TestWorkDoesNotSpendTheBoardAllowance`（20 次做工后板级 spend = 0）；
      `Settle` 只认 `Outcome == done`（否则 `not_accepted`），且同一节点重复结算**不重复计费**（`Charge.Duplicate`）；
      `SubtreeRoot` 由依赖向上走派生子树归属（`TestSubtreeRootWalksDependenciesUp`）

## T8 S6 观测聚合（人读）

- [x] T8-1 按子树折叠、只显异常/争议/停滞/孤儿，可下钻 → `internal/agentbus/observe.go`：`Observe` 折叠 board+queue+hearing →
      `Briefing`（**只给出事的子树画卡片**，健康的只计数）；`Signal` 带下钻地址（subtree/node/kind/detail）；规则见 §13.5
- [x] T8-2 首屏不画 >N 张卡片（阈值可配）；孤儿与停滞必现 → `ObserveLimits{MaxCards, MaxSignals}`（默认 12/40）；
      `SignalKind.Mandatory()`（orphan/stalled）**不受上限影响**，被裁的计入 `Hidden`，卡片超限计入 `HiddenCards`；
      用例 `TestObserveAlwaysShowsOrphansAndStalls`（上限 1 时孤儿与两处停滞全在，多余争议被裁并计数）
- [ ] T8-3 **面板接线**（三段，前两段已落）：① control 读面 `AgentBusBriefing`/`SetAgentBusObserveLimits`/`AgentBusParticipant`；
      ② 桌面**绑定** `App.AgentBusBriefing()` → 扁平 JSON 视图（`desktop/agentbus_briefing.go` + 用例，含"没有活动会话/未入列"两种诚实的报错）；
      ③ 前端**组件**已落：`AgentBusPanel.tsx`（只画卡片与下钻行；孤儿/停滞的严重度排序与内核一致；
      下钻是 `onOpenNode(node)` 回调，交由宿主决定怎么打开）+ 11 条文案 × 3 份 locale + 单测
      `src/__tests__/agentbus-panel.test.tsx`（12 项断言，含用 jsdom+act 真的点一次下钻）；
      ④ **已挂进界面**（用户选定落点：工作区面板新增一节）：契约由**反射 `App` 类型**生成（`cd desktop && go run . -emit-contract frontend/src/generated`）
      ⇒ `DESKTOP_COMMANDS`/DTO/`GeneratedDesktopCommands` 自动带上 `AgentBusBriefing`；`lib/bridge.ts` 补声明 + mock（**守卫就是 `tsc`**）；
      `WorkspaceAgentBusSection.tsx`（取数与"读不到"分开）挂在 `WorkspacePanel.tsx` 的 body 开头（`refreshKey = tabId|sessionPath`）。
      **仍未做**：`onOpenNode` 下钻暂留空（先只读显示）；视觉样式只用了最小内联，未新增 CSS

## T9 S7 e2e + 无人值守贯通 + 百级压测

- [x] T9-1 杀掉编排者，未完成节点仍被认领推进 → `internal/agentbus/takeover_test.go`（真文件四幕：编排者开板并认领一步后**被丢掉**，
      幸存者只拿文件 `Sweep`→认领→完成，再从队列 `Take` 做完停放的那步；第三个新读者折出「无必现信号」且重放同结局）。
      **并抓到真缺陷**：已 `done` 的节点因历史 `NoProgress` 被报「停滞」⇒ 修 `stallReason`（完工节点不是活症状）
- [x] T9-2 一次 `refute` 改变结果 → `e2e_test.go` 的 `TestARefutationChangesTheEnding`，**按 §11.1 迁移表写而不是靠试**：
      `refute` 落在 `done` 上 ⇒ `contested`（结论受质疑，**且不牵连依赖它的已完成工作**）；`revert`（只接受 `done`/`stale`）
      ⇒ 自身回 `open`、**下游 `done` 变 `stale`**；三条板各自折出各自结局、各自重放一致。
      先前挂起的原因已定案：**是我的顺序错**（先 refute 再 revert ⇒ 节点已是 `contested` ⇒ 必判 `illegal_transition`），
      且"下游变 stale"**只由 `revert` 传播**（`abandon` 有意不传播：依赖被放弃 ≠ 下游已完成被推翻）
- [x] T9-3 运行时依赖：节点/边数由参与者新增 → `e2e_test.go` 的 `TestParticipantsGrowTheTreeWhileItRuns`：
      planner 开板后，browser 在运行中 `require` 出一个新步骤、splitter 再 `split` 成两个子节点；新读者看到同一棵树、重放不丢节点
- [ ] T9-4 无人值守贯通：杀掉桌面进程 → 既有看门狗拉起 → 黑板继续被推进（`docs/UNATTENDED.md` §11 自认此链未真机验收，**第一次必须端到端**）
- [ ] T9-5 任务级落地：验收节点全 `done` + 无未决矛盾才宣告完成
- [ ] T9-6 N≈100：任意杀掉 20% 后仍收敛；不超预算；无 429 风暴
- [ ] T9-7 存在一次真实的「缺能力 → 派生获取能力节点 → 完成」链路

## T10 S8 PR 元数据门 + 打包

- [ ] T10-1 `Cache-impact` / `Cache-guard` / `System-prompt-review` / `Documentation-impact` 齐全
- [ ] T10-2 按本机 SOP 打包并 `verify-windows-portable.sh` exit 0

## 已知边界（第二轮评审认定；挂在对应阶段，不在 S1 修）

- [x] **T4-8**（S2）写路径每次全量读 + 全量 fold（O(n)/写，总 O(n²)）→ 已改为**按句柄的内存态缓存**（`board.stateForWrite`：文件大小未变则复用上次折出的状态，
      变了则退回一次全量读；`Apply`/`Sweep` 在成功后 `keepCache`，缓存里显式推进 `Seq` 与 `OpIDs`——这两处不显式推进会被并发/重复用例立刻抓到）。
      **实测预算（同一基准 `BenchmarkApplyAtLogSize`，`-benchtime 20x`，本机）**：
      改前 50/400/1600 条 op = **1.06 / 2.37 / 6.95 ms**（32×日志 ⇒ 6.6×耗时，O(n) 确认）；
      改后 = **0.92 / 0.94 / 1.11 ms**（32×日志 ⇒ 1.2×，剩余量级即 fsync）⇒ 1600 条时 **快 6.2×**，随 n 增长已消失。
      **仍然存在（未做，不冒充）**：① 跨进程/多句柄写会各自退回全量读（缓存按句柄）；② `Snapshot` 读路径仍是全量 fold（O(n)/读）——读在每回合级别，先保留并在此记明
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
