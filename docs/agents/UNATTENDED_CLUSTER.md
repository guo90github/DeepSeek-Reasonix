# 无人值守智能体集群攻坚 —— 能力现状、缺口清单与落地顺序

> 定位：本文件只回答一个问题 —— **无人值守时，一组会话（每会话 = 一个智能体）能不能协作攻坚某个任务**：
> 今天能到哪里（含证据）、还缺什么、按什么顺序补、每条怎么验收。
> **出口条件（什么叫"落地"）以 `docs/agents/AGENT_BUS.md` 为准，本文件不复读契约条文**；执行清单在 `docs/agents/TODO.md`。
> 证据基准：2026-10-03，dev-2 工作区（代码检索 + `TODO.md` 既有真机结论），本文件不新增实验。

## 0. 三条口径（先定口径，否则结论会跑偏）

1. **一个会话 = 一个智能体**：板上的身份就是会话 —— `SetAgentBus(dir, participant)`，participant 缺省回落到会话路径派生的
   branch id（`internal/control/agentbus.go`：`participantID` → `parentSessionID` → `agent.BranchID(c.SessionPath())`）。
   子智能体（`task` 派生的 worker）跑在所属会话的 controller 里，**不是**板上的参与者 ⇒ 想让集群多一个声音，只能多一个会话。
2. **唯一时钟是宿主**：桌面 heartbeat 的 30s tick（`desktop/heartbeat.go` 的 `tick()` → `app.agentBusWakeTick()`）或 headless 的
   30s tick（`internal/cli/agentbus_tick.go`）。宿主不在，板就停住。
3. **无人值守的唯一闸门是总开关**：人干预不改变无人状态（`docs/UNATTENDED.md` §1）。

## 1. 今天能到哪里（已具备，附证据等级）

| 环节 | 实现位置 | 证据等级 |
|---|---|---|
| 黑板（多写者 op 日志 + 确定性 fold + 逐节点租约） | `internal/agentbus/board` | 用例（杀写者 / 多进程并发） |
| 认领即开工、心跳续租、过期回收 + `no_progress` | `board.Sweep`、`internal/control/agentbus_wake.go` | 用例 |
| 杀掉编排者后工作仍被接管推进 | `internal/agentbus/takeover_test.go` | 用例（"多智能体 vs 作业派发器"的判据） |
| 排队 + 本机槽位 + 四级预算 + 调度四则 | `internal/agentbus/{queue,schedule,budget,rank}.go` | 用例 |
| 定向派活（`assign`/`unassign`，非承办者 `claim` 被板拒收 `not_assignee`） | `board`、`internal/control/agentbus_dispatch.go` | 用例 + 真机反例 |
| 写好即唤醒 + tick 兜底 + 注入时按当时的板重算 | `internal/control/agentbus_wake*.go` | 用例（含真机发现的 4.5 分钟陈旧） |
| 唤醒 → 空闲会话真起回合 | `control.TryEnqueueFollowup`（`!Running()` 即派发） | 用例 + 真机 |
| 跨进程投递（地址簿 + 令牌 + 寻址 header） | `desktop/agentbus_deliver.go`、`internal/serve/agentbus_waker.go` | **真机**（dev.104 双向：桌面 → serve `POST /inbox/items` 202，接收方会话文件出现唤醒词） |
| 崩溃后自动接续推进 | `desktop/unattended_resume.go`、契约粘存 | **真机**（dev.113/114：杀 → 看门狗拉起 → 668 字符接续简报 → 无人工输入自己继续） |
| 人读面（首屏异常/争议/停滞 + 节点详情下钻） | `internal/agentbus/{observe,detail}.go` + 面板 | 用例 + 已挂界面 |
| 规模 | `internal/agentbus/scale_e2e_test.go`（100 节点 / 20 参与者 / 掉 20%） | 内核用例（真机未验） |

一句话：**协议、内核、单机宿主三层是通的**。能不能"攻坚"，取决于 §2 的五个条件，以及 §3 的缺口。

## 2. 成立条件（缺一不可；不是缺陷，是要人先铺的路）

