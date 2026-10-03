# TODO — 多智能体集群落地执行清单

> 用途：**落地阶段的唯一执行清单**。每完成一项就在本文件里把 `[ ]` 改成 `[x]`，并在同一行末尾补证据
> （文件路径 / 命令 / 提交号）。条目**只给指针，不复制出口条件**——出口条件一律以
> `docs/agents/AGENT_BUS.md` §11（S1 的实现规格见 §11.1，实现期修正见 §11.3）为准，避免两份真相漂移。
> 规则：本目录（`docs/agents/`）是这批文档的落点；新增文档一律落在这里；推进中发现缺失的节点**追加到本文件**。

## T0 文档收敛（落地前）

- [x] T0-1 契约落盘并迁入本目录 → `docs/agents/AGENT_BUS.md`
- [x] T0-2 两份旧文档加状态块指向契约 → `docs/agents/multi-agent-collaboration-design.md`、`...-development-plan.md`
- [x] T0-3 提交文档基线 → `c414f6e55`（中文信息 + `Documentation-impact: updated - …`；纯文档）
- [x] T0-4 确认是否需要 `*.zh-CN.md` 英文对照（`docs/COLLAB-SURFACE.md` 是中文单份先例，倾向不需要）
      → **结论：不需要（2026-10-02），但原前提必须更正** ✗：`docs/` 下**确实**有成套双语惯例——`docs/*.zh-CN.md` 共 **66** 对，
      抽查的基座英文文件（`ACP.md` / `BILLING.md` / `APP_SHELL.md` …）**都存在** ✓；真正的中文单份先例只有
      `COLLAB-SURFACE.md`（无 zh-CN 孪生 ✓）与 `docs/agents/` 整个目录。
      ⇒ **不做的正确理由**是：`docs/agents/` 从一开始就**以中文撰写**（与 `COLLAB-SURFACE.md` 同类），读者也是同一批
      （本机使用者 + 后续会话），不是为对外发布而写 ✓；**若将来要对外**，那也应是"先有英文基座、再配 zh-CN"——
      方向与 T0-4 原设想**相反** ✗，届时应作为新任务重新评估，而不是把这份中文稿翻译成英文充数。

## T1 对抗评审

- [x] T1-1 一轮对抗评审（独立只读子智能体，任务是「挑错」而非补全）→ 结论：**不能直接开工**，须先补 S1 规格 + 消解范围冲突
- [x] T1-2 结论逐条并入契约（改判留痕）→ `docs/agents/AGENT_BUS.md` §11.1（S1 规格）、§11.2（评审留痕）、§10（3 处处置补正）、§跨工作区与 §9.3（3 处锚点修正）、§集群规模（单机边界）
- [x] T1-3 第二轮对抗评审（**对着 S1 代码挑错**）→ 17 条，处置见 `docs/agents/AGENT_BUS.md` **§11.4**：已修 10 条（可推进集合、revert-from-stale、Sweep 显式、no_progress 仅系统、幂等键不含时间、split 重复子 id、require 闭依赖、读锁预算、跨窗口修复、意图冲突拒收）+ 3 条记为边界（挂 T4-8/T5-6/T7-5）

## T2 补规格（**必须在对应阶段开工前完成**）

- [x] T2-1 视图/摘要规格 → 已定，见 `docs/agents/AGENT_BUS.md` **§13.1**（谁生成 / 三种投影 / 行字段集 / ≤8 KiB 且 ≤200 行 / 游标增量不重发 / 超限取前 K 且计数可见 / 头部字节不变）
- [x] T2-2 「就绪」事件发起者 → 已定，见 **§13.2**（主路径 = 让依赖变 `done` 的写入者在同一事务内发；兜底 = 宿主每 tick 扫描幂等；每参与者每 tick 最多一条唤醒）
- [x] T2-3 预算与既有旋钮对齐（**已定，见 §13.7**；并纠正前提：`REASONIX_SKIP_BUDGET` 是**打包期**尺寸闸门开关、非运行时预算）：`GoalTokenBudget`（`internal/config/config.go:1301`）/ spend budget / `REASONIX_SKIP_BUDGET` 谁统谁，超限后 Goal 变 `blocked` 还是 `complete`
- [x] T2-4 **并发槽的持久队列落点** → 已定：队列落 `<board>/queue.jsonl`（与 board/messages/hearings 同层，复用 `internal/agentbus/jsonl` + `filelock`），
      槽位为**本机 host 级**、队列为**板级**；领取复用 S1 租约语义，不新增并发原语。见 `docs/agents/AGENT_BUS.md` §13.4
- [x] T2-5 **能力的授权链**（**已定，见 §13.6**：一律发新授权，把关移到事后证据 + 审议）：缺能力派生「获取能力」子节点时，谁批装依赖/改仓库（`CapabilityGrant` 交集 vs 新授权）

## T3 S1 黑板内核 — **已完成**（`internal/agentbus/board`：6 源文件 + 6 测试文件；提交 `1b40a2f1d`）

规格：`docs/agents/AGENT_BUS.md` §11.1 + §11.3（实现期修正）+ §11.4（第二轮评审处置）。`go test ./internal/agentbus/board/` → `ok`。

- [x] T3-17 去重：`internal/agentbus/jsonl`（新，日志底座：append/sync、torn tail 修复、损坏行计数）与 `internal/agentbus/board/log.go`
      的同类机械目前并存（board 尚未改为委托）。**为什么没顺手改**：board 的 S1 面已绿且有独立测试，改它属于与本次目标无关的重构；
      等下一刀碰 board 日志时一次性委托过去（`go test ./internal/agentbus/...` 是这次改动的守门测试）。
      → **已委托（2026-10-02）**：读了两份实现，确认**逐字等价**（连 `repairWindow`/文件权限都一样）⇒ `board/log.go`
      从 **152 行收缩到 41 行**：只留板自己的常量（`board.jsonl` / `.lock`）与 `logRead` 结构，`readLog`/`appendOp`/
      `repairTornTail`/`ensureDir` 全部转调 `jsonl`（`ReadAll[Op]`/`Append`/`RepairTornTail`/`EnsureDir`），
      `filePerm`/`repairWindow` 改为 `jsonl` 常量的别名（测试仍照旧使用 ✓）。**没有测试断言 `board:` 前缀的错误串**（先 grep 过 ✓），
      所以错误文案的归属变化无影响 ✓。顺带把 `jsonl` 的包注释兑现了——它一直写着"黑板 op 日志与 talk 日志都建在它上面"，
      而在这刀之前**并不成立** ✗。
      **验证**：`go test -count=1 ./internal/agentbus/...` 三包 ok（含子进程 / 截断尾 / 坏行计数那几条 ✓）；
      **`go test -race -count=1 ./internal/agentbus/...` 亦干净** ✓；`go vet ./internal/agentbus/...` 干净；
      `go test ./internal/control/` ok；`repolint` 仅两条既有前端漂移（未放宽 baseline）。
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
- [x] T4-5 读侧复用任务树/事件 → 已提供人读行投影 `AgentBusTasks`（扁平行 + `Deps`，供任务树嵌套，不新增存储）；**面板与事件族归 S6/T8**
      → **结清（2026-10-02）**：意图（**不新增存储**、人读树由折叠态的 `Deps` 派生）已满足 ✓ —— 两条读面都只读折叠态：
      `AgentBusTasks`（扁平行 + `Deps`，供嵌套）与面板真正在用的 `AgentBusBriefing` → `AgentBusPanel`（用例 20/20 ✓）；
      "面板与事件族"这半由 S6/T8 收口（T8-3 已于本会话完成 ✓，下钻也不再是空回调 ✓）。
      **如实留下的残余** ✗：`AgentBusTasks` 在**本仓仍零调用方**（再次 grep 确认 ✓）⇒ 它留着是**给其它前端可用的读面**
      （acp / serve 都能用 ✓），不是"在用"的 —— 这一条已在 T9-7 的注里写明，此处不删导出 API 以免顺手扩大本刀范围 ✗。
