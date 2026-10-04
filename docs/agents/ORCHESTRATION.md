# 编排规范：把任务写成节点图（G4 的缺件）

> 用途：**编排者没有自动 planner** —— 节点图是**写出来**的。本文件就是那份"可复用的写法"：
> 节点粒度、子树划分、边界节点、交付物根，以及**编排者被删之后**幸存者按同一套规则把图继续长出来。
> 出口条件与判据仍以 `AGENT_BUS.md` 为准（本文件不复读契约）；执行清单在 `TODO.md`（T12-4）。
> 每条规则都给出**内核实现位置 + 用例**，便于核对——本规范只写**内核真做得到**的事。

## 0. 一句话判据（先记住这个，其他都是它的推论）

**杀掉编排者，工作还在往前走 ⇒ 集群；杀掉就停 ⇒ 作业派发器**（`AGENT_BUS.md` §产品判据）。
⇒ 所以**真相必须在板上**（节点/依赖/证据/判决），编排者的会话只是"当前那个在写板的人"。

## 1. 交付物根：任务级落地判定认的就是它

- 判定 `AssessLanding(state, hearings)` 把**依赖图的根**（没有任何其它节点依赖它）当作交付物
  （`internal/agentbus/landing.go` 的 `rootsOf`；§13.10 取方案 A，**不新增 kind/marker**）。
- ⇒ **写图第一步：先把交付物立出来**，再让每一块工作 `require` 到它：
  `require(node=<交付物>, dep=<一块工作>)` 表示"交付物等这块工作"（`require` 的语义是 **node 等 dep**，
  `internal/agentbus/board/node.go` 的 `VerbRequire`）。**方向写反**就会把"最早的那一步"变成交付物，
  于是任务还没做完就被判落地。
- 一个任务可以有**多个根**（多交付物）：每根都要 `done` 才算落地；只想要一个验收标准时，
  就让其余根都 `require` 到你真正认的那一个。
- **落地的四条阻塞**（同一节点只报最可操作的一条）：在争 `contested` > 审议悬念 `deliberating/escalated/undecided`
  > 依赖缺失 `missing` > 被放弃 `abandoned` > 未完成 `not_done`（`landing.go`）。⇒ 图里别留"没人收口的争议"。

## 2. 节点粒度：一次"能被别人复跑"的交付

- `decide(done)` 的门槛是**可核对证据 + 非产出者复跑**（`internal/agentbus/board/node.go` 的 `OutcomeDone`：
  节点上必须有一条带证据的 `assert`；`ReproducedBy` 不能是产出者）⇒ 粒度就是：
  **这一行有一个"别人能跑的命令 / 能打开的文件 / 能重放的 URL"**。
- 太粗（"做完整套集成测试"）⇒ 没人写得出一条能复跑的证据 ⇒ 节点永远停在 `blocked`；
  太细（"打开编辑器"）⇒ 派活/唤醒按步计数，会把预算与注意力烧在零信息量的步上。
- 写节点时顺手写下那条证据（`assert` 的 `reason` + `evidence[].ref`），别等收口时再想。

## 3. 子树划分与边界节点

- **一棵子树 = 一个可独立收敛的交付物**。子树归属由依赖向上走派生（`agentbus.SubtreeRoot`），
  于是**预算按子树记**（`[agentbus] budget_subtree`）与**调度按子树亲和**（`rank.go`）都跟着它走（§13.7 / T7-3）。
  同一个交付物被拆在两个子树里 ⇒ 预算记到两处、亲和失效。
- **跨子树只经边界节点**（§13.1 / T4-3）：要引用别人子树里的东西，就 `require` 出一个**显式节点**当作接口
  （它同时是这块工作的"交付物"），**不要**直接依赖别的子树内部的步。理由有两条：视图是裁剪过的
  （≤200 行且 ≤8 KiB，超限只取前 K，见 §13.1），以及内部步可以被它自己的编排者 `split`/`revert` 掉。
- **建节点要带 Title**：`require`/`split` 用 `NodeSpec{ID,Title}`，**`assert` 用自己的 `title` 参数**（2026-10-03 起内核 op
  也带可选 `Title` ⇒ 面板不再有"只有 id"的节点）。规范：**结构用 `require`/`split` 建；落断言的 `assert` 顺手给自己建的根节点命名**。

## 4. 编排者的回合循环（写一次，反复用）

1. `agent_bus{action=view}` —— 先看板（只显示"与你有关"的那些：你的、等你的、你要的）。
2. **认领再动手**：`claim` 需要 `steps`（边界）与 `leaseSeconds`（默认 900s）；**工期长就 `heartbeat` 续租**
   —— 真机上"派活 30 分钟租约、会话不心跳"的形态是 27 分钟零 op（§11.5 第 4 条）。