| # | 条件 | 为什么 | 今天怎么满足 |
|---|---|---|---|
| C1 | 有一个**编排者会话**且它持有 **Goal 契约** | 没有自动 planner：节点图是写出来的；契约是"中断后自己继续"的前提（无契约不注入接续简报） | 会话里设 Goal + 用 `agent_bus` 工具（16 动词）写节点图 |
| C2 | 每个 worker 是**独立会话且已入列** | 板上一个声音 = 一个会话；两个会话同名会被**拒绝**（不猜、不广播） | 桌面多 tab，或一台 serve 托管多会话；入列已按会话路径持久化（宿主重建时自动回到板上） |
| C3 | **宿主常驻** | 只有宿主 tick 会回收过期租约 + 唤醒等待者 | 桌面保持运行；要被跨机寻址的宿主必须跑 serve（见 G7） |
| C4 | **预算与槽位显式配置** | 零值 = 无上限（内核不替使用者发明天花板），无人值守下没有刹车 | `[agentbus] budget_board/subtree/node/turn`、`dispatch_slots`（账本按进程一份 ⇒ 改配置需重启） |
| C5 | **拆解粒度适配视图裁剪** | 板视图 ≤200 行且 ≤8 KiB，超限只取前 K（计数可见）；跨子树只经边界节点 | 编排者把任务拆成子树 + `require` 出边界节点；大任务分片 |

## 3. 缺口清单（G1–G8，按落地顺序）

### G1 审议与裁决链在生产里没有入口（P0）

- **现象**：`OpenAgentBusHearing` / `AnswerAgentBusHearing` / `SettleAgentBusHearing` / `AgentBusHearings` 只在
  `internal/control/agentbus_hearing.go` 有定义 —— 全仓**非测试代码零调用方**，且**未进前端契约接口**（`internal/control/port.go` 无 `Hearing` 声明）；
  `agent_bus` 工具的 16 个动词里没有审议动作；`desktop/frontend/src` 无 hearing 相关代码。
- **影响**：无人值守时"反证 → 审议 → 判决"这条链走不通。`refute` 能落板（节点转 `contested`，可 `decide(blocked)`），
  但**没有人能开一场审议或收口**；而唤醒面已经为它铺好了路（`WakeTarget.Owes` 会唤醒未答者）⇒ 路铺好了却无人走。
- **验收判据**：一次可复跑链 —— 某节点被 `refute` ⇒ **会话自己**（非人点按）开出审议 ⇒ 未答者被唤醒并作答 ⇒
  收口后节点结局按判决落地，且重放同一 op 日志得同一结局。
- **修法方向**：给 `agent_bus` 加审议动词（open/answer/settle），或让 `refute` 落板后由**写者**（与"写好即唤醒"同一条规则）自动开审议；
  两条路都必须复用既有 `HearingLimits`（轮次/冷却/必答者）以保持有界。
- **已落（入口那一半，2026-10-03，提交 `cff94e184`）**：① `agent_bus` 工具新增 `hearing_open` / `hearing_answer` / `hearing_settle`
  （`internal/tool/builtin/agentbus.go` 的 `BoardPort` 三方法，`internal/boot/agentbus_board_tool.go` 转调 control）；
  ② **审议写入即唤醒**：`Open`/`Answer`/`SettleAgentBusHearing` 成功写入后调 `WakeAgentBus`（与写板同一条规则、key 幂等）。
  可复跑链（`TestTheRefuteToVerdictChainRunsWithoutAnyoneClickingAnything`）：`refute` 落板 ⇒ **会话自己**开审议 ⇒
  未答者被唤醒且 `Owes` 含该节点 ⇒ 作答 ⇒ `settle` ⇒ 节点 `blocked` ⇒ 新读者重折得同一判决；
  工具面 5 条用例 + `internal/boot` 的 `TestEffectAgentBusBoardToolDrivesADeliberation`（模型的工具调用把审议推到判决）。
  **仍未做（本刀抓到的真闸门，同日下一刀已开）**：`SetAgentBusHearingLimits` 当时**零生产调用方**、全仓无
  `HearingLimits{...}` 字面量 ⇒ 生产里 `RoundTTL == 0`，而 `HearingSilent` 要求 `RoundTTL > 0` ⇒
  **"未答者被唤醒"这一步永远不会发生**（路修好了，台阶高度是 0）。**已开（同日，提交 `93b9add8e`）**：`[agentbus]` 四个 `hearing_*` 旋钮 +
  `control.AgentBusHearingLimits` 映射 + `internal/boot/agentbus_wiring.go` 的唯一入列点推送（桌面/serve/cli 一体）+
  缺省时每进程一次警告；**先红后绿**的证据是 `internal/boot/agentbus_wiring_test.go`（设窗口 ⇒ 未答者被唤醒且 `Owes` 含该节点；
  不设 ⇒ 0 人）。契约见 `AGENT_BUS.md` **§13.12**。G1 的三件（入口 / 唤醒 / 窗口）至此齐。

### G2 自愈拉起：一条未定性的印记缺陷 + 一处"假安心"（P0）