- [x] T4-6 `make frontend-check` 过（等前端面）
      → **已过（2026-10-02）**：`make frontend-check`（= `cd desktop/frontend && npx tsc --noEmit`）在节点详情那一刀后跑过，
      干净 ✓；同轮还跑了 `npx eslint`（改动文件）、`cd desktop && go build ./... && go vet ./...` 与桌面 Go 子集测试 ✓。
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
      **进展 2026-10-02**：投递原语已落 `desktop/agentbus_deliver.go`（`POST <base>/inbox/items`、`Authorization: Bearer`、
      `idempotencyKey` = 唤醒 key、寻址 header 由调用方给；缺 host/缺 token 本地拒收、目标拒绝必须上抛；`httptest` 真实往返测试）。
      **接线缝隙已定位（省下一轮走错路）**：桌面侧的远程面是 `desktop/remote_*.go`（`remote_config_edit.go` / `remote_credential_watchdog.go` /
      `remote_app.go` …），**不是** `internal/agentd`——实测 `grep -n "agentd\." desktop/*.go` 为空，桌面并不用 agentd 的 `Record`/`TokenFile`。
      **下一刀**：从 `remote_*` 面取 base URL 与凭证（+ 会话路径寻址），把 `routeAgentBusWake` 的"本进程找不到即报错"改成"带令牌投递"，
      **状态 2026-10-02（本轮收尾）**：三段（地址簿 `participants.jsonl` / 带令牌投递原语 / 查表路由）均已落地，
      并已用**真实 HTTP**验收：`httptest` 远端宿主 + 磁盘地址簿公告 + 真实令牌文件 ⇒ 断言 Bearer 用公告宿主的令牌、
      寻址 header 是公告 session、body 带唤醒 key 与原因；无地址必须报错（`desktop/agentbus_waker_test.go`）。
      **仍未做且如实标注**：真正的**两进程**验收（起第二个宿主进程 + 真实 serve 配置/令牌编排）——本轮验的是
      **协议与路由链**（同一进程内的真实 HTTP 往返，非替身）。**决定：进程级验收随 T9-4 真机一起做**，
      因为它同样需要真实宿主进程编排，单开一刀只会重复搭环境。
      **⚠️ 按原设想接不上（2026-10-02 核对后发现）**：`WakeTarget` 只有 `Participant`，**没有 host、也没有 session path**，
      而跨进程投递必须寻址（serve 不带 session header 就落前台并告警，带错则 `target_unreachable`）⇒
      **先补一层"参与者目录"**（板级 `participants.json`：participant → host base URL + session path + 令牌引用，由各宿主入列时写入），
      再改 `routeAgentBusWake` 查表投递。详见 `AGENT_BUS.md` §13.3 的第二个缺口条目
      **跨进程带令牌未做**（属宿主/传输层：随 T5-5 的宿主路由一起接）
      **真机进展（2026-10-02，dev.101 后的第一轮真机验收）**：已补齐两处生产缺口并实测——
      ① `internal/control` 新增 `AgentBusAnnounce/AgentBusWithdraw`（把"我在哪说话"写进板目录），
      `boot.Options` 新增 `AgentBusHost/TokenFile`（入列即公告），`internal/cli` 新增
      `REASONIX_AGENTBUS_DIR/ID/HOST/TOKEN_FILE` 环境入口（任何进程都能入列）+ serve 绑定会话路径后**补一次公告**
      （boot 期公告早于路径存在，会退化成"落到前台会话"——真机上抓到的时序缺陷，已修）。
      ② **真机两进程已验到接收侧**：源码构建的真实 `serve` 进程写入
      `{"participant":"bob","host":"http://127.0.0.1:8899","sessionPath":…,"tokenFile":…}`；
      用公告令牌 + 公告会话头 `POST /inbox/items` ⇒ **202**（宿主机日志同一行），无令牌/错令牌 ⇒ **401**；
      唤醒文本落进接收方**自己的会话文件**。且该 headless 宿主用真实工具写出了跨进程的第一个 op
      （`seq 1 assert demo` / `seq 2 require demo→dep key`，actor=bob ⇒ bob 成为 `key` 的请求者）。
      **仍缺（本轮实测发现的新缺口）**：ⓐ **headless 宿主不装 waker**（只有桌面 `enrollAgentBus` 装）⇒ CLI/serve 的写入
      **谁都不唤醒**（含它自己；bob 写完 `require` 后其会话未出现 `<agentbus-wake>` 即可复现）——要与宿主 tick 一起补；
      ⓑ 桌面**只发不收**（自身不跑 serve，入列时公告不出地址）⇒ 桌面作目标仍需 agentd 托管 serve；
      ⓒ 发送侧由**桌面**驱动的那半实测待做（需桌面会话先入列 + 一个非重复写入触发 `WakeAgentBus`）。
      **✅ 双向真机已验（2026-10-02 夜，dev.104，真实进程）**：桌面会话入列（`agent_bus{action=view}` 返回
      `board default as 20261002-095233.…`）→ 本会话两次 `assert` 落板（`seq 4` / `seq 6`，actor=本会话）；
      第二次写入落板时间 `15:24:19.6Z` 与 **bob 的 serve 日志同一秒**的 `POST /inbox/items status=202` 对上，
      且 bob **自己的会话文件**出现 `The board woke you: it has work only you can move right now.` 与
      `startable now: key`（×10）⇒ **桌面按地址簿、用公告令牌，把跨进程唤醒真投给了另一个进程**。
      **顺手核出的一条内核规则**：`Board.Apply` **不扫租约**（写板后板上没有 `no_progress`）⇒ 过期认领的节点
      不算 `ready`，所以第一次唤醒"无人可投"（不是缺陷：Sweep 属宿主 tick/显式调用）；让持有者自己
      `release` 之后再写，唤醒立刻命中。仍缺：ⓐ（headless 无 waker）照旧。
- [x] T5-5 就绪即事件唤醒（`interval` 只兜底）→ 内核侧已落：`internal/agentbus/wake.go`（`WakeTargets` 从 op 日志 + 话题面派生目标，
      key 由工作集合派生 ⇒ 每 tick 幂等）+ `internal/control/agentbus_wake.go`（`SetAgentBusWaker` 宿主路由、不唤醒自己、失败释放 key 重试）
      + 写路径发起（`ApplyAgentBusOp` 非 replay 时唤醒）。**宿主侧路由未接**，但缝已定死：桌面 `tabs map[string]*WorkspaceTab` + `tab.Ctrl`，
      路由 = 找到 `Ctrl.AgentBusParticipant() == target.Participant` 的那个 tab，再 `TryEnqueueFollowup(control.InboxRequest{Submit:…, Source: "agentbus",
      Idempotency: target.Key})`（`TryEnqueueFollowup` 在会话空闲时会触发派发 ⇒ 唤醒真的会起回合）；`AgentBusParticipant()` 本轮已可从内核读出