3. **长图**：缺哪块就 `require`（带 Title），要拆就 `split`（children 带 Title）。
   ⚠️ `split` 出的新子节点**没有租约 ⇒ 30 秒内会被派活器发给别人**：**`split` 的下一条 op 就该是 `claim`**
   （§11.5 第 5 条，真机踩过）。`require` 出来的节点同理——你自己要做的就马上 `claim`。
4. **传给别人**：`assign`（指名，只有他能取）/ `unassign`（交回 pool）。板的 `WakeTargets` 会把"可开工/指派给你/等你/问你/欠你"发给对应的人（§13.2）。
5. **争议**：`refute`（带理由）落在某步上 ⇒ **会话自己开审议**：`hearing_open`（`required` 留空 = 该节点所有者 ∪ 全部反证者）
   → 各方 `hearing_answer`（带能核对的证据）→ `hearing_settle`（判 `refuted` ⇒ 该步 `blocked`）。
   审议要有窗口才有人被唤醒（`[agentbus] hearing_round_ttl_minutes`，§13.12）。
6. **收口**：`decide{done}` 需要证据 + 非产出者复跑；`blocked` 要写明缺什么。任务级的 `complete` 由
   `AssessLanding` 把关（§13.10）：**只要有一个交付物没 done 或有未决争议，完整声明就会被拒**。

## 5. 编排者被删之后：幸存者按同一套规则接手

**内核已证的那一半**：`internal/agentbus/takeover_test.go`（T9-1）—— 编排者的进程/句柄/内存全没了，
另一个参与者**只读文件**就能认出、认领并做完那些工作。**它证的是"接手现成的工作"**。

**本规范负责的那一半（内核没有自动 planner）**：幸存者按上面 §1–§4 **继续把图长出来**：

1. `view` 读折叠态，按 §1 找出**交付物根**与它缺的依赖；
2. 缺的依赖如果**没有任何节点在等**（= 图上根本没有那一块）⇒ 那就是幸存者要补的：`require` 出它（带 Title）；
3. 补出来的步按 §4 立刻 `claim`；做不完就 `assign`/`release`，别让它空挂着；
4. 发现"方向不对"（依赖写反、子树混在一起、粒度无法核验）⇒ 用既有动词**改图**：`revert`（把 done 拉回、
   下游转 `stale`）、`abandon`（带证据，放弃一个块并让依赖它的人看见）、`split`（重切粒度）；
5. 收口仍按 §1：所有根 `done` + 无未决争议 ⇒ `AssessLanding` 判落地。

**演练（本规范的可复跑判据）**：`internal/agentbus/orchestration_drill_test.go` —— 编排者写下交付物 + 一块工作后**消失**，
幸存者只读文件，**补出图上没有的第二块**（`require` + `claim` + `decide`），最后交付物 `done`、`AssessLanding` 落地，
且新读者从同一批文件折出同一结局。

## 5.1 同机多会话分工：谁负责哪棵子树（v1 边界，G7）

**G7 要的是同一个宿主里的多会话** —— 桌面多 tab（同进程多 controller、各自 `WorkspaceRoot`）与 serve/CLI 的多会话；
**不是多台机器**。v1 是单机（`AGENT_BUS.md` §集群规模：**范围：v1 是单机**）：唤醒与调度都在**一个宿主**内完成，
`participants.jsonl` 的"地址＋令牌"只是**协议保留可换目标**，跨机不是 v1 的验收项。
分工仍必须写进图里，不能靠调度器猜——下面是可核对的事实与由它推出的写法。

| 事实 | 落点（可核对） |
|---|---|
| 槽位与账本是**本机 host 级**，不是板级 | `desktop/agentbus_waker.go` 的 `hostAgentBusBudget`（**每进程一次**的 `sync.Once`：账本、刹车、审议边界同一次读取）；`internal/agentbus/budget.go` 的 `AcquireSlot` 明写"本机 host 级、每参与者一格" |
| 预算与槽位按机器各记 | 同一个进程级账本；`[agentbus]` 各宿主各自读（`internal/boot/agentbus_wiring.go` 的 `enrolAgentBusController`） |
| 只有**能公告地址**的宿主才是唤醒目标 | `boot` 仅在 `AgentBusHost != ""` 时 `AgentBusAnnounce`；只有 CLI（`REASONIX_AGENTBUS_HOST`）与 serve 设它 ⇒ **桌面自身不发 serve，不能作远端寻址目标**（它只发不收） |
| 跨进程唤醒已真机验过 | T5-4 的 dev.104 双向：桌面按板级地址簿 + 公告令牌，把唤醒投给了**另一个进程**的会话 |
| 板是**共享文件**，同机各会话看的是同一份 | 地址簿 `participants.jsonl` 记 host 与令牌引用，过 TTL 视为不存在（僵尸宿主不再吞唤醒）；同机多会话**只需同一个 `<board>` 目录** —— 共享盘/同步目录是**跨机**才有的前提，不在 v1 |