- **现象**：某些退出路径会把启动印记的 `unattended` 写成 `false`（**写入者未定位**）⇒ 看门狗在该场景"该拉而不拉"；
  `WatchdogStatusView` 只有 `registered`，**没有"上次运行时间"** ⇒ `registered=true` 可能是假的安心。
- **已落（读面那一半，2026-10-03，提交 `93b74eed9`）**：`lastRunAt` 已加进 `WatchdogStatusView` —— Windows 取调度器的
  `(Get-ScheduledTaskInfo).LastRunTime`（**1999-11-30 哨兵 = 从未运行**，故"从未运行"与"运行过"不再无法区分），
  其它平台取巡检日志最后一条**可解析**行（半行跳过、不猜）；`--watchdog-status` 在未运行时显式打 `last run: never`，
  于是"`registered=true` 而从未运行"这件事**看得见**了。落点 `desktop/watchdog_control.go`（`watchdogLastRunAt` /
  `watchdogRunTimeFromTaskInfo` / `watchdogLastLogTime`）+ `desktop/watchdog.go` 的状态文本；
  用例 `go test -count=1 -run TestWatchdog ./desktop/` 全绿，含 `TestWatchdogRunTimeFromTaskInfoIsHonestAboutNever`（6 例：
  本地时区、BOM、1999 哨兵、任务不存在、错误串）、`TestWatchdogLastRunFallsBackToTheInspectionLog`（坏尾行跳过）、
  `TestWatchdogStatusReportsTheRunTimeTheSchedulerGives`、`TestWatchdogStatusSaysNeverInsteadOfStayingSilent`。
  **读面的边界（如实记）**：UI 里**还没有**看门狗面板（`WatchdogStatus` 目前只有 bridge 声明与 mock），
  人读面 = CLI 的 `--watchdog-status` 文本 + 契约里的 DTO 字段（`desktopContract.generated.*` 已重生成）。
  **本条已收口（2026-10-03，提交 `e93686991`）——写入者定位到了，而且**纯读代码**就能定，不必等真机复现**：
  印记 `unattended` 的**唯一**写路径是 `app.go` 的 `noteHostLaunch(a.heartbeat.unattendedEnabled())`，
  而 `unattendedEnabled()` 给的是**有效驱动值** `cfg.Unattended && !hostCrashLoopDegraded()`；看门狗只在印记
  `unattended=true` 时才拉起（`watchdog.go`：`"the marker does not ask for unattended"`）⇒ **降级那次运行把印记写成 false，
  它一崩，看门狗就拒绝拉起**，而那正是唯一能让 streak 归零的运行 ⇒ 宿主永久停摆 = "该拉而不拉"。
  修法：`noteHostLaunch()` 去掉参数、自己读**操作者的开关**（读不到则**沿用上一次的答案**，同"读不到开关时 OS 条目原样不动"之例）；
  降级语义不变（`hostCrashLoopDegraded()` 读的是印记里的 streak，与 `Unattended` 字段无关）。用例
  `TestTheLaunchMarkerAsksForUnattendedFromTheSwitch`（开关 on/off/不可读三种答案，先断言"降级前提成立"）。
- **已不成立（避免重复劳动）**：① 计划任务触发器"从不触发"、② 脚本落点 `Desktop\$` 两条**都已修并真机验过**
  （落点改 `<state home>\watchdog\watchdog.cmd`；任务 `LastRunTime` 会自然推进、`Result=0`；dev.105/113/114 末端到端通过）。
- **影响**：只有"宿主被杀且印记被判为不需要无人值守"这一个窗口会漏拉；**黑板推进本身不依赖看门狗**。
- **验收判据**：每次巡检把读到的印记三要素打进日志 ⇒ 复现窗口被钉死在一次 tick 内；修完后 `WatchdogStatus` 能显示上次运行时间，
  且"杀 → **不手动重启** → 等窗口 → 拉起 → 接续"再走一次全绿。
- **修法方向**：先补诊断（印记三要素入巡检日志），按证据再定位写入者；`WatchdogStatusView` 加 `lastRunAt`（✓ 已落）。

### G3 成本闸门默认全开（P0）

- **现象**：`BudgetLimits` 零值 = 该层无上限（`internal/agentbus/budget.go` 明写"内核不替使用者发明天花板"），
  额度与槽位只在 `[agentbus]` 配置里给，默认全 0。
- **影响**：无人值守"跑一夜"没有刹车；T9-6 的"不超预算"只覆盖"日志有界"，不覆盖真实 provider 花费。
- **验收判据**：预设额度后，超额的攻坚**被拒在 claim 前**（拒绝命名到 `budget_board/subtree/node/turn` 具体一层）、
  槽满时后来者**排队而非失败**、且人能在面板/日志看到"哪一层触顶"。