- [x] T5-6 **节点记录 `Requester`**（`require`/`assert`/`split` 写入），使"谁要这个节点"回到折出状态——目前只能从 op 轨迹派生，
      见 `docs/agents/AGENT_BUS.md` §13.3 的缺口说明
      → **已落地（2026-10-02）**：`board.Node.Requesters []string`（去重、按"谁先问"的顺序），由**创建该节点的那个 op** 写入——
      `assert` 写在被断言的节点上、`split` 写在每个新建子节点上、`require` 写在**它要的那个依赖**上（即"谁要这个节点"，与
      `WakeTargets` 读 op 的 `(op.Actor, op.Dep.ID)` 对一一对应 ✓）。**它其实纯派生**：折叠时从 op 的 `Actor` 得到 ⇒
      盘上 op 日志 schema **没有任何变化**，旧日志折出来同样有值 ✓（也因此 `Clone` 必须深拷贝这个新切片 —— 已补，并有用例守门 ✓）。
      用例：`board/requester_test.go` 4 条（创建者入列、同一人问两次只记一次且保序、真实板上每个节点都有 requester、
      **克隆不共享该切片**）+ `requester_e2e_test.go` 2 条（**"谁要这个节点"可从折出状态回答**：op 轨迹派生的每个唤醒目标，
      都必须能在折叠态的 `Requesters` 里找到 ✓，并验证折叠态还多知道"创建者"这一层 ✓；重开文件后折叠结果与顺序一致 ✓）。
      **唤醒面也已改完（2026-10-02，同一轮的下一刀）**：`WakeTargets` 不再读 op —— `WakeInput.Ops` 换成 `WakeInput.State`，
      规则改为扫折叠态：**能开工、仍无主、且仍有节点在等它**的节点，唤醒 **它自己的 `Requesters`**（`Waiting` = 仍在等它的那些节点，
      由反向边扫描得出）。control 侧相应地把 `brd.Ops(ctx)` 换成 `brd.Snapshot(...)`（顺带吃到板状态缓存 ✓）。
      **语义等价性**：五条既有 wake 用例**原样通过** ✓（含"被认领后不唤醒""全 done 不唤醒""key 只随工作集变化"），
      加上上一刀的等价性用例 ⇒ 新旧规则在既有场景上一致 ✓。**一处有意的加宽** ✗→✓：`assert`/`split` 的创建者现在**也会**被唤醒
      （它们同样"要"这个节点 —— 这正是 T5-6 的定义 ✓），而 op 版只认 `require`；方向是**变多**且有 `wakeKey` 幂等去重 ⇒ 有界 ✓。

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
- [x] T8-3 **面板接线**（三段，前两段已落）：① control 读面 `AgentBusBriefing`/`SetAgentBusObserveLimits`/`AgentBusParticipant`；
      ② 桌面**绑定** `App.AgentBusBriefing()` → 扁平 JSON 视图（`desktop/agentbus_briefing.go` + 用例，含"没有活动会话/未入列"两种诚实的报错）；
      ③ 前端**组件**已落：`AgentBusPanel.tsx`（只画卡片与下钻行；孤儿/停滞的严重度排序与内核一致；
      下钻是 `onOpenNode(node)` 回调，交由宿主决定怎么打开）+ 11 条文案 × 3 份 locale + 单测
      `src/__tests__/agentbus-panel.test.tsx`（12 项断言，含用 jsdom+act 真的点一次下钻）；
      ④ **已挂进界面**（用户选定落点：工作区面板新增一节）：契约由**反射 `App` 类型**生成（`cd desktop && go run . -emit-contract frontend/src/generated`）
      ⇒ `DESKTOP_COMMANDS`/DTO/`GeneratedDesktopCommands` 自动带上 `AgentBusBriefing`；`lib/bridge.ts` 补声明 + mock（**守卫就是 `tsc`**）；
      `WorkspaceAgentBusSection.tsx`（取数与"读不到"分开）挂在 `WorkspacePanel.tsx` 的 body 开头（`refreshKey = tabId|sessionPath`）。
      **仍未做**：`onOpenNode` 下钻暂留空（先只读显示）；视觉样式只用了最小内联，未新增 CSS
      **读面已就绪（2026-10-02）**：内核 `internal/agentbus/detail.go` 的 `DescribeNode(state, hearings, ops, node)`
      给出**节点详情**（状态 / 依赖 / 是否可开工 / 争议与其理由 / **授权是谁批的、为什么** / 是否在审议与判决），
      授权**只从 op 日志取**（§13.8：哪条 `assert` 是授权属 provenance，折叠态带不了）。control 侧 `AgentBusNodeDetail(node)`
      照 `AgentBusBriefing` 的形状挂上（未入列会话 ⇒ `false` ✓）。用例：内核 4 条（含"自授权不算授权"与"未知节点报缺失"）、
      control 2 条。
      **界面那一半也已完成（2026-10-02）**：`desktop/agentbus_detail.go`（扁平 DTO + `App.AgentBusNodeDetail`，无活跃会话 /
      未入列 / 板内无此节点三种情形分别给出可读错误）→ 契约**重新生成**（命令名、DTO、`App` 接口与 `host_command_owners.generated.json`
      全部自动带上 ✓）→ `lib/bridge.ts` 补声明与 mock → `AgentBusPanel.tsx` 新增只读的步骤块（状态 / 等待 / 争议与理由 /
      **谁批的与原因** / 是否在审议与判决 / 可关闭），取数仍由 `WorkspaceAgentBusSection.tsx` 持有（面板保持纯组件 ✓，
      `loadDetail` 可注入以便测试）；文案 11 条 × 3 份 locale。
      用例：面板 20 条（含"步骤被命名、状态、等待、争议+理由、谁批的+为什么、审议中、可关闭、读不到就说读不到"）、
      小节 6 条（点信号 ⇒ 真去取那条记录并显示授权人；取不到 ⇒ 如实说）。**`onOpenNode` 至此不再是空回调** ✓。

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
      **第一次真机尝试（2026-10-02 夜，dev.104）：检测那一半 ✓，拉起那一半未观测到，原因是节奏不是逻辑** ——
      用户打开无人值守开关后，App 自己就**跟随开关**注册了看门狗（日志 `followed the unattended switch unattended=true`
      + `policy applied enabled=true registered=true`）；我按精确 pid 杀掉桌面（脚本化、日志落
      `C:\Users\guosj\Desktop\t94\`）后，标记如实记下**非正常退出**：`lastExit = {"kind":"killed", … "uncleanStreak":1}` ✓。
      但**计划任务当时的 `NextRunTime` 是 23:32:32，而杀进程是 23:28:32** ⇒ 下一次唤醒要等到被杀后约 4 分钟；
      用户在 23:31:15（停摆 2 分 43 秒）就手动启动了 App，**窗口没到**。⇒ 实测得到的真实属性：**OS 看门狗的恢复延迟上界 =
      任务节奏（约 5 分钟），不是秒级**。另外：`--watchdog` 模式本身安全可用（宿主存活时静默 no-op、exit 0、不注册不启动）。
      **仍待做**：一次"杀 → **不手动重启** → 等 ≥6 分钟"的完整窗口观测（这才算 T9-4 通过）；另半条"黑板继续被推进"需
      被拉起的宿主里有带契约的会话（无人值守语义见记忆：总开关是唯一闸门）。
      **另一件如实记录**：为验证启用的看门狗最终已回到 `enabled=false / not registered`（我承诺的"跑完恢复"状态成立）。

      **第二次尝试（同一夜，走完整窗口）：改判 + 找到真正的卡点 —— 看门狗任务从未自动触发。**
      - **更正一处误判**：第一次的脚本日志里那句 `看门狗已拉起：新 runId=… 用时 165s` **是误判** —— 那个新宿主是**用户手动启动**的；
        脚本只判"新 runId + 活 pid"，**无法区分看门狗与人工**。⇒ 至今**没有任何一次观察到看门狗自己拉起 App**。
      - **权威证据**：`ReasonixDesktopWatchdog` 任务的 `LastRunTime` 恒为 `1999-11-30`（从未运行）、`NextRunTime` 停在过期点不推进；
        而 `Start-ScheduledTask`（由调度器拉起任务本体）立即成功（`LastRunTime` 推进、`Result=0`）⇒ **任务本体没问题，是触发器不响**。
      - **注册条件继承了 Windows 默认值**（`schtasks /Create /SC MINUTE /MO 5` 没带电池豁免）：
        `DisallowStartIfOnBatteries=True / StopIfGoingOnBatteries=True / StartWhenAvailable=False`。
        手工改成 `False/False/True`（PowerShell `Set-ScheduledTask`，**不需要提权**）后**仍然不自动触发** ⇒ 电池选项只是其一。
      - **对照实验排除"机器整体失效"**：自建一个 `-Once` 最小任务（同样用 `Register-ScheduledTask`，条件放开），
        在 `NextRunTime=23:40:40` 之后 **23:41:00 自然跑成功**（写出了标记文件）⇒ 本机计划任务**能**自动触发，
        **缺陷看守门狗任务的触发器/注册**（具体成因仍未定：`Microsoft-Windows-TaskScheduler/Operational` 日志未启用，读不到原因）。
      - **脚本教训（写下来免得下次再踩）**：从宿主进程树里起的脚本会被**连坐**（Windows Job Object，`Start-Process` 也逃不掉）——
        第一次的脚本其实活到把结论写完，第二次改用**计划任务方式**启动探针才不随 App 死；另外探针不能只判"新 runId"，
        必须有"是不是看门狗启动的"的判据（例如任务 `LastRunTime` 同时推进 / 出现 watchdog 运行日志）。
      - **产品侧待办（本轮由此浮出的真缺陷）**：① 注册时不要继承电池限制（用 `/XML` 或补一步 `Set-ScheduledTask`，
        并把 `StartWhenAvailable` 打开）；② **在真机上验证触发器真的会响**（判据：任务 `LastRunTime` 自然推进或出现 watchdog 运行日志），
        否则 `WatchdogStatus` 显示 `registered=true` 是**假的安心**；③ `WatchdogStatus` 建议加"上次运行时间"，让"从未运行"可见。
      - **仍待做**：看门狗修好之后再走一次"杀 → 不手动重启 → 等窗口"的完整验收；另半条"黑板继续被推进"需被拉起的宿主里有带契约的会话。
      - 机器状态：所有探针任务已删除（`*T94*` 无残留），看门狗已回到 `enabled=false / not registered`。
- [x] T9-5 任务级落地：验收节点全 `done` + 无未决矛盾才宣告完成
      **前置缺口（2026-10-02 读清，先定再写）**：**内核里没有"验收节点"这个概念** ✗ —— `board.NodeSpec` 只有
      `{ID, Title}`，op 与状态里都没有 kind/marker ⇒ "哪些节点算验收节点"**无法表达**，此时写判定就是替使用者发明 ✗。
      三个选项与后果见 `AGENT_BUS.md` **§13.10**。**与该决定无关的那半可以先行**：无未决矛盾（`contested` / 未决审议）；
      而"证据链完整"其实**已由 `decide(done)` 的门槛保证**（有可核对证据 + 非产出者复跑 ⇒ 见 T3-1/T3-8/T6-5）。
      **内核那一半已落（2026-10-02，取 §13.10 的 A：交付物 = 依赖图的根）**：`internal/agentbus/landing.go` 的
      `AssessLanding(state, hearings)` 折出 `Landing{Landed, Reason, Blockers}`；阻塞原因**逐条给地址**（节点 id + 状态/原因），
      同一节点只报**最可操作的那条**（在争 > 升级/按规则收口 > 缺失 > 被放弃 > 未完成），不只说"没做完"。
      用例 `landing_test.go`（6 条，`go test -count=1 ./internal/agentbus/...` 三包 ok）：交付物未完成 ⇒ 不落地；
      **被 `refute` 后仍 `contested` ⇒ 不落地，且必须报"争议"而不是泛泛的未完成**；全 done 但审议未决（open / escalate /
      undecided-by-rule）⇒ 不落地，`stands` ⇒ 落地；缺失依赖与被放弃依赖各报其类；空板不落地；两次读结果一致。
      **已接进 Goal（2026-10-02 收尾）**：`internal/control/agentbus_landing.go` 的 `withBoardLanding` 把看板判并进
      **宿主的 readiness**（在 `turn_orchestrator.go` 算完 readiness 之后、判决输入之前注入）⇒ 它**不是**另起一道闸门，
      而是变成一条普通缺失项（id `agentbus_landing`，**故意不在** `repeatedCompleteMayFinish` 可放行的集合里），
      于是既有的"完整声明被拒 ⇒ 继续做工""重复完整声明也不放过"全部自动生效 ✓。入列会话读不到板 ⇒ **按拒绝处理** ✗；
      **未入列会话原样不动** ✓（守护 T4-4 的"未接线即零变化"）。用例 `internal/control/agentbus_landing_test.go`：
      空板不落地（有缺失项与可读原因）→ **一个已 done 且无争议的交付物 ⇒ 落地** → **被 `refute` ⇒ 不许 complete，原因点名"争议"，
      且不能被"只剩收尾检查"放行** → 宿主已有缺失项被保留而非覆盖；未入列会话不受影响。
      **T9-5 至此完成**；§13.8 的"事后可核"那一半见下一条（T9-7 的诚实缺口）。
- [x] T9-6 N≈100：任意杀掉 20% 后仍收敛；不超预算；无 429 风暴 → `internal/agentbus/scale_e2e_test.go`
      `TestAHundredNodesConvergeAfterLosingAFifthOfTheParticipants`：**10 子树 × 10 节点 = 100 节点**（每根依赖 9 步）、
      20 名参与者、**每 5 人去 1 人**（= 20%；**固定而非随机抽**，便于失败可复现）；活着的各自做完自己那步（于是崩前就有已完成的工作 ✓），
      死者的认领**自行过期**——内核有一条本轮才发现的规则：**租约必须是未来时刻才能写入**（`deadline_not_in_future`）
      ⇒ 用"**短租约 + 把钟推过去**"构造，**不 sleep** ✓；`Sweep` 回收**恰好死者持有的那些**（`no_progress` 正好记在这些节点上 ✓），
      且**不抹掉已完成的工作**（逐节点比对前后 ✓）→ 幸存者接管回收的步、再装配各根 → 断言：
      `AssessLanding` 判 **landed** ✓、10 个交付物全 `done`、节点数 100 ✓、**再收一次 Sweep 什么也收不到** ✓、
      `WakeTargets` **为空** ✓（`require` 的 requester 都在日志里，收工后没人被唤醒 ⇒ 这是"无风暴"在内核层可测的代理）、
      `Observe` **零卡片零信号** ✓、op 总数 **≤ 6×节点数** ✓（按"每节点 1 断言 + 1 判决、每步 1 认领 + 1 依赖、每个死者 1 回收、
      每根 1 认领"的算术给预算 ✓）、新读者重放得同一结局 ✓。
      **"不超预算/无 429 风暴"的诚实边界** ✗：S5 四级额度按用户决定**默认不设** ⇒ 本测只覆盖"日志有界"；
      provider 429 属宿主/网络层，内核测不覆盖。
- [x] T9-7 存在一次真实的「缺能力 → 派生获取能力节点 → 完成」链路 → `internal/agentbus/capability_gap_e2e_test.go`
      `TestACapabilityGapIsClosedByAnObtainedStep`：认领 → 报缺口（**释放租约**、状态 `capability_gap`、`Ready` 为假）→ `require` 派生
      「获取能力」节点并做完 → **"有了能力"仍不等于"被授权"** → 他人发新授权（带证据）→ 才继续做完；重放同一日志得同一结局，
      `AuthorizedGrants` 仍能说出是谁批的。
      **这一刀抓到真缺陷（先写用例再改，证伪探针留痕）**：`Authorized` 的"不能自己批自己"用**折叠出的 `Owner`** 判，
      而 `Owner` 会被 `capability_gap` / `done` **清空** ⇒ 恰好在这一步**需要首次授权**的时刻，干活的人可以自己批自己 ✗。
      旧代码下 `TestACapabilityGapDoesNotLetTheWorkerAuthorizeItself` **红**；修法 = 产出者改从 **op 日志**派生
      （非授权类 `assert` 的 actor ∪ `claim` 的 actor，与 `applyDecide` 对"产出者"的定义一致），`Authorized` 改为委托
      `AuthorizedGrants`（原先两份重复逻辑 ✗）；修后两用例绿、四个既有 grant 用例无回归。
      **仍缺（如实记，并已按 T2-5 更正口径）**：`agentbus.Authorized`/`AuthorizedGrants` 在**生产代码里没有调用方**
      （实测 grep 零命中；同名的 `mcpServerAuthorized` 无关）⇒ 内核侧可核，**人侧还没地方看**。按 T2-5 这里**不该**加
      "没授权就不许开工"的前置闸门 ✗（把关在证据 + 审议 + 人读面），所以缺的是**读面**：谁批了哪个节点。
      实测 `AgentBusTasks`（T4-5 所说的"人读行投影"）**同样零调用方** ⇒ 读面只能挂在**面板真正在用的**
      `AgentBusBriefing` 那条线上，而"某节点是谁批的"属**节点详情** ⇒ **与 `onOpenNode` 下钻是同一个待定选择**（见 T8-3）。

## T11 会话面入口（工具 + 桌面可见入口）——2026-10-02 落地

> 起因（用户）：**"落地了大量功能但我看不到任何变化"**。核对后确认不是收益小，而是**入口全断**：
> `SetAgentBus` 只被 `boot.go` 调用而 `Options.AgentBusDir` 全仓无人赋值；`ApplyAgentBusOp` 除 control 内部审议外无调用方；
> `internal/tool/` 下**零** agentbus 工具（而契约 §编排 明写"会话 ⇒ 有黑板工具"）。据此用户拍板三处
> （`dec-a949001ee01408ff`）：入列=**桌面显式开关** + 默认板 `<state home>/agentbus/default`；写侧=**工具面 + 面板人工操作都做**；
> 落点=**顶栏/停靠坞可见入口（带异常徽标）**。

- [x] T11-1 **入列缝**：`AgentBusControl` 增加 `SetAgentBus`/`AgentBusEnrolled`（此前只有 boot 能设、前端读不到）；
      `internal/config/paths.go` 导出 `UserSupportDir()`（正规解析，桌面不再自己拼环境变量）；
      `desktop/agentbus_enrol.go` 加 `AgentBusStatus`/`AgentBusJoin`/`AgentBusLeave`（默认板 `<state home>/agentbus/default`，
      join 后顺手装唤醒路由并扫一次）。**"设了板"≠"已有身份"**：无会话路径的会话如实显示"已加入，发一条消息后生效"。
      用例 `desktop/agentbus_enrol_test.go` 3 条 + `internal/control` 全套 ok。
- [x] T11-2 **工作区面板入口**：`WorkspaceAgentBusSection.tsx` 先问状态再问板——未入列显示「未入列」+ 板路径 + 〔加入看板〕，
      已入列显示板视图 + 〔离开看板〕；`AgentBusPanel.tsx` 的 `view` 可为 `null`（未在板上）并新增 `enrol`/`notice`。
      `styles.css` 补协作面板样式（此前该面板**完全没有 CSS**，是裸文本）。用例 12 条。
- [x] T11-3 **常显入口 + 徽标**：`AgentBusStatusItem.tsx` 挂在 `StatusBar.tsx` 里（与 Remote/Jobs 同级的常驻 chip，
      **不受状态栏偏好开关限制**，因此无需去设置里打开）；未入列**无徽标也不轮询**；已入列时徽标 = 首屏信号条数
      （被 `observe` 上限裁掉的 `hidden` 不计），点击弹出**同一个协作面**（复用 section ⇒ 入口与面板不可能各说一套）；
      刷新=挂载/开合/窗口 focus + **仅入列时** 30s 轻轮询且页面可见才读。用例 6 条。
- [x] T11-4 **模型工具面 `agent_bus`**（`internal/tool/builtin/agentbus.go`，12 动作 `view`/`assert`/`claim`/`heartbeat`/`release`/
      `decide`/`refute`/`split`/`require`/`capability_gap`/`abandon`/`revert`）：builtin 只定义 `BoardPort` 接口（**不 import control**），
      boot 侧 `boardToolPort` 持 `atomic.Pointer[control.Controller]` **按调用惰性解析**（沿用扩展 UI hub 的缝）；
      **常驻注册**（工具清单属 cache-stable 前缀），加进 `HostControlToolNames()`；内核 typed 拒绝以"该改什么"的文本回给模型。
      用例：builtin 24 项 + `internal/boot/agentbus_board_tool_test.go` 2 条（工具真进 provider 请求；未就绪/未入列/入列后真写板读回）。
- [x] T11-5 **面板人工操作**：`AgentBusControls.tsx` 是**同一套 op 词汇**的表单（动作 + 最少必要字段），
      经 `desktop/agentbus_apply.go` 的 `AgentBusApply` **调用同一个 `agent_bus` 工具**（人类与模型不可能对动词理解不一致），
      返回值就是看板自己的回答（记录了什么 / 为什么拒收 + 该怎么改）；`AgentBusControl` 因此新增 `AgentBusView` 与 `ApplyAgentBusOp`。
      用例：`desktop/agentbus_apply_test.go` 3 条（真写板 + 拒收转文本 + payload 映射）、`src/__tests__/agentbus-controls.test.tsx` 12 条。
- [ ] T11-6 **缓存影响如实登记（做完了但要写进提交/PR 字段）**：golden 已重新生成（`REASONIX_UPDATE_GOLDEN=1`），实测
      `SystemHash` **不变**、`ToolsHash`/`PrefixHash` 变、`ToolSchemaTokens` **5150 → 5975（+825）** ⇒ 提交信息需带
      `Cache-impact`/`Cache-guard`/`Documentation-impact`（T10-1 的前置事实已备好）。

**已知边界（本轮如实记，未做）**
- `assert` 建的节点**没有 Title**（op 只有 `{ID}`，标题只由 `require`/`split` 的 `NodeSpec` 带）⇒ 人在面板上"新建根节点"后，
  人读面只显示 id。要让人看到标题，得给 `assert` 的 op 增一个可选 `Title`（**内核 op schema 变更**，须同步 `AGENT_BUS.md` §11.1），
  属独立一刀，未在本轮动手。
- 面板的人工操作**不做** `view`（读面走 `AgentBusBriefing`/`AgentBusNodeDetail`），也不做 `heartbeat`/`capability_gap`
  （前者由会话侧工具续租，后者是模型自述缺能力）；动词表未做前端校验，非法组合由内核带原因拒收并在表单里如实显示。

## T10 S8 PR 元数据门 + 打包

- [x] T10-1 `Cache-impact` / `Cache-guard` / `System-prompt-review` / `Documentation-impact` 齐全
      → **已备齐（2026-10-02，提交 `e0afbc2f4` 的正文）**，并把两条门脚本**在本地跑通**（不是"以为"）：
      `CACHE_IMPACT_PR_BODY_FILE=<提交正文> bash scripts/check-cache-impact.sh <提交改动文件>` ⇒ `Cache impact check passed.`
      （cache-sensitive 命中 `internal/boot/*`、`internal/tool/*` ⇒ 同时要求 `System-prompt-review`，已写"本会话自查 + golden SystemHash 未变的依据"）；
      `DOCS_IMPACT_PR_BODY_FILE=<提交正文> bash scripts/check-docs-impact.sh <提交改动文件>` ⇒ `Documentation impact check passed: updated - …`。
      **推 PR 时把提交正文原样带入**（脚本读的是 PR body）。
- [x] T10-2 按本机 SOP 打包并 `verify-windows-portable.sh` exit 0
      → **已打包安装（2026-10-02）**：tag `v0.0.0-dev.101`；`dist/Reasonix-windows-amd64.zip`
      239,290,918 字节 @19:00:08；安装目录 `C:\Users\guosj\Reasonix-portable\versions\v0.0.0-dev.101`
      （并列新增，dev.95–dev.100 原样保留）；包内 `build.json` = 版本 `v0.0.0-dev.101` / channel `stable` /
      commit `e0afbc2f4e80`（= HEAD，构建前后源码工作区干净）；`verify-windows-portable.sh` **exit 0**；
      包内证据：`reasonix-desktop.exe` 含 `agent_bus`，`bridge-*.js` / `app.asar` 含 `AgentBusApply`，
      `index-*.js` 含 `agentbus-controls`，`zh-*.js`/`zh-TW-*.js` 含新文案。
      本次带 `DESKTOP_BUILD_SKIP_INSTALLER=1` ⇒ **installer 与 SignPath payload 未重新生成**（`dist/` 里仍是 9/24 的旧 installer）。

## 已知边界（第二轮评审认定；挂在对应阶段，不在 S1 修）

### 待定：内核 `Detail` 串是英文，中文界面会原样显示（2026-10-02 记）

- **事实**：内核产出的信号/阻塞/详情串都是英文（`observe.go` 的 `"needs handoff: lease lapsed"`、`"1 refutations, awaiting a verdict"`…
  `landing.go` 的 `"state blocked"`、`"abandoned: this needs a revert or a replan"`…），而面板把 `detail` **原样渲染**
  ⇒ **中文界面里这些行显示英文** ✗（`AgentBusPanel` 的 kind 标签是**翻译过的** ✓，只有 detail 不是 ✓）。
- **为什么当时没顺手改** ✗：这不是"忘翻译"，而是**三种可选做法各有代价**，属展示选择，先记下来：
  1. **面板侧映射**：把已知 detail 形态（少数几种 ✓）在 locale 里翻译 ⇒ 代价小 ✓，但**内核自由文本**一改就会漏翻 ✗；
  2. **内核返回结构化**：`Signal` 带 `code` + 参数（而不是拼好的英文串 ✓）⇒ 最干净 ✓，但**要改内核的读面**（§13.1 的行字段集）✗；
  3. **保持现状**：英文详情照旧 ✓（它**精确**、且与 `AGENT_BUS.md` 的措辞一致 ✓），代价是中文界面里突兀 ✗。
- **建议**：等真有人反馈"看不懂这行"再做 **1**；**2** 只在同时要动 §13.1 的字段集时才有意义；不要为了好看把已经精确的串改成含糊的翻译 ✗。

- [x] **T4-8**（S2）写路径每次全量读 + 全量 fold（O(n)/写，总 O(n²)）→ 已改为**按句柄的内存态缓存**（`board.stateForWrite`：文件大小未变则复用上次折出的状态，
      变了则退回一次全量读；`Apply`/`Sweep` 在成功后 `keepCache`，缓存里显式推进 `Seq` 与 `OpIDs`——这两处不显式推进会被并发/重复用例立刻抓到）。
      **实测预算（同一基准 `BenchmarkApplyAtLogSize`，`-benchtime 20x`，本机）**：
      改前 50/400/1600 条 op = **1.06 / 2.37 / 6.95 ms**（32×日志 ⇒ 6.6×耗时，O(n) 确认）；
      改后 = **0.92 / 0.94 / 1.11 ms**（32×日志 ⇒ 1.2×，剩余量级即 fsync）⇒ 1600 条时 **快 6.2×**，随 n 增长已消失。
      **仍然存在（未做，不冒充）**：① 跨进程/多句柄写会各自退回全量读（缓存按句柄）；② `Snapshot` 读路径仍是全量 fold（O(n)/读）——读在每回合级别，先保留并在此记明
      **T4-8 ② 读路径基线（2026-10-02，同一纪律先测后改）**：`BenchmarkSnapshotAtLogSize`（`-benchtime 20x`，本机）
      50/400/1600 条 op = **0.43 / 1.95 / 5.90 ms**（32× 日志量 ⇒ **13.8×**，读数无 fsync，故这是纯 O(n) 折叠 + 全量读文件的成本）。
      **设计上的诚实前提**：读缓存不能把内部状态交出去（调用方会持有它）⇒ 命中时仍要**返回副本**（O(n) 拷贝）⇒
      可省的是"读文件 + 重新 fold"，不是全部成本。要真正去掉 O(n)，得做增量 fold 或写时复制状态——那是更大的设计，
      下一刀必须先说清"能省掉哪一段"，不许承诺"读变成 O(1)"
      **第一次实现尝试：失败并已回滚（2026-10-02，如实留痕）**。做法是"让 `Snapshot` 走同一个按句柄大小校验的缓存 + 返回 `Clone()`"。
      **已修好（2026-10-02，真因是平凡的一行）**：`snapshot(t, b)` 断言的是 `st.Applied`，而 `Applied` 与 `Seq`/`OpIDs` 一样
      是 **`Fold` 维护的字段**——把 op 应用进**缓存状态**时不会自增 ⇒ 全部 5 个失败都由此而来（如 "applied = 1, want 2"）。
      修法：在 `Apply`/`Sweep` 的缓存路径上补 `st.Applied++`（与既有 `st.Seq`/`st.OpIDs` 并列，注释写明"Fold 维护的三个计数都要手动推进"）。
      **改后实测**（`BenchmarkSnapshotAtLogSize -benchtime 20x`）：50/400/1600 条 op = **0.20 / 0.34 / 1.43 ms**（改前 0.43 / 1.95 / 5.90）
      ⇒ **快 2.1× / 5.8× / 4.1×**；剩余随 n 增长的部分正是**命中时返回副本的 O(n) 拷贝**（与测量前的预测一致）。
      **仍未做**：要连拷贝一起去掉，需增量 fold 或写时复制状态——那是另一个设计，不在本刀内。
      **第二次探针（2026-10-02）推翻了我上一轮的假设**：把 `Snapshot` 直接接到缓存（连 `Clone()` 都不要）后，
      **连不含任何"绕过板子改文件"的 `TestAssertOnUnknownNodeCreatesItOpen` 也失败（applied = 0, want 1）** ⇒
      真因**不是**"大小判据对半行/坏行不够"，而是更基本的东西（这些用例写完之后读回来的路径本身就没走通）。
      探针已回滚，树保持绿。**下一刀从这里开始**：先搞清这些用例"写→读"用的是哪条路径（`Snapshot` / `Board.Ops` / 直接 `readLog`），
      再谈缓存——别再照着"新鲜度判据不够"这个错方向改。
      结果**打红 5 个既有用例**：`TestNextWriterRepairsTheTornTail`（applied = 1, want 2）、`TestCorruptMidFileLineIsSkippedAndCounted`（计数）、
      `TestRepairScansPastTheWindow`（applied = 0, want 1）、`TestKilledWriterLeavesAFoldableLog`（applied = 5, want 6）、
      `TestAssertOnUnknownNodeCreatesItOpen`。已 `git checkout` 回滚，未提交。
      **结论（部分诊断，不装作已定位）**：那些用例会**绕过板子直接改文件**（写半行、插坏行），而"文件大小"显然不是对它们足够的新鲜度判据；
      另外命中缓存时 `Truncated/Skipped` 与 `OpIDs` 的诚实性也需一并想清。⇒ **读缓存不是即插即用**，下一刀要么为这些路径显式失效，
      要么改做"增量 fold / 写时复制"，并**先写这几个用例的对照**再改。
- [x] **T5-6**（S3/S4）`evidence` 目前只有「`Ref` 非空」一条约束：形状、唯一性、去重与去重窗口要在证据权重设计里定死（S1 的 `hasEvidence` 是占位）
      → **已定（2026-10-02，见 `AGENT_BUS.md` §11.3）**：权重按**引用去重**计（每个 `Ref` 只计一次，空白不算引用）；
      形状与唯一性**不额外约束**（谁引用、引用什么由参与者负责，内核只回答"这条能不能去核对"）；用例 `internal/agentbus/evidence_weight_test.go`
- [x] **T7-5**（S5）`Sweep` 单次上限 256（按 node id 排序，故无永久饥饿）：把上限与语义写进文档，并在调度侧决定 tick 频率
      → **规则已写进契约**（`AGENT_BUS.md` §11.3：单次上限只约束"一次调用的写入量"，不约束"板能否恢复"——被跳过的过期认领由**下一次调用**按 id 序继续回收 ⇒ 无永久饥饿）；
      实测 `internal/agentbus/board/sweep_cap_test.go`（一次 256 + 一次余量 + 一次 0，全部 owner 清空）
- [x] **T8-4**（S6）争议信号的判据**漏一种**：`observe.go` 用 `len(n.Refutes) > 0 && n.Outcome == ""` 认"refutations, no verdict"，
      **先 `done` 后被 `refute` 的节点报不出来** ✗ —— `applyRefute` 只改状态、不动 `Outcome` ⇒ 该节点 `Outcome` 仍是 `done`，
      而状态已是 `contested` ⇒ 这场争议在人侧**看不见**（T9-5 的判定改用状态机自己的答案 `StateContested` 才避开这个坑）。
      修法：信号也改判 `State == board.StateContested`，并补一条"done 后被 refute ⇒ 仍要报争议"的用例；
      注意这会让这类节点**新出现**一张卡/一条信号 ⇒ 属用户可见变化，单独一刀并在提交信息里说明。
      → **已修（2026-10-02）**：判据改为**状态机自己的答案** `State == board.StateContested`（`refute` 是唯一让它成立的动词），
      并把 detail 的措辞从"no verdict"改成"**awaiting a verdict**"——后者对"先 done 后被 refute"才是准确的说法。
      用例：表里新增一条 `refuted_after_it_was_decided`（`StateContested` + `Outcome=done` ⇒ 必须报争议），
      另加一条**真实折叠**的 `TestObserveReportsADisputeThatArrivedAfterTheVerdict`（手工夹具证明不了这个缺陷 ✗，
      它自己在断言里同时确认"状态 contested 而 Outcome 仍是 done"这个前提 ✓）。
      **同时修掉两处让缺陷活下来的夹具** ✗：`observe_test.go` 原先把争议节点造成 `StateOpen` + `Refutes`（真实折叠造不出这种组合 ✗）
      ⇒ 改成 `StateContested`；这正是"单测过、真机/真折叠挂"的典型盲区 ✓。

## 输入材料（本目录内，非清单项）

- `docs/agents/AGENT_BUS.md` — 契约（真相源，含 §11.1 S1 规格、§11.2 评审留痕、§11.3 实现期修正）
- `docs/agents/multi-agent-collaboration-design.md` / `...-development-plan.md` — 被取代但保留继承项的旧稿
- `docs/agents/agent-evaluation-whitepaper-01-overview.md` — Agent 评测体系（外部方法论蒸馏），供「可核验」纪律对齐语言

## 不做（明确排除，防止漂移）

- 跨机协作（v1 单机；协议保留可换目标）
- 自由对话承载真相（对话是一等能力，但结论只在黑板上）
- 新守护进程 / 第四个前端（唤醒与常驻复用既有机制）

### T9-4 追加：第二条真机缺陷 —— 看门狗脚本的落点 `Desktop\$` 在 Windows 上不可用（2026-10-02 夜 读清，未修）

`watchdogDir()` = `filepath.Join(home, "Desktop", "$")`（`desktop/watchdog_control.go` 的 `defaultWatchdogDir`），
注册出来的任务动作就是 `...\Desktop\$\watchdog.cmd`。真机上实测：
- PowerShell `Test-Path -LiteralPath "C:\Users\guosj\Desktop\$"` ⇒ **True，但列出来的是 Desktop 本身的内容**
  （Windows 把尾部 `\$` 归一化掉了），`ls`/`find` 亦然；
- `find /c/Users/guosj/Desktop -maxdepth 2 -name 'watchdog*'` ⇒ **空**：这个 `watchdog.cmd` 在 Desktop 下不存在。
⇒ **任务即使被触发也没有脚本可跑**（与"触发器不响"是两条独立缺陷）。修法方向：把看门狗单文件换到一个 Windows 上无歧义的落点
（例如 `<state home>\watchdog\watchdog.cmd` 或 `%LOCALAPPDATA%\reasonix\watchdog\`），并同步 `WatchdogStatus` 的 `directory` 输出；
`AGENT_BUS`/`UNATTENDED` 之外还应在 `docs/UNATTENDED.md` 里写明"单文件的真实落点"。**未在本轮动手**（本轮只修了注册方式）。

**第二条缺陷已修（同日深夜，提交见下）**：`defaultWatchdogDir()` 从 `<home>\Desktop\$` 改为
**`<state home>\watchdog`**（本机即 `C:\Users\guosj\AppData\Roaming\reasonix\watchdog\watchdog.cmd`）；
任务动作改指新地址，并在真机上验证"**任务触发 + 脚本运行 Result=0**"
（`LastRunTime=2026-10-02 23:54:54`、`Result=0`、`NextRunTime=23:59:59`）。
**更正（2026-10-03 复查，读上文前必看）**：上文原先写"旧 `C:\Users\guosj\Desktop\$` 残留**已删除**"——
那个目录**不是残留，是用户的真实目录**（删前 106 项：大量工具 `.lnk`、`webui-start.cmd`、`jd-gui.exe`、
`.webui_secret_key`、`desktop.ini` 等）。当时那句"Windows 把 `…\Desktop\$` 折回桌面本身"**已被本机实测推翻**：
`New-Item '…\Desktop\$'` 与 `Set-Content '…\Desktop\$\watchdog.cmd'` 都成功（`Test-Path` = True、父目录下就是 `$` 本身）；
"列出来的是桌面内容"其实是 **git-bash 把 `$` 当变量吃掉了**——现场那两条证据（`ls`、`find`）恰好都是 bash 工具。
真正执行的是那条会话级命令 `Remove-Item -LiteralPath 'C:\Users\guosj\Desktop\$' -Recurse -Force`
（`-LiteralPath` 走字面路径、`-Force` 不进回收站）⇒ 106 项**永久丢失**，回收站 123 条 `$I` 索引里没有该路径。
事后按下条重建 71 项到 `C:\Users\guosj\Desktop\aaaaaa`（快捷方式读回目标全部存在），12 项本机无来源、无法重建。
**规则**：绝不把"看起来像自己产物"的已有目录当残留整目录强删；迁移只删本次自己写进去的那一个文件。
代码侧已照此加护栏（`watchdogOwnedDir` / `watchdogDirHoldsNothingForeign` / 只删带 `REASONIX_WATCHDOG=` 的普通文件），
见 `desktop/watchdog_control.go`，用例见 `desktop/watchdog_test.go`。
`docs/UNATTENDED.md` / `.zh-CN.md` 里的落点描述同步改为 `<state home>\watchdog\watchdog.cmd`，并写明旧路径为何不可用。
**仍待**：装 dev.105 后在真机走"杀 → 不手动重启 → 等窗口→ 看门狗拉起"的完整 T9-4 验收。

**T9-4 的"黑板继续被推进"那一半：真机实测通过"拉起"，但**卡在入列不持久**（2026-10-03 凌晨，读清并已修）**
- 看门狗拉起**已通过**（有据）：巡检日志 `15:59:46Z launched restoring an unattended host that is gone …v0.0.0-dev.105\reasonix-desktop.exe`；
  杀于 23:59:02 → 23:59:46 由看门狗自己拉起（停摆 49s），任务那一刻 `LastRunTime=23:59:59 / Result=0`。
- **但黑板没有推进**：拉起后板上**没有任何新 op**（最后一条仍是 23:24 的 seq 6），而本会话在拉起后调 `agent_bus` 得到
  `this session is not on a board` —— 因为**入列只活在控制器内存里**（`SetAgentBus`），进程一死就丢；
  于是"拉起后没人再入列 ⇒ 黑板停在被杀那一刻"。
- **已修（本刀）**：入列按**会话路径**记进 `<state home>/agentbus-enrolments.json`（`rememberAgentBusEnrolment`），
  宿主重建控制器时（`buildTabControllerBoot` → `restoreAgentBusEnrolment`）先把该会话放回它原来的板，再装唤醒路由并扫一次；
  `AgentBusLeave` 会清掉这条记录（离开是决定，不该被重新施加上）。用例
  `desktop/agentbus_enrolment_test.go` 2 条（重启后自动回到板上、离开后不再回来、无路径会话不记账）。
- **仍待**：装 dev.106 后在真机复验"杀 → 看门狗拉起 → **黑板出现新 op / 被唤醒的会话真的起回合**"。
- 一条岔路的更正：我曾把"拉起后不推进"归到本机**定时任务**（`heartbeat-tasks.json` 里 4 个任务都是 `enabled:false`）——
  那是另一个功能域，与 T9-4 的黑板推进无关；该方向已放弃、改动已回退（工作区干净）。
- **挂钩点更正（本刀，dev.107）**：dev.106 的恢复没生效，根因是**挂钩时机**——`desktop/tabs.go` 建控制器时
  `boot.Options` 只传 `SessionDir`，控制器级 `ctrl.SessionPath()` 要等会话绑定才写入，所以在 build 时读到**空串**、
  拿空键查记录必然落空（记录与键本身都对：与 `desktop-tabs.json` 里的路径逐字符一致）。
  改为在**会话文件真正绑定的那一刻**恢复：`tabs.go` 的 `a.persistTabSessionPath(tab, path)` 之前调
  `a.restoreAgentBusEnrolmentFor(path, ctrl)`；`buildTabControllerBoot` 里那次保留为兜底（路径为空时无害）。
  另修一处必然的次生问题：`enrollAgentBus` 在**调用时**就把 `boardDir` 捕获进唤醒器，build 时入列为空 ⇒ 唤醒器拿着空板；
  恢复路径在 `SetAgentBus` 之后**重挂唤醒器**（与点击 join 同路径）。
  用例：`TestRestoreAgentBusEnrolmentUsesTheTabPathNotTheControllerPath`（无路径的控制器不得入列；给出标签路径后入列；重复恢复不扰动）。
- **接续推进的缺环（本刀，dev.108）**：用户给出的可用解法 = **启动后往会话里自动发一条「继续」**（手输就能接上，
  说明缺的不是机制而是"没人发起这一回合"）。落地：`desktop/unattended_resume.go` —— 上一次宿主运行是**非正常结束**
  （`exitKindClean` 之外）且无人值守总开关为开、且该会话**有 Goal 契约**时，在标签发布完成处
  （`finishStartupPublication` 之后、无锁）用与唤醒同一条 durable 路径 `TryEnqueueFollowup(control.InboxRequest{...})`
  排入一条 `继续`，**每宿主运行每会话仅一次**（幂等键含 runID+会话路径）。
  开关判断沿用 `heartbeat.unattendedEnabled()`（含崩溃连击降级），与"总开关是唯一闸门"的语义一致；无 Goal 的会话不注入。
  用例：`desktop/unattended_resume_test.go`（判定表 5 例 + 真机语义那 4 条断言）。
- **还差的最后一步（本刀，dev.109）**：真机截图显示排入的「继续」**停在"待处理引导"里等确认**（横幅：已恢复 N 条待处理指令 /
  收件箱已暂停 / 按钮「继续执行」「保持暂停」）⇒ 崩溃恢复**故意把收件箱暂停**是安全门，但无人值守时没人去点它。
  落地：`desktop/unattended_resume.go` 的 `resumeUnattendedGates` —— 排入「继续」之后，对该会话
  **解除收件箱暂停 + 关 plan mode**（与 `heartbeat_converge.go:clearUnattendedGates` 同一语义、同一守卫接口
  `heartbeatSessionGuards`，断言不到接口就静默跳过）。语义依据："总开关是唯一闸门，人干预不改变无人状态"。
  用例：`TestResumeUnattendedSessionClearsTheGatesACrashLeft`（先暂停收件箱 → 恢复后必须不再暂停）。

## T9-4 结论（2026-10-03 凌晨，真机）

**"杀宿主 → 拉起 → 继续推进"两半都已单独验证通过**（每一半都有可复核的证据）：

1. **看门狗自己拉起**：巡检日志 `2026-10-02T16:39:50Z launched … v0.0.0-dev.107`、`16:53:40Z launched … v0.0.0-dev.108`；
   任务此时 `Result=0`。杀进程用的是精确 pid（非进程名通配）。
2. **拉起后继续推进**（本刀要证的东西）：宿主**自己**排入并投递了「继续」回合，两轮各一次：
   - `00:49:00 dev.108`：`msg="desktop: unattended session resumed after an interrupted host run"`
   - `01:08:11 起 dev.109 → 01:08:33` 同一条日志 + 收件箱条目
     `{"displayText":"继续","idempotencyKey":"unattended-resume|ff1be0d8099f1d75|<sessionPath>","source":"unattended-resume"}`
     ⇒ 该回合**无人输入、无人点击**就自己跑起来了（本条结论就是它发起的）。
3. 触发条件（均在真机核实，非假设）：印记 `lastExit.kind="killed"`（非 clean）＋ 无人值守总开关为开
   ＋ 标签已持久化 `goal`（`desktop-tabs.json`，本会话 11 字符）⇒ `resumeUnattendedSessionAfterAnInterruptedRun` 成立。

**仍未在同一轮里合起来跑过**：看门狗拉起与自动恢复各自跑过（分别在不同轮次），联合条件是同一份数据
（印记 lastExit + 开关 + 标签 goal），无额外机制；要一次性看到"看门狗拉起 → 自动继续"，需要看门狗那次 tick 拉起的宿主
恰好是含本刀的构建（下次自然发生即可确认）。

**新发现（未定性，超出本刀范围，留待查）**：00:58:35 杀 dev.108 之后，巡检连续两次
`skip  the marker does not ask for unattended`（16:58:42Z / 17:03:42Z）⇒ 那一刻**印记里的 `unattended` 变成了 false**，
于是看门狗拒绝拉起；而配置开关（`heartbeat-tasks.json` 的 `unattended`）始终为 true，01:08:11 起来的 dev.109 印记又是 true。
即：**某些退出路径会把印记的 unattended 写成 false**，这会让看门狗在该场景下"该拉而不拉"。
怀疑写入者＝退出观测器/电子壳的 hostState 写回（`desktop/electron/src/main/hostState.ts`、`spawnDesktopExitObserver`），
本刀未取证，不做结论。

**印记 unattended 变 false 的排查进展（2026-10-03，未定性但有界）**
- 现象：00:58:35 杀 dev.108 后，巡检 16:58:42Z / 17:03:42Z 连续 `skip the marker does not ask for unattended`；
  该文案来自 `desktop/watchdog.go:40` ⇒ 印记**读到了**但 `Unattended=false`（不是"没有印记"，
  那条是 `!Seen` 的 "no marker: the last run exited cleanly or never started"）。
- 已排除（都有证据）：
  1. 电子壳不写也不删印记 —— `desktop/electron/src/main/hostState.ts` 只有 `readHostState`/`unattendedDesired`（只读），
     electron 侧无任何写 `HOST_STATE_FILE` 的语句；
  2. 决策者是独立进程（`desktop/main.go:67 maybeRunDesktopWatchdog`），它只写巡检日志，不写印记；
  3. Go 侧印记只有一个写入点 —— `desktop/host_state_marker.go:214` 的 `note("running", unattended)`（启动时一次），
     `writeHostState` 无其他调用者；
  4. 无人值守开关在那段时间**是开的** —— 日志 `23:26:10 followed the unattended switch unattended=true`，
     之后（含 00:53:40 那次 dev.108 启动）无翻转记录 ⇒ 启动时写下的应是 true。
- 结论：**在"启动写 true"与"下一次巡检读 false"之间，有某条路径改写了印记**，写入者未定位；
  下一步最小动作＝在每次巡检读印记时把 `unattended` 值一并打进巡检日志（把下次复现钉死在一次 tick 内），
  再按证据改。影响面：该窗口内看门狗会"该拉而不拉"，与本次已验证的自动恢复链相互独立。

### T9-4 之后"时好时坏、最终彻底不推进"的根因与修复（dev.111，2026-10-03）

**根因链（每一环都有物证）**
1. 宿主**被杀**会让该会话 Goal 状态不再是 `running`：`internal/control/cancel.go:50 stopGoal(GoalStatusStopped)`；
   本会话 `….goal-state.json` 实测 `{"status":"stopped",…}`，mtime 正是那次启动（01:17:45）。
2. 下一次保存把**契约擦掉**：`desktop/tabs.go:persistedTabGoal` 只要状态非 `running` 就返回空串，
   写回 `desktop-tabs.json` 即 `goal=""`。物证：本会话该条目的 `goal` 由 11 字符变为不存在。
3. 重启后无契约可用：`runningTabSessionGoal(path, "")` 在 fallback 为空时直接返回 ""（`desktop/tabs.go:7702`）
   ⇒ `tab.goal==""` ⇒ 恢复钩子第三条件不成立 ⇒ 不排「继续」。
4. 于是表现是"时好时坏、最后彻底不推进"：`00:49:00`、`01:08:33` 两次成立是因为那一刻状态恰为 `running`；
   契约被擦掉后（`01:17:38` 那次启动，日志里本会话控制器确实建起来了但钩子一行未打）**永不再成立**。

**修复（本刀）**
- `persistedTabGoal`：持久化 **Goal 文本本身**，停止状态**不再擦契约**；"不要这个 goal"仍由 `clearTabGoal`（清会话）负责。
  依据用户既定语义：总开关是唯一闸门，人干预不改变无人状态。
- 恢复钩子：契约来源改为 `tab.goal` 优先、否则读 `desktop-tabs.json` 该会话条目的 `goal`
  （`unattendedGoalContract`）⇒ `stopped` 状态**不能**否决恢复；并在**拒绝时打印判定输入**
  （`unattended resume declined` + interrupted/contractLen），补上此前"拒绝无日志"的缺口。
- 用例：`TestPersistedTabGoalKeepsAStoppedContract`、`TestUnattendedGoalContractFallsBackToThePersistedTabFile`
  （后者用 `REASONIX_HOME` 隔离——`desktopConfigDir()` 走 home 解析器，不是 state home）。
- 待办：装 dev.111 后复验"杀 → 看门狗拉起 → 「继续」到达"，这次状态为 `stopped` 也必须成立。

### 恢复回合的"内容"问题（dev.112，2026-10-03，用户提出）

用户指出：**用裸「继续」当推进条件不合适**——没有上下文，等于让模型乱指挥。采纳并改为一封**带上下文的接续简报**
（`desktop/unattended_resume.go:unattendedResumeText`），排入的回合文本由宿主拼装：

- 首句声明这是**接续**（上一次宿主运行被中断），不是新指令；
- `任务契约（Goal）`：来自持久化契约（`unattendedGoalContract`，dev.111 起不再被擦）；
- `中断前在处理的指令`：取会话历史里**最后一条真人输入**——用 `provider.MessageOriginHost` 把宿主自己生成的
  user-role 消息（上一次恢复、看板唤醒）排除掉，避免自我循环；
- `队列里待处理的工作`：收件箱快照里除本类条目外最多 2 条 `Preview`；
- 末尾明确要求：先核对已完成步骤（待办/看板/工作区现状）再从中断点继续，**不要重做已完成的工作、不要偏离契约另起炉灶**，契约已完成就直接给结论。

用例：`TestUnattendedResumeTextNamesTheContextAndForbidsImprovising`、
`TestResumeUnattendedSessionQueuesTheContextualBrief`（断言入列条目的 `Preview` 里确实带着契约与"被中断"）。

### 契约必须活在 App 不会重写的地方（dev.113，2026-10-03，真机定位）

**上次修复不够的证据**：dev.112 复验日志里钩子自己说了 —— `unattended resume declined interrupted=true contractLen=0`
（诊断日志是我为自己加的）。而探针**确实**把契约写回了 `desktop-tabs.json`（写回条目数=1），却在 App 启动后被**再次擦掉**：
保存走的是 `currentTabGoal(tab)`＝**活控制器的 `Goal()`**，停止状态的控制器报空 ⇒ dev.111 只堵了 `tab.goal` 那条支路，
这条支路照样把契约写成空（复核：该条目 `goal` 又变回不存在）。

**修复**：新增 `desktop/unattended_contract_store.go` —— `<state home>/unattended-contracts.json`，按会话路径**粘存契约**：
- 写入时机：① 载入标签时（`app.go` 的 `rememberUnattendedGoalContract(entry.SessionPath, entry.Goal)`，契约还可见的最后一刻）；
  ② 解析契约命中时（`unattendedGoalContract` 内 seen ⇒ remember）。
- 解析顺序改为：`tab.goal` → `desktop-tabs.json` 该条目 → **契约粘存**（唯一 App 不会重写的地方）。
- 用例：`TestUnattendedGoalContractSurvivesTheTabFileErasingIt`、`TestUnattendedGoalContractResolutionRecordsWhatItSees`。
- 语义：契约只有**明确丢弃**才会消失（清会话），与"总开关是唯一闸门、人干预不改变无人状态"一致。

### T9-4「拉起后自动接续」端到端通过 + 内容修正（dev.113/114，2026-10-03 凌晨）

**终验证据（三处相互独立）**
- 探针：`01:35:23` 杀宿主（procs=0）→ `01:35:24` 调巡检
- 看门狗：`2026-10-02T17:35:24Z launched restoring an unattended host that is gone … v0.0.0-dev.113`
- 宿主自己发的钩子行：`01:35:34.294 desktop: unattended session resumed after an interrupted host run … chars=668`
  ⇒ 排入的是**668 字符的接续简报**（不是两个字）；同轮另两条无 Goal 会话正确被拒（`unattended resume declined … contractLen=0`）。
- 本会话随后**在无任何人工输入**的情况下自己收到了那封简报并开始这一轮 —— 即"杀 → 拉起 → 自动接续推进"闭环成立。

**这一条链最后收敛到 9 刀**（每一刀都由真机证据推动，非猜测）：
dev.106 入列持久 → dev.107 恢复挂在会话绑定处 + 重挂唤醒器 → dev.108 拉起后自动排恢复回合 →
dev.109 自己解除崩溃留下的闸门（收件箱暂停/plan mode）→ dev.110 巡检诊断（印记三要素）→
dev.111 停止状态不再擦契约 → dev.112 恢复回合改为带上下文简报 → dev.113 契约粘存（App 重写 tabs 也擦不掉）→
dev.114 简报过滤宿主自己的中断包装文本（`<interrupted-turn-recovery>` 不是指令）。

**诚实备注（复验前提）**：本次复验前，我**手工把契约种入粘存**一次 —— 因为更早的构建已经把该会话的契约从
`desktop-tabs.json` 里擦掉了（那时还没有 dev.113）。产品路径的捕获是自动的（载入标签时 + 解析命中时），
所以**从 dev.113 起**用户设定的任何 Goal 都会被记住；对"契约已被旧版擦掉"的会话，需要重新设定一次 Goal 才会恢复。