**写法（由上面的事实推出）**：

1. 一个会话负责**一棵或几棵子树**；跨会话交接只能落在双方都看得见的**边界节点**上（§3 的规则跨会话时同样更硬：两边视图都可能被裁剪）。
2. 委派给另一个会话时写成 `assign` 到那个参与者的名字，并在边界节点上留 `require`；**不要**按"谁现在闲"临时决定谁取。
3. 槽位与账本是**宿主级**（`hostAgentBusBudget` 每进程一次）⇒ **同机多会话共享一池**：一个会话跑满 host 的槽，
   同宿主的其它会话一起排队 —— 这正是"本机 host 级"的本意；涨落留在宿主内，跨会话的方向感由人写进图里。
4. 谁都不动的步有两条既有出口：宿主 tick 的**停滞唤醒**（`AGENT_BUS.md` §13.13）与编排者按 §5 **继续长图**。

### 5.1.1 演练配方：把一个会话手上的工作交给**同机的**另一个会话（可复跑）

**G7 的形态就是同机多会话**（一个宿主：桌面多 tab，或 serve/CLI 的多会话），**不需要第二台机器**。
下面只列**已核实的接口**与**可核对断言**；哪些步已有真机/装配读数、哪些属"不在 v1"，见本节末。

**准备**：① 两侧会话入列**同一块板**（同机 ⇒ 同一个 `<board>` 目录即可，不需要共享盘）；② **桌面可直接作派活方**
（它只发不收 ⇒ 不作为寻址目标，但作为派活方够用；能被别的宿主机唤醒需要它自己发 serve，见上表第 3 行）；
③ 两侧参与者名不同（`SetAgentBus(dir, id)`）。

| 步 | 在哪 | 动作（照抄） | 断言（板侧只读，可复跑） |
|---|---|---|---|
| 1 | 会话 A | `run_skill agentbus-orchestration`；再 `agent_bus action=assert node=<交付物> title=<名字>` + `action=require node=<交付物> dep=<块> title=<块名>` | `agent_bus action=view` 里交付物在、且**它在等**那一块 |
| 2 | 会话 A | `agent_bus action=assign node=<块> assignee=<会话 B 的参与者>` | `view` 里该块带 `assignee=B`（他人 `claim` 会被拒 `not_assignee`） |
| 3 | 会话 B | 收到唤醒后 `agent_bus action=view` → **下一个 op 就** `action=claim node=<块> steps=<n>` | `view` 里该块 `claimed by B`；**若没人取**，租约过期计入重试、耗尽后被宿主**交回池子**（§13.15） |
| 4 | 会话 B | 干活；`agent_bus action=decide node=<块> outcome=done evidence=[…] reproducedBy=<非产出者>` | `view` 里该块 `done` 且带复跑者 |
| 5 | 会话 A | （无需动作，读即可） | 交付物根的 `Ready`/`Assigned` 组**随之外移** ⇒ "被唤醒的会话确实推进了节点"成立 |

**哪些步必须真机、哪些可单机近似（如实标注）**：
- **可单机近似：已跑（2026-10-04）** —— `internal/boot/agentbus_drill_offline_test.go` 的
  `TestEffectASessionHandedAnAssignedBlockAdvancesTheDeliverable` 在一块共享板上跑完第 1–5 步的**板侧断言**：
  编排者把 `block` **指派**给会话 `worker`（第 2 步）⇒ 断言 `WakeTargets` 把该块**按名**交给 worker（`Assigned`），
  且在块 `done` 之前**不**把交付物交给请求者 ⇒ 真装配的会话用**它自己的工具调用** `view`/`claim`/`decide` 走完第 3–4 步
  （判据：op 日志里 `claim` 与 `decide` 的 `actor == worker`、块 `done`）⇒ 第 5 步的"组随之外移"由**内核自己的派生**读出
  （交付物开始被交给它的请求者，已 `done` 的块不再被交出）。**可复跑**：`go test -count=1 -run 'HandedAnAssignedBlock' ./internal/boot/`。
- **已验（同机、真实进程，2026-10-02，T5-4 节）**：第 3 步里"**会话 A 把唤醒投给同机的会话 B**" —— 同一台机器上的两个真
  `serve` 宿主：alice 的 tick → bob 的 `POST /inbox/items` 202 → 唤醒在 bob 自家会话里成 `role=user` 回合。
  桌面多 tab 那一形态由**宿主内路由**承担（`desktop/agentbus_waker.go` 按 participant 找 tab，且不唤醒自己）。