- **修法方向**：把"无人值守必须有预算"做成交互默认（开总开关时提示或落一份默认额度），不要只靠文档提醒。
- **已落（「交互默认」那一半，2026-10-03，提交 `c3f0f08f7`）**：走的是"**开总开关时提示**"这一支，没有落默认额度。开总开关时若
  `[agentbus]` 四个层级一个都没设：① 宿主 **自己说出来**（`HeartbeatConfigView.AgentBusBudget` + 一条点名旋钮的日志）；
  ② 无人值守开关的标签变成「已开启（无预算）」并给出设置位置（三语）。判据：`agentBusBudgetBrake` 只认花费层级
  （只有 `dispatch_slots` 不算刹车 —— 它限并发，不限花多少）；`Brake` 与执行额度的账本**同一次读取**（同一个 `sync.Once`），
  所以视图不可能报出账本没有的刹车。证据：`desktop/agentbus_waker.go`、`desktop/heartbeat.go`、
  `heartbeat.presentation.ts::unattendedPresentation`、`UnattendedToggle.tsx`；用例
  `go test -count=1 -run 'TestAgentBusBudget|TestTheConfigViewReports' ./desktop/`、
  `npx tsx src/__tests__/unattended-budget.test.ts`（9 条）、`npx tsc --noEmit`（顺带证三语 key 齐备）。
  **为什么不做默认额度**：无人值守**正在跑**的会话被宿主自造的额度拦下，等于"没跟人商量的暂停"；而内核既定语义是
  **天花板归使用者**（`internal/agentbus/budget.go`）⇒ 自造数字是宿主越权。要刹车就由使用者在 `reasonix.toml` 的
  `[agentbus]` 里给（账本按进程一份，改动需重启）。**留白（未做）**：①「哪一层触顶」的**聚合卡片**（逐次记录已落，见下）；
  ②槽满排队的宿主级用例（**已落**，见下）。
- **「哪一层触顶」已落（2026-10-03，提交 `9299d62c3`）**：`internal/control/agentbus_budget.go` 的 `recordBudgetRefusal` —— claim 被额度拒时，
  宿主自己打一条结构化记录：`level` / `reason`（`budget_node` 等）/ `key`（具体那一桶）/ `limit` / `board` / `node` /
  `refusals`（逐进程计数，一眼看出是一次还是常态），node 层再附 `remaining`（拒绝不花钱 ⇒ 剩余 = 上限）。
  用例 `TestABudgetRefusalIsRecordedWithItsCeiling`（capture handler 断言人读日志里能说出是哪一层）。
- **「哪一层触顶」上人读面（2026-10-03，提交 `e3e4b68aa`）**：① 判据里"人能在面板/日志看到哪一层触顶"那一半补齐 —— 拒绝计数**按层**记
  （`BudgetRefusalCounts` / `AgentBusBudgetRefusals()`），宿主折成**一条 `kind=budget` 信号**进协作面板（detail 形如
  `a ceiling refused 3 claim(s): node 2, turn 1`）；前端原先**只按卡片子树过滤信号** ⇒ 这一行（主机级、无子树）渲染不出来，
  故加了只渲染无子树信号的页脚列表 + `budget` 三语 label（**契约形状未变 ⇒ 无需重生成**）。用例见 TODO T12-3。
- **槽位两件已落（2026-10-03，`c48f7432c` + `f810518f5`）**：②「槽满排队而非失败」的**宿主级**用例
  （`internal/control/agentbus_dispatch_slots_test.go`：slots=1 时第二次派发返回 0 且无错、被留下那步场景不变，
  外加去掉上限的**正面对照**证明那个 0 是上限所致）；以及**槽位回收**——全仓原本**没有生产代码调用 `ReleaseSlot`**，
  而宿主账本经 `SetAgentBusLedger` 共享一份 ⇒ `dispatch_slots` 一满就**永久停摆**（内核那句 "a refused acquire means
  wait, never fail" 会落空：那次"等待"永不结束）。现按内核语义"谁在干活谁占位"：宿主在派发前收回"板上已无 `claimed`
  节点"的持有者的槽（`releaseIdleSlots`，仍持有 claim 的不动），用例先红后绿。**未做**：①的面板聚合；
  `Ledger` 无锁 ⇒ 多协程并发派发存在瞬时超额窗口（当前宿主单循环派发）。

### G4 编排者角色无接替，且缺"任务 → 节点图"的规范（P1）

- **现象**：被删的编排者，其**工作**能被接管（`takeover_test.go` 已证），但**"谁来继续拆解/新增节点"没有机制**；
  `assert` 建的节点不带 Title（面板上只见 id，已知边界），而任务级落地判定 = **依赖图的根**
  （`internal/agentbus/landing.go` 的 `AssessLanding`，§13.10 取 A）。
- **影响**：集群"能往前走"，但方向感会丢；没人规定"根/验收节点怎么写"时，`AssessLanding` 的语义会空转（要么过宽要么过严）。
- **验收判据**：一份可复用的编排规范（节点粒度、子树划分、边界节点、交付物根的写法）+ 一次演练中"编排者被删后，
  幸存者按规范继续把节点图长出来"（不是只领走现成的步）。
- **修法方向**：规范落在 `docs/agents/`，或做成编排者会话的 Goal 模板；给 `assert` 的 op 增可选 `Title` 属内核 schema 变更，单独一刀。
- **已落（规范 + 入口 + 演练，2026-10-03，提交 `9f47ea955`）**：① 规范 → **`docs/agents/ORCHESTRATION.md`**（节点粒度 = 一次"别人能复跑"的交付 /
  子树划分 = 一个交付物一棵 / 跨子树只经边界节点 / 交付物根与 `require` 的方向 / 编排者被删后按同一套规则继续长图），
  每条规则都带内核实现位置与用例；② 会话入口 → 内置技能 **`agentbus-orchestration`**
  （`internal/skill/builtincontent/agentbus-orchestration/SKILL.md`，`runAs: inline`：任何会话可 `run_skill` 取到打法清单，
  正文不进系统提示、golden 未变）；③ 演练 → **`internal/agentbus/orchestration_drill_test.go`**：编排者写下交付物 + 一块工作后消失，
  幸存者只读文件、**补出图上没有的第二块**（`require` + `claim` + `decide`），交付物 `done`、`AssessLanding` 落地、新读者折出同一结局。
  契约指针见 `AGENT_BUS.md` **§13.14**。**仍未做**：给 `assert` 的 op 增可选 `Title`（S1 结构变更，单独一刀）——
  规范用"结构用 `require`/`split` 建"**绕开**它，属写法而非修复。

### G5 停滞/孤儿没有无人值守的自动出口（P1）

- **现象**：自动派活有重试上限（`agentBusDispatchTries = 2`，`internal/control/agentbus_dispatch.go`），耗尽后**只留人读信号**
  （`observe` 的 `stalled`/`orphan`）；指派迟迟无人取也只在面板显示（`ObserveLimits.AssignedWait` 默认 10 分钟）。
- **影响**：无人值守时"没人看的信号 = 没有信号"，卡住的步会一直卡着。
- **验收判据**：一条无人值守用例/真机链 —— 某步重试耗尽后，宿主自动把它交回 pool（或唤醒编排者），并留痕"为什么"，
  而不是等人在面板上看见。
- **修法方向**：给"重试耗尽""指派超时"各配一个自动动作（回落 pool / 升级为"需要人"的显式节点），动作走既有 op 词表。
- **已落（"唤醒编排者"那一支，2026-10-03，提交 `2e7807546`）**：唤醒多一个分组 `WakeTarget.Stalled`，判据由宿主注入（`WakeInput.StallAfter`
  = 派活器自己的 `agentBusDispatchTries`；0 = 没这条预算）：`NoProgress >= StallAfter`、**无主**、未收口 ⇒ 唤醒它的 **`Requesters`**
  （就是"编排者"），文案带原因与出路（"handed out N times with no progress: take one, replan it, or say why it cannot move"）；
  触发者是**宿主 tick**（`AgentBusTick`），无人点按。命中停滞后该节点不再进 `Ready`/`Assigned`（同一节点一次只报一种事实），
  于是它**换了一个 key** ⇒ 从前"key 不变、于是再也不唤醒"的死角消失。契约见 `AGENT_BUS.md` **§13.13**；
  证据：`TestWakeTargetsNameStalledWorkInsteadOfOfferingItAgain`（真折叠两次回收 ⇒ `NoProgress=2` ⇒ 请求者收到 `Stalled`；
  不设阈值 ⇒ 照旧 `Ready`）、`TestWakeTargetsStayQuietAboutAStalledStepThatFinished`、
  `internal/control` 的 `TestAStalledStepIsWokenBackToWhoeverAskedForIt`（端到端 + 文案 + 第二次 tick 不重复）。