- **已验（同一个宿主、两个桌面会话，2026-10-05 首次真机；G7 的目标形态）**：用户在 dev.130 上开了两个 tab 并加入默认板。
  会话 A（participant `20261004-060209.690079400-…`）写下 `assert g7-drill` → `require g7-drill dep=g7-block` →
  `assign g7-block assignee=20261004-064657.426824700-…`（板上 `seq 247/248/249`）。**会话 B 自己动了起来**：
  它的 session 文件里出现唤醒回合、其后是 B 自己的 `agent_bus action=view` 与读板动作（"the agentbus wake says
  there's work: g7-block addressed to me"），**全程没有人给它打字**。⇒ 同机跨会话那一跳成立。
  **同一次真机抓到并当场修掉一个读面缺口**：B 的 `view` 是空的（`owned=0 waiting=0 needed=0`）—— 视图三组都不认
  "被指派给我的节点"，而唤醒词告诉它"这块归你"；修法 = `participantNodes` 把 `assignee` 也算作"我的"
  （`internal/agentbus/view.go`，用例 `internal/agentbus/view_assignee_test.go`）。
  **仍未完成的那半（如实）**：B 在 dev.130 上（该 exe 不含上面的视图修复）没有认领那块，板停在 `seq 249`；
  待下一个包含该修复的包再走一遍第 3–5 步，判据 = 板上出现 `claim`/`decide` 且 `actor == B`。
- **不在 v1，因此不是 G7 的验收项**：**第二台机器**上的会话（那才需要共享盘/同步目录做跨机寻址与令牌；
  `AGENT_BUS.md` §集群规模 写明 v1 单机，协议只保留"地址＋令牌可换目标"）。
  **2026-10-05 更正**：本节此前把这一跳写成 G7 的"必须真机"项 —— 那是把**跨会话**误读成**跨机**；按用户口径，
  G7 ＝ **同一个宿主里的多会话**（桌面多 tab / serve 多会话），不需要第二台机器。
- **首次执行时请把每一步的 `view` 原文与时间戳留档**，作为 T9-4/G7 的验收证据（装配边界近似已有上面那条用例可复跑）。

## 6. 反例清单（这样读就算错）

- **真相在对话里**：结论只在板上（契约 §产品判据）。对话是一等能力，但它不承载真相。
- **一个节点等所有人**：把一棵子树的收口写成一个巨型节点 ⇒ 没人能给它证据；
  正确形状是"每个交付物一个根，根等它的块"。
- **依赖写反**（`require(块, 交付物)`）⇒ 交付物变成最早的那一步，任务会被判过早落地。
- **跨子树直接依赖内部步** ⇒ 视图裁剪看不见 + 对方 `split`/`revert` 会把你的依赖抽走；要接口就立边界节点。
- **用命名约定承载语义**（`Title` 前缀当"验收节点"）：§13.10 选项 C 已被否——核不上、易漂。
- **`abandon` 当"取消"用**：它要求证据，且**有意不向下游传播**（依赖被放弃 ≠ 下游已完成被推翻，T9-2）。
- **只领走现成的步**：那是作业派发器（"杀掉编排者还能往前走"只在有人能**继续写图**时成立）。

## 7. 与内核的对应（自查表）

| 规范条款 | 实现位置 | 用例 |
|---|---|---|
| 交付物 = 依赖图的根 | `internal/agentbus/landing.go` | `landing_test.go`（6 条） |
| `require` 语义 = node 等 dep，并写入 Requester | `internal/agentbus/board/node.go` | `requester_test.go`、`requester_e2e_test.go` |
| 子树归属由依赖向上派生 | `agentbus.SubtreeRoot` | `budget_test.go` 的 `TestSubtreeRootWalksDependenciesUp` |
| `decide(done)` 门槛（证据 + 非产出者复跑） | `internal/agentbus/board/node.go` | `transition_test.go`、`e2e_test.go` |
| 派活按可达性/亲和/最难排序 | `internal/agentbus/rank.go` | `rank_test.go`（6 条） |
| 唤醒面：可开工/指派/等你/问你/欠你/停滞 | `internal/agentbus/wake.go` | `wake_test.go`（含 `Stalled`，§13.13） |
| 审议三件与写入即唤醒 | `internal/control/agentbus_hearing.go` | `TestTheRefuteToVerdictChainRunsWithoutAnyoneClickingAnything` |
| 编排者被杀后工作继续 | 文件即真相 | `takeover_test.go`（T9-1） |
| 编排者被杀后**继续长图** | **本规范**（无自动 planner） | `orchestration_drill_test.go` |