- **另一半已落（2026-10-03，提交 `b36e8f3d1`）**：**指派超时回落 pool**。板子本就支持"空 `Assignee` = 交回池子"
  （`board.applyAssign` 的注释明写），缺的是**触发者** —— 受派人不来取时，`takeableFor` 会把活锁死在那一个人身上
  （非承办者 `claim` 还会被板子拒 `not_assignee`）。现由宿主在派发路径上、取快照后 `parkStartableWork` 前调用
  `releaseStalledAssignments(ctx, actor, st)`：节点花光**既有**重试预算（`NoProgress >= agentBusDispatchTries`，
  与 `dispatchable()` 同一判据）时写一条空 `Assignee` 的 `assign` op，**不新造任何时间刻度**；
  自动派发仍受重试预算约束（停滞节点不会被重新发出去），改变的是**谁能取**。用例
  `TestAStalledAssignmentGoesBackToThePool`（**先红后绿**：修前 `assignee = "alice"`，且第三者 `claim` 被 `not_assignee` 拒）。

### G6 宿主层限流与 429 观测缺失（P1）

- **现象**：内核有话题面限速（`agentbus.RefuseRate`），但 provider 429 / 网络抖动属宿主层，内核测不覆盖（T9-6 的诚实边界）；
  预算不设时，"不超预算/无风暴"在真机上无从判断。
- **验收判据**：一次多会话并发攻坚里，429 与退避**可观测**（人读面有计数），且不因重试造成自我放大。
- **修法方向**：先在宿主侧记录 provider 限流/退避指标，再谈自适应节流。
- **已落（"429 可观测 + 可计数"这一半，2026-10-03，提交 `f85600ad7`）**：① 重试事件带**原因**（`event.RetryReason`：`rate_limited` / `server_error` /
  `timeout` / `network`，由 `internal/agent/retry_reason.go` 从重试的错误或 recovery 的 `Phase/Status` 判定，**判不出就留空、不猜**），
  随 `eventwire` 的 `retryReason` 进桌面契约；② **宿主侧计数**：`provider.RateLimitRetries()`（进程级，每吸收一次 429 加一，
  **最后那次不算吸收**），并在每次限流退避时打一条自解释日志（`agent: provider rate limited this host, backing off` +
  `attempt/max/delay_ms/rate_limited_total`）——这就是"人读面有计数"。**"不自我放大"早就有界**：`MaxRetries = 10`、
  `maxBackoff = 15s`、`Retry-After` 上限 60s、托管恢复**不重试**（`internal/provider/retry.go`，`TestBackoffDelay` 守着上限）；
  **本刀不发明任何节流策略**。证据：`TestRetryReasonNamesTheRetriesAHostMustTellApart`（10 例，含"判不出留空"）、
  `TestAgentNamesARateLimitedRetryAndCountsIt`（真 429 ⇒ 事件带 `rate_limited`、进程计数 +1）、
  `TestAnAbsorbedRateLimitIsCountedAndTheLastAttemptIsNot`（两次骑过 +2；托管恢复不重试不计数）。
  **仍未做（原"观测面板"已落，见下）**：自适应节流（本刀明确不做）。
  **归因已落（2026-10-03，提交 `194bf362c`）**：日志那一行现在也回答"**在哪条通道上**"——`noteRetry` 收 `provider.RetryInfo`，
  附 `provider` / `protocol`，响应带了 trace 头时再附 `trace_id`（全部取自 `*provider.APIError` 已携带的字段；**取不到不写、不猜**）；
  header 相位与流式恢复两个调用点同步。用例 `TestTheRateLimitLogNamesTheLaneThatThrottled`。
  面板聚合仍**未做**（需要前端契约重生成，单独一刀）。
  **面板聚合已落（2026-10-03，提交 `0355079a1`）**：`provider.RateLimitRetriesByProvider()`（按 provider 实例的进程级 `sync.Map` 计数，
  总数语义不变、无名 429 只进总数）⇒ `desktop` 的 `rateLimitSignal` 折成一条 `kind=rate_limited` 信号，
  走协作面板已有的**无子树页脚**（上一刀 `e3e4b68aa` 加的）渲染，前端补 `rate_limited` 三语 label。
  **契约形状未变 ⇒ 重生成并不需要**（原先"需要重生成"的预判被证实是多余的：只加 kind 取值，DTO 未动）。
  **前端那一半已落（2026-10-03，提交 `e9b5f7deb`）**：`retryReason` 进状态行 —— `recoveryStatus.ts` 的 `retryReasonLabel` 命中已知原因时
  附在 `status.retrying` 之后（三语文案），`useController` 把事件里的 `retryReason` 带进 `retry` 状态；
  **判不出就不追加**（用例 `src/__tests__/retry-reason-status.test.ts` 16 条）。

### G7 跨机：只有唤醒能跨机，调度仍单机（P2）

- **现象**：v1 明确不做跨机调度（`TODO.md` 的"不做"），槽位与账本都是 host 级；跨进程**投递**已真机验过（dev.104）。
  桌面自身不发 serve ⇒ 不能作为远端寻址目标（要有地址可公告：`AgentBusAnnounce(host, tokenFile)`，需 serve 或 agentd 托管）。
- **验收判据**：两台机器各跑宿主、各自入列，一台的写入能唤醒另一台的会话并推进节点（已部分验过）；调度与预算仍按机器各自计。
- **修法方向**：守住 v1 边界，把"哪台机器负责哪棵子树"写进编排规范（G4），不要靠调度器猜。
- **已落（边界写清，2026-10-03，提交 `9ec9aad12`）**：编排规范新增 **`docs/agents/ORCHESTRATION.md` §5.1「多机分工」**，事实表逐条给落点：
  槽位/账本是**本机 host 级**（`desktop/agentbus_waker.go` 的 `hostAgentBusBudget` 每进程一次 `sync.Once`；
  `internal/agentbus/budget.go` 的 `AcquireSlot` 明写 host 级）、**只有能公告地址的宿主**才是唤醒目标
  （`boot` 仅在 `AgentBusHost != ""` 时 `AgentBusAnnounce`；只有 CLI 的 `REASONIX_AGENTBUS_HOST` 与 serve 设它 ⇒
  **桌面不发 serve、不能作远端寻址目标**）、跨进程唤醒已真机验过（dev.104 双向）、板是共享文件（地址簿 + TTL）。
  由事实推出的写法：一机负责一棵/几棵子树、跨机交接只落**边界节点**、用 `assign` 指名而非按"哪台快"、各机槽位与预算各自计。
  **仍未做**：两台机器上跑一次"被唤醒的会话真的推进节点"的完整演练（dev.104 验的是**唤醒到达**那一半）。

### G8 读面与流程的残留（P2）

- `AgentBusTasks`（人读行投影）与 `agentbus.Authorized/AuthorizedGrants` 仍是**零生产调用方**（T4-5 / T9-7 留档）；
- `TODO.md` 的勾选状态滞后于后续结论（T9-4 的"黑板继续被推进"、T5-4 的 ⓐ headless 无 waker 已分别落地）；T11-6（PR 元数据）未做。
- **验收判据**：条目与实现状态一致化；读面要么接上、要么明确删掉（不留"看起来在用"的导出 API）。
- **已落（2026-10-03，提交 `9ec9aad12`）**：① **删掉** `AgentBusTasks` / `AgentBusTask`（`internal/control/agentbus.go`）—— 全仓**零生产调用方**、
  面板读的是 `Briefing`/`NodeDetail`、工具读的是**参与者视角**的视图；原先靠它断言的三处测试改用**真实存在的面**
  （作者自己的 `AgentBusView`、"未入列读不到板"、以及"两个写者一块板"用第三个读者折 `board.Snapshot`）⇒
  删的是 API 不是证据。② **上面那句的前提有错，已更正**：`agentbus.AuthorizedGrants` **有**生产调用方 ——
  `internal/agentbus/detail.go` 的节点详情（T8-3 的"谁批的、为什么"）；真正没有生产调用方的只有 `Authorized`
  （`AuthorizedGrants` 的布尔简写，被授予规则的五条用例断言，属内核谓词、不是前端读面 —— **保留并在此写明**）。
  ③ `T11-6`：缓存影响字段已进每一次提交正文（本会话每笔提交都带 `Cache-impact`/`Cache-guard`/`Documentation-impact`）。

## 4. 落地顺序（建议四阶段）

- **阶段一「刹车与自愈」**（先让无人值守安全）：G3 预算/槽位默认 → G2 印记诊断。验收 = 各自判据。
- **阶段二「自动裁决」**（让无人值守有结论）：G1 审议链入口 → G5 停滞自动出口。
  验收 = 一次真机"`refute` → 自动审议 → 判决落地"。
- **阶段三「方向感」**（让集群接得住）：G4 编排规范 + 演练（编排者被删）→ G6 限流观测。
- **阶段四「收尾」**：G7 边界写清、G8 读面与文档一致化。

## 5. 端到端演练配方（一次"攻坚"）

**准备**：≥3 个会话各自入列同一 board、至少一个持 Goal 契约、`[agentbus]` 设上额度与 `dispatch_slots`、宿主（桌面或 serve）在跑。

**步骤与断言**（每步都要能复跑）：

1. 编排者写节点图（`require` 子树 + 交付物根）→ `action=view` 能看到根与边界节点。
2. 宿主 tick 派活 → 板上出现 `claim`（actor = 承办者），承办者会话**自己起回合**（无人按回车）。
3. **杀一个 worker**：租约到期后 `Sweep` 记 `no_progress` 并回收 → 幸存者接管 → `AssessLanding` 仍能收敛。
4. **杀宿主**：看门狗拉起 → 宿主自己排入带上下文的接续简报 → 会话继续推进（dev.113/114 验过一次）。
5. 验收节点全 `done` 且无未决矛盾 → `AssessLanding` 判 landed；重放同一 op 日志得同一结局。
6. 反例断言（必须成立）：预算触顶时拒绝发生在 claim 前；同名 participant 两个会话时投递**拒绝**而非猜；唤醒投递不到必须报错留痕。

## 6. 与其它文档的关系

- 契约与出口条件：`docs/agents/AGENT_BUS.md`（唯一真相源，本文件不复读）
- 执行清单：`docs/agents/TODO.md`（本文件 §3 的 G 条目在那里只留指针）
- 无人值守语义：`docs/UNATTENDED.md`（总开关、合约、看门狗）
- 旧稿（被取代但保留继承项）：`multi-agent-collaboration-design.md`、`multi-agent-collaboration-development-plan.md`

## 7. 维护规则

- 每条缺口修完，在 §3 **原位**补证据（提交号 / 命令 / 真机日志行），并同步 `TODO.md` 对应指针的勾选状态。
- 不复制契约条文；只写"能验的"（命令、文件、日志行），与 `AGENT_BUS.md` 的三条不让步纪律（工件优先 / 有界 / 可核验）一致。
- 新增缺口一律追加到 §3 并同步 `TODO.md`；不要另起第二份清单。

## 6. 本会话收尾索引（2026-10-03/04，未 push）

按本文档的规矩，逐条落在 §3 原位并同步 `TODO.md`；此处只做**索引**，不复制条文。

| 类 | 提交 | 一句话 |
|---|---|---|
| G1 | `cff94e184` / `93b9add8e` | 审议工具入口 `hearing_*` + 写后唤醒；四个 `hearing_*` 旋钮（0 = 关） |
| G2 | `93b74eed9` / `e93686991` | 看门狗 `lastRunAt`；**印记取值**改为"操作者的开关"（纯读代码定位写入者，无需真机复现） |
| G3 | `c3f0f08f7` / `9299d62c3` / `e3e4b68aa` / `faea3ac3c` | 「已开启（无预算）」交互默认；逐次拒绝结构化记录；**按层计数 + 面板一行**；人读面文档 |
| G4 | `9f47ea955` / `e30c652c3` | `ORCHESTRATION.md` + 内置技能；`assert` 可选 `Title`（规范里的绕行写法退休） |
| G5 | `2e7807546` / `b36e8f3d1` | 停滞唤醒请求者；**指派超时回落 pool** |
| G6 | `f85600ad7` / `e9b5f7deb` / `194bf362c` / `f7dd78963` | 429 原因可观测；状态行；日志归因（provider/协议/trace）；**按 provider 实例计数 + 面板一行** |
| T12-3 | `c48f7432c` / `f810518f5` | 槽位"排队而非失败"宿主级用例；**槽位回收**（修前全仓没有生产代码调用 `ReleaseSlot` ⇒ 一满即永久停摆） |
| G8/G7 | `9ec9aad12` / `faea3ac3c` | 读面清理；`ORCHESTRATION.md` §5.1 多机分工 |
| 质量 | `1e13a47f3` / `e202afaa9` / `da4b6c84c` / `5b5cb7cf9` / `4bcfa5ca9` / `954029204` / `dc283fab8` | `make lint` 两侧转绿；注释收敛；两处时序 flake；自查 repolint 债；打包守卫；recap 误判结案 |
| 装配边界 | `db1a38f54` / `5578284b8` | 会话自己用 `agent_bus` 推进节点；宿主派发回环三规则（见 `AGENT_BUS.md` §13.15） |

**仍未做（真机/两进程，明记不冒充）**：T9-4 无人值守贯通 · G7 双机演练 · T5-4 两进程验收
**明确不做**：T4-8 去掉命中拷贝 · 自适应节流 · recap lane 随轮换刷新（`AGENT_BUS.md` §13.15 末记档的边界）
