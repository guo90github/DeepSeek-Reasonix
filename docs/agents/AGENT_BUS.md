# 多智能体契约（AGENT_BUS · 黑板为骨）

> **状态：部分已实现（2026-10-03 校）。** `internal/agentbus` 已落地：内核 `board/`（op 日志 + 确定性 fold +
> 节点状态机 + 逐节点租约），以及队列/排班/预算/投影/唤醒/落地判定的实现与 `internal/control`、桌面宿主的接线
> （`internal/cli` 的 `REASONIX_AGENTBUS_*` 是 headless 入列口）。**接线情况（2026-10-04 复核）**：talk 的写入口**已接**
> （面板 `desktop/agentbus_apply.go` 的 `AgentBusAsk/Answer` + 模型工具 `internal/boot/agentbus_board_tool.go`）；
> `TakeRanked` 与预算账本已由桌面宿主接线（`desktop/agentbus_waker.go` 用 `[agentbus]` 的四个额度加 `dispatch_slots`
> 建账本并装到每个 controller 上）；**宿主侧唤醒 tick 也已接**（桌面 `agentBusWakeTick`/`agentBusDispatchTick` +
> headless `internal/cli/agentbus_tick.go`）；**仍只测试用**的是非排班版的 `Take`；去重以进程内 `WakeLedger` 为主，
> 落盘那层靠 inbox 的 `idempotencyKey`（同 key 重投在接收侧收敛）；过期租约回收**不只**在写路径 —— 宿主 tick 也 sweep
> （`AgentBusTick`，2026-10-04 进程级验过：t+20s `claimed` ⇒ 跨过 30s tick 变 `open/noProgress=1`）。（本稿旧标 *待建* 的行已按现状改写；S1 规格仍以 §11.1 为准。）
> 并列读：`docs/COLLAB-SURFACE.md`（**已实现**的跨仓封闭面，由 `tools/collabgate` 钉住）、
> `docs/agents/multi-agent-collaboration-design.md`（本稿部分取代之，见 §10）。
> 执行清单：`docs/agents/TODO.md`；**S1 的可实现规格见 §11.1**（S1 实现以 §11.1 为准）；评审留痕见 §11.2。

用途：把「给人看的」与「给智能体读的」两类数据钉成两条**不许互相渗透**的通道；并钉住多智能体的**唯一真相**
——协调作用域级黑板（共享工件，**可跨工作区、可跨进程**）——以及它的写侧（多写者 op 日志）与读侧（既有任务目录/任务树的投影）。目标形态是**百级 agent 无人值守集群**（§集群规模、§失败模式）。

## 产品判据（先钉判据，再谈实现）

> **杀掉编排者，工作还在往前走 → 多智能体；杀掉就停 → 作业派发器。**
> **规模判据：杀掉任意一部分参与者（含编排者），任务树仍能收敛 → 集群；一个都不能少 → 只是个作业系统。**

编排者不是链路的一环，只是参与者之一。**v1「三动词单向派活」（本稿 2026-10-02 前的一版）按这两条判据都不合格，
已作废**：它把真相放在编排者的回合里，编排者一死，黑板（若有）也没人推。合格形态必须让
**认领、依赖、裁决都落在共享工件上**，任何参与者都能继续推进（§4 首两行）。

同时保留三条**不让步**的纪律：

1. **结论落黑板**——对话**可以自由**（§3.2 第一档），但只有黑板上的 op 影响结论与后续；转录本身不是结论；
2. **有界**——每个 `correlation`、每个话题、每个节点都有 token 预算、TTL 与轮次/迁移上限；
3. **可核验**——节点 `done` 只认可核对证据，**且由非产出者复跑**（§集群规模第 4 条），模型自评不算。

缺前两条，多智能体是烧钱演示；缺第三条，它只是"看起来很忙"。

权威面：本机仓 `C:\guosj\ai\deepseek-reasonix\DeepSeek-Reasonix`（分支 `dev-2`）。表内路径都是仓内相对路径。

## 集群规模（N≈100）：复杂度压在哪儿

**先把"指数增长"精确化**——不是指数，是**超线性**，而且只有一项真会爆：

| 量 | 随 N 怎么长 | 控制点 |
|---|---|---|
| 全互联通信 / 上下文 | **O(N²)**——每人看所有人的话 | **分解成树** + 视图裁剪 ⇒ 回到 O(N) |
| 每 agent 每轮的视图读取 | O(视图大小) | 只读「我的子树 + 我认领的节点 + 我订阅的话题摘要」 |
| op 日志 | O(总步数)，append-only 且每条很小 | 增量读（cursor/seq），**不重放全量** |
| 并发执行 | 排队延迟随 N 线性 | 全局（host 级）并发槽 + 排队 |

**真会爆的是"每个人看见多少"与"醒得多勤"**，不是 agent 的数量。100 个 agent 不是 100 倍能力，是 100 倍**上下文成本**的风险。

结构和四个机制（百级必需，**不是调参**）：

1. **拓扑：根 board + 子树分片。** 跨子树只经**边界节点**——`require` 到别的子树时生成一个边界节点，由目标子树成员认领。这一条把 O(N²) 压回 O(N)。
2. **租约回收（孤儿节点）**：`claim` 带 deadline 且**须心跳续租**；超时未续 ⇒ 节点回 `open` 并记 `no_progress`。没有它，100 个 agent 里死几个就能把整棵树卡住。
3. **速率与退避**：每节点每分钟状态迁移上限 + 连续 `refute` 触发冷却。provider 层已有退避（`internal/provider/retry.go:22`：`maxBackoff = 15s`，尊重 `Retry-After`），但集群层要的是**主动限流**，不是重试。
4. **独立复现**：`done` 的证据必须由**非产出者**复跑一遍（证据里带 `reproduced_by`）。百级下没人看每一份产出，"谁跑的"就是质量闸。
5. **观测聚合**：人读默认按子树折叠，只显**异常 / 争议 / 停滞 / 孤儿**，可下钻到节点——不画 100 张卡片。

**四条硬上限（缺一条就上不了百）**：视图（每人不读全板）／并发（全局 host 级槽）／预算（board → 子树 → 节点 → 回合，四级封顶）／速率（节点级迁移上限）。任一触顶 = **暂停协作**并保留现场，绝不杀宿主会话。

**现实边界（诚实）**：现有并发控制是**会话级**的——`SubagentScheduler` 是 session-scoped（`internal/agent/scheduler.go:36`），`max_subagent_concurrency` 也是每会话一档（`internal/config/config.go:1304-1307`），而且**嵌套子 agent 会 fail-fast**。模型的 rate limit 却是**全账号共享**的 ⇒ 100 个 agent 同时打就是互相 429。所以**全局并发槽是本设计新增的东西**，不是把现有旋钮调大。

**范围：v1 是单机。** 一台机器的进程/内存/会话数是硬限；**"跨机同算一池"不在 v1**（协议保留可换目标：地址 + 令牌换一个就行，但"多机共享同一个槽池/预算池"是另一件事，需要跨机协调者，明确不做）。评审指出上一版 §11 的 S5 出口条件写成"多机同算一池"，与本节的"v1 单机"冲突——已按本节收敛为"**本机 host 级**"。

## 失败模式与对应规则（百级特有）

下面每一条都是"集群一上线就会咬"的，且都**不需要新守护进程**——它们是协议里的状态、规则与配额。

| 风险 | 症状 | 结构应对 |
|---|---|---|
| **交互低效** | 靠 `interval` 空转；往返问个不停 | ① **就绪即事件唤醒**（inbox 唤醒），`interval` 只作兜底；② 派活信封**必须带验收条件**，否则 worker 必然回来问；③ 视图**头部稳定、尾部追加**——这一条同时是**前缀缓存命中率**的前提，且只有配合"**增量读、不逐轮全量重发**"（§集群规模 op 日志那行）才成立：全量重发时"头部稳定"没有落点 |
| **缺能力被"偷懒放弃"** | 节点无声停住，整棵树卡在那一格 | 缺能力是**一等状态**：`capability_gap`（记：要什么 / 试过哪些路径 / 为什么不行）；**放弃只能由 `decide` 批准且须带证据**（`abandon`）；缺能力会生成"获取能力"子节点（装依赖 / 写脚本 / 建 skill / 挂 MCP） |
| **新造的能力没人知道** | A 写了脚本或工具，B 继续手搓 | 能力**登记到黑板**（能力节点：名称 / 入口 / 证据 / 验证状态 / 谁能用）。**没有登记等于没造**——自建能力的通道本来就有（skill 创建、改代码、挂 MCP），缺的只是"登记 + 可发现" |
| **错误会（坏结论污染下游）** | 一个 `done` 错了，下游 20 个基于它继续 | `revert`：把 `done` 打回 `open`，并把下游标 `stale`。**已经 `decide` 过的结论必须可撤销** |
| **单 agent 行为不可预知** | 同一节点 3 分钟 vs 40 分钟；2k vs 200k token | ① 认领即**定界**（步数 / token / 预计产出），超界定即回收；② 高风险节点**冗余比对**（两个 agent 独立做，比结果）；③ 按**历史档案**选人（成功率 / token 每节点 / 返工率）——**用统计对抗方差，不预测单次** |
| **调度不公与抖动** | 难的没人做；百人抢单震荡 | 优先级 = **关键路径长度 / 预估成本**；**批量领取**相关节点（不逐节点抢）；同子树**亲和**（复用上下文）；**最难优先**防饥饿 |
| **一致同意被当成证据** | 互相 `refute` 之后趋同妥协 | **consensus ≠ evidence**：权重只看证据类型，不看赞成票数 |
| **单点污染扩散** | 一个 agent 被外部内容带偏，写进黑板 → 100 个都读 | 黑板 op 带**来源标记**；**受外部内容影响的断言不能直接进 `done`**（要另一条独立证据） |
| **人的注意力被淹没** | "升级给人" = 无人值守失效 | **升级配额**（每小时上限 N 条），超配额自动降级为 `undecided-by-rule` + 记账 |
| **重心错位（成本 ×100）** | 一堆节点"完成"了，任务没落地 | 预算**绑在验收节点**上，不绑在做工节点上；做工再多也不消耗总预算 |

## 什么叫「落地」（完成判定与证据门槛）

| 项 | 规则 |
|---|---|
| 节点 `done` | **只能由 `decide` 给出**，且前置条件含「存在**可核对证据 + 非产出者复跑**」——证据要可重跑/可复现：测试通过、构建产物、量测数字、评测记录，或带稳定地址的引用（如 `room:<seq>`、`<board>/#seq`）。**不可核对的产出只算 `assert`，进不了 `done`** |
| 权重 | 证据类型分级（实测 > 引用源码 > 推理 > 无据）；**模型自评既不是权重，也不是完成依据**；**票数也不是**（§失败模式） |
| 任务级落地 | 全部**验收节点** `done` + **无未决矛盾** + 证据链完整 ⇒ 才可宣告「任务落地」。**节点全 `done` 但仍有未决矛盾 = 未落地** |
| 与 Goal 对齐 | 落地 ⇔ Goal `complete`；预算耗尽或无进展 ⇒ `blocked` 并**保留黑板现场**（不把未决矛盾冻结成结论）；续期语义沿用总开关（`docs/UNATTENDED.md` §1–§2） |

**为什么单列一节**：派活可以自动，**验收不能自证**。没有可核对证据时，审议只产出争论，不产出质量。

## 跨工作区与跨进程是**两条轴**（先钉这条，它决定编排落在哪）

轴一：**同一进程内的多个工作区**。桌面可同时持有多个 tab，**每个 tab 带自己的 `WorkspaceRoot`
和它自己的 controller**——所以「一个进程 = 一个工作区」不成立。

轴二：**跨进程**。参与方可能是不同进程：`agentd` 托管的 `reasonix serve`（每实例一个工作区、一个令牌）、
remote/SSH 远端、CLI、手机遥控端。

| 事实（可核对） | 锚点 |
|---|---|
| 会话按**工作区**分目录：`<state home>/projects/<WorkspaceSlug(root)>/sessions` | `internal/config/paths.go:462` |
| 桌面 tab 带 `WorkspaceRoot`（`""` = 全局），并各带自己的 controller | `desktop/tabs.go:72`、`desktop/app.go:759`、`:873`、`:1349` |
| serve 的会话清单带 `all bool` 开关：`false` = 当前工作区，`true` = 跨工作区 | `internal/serve/session_list.go:21` |
| 枚举工作区已有出口 | `GET /projects`（`internal/serve/serve.go:647`） |
| 每实例一个托管 serve + 注册表 + **每实例令牌** | `internal/agentd/agentd.go:36-48`（`Record`）、`:41`（`TokenFile`）、`:49-50`（`URL()`，注释自述「the loopback base a coordinator posts to」）、`:15`（`RegistrySchemaVersion`）；注册表 `<home>/agents.json` |
| **跨进程互斥的真实原语**（Windows `LockFileEx` / unix `flock`，进程死亡由 OS 释放） | `internal/filelock/filelock.go`、`lock_windows.go:30`、`lock_unix.go`；既有使用者 `internal/workspacelease/lease.go`、`internal/agentd/registry.go` |

三条直接后果（本稿据此收紧）：

1. **`board` ≠ workspace。** 黑板是**协调作用域**（一次协作一个），它可以横跨多个工作区；因此它**绝不**落在
   `<state home>/projects/<slug>/` 里——落在那里就等于把编排绑死在一个工作区上。
2. **机器通道必须进程无关**：参与方**可能**同进程（桌面多 tab 的多个 controller），也**可能**跨进程（托管 serve /
   remote / CLI / 手机）。因此持有权不能是"某个进程持有黑板"，只能是**文件锁 + 可从日志重算的逻辑租约**
   （`internal/filelock`；§11.1）。物理互斥用文件锁（跨进程、进程死亡自动释放），逻辑租约（`claim` 的 deadline）
   让"谁该推进"在进程被杀后仍**可从 op 日志重算**——两者缺一不可。
   （上一版把这条写成「跨工作区 = 跨 serve 进程」，把两条轴压成一条——错；上一版还只有"租约"没有"文件锁"——也不够。）
3. **常驻性不等于新守护进程。** 真相在文件里（append-only + 文件锁），通知在各宿主自己的 HTTP 面上，
   所以仍然**不新造守护进程**。

## 编排落在哪（形态：内核内建；可跨进程，但不出新守护进程）

| 项 | 做法 | 理由 |
|---|---|---|
| 真相与协议在内核 | 黑板与消息信封放 `internal/agentbus`（**已落**：黑板 `internal/agentbus/board`、信封/投影 `talk.go`/`talklog.go`/`results.go`）；命令面加在 `control.Controller`；传输面加在 `serve` | 按 `REASONIX.md` 约定，CLI / serve / 桌面**自动继承**（`tools/repolint/layers.go:71-74`） |
| **分层：agentbus 是工具层** | `frontends → control → {agent, agentbus}`；**`agentbus` 只 import 工具包**（`internal/fileutil`、`internal/filelock`），**不得 import `internal/agent` 或 `internal/control`** | 黑板是数据与规则，不懂调度；`SubagentScheduler` / `CapabilityGrant` 的**接线留在 control/host**（评审指出的"链缺环"由此消解） |
| 编排者是**参与者**，不是一个特殊角色 | 编排的决策跑在某个会话的回合里；它读写黑板的方式与其他参与者完全一样 | §产品判据要求编排者可被杀掉而不影响推进 |
| 黑板**不属于任何进程** | op 日志 + **board 级文件锁**落在 state home 的协调作用域目录；任何进程按同一纪律读写 | 同进程多控制器与跨进程两种情形都要能写；只有"文件锁 + 日志可重算"同时覆盖两者 |
| 编排客户端 = 任何进程 | 桌面 tab、CLI、未来的独立 exe、仓外房间**都只是客户端**。同进程的直接用已有 controller；跨进程的走该宿主 serve 的回环地址**＋该实例的令牌** | 地址与令牌来自 `agentd` 注册表（`internal/agentd/agentd.go:49-50` + `:41`） |
| 常驻性复用 `internal/agentd` | 每实例一个托管 `reasonix serve` + 注册表 | 已有；**不新造第二个守护进程** |
| **时钟与唤醒也复用** | 编排者（一个会话）由两套既有机制被唤醒：① `heartbeat`（间隔，绑 `topicId` + `goal` 契约）；② inbox 唤醒（事件推送） | `docs/UNATTENDED.md` §1–§5、`POST /inbox/items`（`internal/serve/inbox.go:21`） |
| **规模化的三闸** | 视图裁剪 / 全局 host 级并发槽 / 四级预算，见 §集群规模 | 百级下这三样是**成立条件**，不是优化项 |

**"谁来派活"不是缺件，就是编排者本人。** 它是会话 ⇒ 有黑板工具 ⇒ 能 `split` / `claim` / 派发到别的会话；
它被唤醒 ⇒ 无人时也在跑。无人值守链条因此是：

```
时钟（既有 heartbeat，按 interval）＋ 事件（既有 inbox 唤醒）
        → 编排者被唤醒 → 它读黑板、派活、裁决
        → 黑板落文件（真相）
```

三件**都已落**：时钟与事件本就复用既有机制，黑板也在仓里（`internal/agentbus/board` + `talk.go`/`talklog.go`/`results.go`）。

（本节曾写过一版说「无人值守需要一个常驻推进者／独立 exe」——**那是错的**：把"能力"当成了"缺件"。
独立 exe 只在一种情形下可能被需要：该协作涉及的宿主里**根本不跑**桌面或 CLI 进程；而 `agentd` 已能为每个
工作区托管 serve，所以连这种情形通常也不需要新 exe。另注：`heartbeat` 引擎住在桌面宿主内
（`docs/UNATTENDED.md:5-8`：everything lives in `desktop/`，kernel 未参与），所以**纯 CLI/serve 环境目前没有这个时钟**——
那是能力边界，不是缺件。）

## 0 七句自证

```
ls internal | grep -E '^agentbus$'                    # 本稿落地前：无输出（= 目标稿）
rg -n 'no reply channel' internal/serve/inbox.go      # 单向投递的现状证据，§7 引用
sed -n '1,12p' internal/taskmonitor/model.go          # 读侧已存在：自述为纯观察层
rg -n 'WorkspaceSlug' internal/config/paths.go        # 跨工作区：会话按工作区分目录
rg -n 'TokenFile|func \(r Record\) URL' internal/agentd/agentd.go   # 跨进程：地址 + 每实例令牌
rg -n 'LOCKFILE_EXCLUSIVE_LOCK|flock' internal/filelock/            # 跨进程互斥的真实原语
rg -n 'session-scoped concurrency' internal/agent/scheduler.go      # 并发闸是会话级，不是宿主级
```

## 1 两个维度是两条通道（硬边界）

| 项 | 人读维度 | 机读维度 |
|---|---|---|
| 载体 | `eventwire.Event`（`Kind` + 展示字段） | 黑板 op、对话行、`agentbus.Envelope`（结构化 + `json.RawMessage` payload） |
| 传输 | `GET /events`（SSE，`data: {json}`，15s `: ping`） | `GET /agentbus/stream?cursor=N` + `POST /agentbus/{publish,op,say}`（**不是** `/events`） |
| 过滤/寻址 | 视图过滤（`Broadcaster`、`?all=1`）；百级默认按子树聚合 | `board` + `to`（节点 id / agent id / 工作区+会话 / `topic:<name>`）按围栏校验 |
| 存储 | 现有会话 JSONL transcript + `trajectory` / `turnevent`（**不新增面**） | `<state home>/agentbus/<board>/…`（§5） |
| 写者 | 内核（sink 发射） | 参与者（黑板 op / 对话 / 消息）+ 裁决与调度规则引擎 |
| 读者 | 人、诊断录制 | 参与者、裁决与调度规则引擎 |
| 禁止 | 机器从人读文案里抠字段 | 机读载荷渲染成会话正文、或进系统提示前缀 |

**隔离的最小充分条件只有 R1 与 R3**（§8：机读不入正文、两者都不进系统提示前缀）。`/agentbus/*` 之所以独立，
是因为机器需要可寻址与因果（§2 的 `to`/`correlation`），**不是**因为「必须多出一条管道」——独立是手段，不是目的。

**反例一（本稿要防的头号错误）**：拿人读正文当机器协议。既有先例已出过这种耦合——
`docs/COLLAB-SURFACE.md` §1 记着「宿主实际消费**只有 `text`**，不重解析 `mentions`」（`internal/boot/inbox_wake.go:157`）。
那一步是**把人写给机器看**；本稿的方向相反：机器只读结构化字段，人只读投影。

**反例二**：把 `/events` 当机器通道用。它的帧带渲染意图与顺序，不带因果；机器要的是可寻址、可重放、
带 `correlation` 的因果链（§2）。两者共用一个流，等于把「怎么显示」混进「是什么」。

**反例三**：把「严格隔离」读成「两套真相」。隔离的是**通道与字段**，不是**事实**：黑板只落一份真相，
两个维度是它的两个投影（读侧投影今天已经存在，见 §4）。

## 2 消息信封（**已落**：`internal/agentbus/talk.go`（`say`/`ask`/`answer` 与话题边界）、`talklog.go`（`messages.jsonl`）、`results.go`（`results/<correlation>.json`）；本稿从未创建它当时点名的 `message.go`）

信封承载**通知、自由对话与直问直答**；真相在黑板（§3）。

| 字段 | 取值/类型 | 说明 |
|---|---|---|
| `id` | string | 幂等 id（信封语义沿用 `internal/sessioninbox`；**黑板 op 的幂等键见 §11.1**） |
| `board` | string | **协调作用域 id**（一次协作一个；可跨工作区）；子树分片另带 `subtree` |
| `from` / `to` | agent id / `工作区+会话` / `topic:<name>` / 节点 id | 寻址；`to` 缺省 = 广播到订阅者 |
| `type` | 封闭枚举（§3） | 未知取值**拒收**，不静默降级 |
| `correlation` | string | 一条因果链；**预算/TTL/hop 挂在它上面** |
| `reply_to` | string | 这条要回给谁；缺 = 不需要回复 |
| `payload` | `json.RawMessage` | 有上限；超限**拒收**；派活载荷**必须含验收条件** |
| `source` | string | 这条 op/消息的**来源标记**（本地 / 房间 / 外部内容影响）——污染扩散的闸（§失败模式） |
| `created_at` / `expires_at` | RFC3339 | TTL 过期不投递 |
| `seq` | uint64 | append-only 序号；**`<board>/#seq` 是稳定地址**，可作证据引用 |

**命名（别撞车）**：这个字段**不要**叫 `scope`。仓库里 `Scope` 已经有用处（桌面 tab 的 scope 取值如 `"global"`／项目，
`desktop/app.go:759`、`:873`），两者不是一回事；复用同一个名字会让后来的人把一个当另一个读。

## 3 协议：黑板动词 + 交流三档

### 3.1 黑板操作（唯一真相的写侧）

`assert` / `refute` / `claim` / `release` / `split` / `require` / `assign` / `unassign` / `decide` / `abandon` / `revert` / `yield`

| 动词 | 语义 |
|---|---|
| `assert` | 在节点上落一个断言或产出，**须带证据引用**（证据须可核对，否则只算 `assert`） |
| `refute` | 反驳某条断言/产出，**须带理由**；反驳与断言同级，不是「下级打小报告」 |
| `claim` / `release` | 认领 / 放回一个节点（认领 = 承诺，**带 deadline 且须心跳续租**；超时被回收） |
| `split` / `require` | 拆分节点 / 声明对新节点的依赖——**DAG 由此在运行时长出来**；跨子树时生成**边界节点** |
| `assign` | 把一个节点**指派给一个参与者**（`Node.Assignee`）：`WakeTargets` 只向他发 `Ready`，`Take`/`TakeRanked` 也**只许他取**（不许改派已 `claimed` 的节点）；**无指派 = 板级 pool**（接管自愈不变）；**已指派节点的 `claim` 由板本身拒收**（`not_assignee`，不只靠队列取用过滤）；**承办者不在了不会自动回落 pool**——宁可停住也不给错会话；出路有两条且都是显式 op：改派（`assign` 给别人）与**收回**（`unassign` = `assign` 不带 `assignee`，交回板级 pool） |
| `decide` | 对一个有争议的节点下结论（人也可以下，见 §4）；**前置条件含「可核对证据 + 非产出者复跑」** |
| `abandon` | 放弃一个节点：**只能由 `decide` 批准**，且须带「试过哪些路径 + 为什么不行」的证据——**不许静默停手** |
| `revert` | 撤销一个已 `done` 的节点（打回 `open`），并把下游标 `stale`——**错误必须可回滚** |
| `yield` | 撤下自己的断言或认领 |

写侧三条硬线：**`assert` 的证据必须可核对**（不可核对的产出进不了 `done`）；**节点的 `done` 只由 `decide` 给出**（判据见 §什么叫「落地」）；**缺能力不是可以停的理由**（走 `capability_gap` → 获取能力，或 `abandon` 经批准）。另有两条规模硬线：**认领须续租**、**每节点迁移有速率上限**（§集群规模）。

**合法迁移的闭集在 §11.1**——不在那张表里的组合一律**拒收，且必须带原因**（静默忽略是最坏的实现）。

### 3.2 交流三档（同一管道，三种承诺强度）

| 档 | 动词 | 承诺 | 谁看得见 | 边界 | 对黑板的影响 |
|---|---|---|---|---|---|
| **① 自由对话** | `say`（发到 `topic:<name>`） | **无**——想说什么说什么，不必有结论 | 话题参与者；每一行带稳定地址 `<board>/#seq` | 话题轮数上限 + 该 `board` 的 token 预算 + 静默窗口 | **无**（要进结论，得另外 `assert`） |
| **② 点对点问答** | `ask` / `answer` | 有——被点名者须在 TTL 内 `answer` 或 `yield` | 只两方 | `correlation` 的预算 / TTL / hop | 无（结论仍回黑板） |
| **③ 审议** | 复用 `assert` / `refute` | 有——每方必须出证据 | 参与者集合可见（跨工作区也可见） | 默认 2 轮 + 每轮截止 | **有**——它是争议节点 `decide` 的输入 |

**三档共用的三条硬规则**：

- **默认不灌上下文**：自由对话只有「点名给你的行」与「每轮摘要」进入参与者的上下文；全量在流里可订阅、默认不投。
  否则自由对话就是成本炸弹（这是唯一一条会真烧钱的路径）——百级下它也是 O(N²) 的唯一入口。
- **有界**：话题轮数、token 预算、静默窗口任一触顶即收口（`say` 被拒，话题转只读）。
- **不进正文、不进前缀**：三档都受 §8 的 R1/R3 约束。

**③ 审议（hearing）** 的细节——它不是新动词集，而是**节点的一个状态**：

| 项 | 取值 |
|---|---|
| 触发 | 一个已有 `assert` 的节点被 `refute` → 节点进入 `contested`，审议开始（**无需新动词**） |
| 参与者集合 | 该节点上有 `assert`/`refute` 的参与者 + 被点名者；**可观测**（跨工作区时也可见） |
| 轮次上限 | 默认 2 轮（配置可改）；每轮有截止（`expires_at`）；**连续 refute 触发冷却** |
| 必答 | 被点名者**必须**在本轮内 `answer` 或 `yield`；否则记一条 `no_answer`——**弃答必须可见，不许静默** |
| 每轮产出 | 只有两种：`assert`（带证据）/ `refute`（带理由）。**一条对话只有在变成黑板 op 时才进入结论** |
| 权重 | 按证据类型分级（实测数据 > 引用源码 > 推理 > 无据）；**绝不用模型自评**；**票数不算证据** |
| 结束 | ① 规则 `decide`（权重最高者胜）；② **等重不掷硬币**：升级给人（受升级配额约束）；③ 轮次上限到 → 自动 `decide` 并标 `undecided-by-rule`；④ 人可随时 `decide` 覆盖 |

**①与③不是替代关系**：自由对话管**探索**（把问题问开、把边界摸出来），审议管**收口**（有争议时得出可核对的结论）。
把探索塞进审议会僵住；只有探索不收口就永远没有结论。

### 3.3 竞争分配（仅当同一节点有 ≥2 个候选者时）

`bid` / `award`。没有第二个候选者时**不进入**这条路径（多写者不等于每次都要竞价；百级下它是**控速率**的手段之一）。

### 3.4 与既有词表的映射

| 旧词 | 新落点 |
|---|---|
| `propose` | `assert`（节点） |
| `inform` | `assert`（无承诺的事实）**或** `say`（纯粹说出来、不求结论） |
| `request` | `ask` |
| `bid` / `accept` | `bid` / `award`（§3.3） |
| `commit` | `claim` |
| `done` | `assert`（带证据）→ 由 `decide` 定 |
| `reject` | `refute` |
| `withdraw` | `yield` |
| `announce` | `assert`（agent 状态的节点） |

仓外房间的 `room_say` ≈ 本稿的 `say`：房间行与 `<board>/#seq` 是**同一档的两种实现**，在证据引用上等价（§4）。

## 4 黑板与寻址（写侧新增，读侧复用）

| 项 | 做法 | 依据 |
|---|---|---|
| 黑板是**唯一真相** | 协调作用域级共享工件：节点 + 认领 + 断言/反驳 + 裁决；多写者按 op 追加 | 读侧已存在：`internal/taskcatalog`（SQLite 索引）+ `internal/taskmonitor`（自述纯观察层）+ 桌面任务树 |
| 逐节点**单写者** | 每个节点的状态迁移由一个租约持有者执行；多写者只体现在**不同节点**与 op 追加。**租约管"谁能推进"，不管"谁能写日志"**（后者由 board 级文件锁串行化，§11.1） | 免去共享可变结构；跨进程互斥用 `internal/filelock`（文件锁，不是内存里的 owner） |
| 跨进程安全来自**文件锁 + 日志可重算** | 物理互斥 = board 级文件锁；逻辑租约 = `claim` 的 deadline + 心跳，**可从 op 日志重算**（进程被杀不丢） | 参与方可能同进程（桌面多 tab）也可能跨进程（托管 serve / remote / CLI / 手机） |
| 杀掉编排者仍能推进 | 认领、依赖、裁决规则都在黑板上，任何参与者可继续；编排者只是参与者之一 | **这就是 §产品判据的落点** |
| 人是参与者 | 人可对争议节点直接 `decide`，也可在话题里 `say`（两个身份都算参与者）；**升级受配额**（§失败模式） | 满足「可观测」不只是看，还能裁、能插话 |
| 审议的写者 | 审议每轮的 op 仍落**同一节点**；**不写第二条日志** | 一份真相（§1 反例三） |
| 房间接线 | 仓外房间的每一行按 `room:<seq>` 作为**证据引用**（`docs/COLLAB-SURFACE.md` §2 的 `room.seq`）；内核自由对话按 `<board>/#seq` | 房间负责「说」、黑板负责「算」；房间不在时该项证据缺失，**不阻塞** |
| 跨工作区寻址 | 目标按 `工作区 + 会话` 寻址；连接取自 `agentd` 注册表（`internal/agentd/agentd.go:49-50` + `:41`） | 会话清单的 `all bool`（`internal/serve/session_list.go:21`）已经是"跨工作区"这一维 |
| 跨工作区**可见性** | 黑板默认只共享**节点元数据与结果摘要**；证据引用（文件内容、转录片段）**按需授权**，默认不外泄到其他工作区 | 别的仓库可能是私有的；编排不该顺手把它搬到一个共享文件里 |
| 跨工作区**能力** | worker 的能力天花板由**它自己所在工作区**决定，编排者**不得提升** | `CapabilityGrant` 是能力交集（∩），没有"跨区授权"这回事 |
| 跨工作区**所有权** | 要搬会话/接管的，复用既有的 `GET /ownership`、`POST /handoff`、`/adopt`、`/reclaim`、`/mirror-end` | 已有；`internal/serve/serve.go:648-653` 就是寄存器 |
| 身份是**节点**，不是会话 | 身份 = `(scope, workspaceRoot, topicID)`（仓库既有三元组）+ 会话 +（可选）子 agent id | 只按会话寻址会在**观测**与**成本归属**上都对不上号 |
| 会话内的再委派 | 一个会话内还能 fan-out：`task` / `parallel_tasks` / `fleet`（后两者上限 64） | `internal/agent/fleet.go:22`、`internal/agent/parallel_tasks.go:84`；**百级靠跨会话展开，不靠单会话扇出**（嵌套会 fail-fast） |
| 谁在写会话 | 复用既有 ownership / lease 面，**不靠父子链接** | 人类随时可能在同一个 tab 里打字——父子关系不表达「谁持有」 |
| 成本归属 | 每次参与消耗的 token 记到这次参与（`correlation` + 节点 id + **工作区**）上，而不是只记在目标会话名下 | `UsageSource` 已有 `subagent` 这一档前例；四级预算见 §集群规模 |
| 跨进程投递 | 落到目标会话 inbox，再由宿主唤醒（跨进程时带该实例令牌） | `POST /inbox/items`（`internal/serve/inbox.go:21`）可按会话寻址；`activateAddressedSession`（`:153`） |
| 并发与能力天花板（**接线在 control / host 层，agentbus 不参与**） | 会话内仍走 `SubagentScheduler` + `CapabilityGrant`；会话间另加全局 host 级并发槽 | `internal/agent/scheduler.go:36`（session-scoped）、`internal/agent/profile_spec.go`；分层见「编排落在哪」 |
| 能力**登记** | 新造的能力（脚本 / skill / 工具 / MCP）制成**能力节点**上板：名称 / 入口 / 证据 / 验证状态 / 谁能用 | 自建能力的通道已存在（`internal/skill/tools.go:642` 的 `CreateWithContent`、改代码、挂 MCP）；缺的是登记与可发现（§失败模式） |

> 读侧**不新建**：`internal/taskmonitor` 的包注释自述为「纯观察层……不实现第二套状态机」——
> 那正是本稿要的投影层。本稿只补它的**写侧**（多写者 op）与「跨会话 + 跨工作区 + 百级聚合」这一层。

## 5 存储（**已落**：`<board>/board.jsonl`、`queue.jsonl`、`messages.jsonl`、`hearings.jsonl`，共用 `internal/agentbus/jsonl` + `internal/filelock`；落点见 §13.4）

| 内容 | 位置 | 形态 |
|---|---|---|
| 黑板操作日志（真相） | `<state home>/agentbus/<board>/board.jsonl` | append-only op（§3.1 + §3.2 审议），按 `seq`；**确定性 fold 得黑板状态**；**结尾半行（无 `\n`）= 未提交的写，不算 op**；中段坏行跳过并**计数可见**（§11.1），两类都不中断启动 |
| 黑板读投影 | 复用 `internal/taskcatalog`（SQLite）+ `internal/taskmonitor` | **不新开人读面** |
| 对话与消息流 | `<state home>/agentbus/<board>/messages.jsonl` | append-only，按 `seq`；**`<board>/#seq` 即稳定地址**；损坏行处理同上一行 |
| 邮箱游标 / 视图游标 | `<state home>/agentbus/<board>/cursor.<agent>.json` | 原子替换（`internal/fileutil/atomicwrite.go:49`）；**百级下必须增量读，不重放全量** |
| 结构化终态 | `<state home>/agentbus/<board>/results/<correlation>.json` | 含 `transcript_ref` 指向已有 JSONL，**不复制正文**；跨区内容按 §4 可见性规则 |
| 能力登记 | `<state home>/agentbus/<board>/capabilities.json` | 名称 / 入口 / 证据 / 验证状态 / 谁能用（§4 末行） |
| 地址簿 | `agentd` 的 `<home>/agents.json` | 复用，不新建 |
| 人读 | 现有会话 JSONL + 事件；话题也可由房间面板看 | **不新增存储面**，只加事件种类 |

**`<board>` 的取值不是 workspace**：会话目录是 `<state home>/projects/<WorkspaceSlug(root)>/sessions`
（`internal/config/paths.go:462`），黑板**不落**在那里；否则一个协作会被绑死在一个工作区上（§跨工作区后果 1）。

## 6 拒绝分类

既有三档（**不改语义**）：`X-Reasonix-Reject-Class` = `target_unreachable` / `not_accepting` / `invalid_request`
（`internal/serve/reject_class.go:11`、`:16`、`:19`、`:22`）。机读面若要新增取值，只许**追加**，不许改动这三档的判定。
百级下新增一档 **`rate_limited`**（速率/并发槽拒绝——区别于 `not_accepting`：前者是"现在不行、待会再来"，后者是"不接受"）。

## 7 一条完整回路（编排者不在链路上；可跨工作区；可跨子树）

1. **任一**参与者 `assert` 一个目标节点（不是编排者专权）。
2. 有能力的参与者 `claim`（可来自别的**工作区**；**须续租**，且**须定界**：步数/token/预计产出）；同一节点出现 ≥2 个候选者时才 `bid` / `award`。
3. 认领者在自己的会话里执行；期间可在话题上 `say` 与同伴自由对齐；可 `require` 新节点、`split` 现有节点——**DAG 是运行时长出来的**；跨子树走**边界节点**；缺能力时开 `capability_gap` 并派生"获取能力"节点。
4. 产出 `assert(node, evidence)`；**任何**参与者可 `refute`（带证据）。
5. 争议节点进入**审议**（§3.2 ③）：多轮 `assert`/`refute` + 必答 + 证据权重；**等重升级给人**（不掷硬币，受配额约束）；
   轮次上限到则由规则 `decide` 并标 `undecided-by-rule`。
6. 收敛：**预算 > 未决矛盾 > 轮次/速率上限 > 静默窗口**。收敛后黑板冻结为结论，投影成人读结论。
7. **任务级落地**：全部验收节点 `done` + 无未决矛盾 + 证据链完整 ⇒ 宣告落地（与 Goal `complete` 对齐）；
   否则停在 `blocked` 并**保留现场**（判据见 §什么叫「落地」）。

其中「直问直答」仍需一条回执通道，而它在仓里**不存在**——当前唯一的原话是 `internal/serve/inbox.go:85`：

```
// A push wake has no reply channel, so a sender asks here instead of assuming the line landed.
```

现在只有「按 `seq` 回问」这一条路（`GET /inbox/room-line`，见 `docs/COLLAB-SURFACE.md` §7）：那不是回复，是查询。
`ask` / `answer` 与 `results/<correlation>.json` 是补上的那半边。注意：**这是缺件的补法，不是本稿的骨架**——
骨架是黑板；没有黑板，回执通道只会把「作业派发器」修得更顺。

## 8 三条隔离规则 + 三条守卫

**R1 机读不入正文**：黑板 op、对话行与消息载荷不得渲染进会话 transcript 正文；正文只允许 host-receipt 形态的摘要
（先例：`internal/agent/subagent_report.go` 的 `splitHostReceipts`）。

**R2 人读正文不被机器解析**：内核里任何 dispatch/路由分支都**不许**读 message text 来定投递——
路由只认 §2 的寻址字段；**也不许有 agent 靠读 `/events` 做人读面决策**（那是第二个机器通道）。

**R3 两者都不进系统提示前缀**：黑板、对话与消息内容只骑 **turn tail**（`internal/control/input.go:146` 的 `Compose`；
`<memory-update>` 在 `:188`/`:193`，`<background-jobs>` 在 `:202`），前缀字节稳定——**且视图内部顺序也要稳定**
（头部固定、尾部追加），否则前缀缓存命中率归零；`agent_*` 工具清单在 `boot` 时一次性定死。

**守卫（按 `internal/boot/effect_test.go` 的边界效果测试模式，走真实 `boot.Build`）**：

- 断言黑板/对话/消息载荷**不出现**在 provider 请求与前缀里；
- 断言人读 SSE 帧里**不含** §2 的机读字段（两个类型不许互相渗透）；
- 断言一个 agent 的一轮请求里**只含它自己的视图**（不含别人的子树/全板）——这是 O(N²) 的闸。

## 9 本稿引用的既有面（分三类，来源不同，别混）

**9.1 已冻结的跨仓封闭面**（引自 `docs/COLLAB-SURFACE.md` §1–§8；由 `tools/collabgate` 钉住）

| 面 | 锚点 |
|---|---|
| 唤醒载荷字段集 | `internal/boot/inbox_wake_contract_test.go:10` |
| 宿主只消费 `text` | `internal/boot/inbox_wake.go:157` |
| 外来引导来源具名集合 | `internal/control/inbox_wake_marker.go:13` |
| 拒绝分类头 + 三档 | `internal/serve/reject_class.go:11`、`:16`、`:19`、`:22` |
| inbox 条目字段集 / `state` 闭集 | `internal/sessioninbox/types.go:118`、`:32` |
| 「武装」声明处（空 = 零行为变化） | `internal/plugin/plugin.go:95` |
| 队列全局只读出口（`gate`） | `internal/control/inbox_query.go:193`，判定与回执共用 `internal/control/inbox_dispatch.go:234` |
| 「队列里没有」那一档 | `internal/serve/inbox.go:109` |

**9.2 本稿自证的缺口与读侧**（不来自 COLLAB-SURFACE）

| 面 | 锚点 |
|---|---|
| **无回执通道**（§7 要补的缺口原话） | `internal/serve/inbox.go:85` |
| 读侧投影层（自述纯观察、不实现第二套状态机） | `internal/taskmonitor/model.go:1-8` |
| inbox 按会话寻址 | `internal/serve/inbox.go:21`、`:153` |

**9.3 现状事实锚点**（跨工作区 / 跨进程 / 规模 / 无人值守 / 闸门；本稿逐条核实过）

| 面 | 锚点 |
|---|---|
| 会话按工作区分目录 | `internal/config/paths.go:462` |
| 桌面 tab 带 `WorkspaceRoot`（空 = 全局） | `desktop/tabs.go:72`、`desktop/app.go:759`、`:873`、`:1349` |
| 会话清单的 `all bool`（跨工作区开关） | `internal/serve/session_list.go:21` |
| 会话所有权路由（跨进程/跨宿主接管） | `internal/serve/serve.go:648-653` |
| 托管实例的 `Record` / 令牌 / 回环地址 | `internal/agentd/agentd.go:36-48`、`:41`（`TokenFile`）、`:49-50`（`URL()`） |
| **跨进程文件锁（真实原语）** | `internal/filelock/filelock.go`、`lock_windows.go:30`、`lock_unix.go`（使用者 `internal/workspacelease/lease.go`、`internal/agentd/registry.go`） |
| 原子替换写 | `internal/fileutil/atomicwrite.go:39`（允许 EXDEV 拷贝）、`:49`（仅重命名） |
| 会话租约锁侧车名（**旧设计引用；不是本稿的锁原语**） | `internal/agent/save.go:35` |
| 委派上限 64 | `internal/agent/fleet.go:22`、`internal/agent/parallel_tasks.go:84` |
| **并发闸是会话级**（无 host 级闸；嵌套 fail-fast） | `internal/agent/scheduler.go:36`、`internal/config/config.go:1304-1307` |
| provider 退避与 `Retry-After` | `internal/provider/retry.go:22` |
| **自建能力的现有通道**（skill 创建） | `internal/skill/tools.go:642`（`CreateWithContent`） |
| **现状没有**关键路径 / 优先级调度（本稿核实：`internal/agent/` 无对应符号） | `internal/agent/scheduler.go`（只有会话内并发上限） |
| turn-tail 注入点 | `internal/control/input.go:146`、`:188`、`:202` |
| 变异体字段数上限（闸门） | `tools/repolint/structstate.go:13` |
| 无人值守：唯一总开关、goal 契约、保活/自愈/交接/看门狗 | `docs/UNATTENDED.md` §1–§10；`desktop/heartbeat_converge.go:46` |

改 9.1 里的面之前先跑 `go test ./tools/collabgate/`；改任何面之前先跑 `go run ./tools/repolint`。

## 10 与既有两版文档的关系

| 既有内容 | 处置 | 理由 |
|---|---|---|
| design §3.3 言语行为集合 | **继承词表，改语义落点**（映射见 §3.4） | 词表已过审；但真相载体从消息换成黑板 |
| design §3.4「Contract Net + Blackboard」 | **本稿骨架**（不再后置） | 用户 2026-10-02 否决「单向分活」：没有共享工件与裁决就不算多智能体 |
| design §3.5「有界 propose/reject 表决」 | **继承语义，移到节点上**（§3.2 ③） | 表决从「facilitator 发起的一轮」变成「争议节点的固有状态」——因此不需要 facilitator 存在 |
| design §3.5 收敛优先级（预算 > 轮次/消息数上限 > quorum > 静默窗口） | **继承，两处改判** | 优先级次序未变；① quorum 换成「未决矛盾」；② 加节点级速率上限 |
| design §3.5 平局规则「最小权限默认值 / 保守方案胜出」（design.md:137） | **取代** | 本稿 §3.2 ③ 改为「**等重不掷硬币 → 升级给人**」，受升级配额约束（评审指出此条原先未处置——已补） |
| design §3.5「写冲突直接复用 `write_claims` 与 `<id>.jsonl.lease.lock`」 | **取代落点** | 那两个是**会话内**的写者锁；跨进程互斥改用 **board 级文件锁 `internal/filelock` + 逻辑租约**（§11.1） |
| design §2.2「Facilitator 是确定性 Go 引擎、无规划权、无 blackboard 写权限」 | **继承并强化** | 裁决是**规则**不是角色：`decide` 执行前置条件判定，不产生规划 |
| design §4.1 分层方向（不得让协作层 import control） | **继承并收紧**：`frontends → control → {agent, agentbus}`；`agentbus` 只 import 工具包 | `tools/repolint/layers.go:71-74`；`agentbus → agent` 这条链被**否**（评审指出原链缺环） |
| design §5.1 内核内实现、不引入 Python 运行时 | **继承**（独立 exe 只在"该协作涉及的宿主里不跑桌面/CLI"时才可能被需要，见 §编排落在哪） | 打包形态 + 复用既有原语 |
| design §1.3「v1 限定单进程」 | **取代** | 参与方可能同进程也可能跨进程（§跨工作区两条轴）；`internal/sessioninbox`（durable、按会话围栏、`SchemaVersion=2`）与 `internal/agentd`（每实例托管 serve + 注册表 + 令牌）已把 durable 投递与跨进程寻址落地 |
| design §6 缺陷 3「进程退出即丢在途队列，durable queue 列入 v2」 | **取代** | 同上：durable **已存在**，缺的是黑板与回执 |
| design §2.1「peer = collab host session 的子 agent」 | **取代**（不再作为唯一形态） | 本稿以**顶层会话**为智能体单位——执行发生在它自己的 Tab/Context；百级靠跨会话展开 |
| design §2.1「peer 是持久化子 agent 会话」 | **部分取代** | 身份改为节点制（§4）：`(scope, workspaceRoot, topicID)` + 会话 + 子 agent id |
| plan §3 P0–P7 的阶段划分与验收写法 | **继承**（编号不复用，见 §11） | 阶段/出口条件/回滚的写法已过审，只是对象换了 |

## 11 落地顺序与出口条件

> **顺序原则：先立骨架，再补手，最后再上量**。骨架是黑板（S1–S2，S2 必须**一开始就跨工作区**验证）；
> 交流面进 S3；审议进 S4；**规模三闸（host 级并发槽 / 四级预算 / 观测聚合）在 S5–S6**——它们只在 N 上去以后才咬，
> 但**必须在压测之前就位**（S7 是百级压测，先把闸装好再去撞它）。
> **无人值守不需要新阶段**：它是「编排会话绑到既有 heartbeat 任务」的接线（§编排落地），验收落在 S7。

| 阶段 | 内容 | 出口条件 |
|---|---|---|
| S1 | 黑板内核：op 日志 + 确定性 fold + **节点状态机（含 `capability_gap` / `abandon` / `revert`）** + 逐节点租约 + **心跳/回收**（**已落** `internal/agentbus/board`，提交 `1b40a2f1d`；规格见 §11.1） | 非法迁移拒收（**带原因**）；op 重放幂等；并发 op 折叠结果与串行一致；**杀掉推 op 的进程后，剩余 op 仍能折出完整状态**；**两个不同进程**并发写同一作用域不丢 op；**认领者被杀后节点在 deadline 内被回收并记 `no_progress`**；`revert` 后下游正确标 `stale`；**无证据的 `abandon`（与无证据的 `decide(done)`）被拒**；`seq` 跨进程单调不重号；**截断尾行不计入 op 且计数可见**；成环的 `split`/`require` 被拒；`agentbus` 不 import `agent`/`control` |
| S2 | 写侧接线 + 读侧投影（**含跨工作区与子树分片**）：多会话可写；**视图裁剪**（每轮只读我的子树+我的节点+订阅摘要）；读侧复用 `internal/taskcatalog` / `internal/taskmonitor` / 桌面任务树 | **两个不同工作区**的会话对同一黑板 `claim`/`assert`/`refute` 全部可见；**跨子树只经边界节点**；某 agent 的一轮请求里**不含别人的子树**（§8 第三条守卫）；视图**增量读**（cursor/seq）而非每轮全量；`make frontend-check` 过 |
| S3 | **交流三档**（§3.2）：① 自由对话 `say` + 话题边界；② 有界点对点 `ask`/`answer` + 回执（§7 缺口）；③ `results/<correlation>.json` | 话题轮数/预算/静默窗口任一触顶即收口；**自由对话默认不灌上下文**（只投点名与摘要）；`correlation` 的预算/TTL/hop 生效；跨进程目标带令牌；超速被拒时返回 `rate_limited`；**就绪节点用事件唤醒（`interval` 只兜底）** |
| S4 | **审议**与裁决：审议状态（参与者/轮次/必答/权重/冷却）+ `decide` 规则 + 人作为参与者 + **升级配额** | ① 一次 `refute` **改变**结论（可回放：同一 op log 折叠出不同结局）；② **等重升级给人**，不被规则拍死；③ 弃答以 `no_answer` 可见；④ 争议节点可由人在桌面下结论；⑤ **无可核对证据时 `done` 被拒**（只允许留 `assert`）；⑥ **连续 `refute` 触发冷却**，速率上限生效；⑦ 超升级配额时**自动降级为 `undecided-by-rule`** 并记账；⑧ **票数不改变权重**（consensus ≠ evidence） |
| S5 | **集群调度与预算**：**本机 host 级**并发槽 + 排队 + 四级预算（board → 子树 → 节点 → 回合）+ 触顶即暂停（不杀会话）+ **调度四则**（关键路径优先 / 批量领取 / 同子树亲和 / 最难优先） | 并发槽满时新回合**排队而非失败**（依赖 T2-4 定死的持久队列落点；**宿主级用例**见 `internal/control/agentbus_dispatch_slots_test.go`）；槽与预算在**本机**被 host 级共享（多会话/多进程同算一池；**跨机不在 v1**）；**槽位跟着活走**——持有者手上已无 `claim`（被 release/结算/sweep 掉）时，宿主在下次派发前收回其槽，否则宿主一满就永久停摆（`releaseIdleSlots`，2026-10-03）；触顶后**现场保留**、重启可续；`rate_limited` 与 provider 429 不再互相放大；**关键路径上的节点先跑**；**最难节点不被饿死**；**预算只被验收节点消耗**（做工不消耗总预算） |
| S6 | **观测聚合**（人读）：按子树折叠、只显异常/争议/停滞/孤儿、可下钻到节点 | 100 节点规模下首屏不画 >N 张卡片（阈值可配）；孤儿节点与停滞节点**必现**；下钻到节点能看到参与者、证据与 `no_progress` 记录 |
| S7 | e2e + **无人值守贯通** + 完成判定 + **N≈100 压测** | ① 杀掉编排者，未完成节点仍被其他参与者认领并推进；② 一次 `refute` 改变结果；③ 运行时依赖：节点/边数在运行后增加，且由参与者而非派发方新增；④ **无人值守贯通**：杀掉桌面进程 → 既有看门狗拉起 → 黑板继续被推进（`docs/UNATTENDED.md` §11 自认这条链还没真机验收，所以第一次必须端到端跑一次）；⑤ **任务级落地**：验收节点全 `done` + 无未决矛盾才宣告完成，否则停在 `blocked` 并保留现场；⑥ **百级压测**：N≈100 并发参与、任意杀掉 20% 后仍收敛；总支出不超预算；无 429 风暴；**存在一次"缺能力→派生获取能力节点→完成"的真实链路** |
| S8 | PR 元数据门 + 打包 | `verify-windows-portable.sh` exit 0；`Cache-impact`/`Cache-guard`/`System-prompt-review`/`Documentation-impact` 齐全 |

### 11.1 S1 规格（实现前定死；S1 实现以此为准）

**包与分层**：`internal/agentbus`（信封与投影，**已落**）+ `internal/agentbus/board`（S1 本体，**已落**）。
`board` **只 import 工具包**（`internal/fileutil`、`internal/filelock`）；**不得** import `internal/agent`、`internal/control`。
目录由调用方给出（`board.Open(dir)`），**本包不解析 state home**。

**节点状态闭集（8 个）**：`open` / `claimed` / `contested` / `done` / `blocked` / `abandoned` / `stale` / `capability_gap`。
`ready`（自身 `open` 且所有依赖 `done`）与 `expired`（`claimed` 且 `now > deadline`）是**派生谓词，不落状态**。

**迁移表**（不在表内的组合**一律拒收并带原因**；`-` = 状态不变）：

| 动词 | 允许的前置状态 | 结果 |
|---|---|---|
| `claim` | `open`（且无未过期的他人租约） | `claimed` + owner/deadline/定界 |
| `heartbeat` | `claimed` 且 owner 是本人 | `claimed`（deadline 延长） |
| `release` / `yield` | `claimed` 且 owner 是本人 | `open` |
| `assert` | `open` / `claimed` / `contested` | `-`（记录断言与证据；须带证据引用）；若该节点已有 `refute` ⇒ `contested` |
| `refute` | 除 `abandoned` / `stale` 外任意 | `contested` |
| `split` | 除 `done` / `abandoned` 外任意 | 子节点 `open`；父节点 `blocked`（建 `parent dependsOn child`） |
| `require` | 除 `done` / `abandoned` 外任意 | 依赖节点 `open`（按给定 spec 创建，或引用既有）；本节点 `blocked`（建 `node dependsOn dep`） |
| `capability_gap` | `open` / `claimed` | `capability_gap`（须带「要什么 / 试过什么 / 为何不行」） |
| `abandon`（请求） | 除 `done` / `abandoned` 外任意 | `-`（请求记入；**须带证据，无证据拒收**） |
| `decide`(outcome=`done`) | 除 `abandoned` / `stale` 外任意 | `done`（须 ≥1 条带证据 `assert`，**且** `reproduced_by` 不是产出者） |
| `decide`(outcome=`blocked`) | `open` / `claimed` / `contested` | `blocked` |
| `decide`(outcome=`abandoned`) | 该节点存在带证据的 `abandon` 请求 | `abandoned` |
| `revert` | `done` | `open`，且**传递下游**中的 `done` ⇒ `stale` |
| `no_progress`（系统，非人工） | `claimed` 且租约已过期 | `open`（记 `no_progress`；同 `(node, deadline)` 幂等） |

**边模型**：只有一种边 `node dependsOn dep`。`split` 建 `parent dependsOn child`；`require` 建 `node dependsOn dep`。
**加边前查环，成环即拒收**。`revert` 的"下游" = 沿 `dependsOn` **反向**可达闭包（谁依赖它）。

**seq、幂等与互斥**：
- `seq` 在**持有日志锁期间**由"当前末条 `seq` + 1"分配 ⇒ 跨进程单调不重号；`<board>/#seq` 即稳定地址。
- op 幂等键 = `op.id`（缺省由 `(verb, node, actor, payload 哈希)` 生成）；**同 id 重复提交直接返回首次结果，不追加第二条**。
- 写事务 = `读状态 → 校验 → 追加` **全程持 board 级文件锁**（`internal/filelock`）⇒「两个不同进程并发写不丢 op」成立。
- **租约只表达"谁能推进"，不表达"谁能写日志"**（后者由文件锁管）——所以「逐节点单写者」与「多写者按 op 追加」不冲突（评审第 3 条缺口就此消解）。

**时钟**：deadline / 心跳 / TTL 全用**墙钟 UTC**（跨进程唯一可比），判定只用 `now > deadline`，不依赖瞬时精度；
**`now` 由调用方注入**（`Fold(ops, now)` 是纯函数），测试用注入时钟。
**跨机时钟假设（2026-10-04 补一句，把隐性依赖写明）**：**写者本地钟是权威** —— 每条 op 的 `At`/`Deadline` 由写它的那台机器给出，判定只用"读的那台机器的 `now` 比记录的 `deadline` 晚"，因此两机偏差的后果是**回收的早晚**，不是错结论：偏快的宿主会把别人的租约**提前**回收（同一份工作被再取一次 ⇒ 多一次重试），偏慢的宿主写的租约在别人眼里更"短"。⇒ 只要各机钟差**远小于**租约/心跳量级（30s 心跳、分钟级租约），最坏就是多一次回收重试；若钟差接近租约长度，才会看到"工作被反复重取"。链条其余部分不受影响（`Fold` 不读时钟，同一份日志永远折出同一状态）。

**日志与损坏行**：每行一条 JSON + `\n`，**只追加、从不改写**。
**结尾半行（无 `\n`）= 未提交的写，不算 op**；文件中段解析失败的行跳过并**计数**（`Truncated` / `Skipped` 可见）。
**不静默、不中断启动**——静默跳过会毁掉"重放幂等"的可信度，所以计数必须能被读到。

**过期扫描者**：**任何**参与者在自己的写事务开头顺手 `Sweep(now)`（**无新守护进程**）；`no_progress` 幂等。

### 11.3 实现期修正（2026-10-02，S1 落地时发现；**覆盖 §11.1 的对应行**）

实现 S1 时发现五处规格要收紧，本节是这些点的权威版本：

1. **`Fold` 不读时钟**（覆盖「时钟」段）：重放必须是日志的函数。`Fold(ops, now)` 被否——签名是 `Fold(ops []Op) *State`；时限的"未来性"是**写时守卫**（`validateFreshness`，只在 `Apply` 里跑，不进日志、不进 fold）。否则同一份日志在不同时刻折出不同状态——实现时被 `Sweep` 测试抓到：claim 写时合法、读时被判非法，整棵树回退。
2. **`claim` 的接管判据在日志内比较**（覆盖迁移表 `claim` 行）：允许 `open`，或 `claimed` 且**该 op 自己的 `At` 晚于当前租约的 `Deadline`**（两个时间都取自日志）⇒ 可重放；"对方租约此刻是否过期"只在写时判定。
3. **形状校验先于状态校验**：动词必填项（证据 / 理由 / deadline / bounds / children / dep / outcome）由 `validateOpShape` 在任何节点查找之前判——理由才可行动（"缺 bounds" 比 "unknown_node" 有用），且与节点状态无关。
4. **截断尾行要在下一次写事务里修掉**（覆盖「日志与损坏行」段）：只跳过不够——新纪录会追加在那半行之后、被埋在坏行里。`appendOp` 先 `repairTornTail`（截到最后一条完整行）再写；修不动（尾行超过 64 KiB 窗口）**报错，不静默丢**。
5. **派生谓词的边界**：`ready` 允许 `open` **或** `blocked`——`blocked` 兼有"等依赖"与"被裁决阻塞"两义，用 `Outcome != blocked` 区分；split 出的容器在子节点全部完成后重新 ready（组装它们本身就是它的工作）。`revert` 把下游 `done` 变 `stale` 时**同时清掉 Outcome**（stale 不是结论）。
6. **`decide(done)` 不设依赖门**（澄清迁移表 `decide` 行）：`done` 是**裁决**，不是"装配确认"——依赖门只在 `claim` 上（`blocked` 须依赖全 `done`）；"容器先于子节点被裁决"由**落地判定**兜住：`AssessLanding` 把未完成的依赖报成 `not_done`，verdict 不会绿。落地时真撞到一次（先给容器 `decide(done)`，板面立刻报 `not landed`），故在此写死：这条不是待修的缺口，而是规格本身（`internal/agentbus/decide_deps_test.go` 钉住）。

### 11.5 编排期修正（2026-10-03，agentbus 编排落地时发现）

对着**正在跑的看板**（不是文字）发现三点，都是"用起来才知道"的语义：

1. **`require` 不随 `split` 传给子节点**：容器上的依赖只管容器自己的收口。拆出的子节点必须各自声明依赖，否则调度会认为它已经能开工——真机上 `ab-participant-ttl`（依赖 headless 重申）因此被立刻派给了一个无法开工的参与者，而它自己 `deps_open=0`。**推论**：`split` 之后要把"为什么这个子节点还不能动"重新记在子节点上，不然那条依赖在调度面上等于不存在。
2. **`split` 后的父节点就是容器**：子节点未完成前它即 `blocked`、`deps_open` 等于未完成的子节点数；它自己不需要额外处置，子节点全部 `done` 即收口。落地时核对过一次（`ab-participant-freshness` 在两个孩子一 `done` 一 `blocked` 时正是这个形状）。
3. **唤醒的清单是入队时刻的快照**：唤醒在**入队**时被渲染成固定文本（`InboxRequest` 的 `Submit`/`Raw`），而**注入要等当前回合结束**——真机两例延迟 3–5 分钟（内容构建于 ≤11:27:32Z、≥11:31:52Z 才被读到；另一例构建于 11:28:40–11:30:30Z）。模型照过期清单行动不会写错（`claim` 有依赖门），但会白费一轮并把注意力引向已经不能开工的节点。**修法**：注入前按 `wakeKey` 重算（`agentbus.WakeTargets` 已导出）——该 key 不再出现即说明理由已不成立，天然等价于"拒绝旧清单"；落点可直接复用 `sessioninbox.Store.UpdateItem`（steer 流程已在用）对待处理条目原地改写，因此**宿主在注入前刷新**就够了，渲染文本已归位 `control.AgentBusWakePrompt` 供两处共用。**已落地**：`internal/control/agentbus_wake_inject.go`（注入时重建，旧清单改为显式拒绝）。
4. **`heartbeat` 内核支持、工具面已暴露，但宿主从不调它**：`applyHeartbeat`（`board/node.go:344`）允许**所有者**续租（新 deadline 直接覆盖），模型工具面也列了 `heartbeat`（`leaseSeconds` 默认 900 秒）。缺口不在内核而在调用——`desktop/agentbus*.go` 零命中 ⇒ 宿主派活写下的长租约（编排期取 30 分钟）在会话不主动心跳时只能等过期回收：真机上两条派活租约被认领后 **27 分钟零 op**，这就是它的形状。**修法**：① 在被派活者的唤醒块里教它"做久了就 `heartbeat` 续租"（已做）；② 若要宿主代劳，需记住自己写过的 claim（或从板面按 `Owner == 本会话` 反查）——但**不能无条件续租**，那等于让闲置会话永久占位，比过期回收更糟。
5. **`split` 出的新子节点会被派活器立刻认领**：宿主每 30 秒按"可开工"派活，而 `split` 刚创建的节点没有租约、派活器认为它可开工 ⇒ 拆分者若不立刻认领，节点会在半分钟内落到别的参与者名下。真机上把 `ab-headless-tick` 拆成 tick/waker 两半后，tick 半在 30 秒内被派给了 peer，而代码已经在写。**做法**：`split` 的下一条 op 就该是 `claim`；若要"先写后拆"，就别拆（在容器上说明分工）。
6. **同仓多写者：`git add <文件>` 会连带别人的未提交 hunk**：两个会话同时在写同一仓库，而 git 的暂存是**整文件**语义——真机上我 `git add desktop/agentbus_waker.go` 提交自己的重构时，把另一会话在**同一文件**里的四处未提交 hunk 一起提交了；提交前我只看了 `git diff --stat`，没逐 hunk 看。后果：那条提交**不能单独编译**（它引用的 config 字段还没提交）。**做法**：① 提交前 `git diff <文件>`（而不是 `--stat`）逐 hunk 看一眼有没有外来改动；② 别改写别人工作树所基于的历史——内容不变时可以只改提交信息；③ 一旦带错，**如实写进提交信息**并请对方补齐剩余文件，比悄悄重写安全。
7. **审议的入口在会话面，而"未答者被唤醒"被 `RoundTTL` 闸住**：`agent_bus` 工具新增 `hearing_open`/`hearing_answer`/`hearing_settle`（`BoardPort` 三方法 → `internal/boot` 转调 `control`），且 `Open`/`Answer`/`SettleAgentBusHearing` 成功写入后按与写板同一条规则调 `WakeAgentBus` —— 这是 `WakeTarget.Owes` 第一次有**生产发起者**（此前唤醒面算得出来，却没有任何一条生产路径去问）。**但**：`HearingSilent` 第一行就是 `lim.RoundTTL <= 0 ⇒ nil`，而 `SetAgentBusHearingLimits` 全仓**零生产调用方** ⇒ 默认配置下这条唤醒**永远不会成立**。⇒ 入口不等于通路：操作者必须能给审议设界（与 §13.7 同族：宿主不发明天花板，但也不能让它默认死在 0）。

### 11.4 第二轮评审（对着 S1 代码）的处置（2026-10-02）

第二轮改看**代码**（不是文字），抓到 17 条：已修 10 条、明记为边界 3 条、其余同类归并。对本稿的**规格修正**：

6. **可推进的集合放宽到"能开工"**（覆盖迁移表 `assert` / `claim` 行 + §11.3 第 5 条）：`claim` 接受 `open`、`capability_gap`，以及**依赖全部 `done` 的 `blocked`**（与"被裁决阻塞"用 `Outcome != blocked` 区分）；`assert` 只拒绝 `done` / `abandoned` / `stale`。原表把 `blocked` 与 `capability_gap` 写成不可推进 ⇒ 容器与被依赖节点在依赖满足后**没有入口**，`capability_gap` 成了死胡同。
7. **`revert` 也接受 `stale`**（覆盖迁移表 `revert` 行）：`revert` 可把 `done` **或** `stale` 打回 `open`（否则 `stale` 没有出口）；下游中的 `done` 仍标 `stale`。
8. **`Sweep` 是显式调用，不藏在 `Apply` 里**（覆盖「过期扫描者」段）：原写"任何参与者在写事务开头顺手 Sweep"——实现为**显式 `Sweep(now)`**，由宿主/调度器每 tick 调一次。理由：若 `Apply` 隐式代跑，一次写会追加多条 op，回执里的 `seq` 就不再诚实。
9. **`no_progress` 只能由 `Sweep` 写入**：`Apply` 直接拒它（`system_only`）——否则任何调用方都能伪造系统回收记录。
10. **幂等键不含时间**：`DeriveID` 的载荷排除 `At`（重试几小时后仍是同一意图）；同 id 但**意图不同**的写入不再静默回 `Duplicate`，而是拒收（`idempotency_conflict`）。
11. **`split` 同一 op 内重复 child id 拒收**（`duplicate_dependency`）；**`require` 拒绝指向 `abandoned` 的依赖**（`dependency_closed`）——否则会造出永远不可能 ready 的节点。
12. **读锁也有预算**：`Snapshot` 的共享锁与写锁同样受 `defaultLockWait` 约束（原实现读方可能无限阻塞）。
13. **尾部修复要能跨窗口**：`repairTornTail` 在 64 KiB 窗口里找不到 `\n` 时继续向前扫，直到文件头（整个文件是半行 ⇒ 截到 0）；不再把"窗外的半行"当成不可修错误。

**明记为边界（不在 S1 修，已在 TODO 挂节点）**：① 写路径每次全量读 + 全量 fold（O(n)/写，总 O(n²)）⇒ S2 用增量 fold/游标替换并带实测预算（TODO T4-8）。**已闭合（2026-10-02）**：读、写两条路径都接入按句柄的大小校验缓存（`foldedState`），实测 1600 条 op 时写 **6.95 → 1.11 ms**、读 **5.90 → 1.43 ms**；剩余随 n 增长的部分是"命中返回副本"的 O(n) 拷贝，要连它一起去掉需增量 fold / 写时复制；② `evidence` 只有"`Ref` 非空"这一条约束，形状与唯一性留给 S3/S4 的证据权重设计（TODO T5-6）。**已定（2026-10-02）**：权重按**引用去重**计（每个 `Ref` 只计一次，空白不算引用）⇒ "同一条 test 引用三遍"仍只是一条；形状与唯一性**不额外约束**（谁引用、引用什么由参与者负责，内核只回答"这条能不能去核对"）；③ `Sweep` 单次上限 256，节点按 id 排序故无永久饥饿（TODO T7-5）。**此处写明该规则**：单次上限只约束"一次调用的写入量"，不约束"板能否恢复"——被跳过的过期认领由**下一次调用**按 id 序继续回收，因此不存在永久饥饿；实测见 `internal/agentbus/board/sweep_cap_test.go`（一次 256 + 一次余量 + 一次 0，且全部 owner 清空）。

### 11.2 对抗评审留痕（2026-10-02，独立子智能体，只读）

结论：**不能直接开工**——须先补 S1 规格（已补为 §11.1）并消解范围冲突。已并入本稿的：

1. **锚点错误（3 处，逐条实测核对后修）**：`internal/agentd/agentd.go:15` 是 `RegistrySchemaVersion`，地址与令牌在 `:41`/`:49-50`；`/mirror-end` 在 `internal/serve/serve.go:653`（原写 `:648-652`，漏一格）；租约原语应锚 `internal/filelock`，不是 `internal/agent/save.go:35`（那只是侧车后缀常量，且是会话内的锁）。
2. **自相矛盾（3 处）**：design §3.5 的**平局规则**原先未处置 ⇒ 补「取代」行；「收敛优先级」一格的"未受本稿影响"与同格自述冲突 ⇒ 改为"两处改判"；S5 的"多机同算一池"与 §现实边界"v1 单机"冲突 ⇒ 收敛为"**本机 host 级**；跨机不在 v1"，并把持久队列列为 TODO T2-4。
3. **分层链缺环**：`agentbus → agent` 被否 ⇒ `agentbus` 只 import 工具包，Scheduler/Grant 接线留在 control/host（§编排落在哪、§10）。
4. **S1 规格缺口（8 条）**：状态闭集与迁移表、`seq` 分配、op 幂等键、租约 vs append 的互斥粒度、时钟来源、损坏行边界、过期扫描者、`revert` 下游闭包——**全部落为 §11.1**。

**未消解、留给 T2 的五条**（落地到对应阶段时**不得默认**）：视图/摘要的**廉价物化**（S2 成本模型）、
**就绪事件由谁发**（S3/S5 接缝）、**四级预算与既有旋钮**谁统谁（超限后 Goal 变 `blocked` 还是 `complete`）、
**并发槽的持久队列落点**（inbox 是会话级且嵌套 fail-fast）、**能力的授权链**（缺能力要装依赖/改仓库时谁批）。

每阶段末：`gofmt -w .` → `go vet ./...` → `make lint`（含 `repolint`，会查**注释长度**与
**变异体标量字段 ≤12**，`tools/repolint/structstate.go:13`；**不许放宽 baseline**）→
`go test ./internal/tool/builtin/ ./internal/boot/`。

## 12 反例清单（怎么读算错）

- **把「多智能体」读成「消息多」**：消息多不是本事；**共享工件 + 反驳 + 裁决**才是。只有消息没有黑板 = 作业派发器换了身衣服（本稿 v1 曾犯此错）。
- **把「指数复杂度」当成"不可设计"**：真实增长是**超线性**，只有全互联通信是 O(N²)；控制点在**视图**与**并发**，不在 agent 数（§集群规模）。
- **把 100 个 agent 当 100 倍能力**：它是 100 倍**上下文成本**的风险；能力来自分解成树 + 视图裁剪 + 可核对证据（§集群规模）。
- **让每个 agent 读全板**：视图必须是「我的子树 + 我的节点 + 订阅摘要」；读全板 = 每轮 O(N²)（§集群规模、§8 守卫三）。
- **没有 host 级并发槽就拉百个**：现有旋钮是**每会话**一档、嵌套还 fail-fast，而模型 rate limit 是全账号共享 ⇒ 互相 429（§集群规模）。
- **把「没能力」当「做不了」而静默停手**：缺能力是一等状态 `capability_gap`；`abandon` **要 `decide` 批准 + 带证据**，否则不许停（§失败模式、§11.1）。
- **以为「已 `done` 就不能改」**：`revert` 是必备——错误会放大，下游要标 `stale`（§失败模式、§11.1）。
- **把「大家都同意」当证据**：consensus ≠ evidence；**票数不改变权重**（§失败模式）。
- **让 `interval` 当唯一触发**：**就绪即事件唤醒**，`interval` 只兜底；否则百级下大半时间在空转（§失败模式）。
- **把预算绑在做工而不是验收上**：做工多 ≠ 落地；**预算只被验收节点消耗**（§失败模式）。
- **把对话当真相载体**：对话只有在变成黑板 `assert`/`refute` 时才进入结论；转录不是结论（§3.2）。
- **把「自由对话」当成本稿禁止的东西**：本稿禁的是"让对话承载真相"，**不是**不让 agent 说话——自由对话是一等能力（§3.2 ①）。
- **把「审议」当自由聊天**：审议有参与者集合、轮次上限、截止、冷却与 `no_answer`；无界的自由聊天不是审议（§3.2 ③）。
- **把自由对话全量灌进每个参与者的上下文**：默认只投「点名给你的行 + 每轮摘要」；全量塞上下文是成本炸弹（§3.2）。
- **把「节点 `done`」当「任务落地」**：节点全 `done` 但仍有未决矛盾 = **未落地**（§什么叫「落地」）。
- **把「模型自评」当验收证据**：权重与完成依据都不得来自模型自评；`done` 只认可核对证据 + 非产出者复跑（同上）。
- **让 agent 读 `/events` 做决策**：那是人读面，也是维度分离的红线（§8 R2）。
- **把 `<board>` 当 workspace**：黑板落进 `<state home>/projects/<slug>/` 就等于把协作绑死在一个工作区上（§5）。
- **把「跨工作区」当成「必然跨进程」**：桌面多 tab 是**同进程、多 controller**（各带自己的 `WorkspaceRoot`）；托管 serve / remote / CLI 才是跨进程。两条轴都要覆盖（§跨工作区）。
- **把 `board` 写成 `scope`**：仓库里 `Scope` 已另有所指（`desktop/app.go:759`）；撞名会让后来的人把一个当另一个读（§2）。
- **把编排当成第四个前端或新守护进程**：编排是作用域里的参与者，唤醒复用既有 `heartbeat` / inbox 唤醒（§编排落在哪）。
- **把「v1 单机」读成「永远单机」或反过来**：协议（地址 + 令牌 + 文件锁语义）保持可换目标，但**跨机同算一池不在 v1**（§集群规模、§11 S5）。
- **拿会话内的写者锁当跨进程锁**：`internal/agent/save.go:35` 与 `write_claims` 都是会话内的；跨进程互斥是 `internal/filelock`（§10、§11.1）。
- **把"跳过坏行"当无代价**：截断尾行不算 op，中段坏行跳过**必须计数可见**——静默跳过会毁掉重放的可信度（§11.1）。
- 把本稿当已实现面读：§2–§5 的「定义处」在落地前是**目标**，不是指针。
- 拿 §2 的信封去解析人读正文：正文是投影，字段是契约，两者永不互换（§1 反例一）。
- 把 `GET /inbox/room-line` 当 reply：它答「这一行现在怎么了」，不答「谁回了你什么」（§7）。
- 把 durable **投递**读成 durable **编排**：落盘的是消息，不是「节点被谁认领、跑到哪一步」——后者是黑板（§3.1）。
- 把「独立通道」当成目的：隔离只要求 R1/R3（§1 注），多一条管道不是成绩。
- 把「多写者」读成「每次都要竞价」：同一节点单写者；只有 ≥2 候选者才走 `bid`/`award`（§3.3）。
- 把「有界」当成可以后补的优化：预算/TTL/hop/轮次/速率是**成立条件**——没有它，自由对话、反驳、审议与直问都会变成无限对话（§产品判据）。

## 13 视图与唤醒规格（S2 前置：T2-1 / T2-2）

### 13.1 视图：谁生成、多大、多新鲜（T2-1）

**谁生成**：内核（`control`）从**折出的黑板状态**生成，不是每个参与者各自去读日志。参与者只能通过工具或
turn-tail 注入看到视图；**没有直接读 `board.jsonl` 的通道**——否则"视图裁剪"会退化成人人自觉的君子协定。

**视图是同一份 fold 的三个投影**（不新增任何存储）：

| 面向 | 形状 | 大小上限 | 新鲜度 |
|---|---|---|---|
| 智能体（机读） | `agentbus.View`：固定头（`schema` / `board` / `participant`）+ **自游标起的增量行** | 默认 ≤ 8 KiB/轮、行数 ≤ 200 | 每轮拉一次（游标 = `seq`），**不重发历史**；`action=view` 的窥视不推进游标，**增量为空且该参与者仍有工作时回落到从头读当前工作集**——否则被唤醒那一轮消耗掉的 delta 就再也复核不到 |
| 人（桌面 / CLI） | 既有任务树事件（`agent.*` 事件族）+ 节点行 | 复用现有面板阈值 | 沿用既有 5 s 轮询，**不新增轮询** |
| 编排者 | 同智能体视图，scope 覆盖整棵子树 + 全局计数 | 同上，另加 ≤ 1 KiB 计数头 | 同上 |

**行字段集**（机读行；顺序固定、只追加）：`id` / `title`（截 120 字符）/ `state` / `outcome` / `owner` /
`deadline`（RFC3339 或空）/ `startable`(bool；= `Node.Ready`，不是"依赖已完成") / `deps_open`(int) / `evidence`(int) / `refuted`(bool) /
`no_progress`(int) / `last_seq`。

**「我的子树」怎么廉价物化**（T2-1 的关键）：**不做逐参与者索引**。每轮 fold 一次（一次 O(nodes) 遍历）后用
三条集合运算裁剪：① 我拥有、我**被指派**（`assign` 的 `assignee`）、或我断言过的节点（`owned`）；② 反向可达闭包里**非 `done`/`abandoned`** 的节点
——谁在等我（`waiting`）；③ 我自己的节点**正向可达**里非 `done`/`abandoned` 的节点——我在等谁（`needed`；
`stale` 也算，它正是"底下的东西变了"）。第三条是容器能开工的前提：看不见自己卡在哪个子节点，等于没有视图；
第①条里的**「被指派」是 2026-10-05 真机补上的** —— 唤醒词对承办者说"这块归你"，而视图三组都不认它，
被唤醒的会话于是读到一片空白（`internal/agentbus/view.go` 的 `participantNodes`，用例 `view_assignee_test.go`）。
N≈100、节点数百量级时这是几百次访问的过滤，比维护逐参与者索引的失效成本低一个量级。

**交付顺序按 `last_seq` 升序**（不是按状态优先级）：游标就是 `last_seq`，按优先级挑选会让"低优先级的旧行"
永远拿不到——那正是静默丢。优先级排序只用在**人读面**（每次都是全量快照，没有游标）。
超上限时头部给出 `owned`/`waiting`/`needed`/`ready`/`truncated` 计数与"用游标继续"的指针——**不许静默丢**
（与 §11.1「坏行必须计数」同一条纪律）。

**前缀缓存稳定性**：头部（`schema`/`board`/`participant`）逐轮**字节不变**；正文只追加更大的 `seq`；
**绝不把"最新摘要"提到头部**（§失败模式点名的缓存杀手）。

### 13.2 「就绪」事件由谁发（T2-2）

**主路径：由让依赖变成 `done` 的那个写入者发。** 同一次写事务里（`decide(done)` 落板后）算一遍"新就绪"集合
= 因这次完成而依赖全满足的节点，然后向**这些节点的子树**各发一次唤醒（`POST /inbox/items`，带 agentbus
来源标记）。写入者手上有刚折出的状态，判定是纯函数，不需要第二个扫描者。

**兜底：宿主每 tick 扫一次**（`Sweep(now)` + 一次就绪扫描），覆盖"写成功但唤醒没发出去"的崩溃窗口。
兜底**幂等**：唤醒的语义是"给你一轮"，参与者醒来看到的是**当下视图**，重复唤醒只多一次多余回合，
不会重复做事。

**防风暴**：每个参与者每 tick **最多一条**唤醒，批量携带该 tick 内全部就绪节点（不是一节点一条）；
并发上限与排队规则归 §11 的 S5。

**本节的关闭情况**：T2-1 ✓、T2-2 ✓；T2-3（预算与既有旋钮）/T2-4（并发槽持久队列）/T2-5（能力授权链）
仍在其对应阶段开工前处理（见 `docs/agents/TODO.md`）。

### 13.3 唤醒规格（实现修正，2026-10-02）

**规格原文的缺口**：§13.2 写「节点变 ready 时向这些节点的所属子树发一次唤醒」。实现时发现**折出状态里没有"谁要这个节点"**：只有 `claim` 会写 `Owner`，而 `claim` 只接受依赖已满足的节点，于是"有主人且还在等"的目标不存在；未认领的 ready 节点则**没有任何字段记录谁提出过它**。

**因此落地的唤醒目标（2026-10-03 起为三类；第一、三类读折出状态）**：

1. **提出者**：某条 `require` 提出的节点当前 `Ready` 且仍无人认领 ⇒ 唤醒该 `require` 的 `Actor`（这正是"那一步可以开工了"该被告知的人）。要求它的节点若已 `done`，则不发——这份唤醒已经没有意义。
2. **被点名的未答问题**：链上只有一次 hop（= 问了没答）⇒ 唤醒 `To`（或唯一 `Mention`）。
3. **被指派的人**：节点带 `Assignee`、当前 `Ready`、仍无主 ⇒ 唤醒该 `Assignee`（`WakeTarget.Assigned`），**不管有没有人在等它**。
   这不是第 1 类：「可以开工」与「这是你的活、别人不许取」是两件事（§13.4 的 `assign`），所以唤醒词必须说清归属，
   否则被点名的人会把它当成池子里的活而以为别人会做。

**纪律**：

- **不唤醒自己**：本会话下一回合自己就带着工作；唤醒自己等于把工作排在产生它的那一回合之后。
- **唤醒 key 由工作集合派生**（sha256，不含时间）⇒ 宿主每个 tick 都可以重跑而**幂等**；只有工作集合变化才再唤醒一次。
- **唤醒失败释放 key**，下一个 tick 重试，不静默吞掉。
- **路由是宿主的责任**：内核只决定"谁、为什么"；`SetAgentBusWaker` 由宿主安装（同进程兄弟会话的 inbox、别的 tab、别的机器），未安装 = 谁都不唤醒（安全默认），宿主自己的 tick 扫描仍是兜底。
- **已补（2026-10-02，T5-6）**：`require` / `assert` / `split` 现在都在**它们创建的那个节点**上记录 `Requester`
  （`board.Node.Requesters []string`，去重且保序）⇒ "谁要这个节点"**回到折出状态** ✓，不再只能从 op 轨迹派生。
  它是**纯派生**的（折叠时读 op 的 `Actor`）⇒ 盘上 schema 不变、旧日志折出来同样有值 ✓。
  **唤醒面已随之前移（2026-10-02）**：`WakeTargets` 现在只读折叠态（`WakeInput.State`），规则 = "能开工、仍无主、仍有节点在等它"
  ⇒ 唤醒它自己的 `Requesters`，`Waiting` 由反向边扫描得出；一处**有意加宽**：`assert`/`split` 的创建者也会被唤醒（它们同样"要"它 ✓）。
  届时 §13.2 的"所属子树"规则可以按 `Requester` 直接派生。
- **已补（第二个真缺口，2026-10-02 发现、此后落地）**：**唤醒目标没有地址**。`WakeTarget` 只有 `Participant`，没有 host、也没有 session path；
  而跨进程投递必须**寻址**——serve 的 `/inbox/items` 若不带 session header 就落进该宿主**前台**（它自己会为此告警），
  带错地址则被 `target_unreachable` 拒。也就是说"带令牌投递给持有者"在当初的数据结构下**不可能**：
  桌面能在本进程按 `AgentBusParticipant()` 找到 tab，但**没有任何一张表回答"这个 participant 在哪个宿主、哪个 session"**。
  **落地形态**：板级 `participants.jsonl`（participant → host base URL + session path + 令牌文件引用），各宿主在自己入列时写入；
  投递原语上提内核（`agentbus.DeliverWake`，含**15s 上限**与宿主 ctx）⇒ serve 侧"本地优先、否则查地址簿投递"，无地址仍是拒绝（`26c1a365f`、`6fac46ceb`）。
  投递失败**可读**：按参与者计入 host 读数 `AgentBusWakeFailures`，协作面板有一行（`d2081d750`，判据见 §13.17）。

### 13.4 并发槽与队列落点（T2-4 定案，2026-10-02）

**问题**：T7-1 要求"槽满**排队**而非失败"，而既有 inbox 是**会话级**且嵌套 fail-fast；槽满排队需要一个**持久、跨进程可领取**的队列。

**定案**：队列落在**黑板目录内**（`<board>/queue.jsonl`），与 `board.jsonl` / `messages.jsonl` / `hearings.jsonl` 同层，复用同一个 `internal/agentbus/jsonl` 底座 + `filelock`：

- 槽位是**本机 host 级**（`Ledger` 的 slot 计数；跨机不在 v1），**队列是板级**——这样"排队"跨进程可领取，"槽位"仍是本机的。
- 入队 = 追加一条 `QueueEntry{Node, Subtree, Participant, Key, EnqueuedAt, Seq}`（字段与顺序见 `internal/agentbus/queue.go:51-58`；`Seq` 的铸号见下方实现细节）；领取 = 读队列 + 抢租约（复用 S1 的 claim 语义），**不新增并发原语**。
- 幂等键与唤醒一致：由**工作集合**派生，不含时间。
- 顺序先按 `EnqueuedAt`、**同刻按 `Seq`** 升序（`internal/agentbus/queue.go:149-152`：先比时间，相等再比序号），避免静默饿死；T7-3 的"关键路径优先"只影响**排序建议**，不改变 FIFO 可见性。

**排序建议（T7-3，2026-10-02 落地）**：`Rank` 的比较链是"**同子树亲和 → 关键路径（传递依赖数）→ 最难（声明步数）→ 到达顺序**"；最后一级用**稳定排序**回落到到达序，
所以建议**只会重排**（集合不增不减，任何一项都不会因建议而不可见）。`TakeRanked` 用建议选批次，队列文件与 `Next` 的可见顺序仍是到达序。

**预算触顶的语义**：`Ledger` 的拒绝是"**现在不行**"（`IsBudgetReject` 给出层级），不是失败——调用方**停手但保留现场**（节点保持状态、租约与工作），由队列与唤醒负责之后恢复。内核里**没有**"预算耗尽即放弃"这条路。

**实现细节（2026-10-02 落地后补）**：入队幂等由**节点身份**保证——同一节点重复入队返回 `duplicate` 而不追加；已被 `drop` 的节点不会被再次入队；
`seq` 由日志的**全部**记录（含 `claim`/`drop`）决定下一号，否则领取过的板会重复铸号。
槽位是**每参与者一格**（`Ledger.AcquireSlot(holder)` 对同一 holder 幂等），因此"批量领取"不额外占槽，"宿主已满"只挡新参与者。
领取**先读队列、后花槽**（`Take`/`TakeRanked`，2026-10-04）：没有可领的活时既不占槽、也不返回拒绝 ⇒「宿主已满」只对**真有活要交出去**的
claimant 成立，宿主那行按层拒绝计数（G3 / T12-3）里的 `slots` 因此只记真正被挡下的活，而不是每个 tick 一条。

### 13.5 观测投影（S6/T8，2026-10-02）

内核只投影**该看的东西**，供人读首屏消费：`Observe(state, queue, hearings, now, limits) *Briefing`。

- **只给"出事的子树"画卡片**：健康的子树不进卡片，只计入 `HealthySubtrees`——一屏画全部等于什么都没说。
- **信号封闭集**：`orphan`（依赖已消失或已被放弃 ⇒ 永远无法开工）/ `stalled`（租约已过期、已被记 `no_progress`，或**已指派给某人而对方迟迟未取**——见 §13.4 的 `assign`）/
  `escalated`（审议升级给人）/ `disputed`（有 refute 无判决，或正在审议）/ `undecided`（按规则收口，**谁都不许当它已定论**）。
- **`orphan` 与 `stalled` 是必现信号**（`SignalKind.Mandatory()`）：`MaxSignals` 只裁可选信号，被裁的**计数**（`Hidden`），不静默消失。
- **排队不是信号**：等槽位是常态，只作为卡片上的计数（`Parked`）；但「指派了却没人来取」不是等槽位——被点名的人没到，且别人不许取，按 `stalled` 报出（窗口 `ObserveLimits.AssignedWait`，默认 10 分钟）。
- 排序：先按最严重信号，再按信号条数（出的事多的子树先出屏），最后按子树名；卡片超限计入 `HiddenCards`。
- 投影**只读**：不写任何面，也不改变任何状态。

### 13.6 能力授权链（T2-5 定案，2026-10-02，用户拍板）

**问题**：缺能力时派生「获取能力」子节点，子节点拿到的权限怎么定——取既有 `CapabilityGrant` 的**交集**，还是**发新授权**？

**定案（用户选择）：一律发新授权**。派生出的子节点携带**自己那一份显式授权**（装依赖、改仓库都算在内），由编排者/参与者判定并**记入黑板**；
把关**从"事前取交集"移到"事后证据 + 审议"**。

**为什么这条与既有机制自洽（不需要新机器）**：

- **可审计**：新授权是一条**记录**（落黑板），与其它 op 一样可回放、可被 `refute`、可被 `revert`——不是进程内的临时决定；
- **可把关**：结论要过的仍是既有门槛——`done` 必须带**可核对证据**且由**非产出者复跑**（T6-5），
  证据权重按引用去重计数（§11.4 边界② 已定），等重升级给人、超配额降级 `undecided-by-rule`（S4）；
- **有界**：子节点仍受 §S5 的四级预算（board/子树/节点/回合）与速率约束，租约到期由 `Sweep` 回收（§11.1）——
  "发新授权"不等于"无界"。

**代价与残余风险（说清楚，不含糊）**：

- 事前不再挡：一个子节点**可能在被评审之前就拿到了较宽的权限**并动了依赖/仓库 ⇒ 事后靠证据与审议追责；
- 因此**预算与速率上限成为主要的前置闸门**（S5），租约/心跳/`Sweep` 是兜底的回收路径；
- **本稿不声称**该组合等价于"事前最小权限"。若将来要收紧，最小改动是在派生处**叠加一次交集检查**（本节的"发新授权"仍保留），
  届时按新证据重新拍板，而不是悄悄改语义。

### 13.7 预算旋钮的统属（T2-3 定案，2026-10-02）

**先纠正本条目自身的前提**：TODO 把 `GoalTokenBudget` / spend budget / `REASONIX_SKIP_BUDGET` 并列成一组，其实**它们不同族**。
实测：`grep -rn "REASONIX_SKIP_BUDGET" --include=*.go --include=*.sh --include=Makefile .` **零命中** ⇒
它是**打包期**绕过桌面端尺寸闸门的开关（构建期，见本机打包 SOP），**与运行时预算无关**。

**运行时预算（同一族，都是"操作者给的封顶"，默认关闭）**：

| 旋钮 | 管什么 | 默认 |
|---|---|---|
| `GoalTokenBudget` | 无人值守 **Goal 循环**的累计 token 封顶 | 关（"跑到跑完，或你叫停"） |
| `TaskCostBudget` | 同一类闸门，按**费用** | 关 |
| `TaskTimeBudgetMinutes` | 同一类闸门，按**墙钟** | 关 |

**与 S5 四级预算的关系（不是竞争，是"设定 vs 机制"）**：上面的配置项是**操作者面向的封顶**，
§S5 的 `BudgetLimits`（board / 子树 / 节点 / 回合）是**内核里的记账与拒绝机制**。前者的值应当被**翻译成**后者的上限，
而不是两套各记一遍；S5 的账本才是拒绝发生的地方（`IsBudgetReject` 命名到具体层级）。

**超限后的 Goal 状态：`blocked`（定为口径）**。理由：预算耗尽**不是合同被满足**，所以不是 `complete`；
它属于"没有人来就推进不下去"那一类，正是 `blocked` 的定义（而无人值守侧的总开关仍可让它后续被恢复/续跑）。
本口径与 §11.1 的状态闭集一致。

### 13.8 能力授权的承载记录（§13.6 的落地形式，2026-10-02）

**问题**：按 §13.6，派生出的「获取能力」子节点携带**自己那份显式授权**。授权用**哪种记录**承载——
新增 op 动词，还是复用既有动词？

**定案：复用 `assert`，用 `Op.Source` 打标记，不新增动词。**

- **为什么不加动词**：S1 的动词集是**冻结的、带迁移表与拒收原因的闭集**（§11.1），加一个动词要动迁移表、形状校验、
  拒收分类与整套 S1 用例——而 §13.6 的要求是"**不需要新机器**"。`assert` 的语义本就是"记录断言与证据"，
  而"这份工作被授权去动依赖/仓库"**就是一条关于该节点的断言** ✓。
- **记录形状**：`assert` op，`Actor` = **批准者**（授权链的可审计来源就是它），`Source` = `"agentbus-grant"`，
  `Evidence` = 该授权的**可核对引用**（决策记录、工单、diff 都行）。由此：落黑板 ✓、可回放 ✓、可 `refute` ✓、可 `revert` ✓。
- **读侧（诚实说明）**：折出的 `Assertion` **不携带 `Source`**（`Assertion` 只有 actor/at/seq/evidence/summary），
  所以"哪些断言是授权"只能从 **op 日志**读回来——**与唤醒目标同一先例**（后者自 T5-6/2026-10-02 起已把 `Requester` **折进状态**，
  而授权的 `Source` **仍**只能从 op 读：`Assertion` 不带它 ⇒ 这条先例被**收窄**过，没有被推翻 ✓）。
  若将来要让折出状态直接答"谁批的"，最小改动是给 `Assertion` 加一个字段（S1 变更），届时按新证据再来一次，而不是现在偷偷加。
- **验收（下一刀）**：① 对授权做 `refute`/`revert` 能改变折出结果；② 同一 op log 重放得到同一份授权
  **实证（2026-10-04 抽查 ⇒ 这两条已可复跑，不再是"下一刀"）**：① `TestARefutationChangesTheEnding`、
  `TestAnAuthorizationGoesWithTheNodeThatLostItsStanding`、`TestNodeDetailLeavesOutAnAuthorizationThatDoesNotHold`；
  ② 同一 op log 的重放一致性由 `internal/agentbus/e2e_test.go` 与 board 的重放用例覆盖。；
  ③ 缺能力的子节点在**没有** `agentbus-grant` 断言时不得被计为"已授权"。

**落地现状与更正（2026-10-02，动手前先核对了链路）**：

- 本节标题旧写法（"把旋钮翻译成 `BudgetLimits`"）**预设了账本已经存在** ✗，实测**并非如此**：
  `internal/agentbus.Ledger`（S5 四级预算）目前**只在测试里被实例化**，宿主侧（boot/control）没有任何地方创建它 ⇒
  四级预算现在是"**机制就绪、没有宿主**"，要落地得先决定**谁持有账本、在哪个 tick 上 `Charge`/`Settle`**，那是独立一刀。
- 另一方面，**配置→Goal 的通路已经存在** ✓：`cfg.Agent.GoalTokenBudget` → `internal/boot/boot.go`（装进 `control.Options`）
  → `internal/control/controller.go` 的 `goalTokenBudget`；`GoalStatusBlocked` 也已在 `internal/control/goal.go` 多处写入
  （即"超限 ⇒ `blocked`"这条口径**已有实现路径**，只是本轮**未逐行确认**哪一个分支正是预算耗尽的那一个 ✗）。
- ⇒ 所以本节的"统属"结论不变（配置=设定、账本=机制），但**落地顺序要改**：先给账本找宿主（谁持有、何时记账），
  再谈把哪些旋钮映射到哪一级上限；**不要**先写"翻译"代码而假定账本已存在。

**逐行核对结果（2026-10-02，收窄上一段的说法）**：`internal/control/goal.go` 里停止原因是**被标注过历史的**——

| 停止原因 | 代码自述 |
|---|---|
| `stopCauseBudgetSpend` = `budget_spend` | **当前在写**的那一条 |
| `stopCauseBudgetTokens` = `budget_tokens` | `// legacy; never written by current runtime` ⇒ **`GoalTokenBudget` 很可能已是死配置**（字段仍在、仍被装进 `control.Options`，但运行时从不写这条停止原因） |
| `stopCauseBudgetTurns` / `stopCauseGoalRunBudget` | 同为 legacy（回合配额与每 Run 上限都已废弃） |

⇒ 上一段说的"超限 ⇒ `blocked` 已有实现路径"**只对 `budget_spend` 成立**；**token 那一路未经证实、且有反证**。
因此 T2-3 的落地还多一条待办：**先判定 `GoalTokenBudget` 是要复活（真的在运行时记账并写 `budget_tokens`）
还是标记为废弃（从配置与 Options 里撤掉）**——两种都行，但必须选一种，不能让"配置项存在而无人读"的中间态留着。

**`GoalTokenBudget` 的处置：复活，真的记账（2026-10-02 用户拍板）**。

- **要做的**：在运行时按回合累计 token，越过 `GoalTokenBudget` 时写 `budget_tokens` 停止原因并把 Goal 置 **`blocked`**——
  与当前**在写的** `budget_spend` 路径**同形**（那条就是模板：同一个记账点、同一个状态迁移、同一个停止原因字段）。
- **不再留中间态**：`stopCauseBudgetTokens` 上"legacy; never written by current runtime"这句注释必须随实现一起改掉，
  否则下一个人会继续以为它无效（这正是本次发现的起因）。
- **验收（下一刀）**：① 累计 token **真的到达**配置上限（走真实装配，配置→`Options`→运行时记账）
  **已钉（2026-10-04）**：`TestTaskBudgetGateLandsARunawayOnTokens`（真装配：`New(&spendingProvider{max: 500}, …, Options{TaskBudget:{Tokens: 3000}})`，
  该 double 每轮 1100 tokens ⇒ 第三轮落成 `task_budget` 暂停，轴名 `"token"`、`HostOwned`，理由点出所到上限）+
  `TestTaskBudgetGateNamesTheTokenAxisAtTheCeiling`（9/10 不过线、10/10 过线；轴名是**单数** `"token"`）。
  **配置那一跳也核过（2026-10-04 补查）**：链路 = `goal_token_budget`（`internal/config/config.go:1326`）→
  `control.Options.GoalTokenBudget`（`controller.go:507/737`）→ `Controller.goalTaskBudget()` 里 `b.Tokens = c.goalTokenBudget`
  （`internal/control/run_ceiling.go:29-32`）→ `agent.WithTaskBudget` 注入 ⇒ agent 的 `exceeded` 报 `"token"`；
  control 侧由 `TestGoalTokenBudgetPausesAndResumes`（`internal/control/goal_spend_budget_test.go:37`）与 `goal_runtime_test.go`
  的 token 用例钉住 ⇒ **本项两端都有用例**。
  **一句设计说明（免得被当成缺口）**：**任务级**配置只有 `task_cost_budget` / `task_time_budget_minutes`，**没有** `task_token_budget` ——
  token 上限是 **Goal 的**旋钮（`goal_token_budget`），走上面那条链注入；**不是漏配**。
  **为什么 boot 侧那条 effect 用例只跑 `time`（2026-10-04 补查后确认：不是缺口）**：token 轴的注入点在
  `Controller.bindTurnScope`（`internal/control/run_ceiling.go:14-23`）——**只有 Goal 作用域的回合**才会
  `agent.WithTaskBudget(ctx, c.goalTaskBudget())`，普通会话那一支 `return ctx` 原样不动 ⇒ 走 `boot.Build` + `ctrl.Run` 的
  effect 用例**原理上就跨不过 token 轴**（这正是"token 上限归 Goal"的设计本身）。
  所以 token 轴的端到端由 control 侧的 Goal 用例承担（`TestGoalTokenBudgetPausesAndResumes`），agent 侧那两条只钉"轴本身正确"，
  **两端合起来覆盖同一根链，没有漏掉**；`配置 → Options` 那一跳（`internal/boot/boot.go:1823` 一个字段赋值）未被单独用例覆盖，
  属"类型检查得到的映射"，已记在交接区。；② 超限后 Goal 状态为 **`blocked`**
  且停止原因是 `budget_tokens`；③ 关掉该配置（默认值）时**行为与现在一致**（跑到跑完或叫停）——即复活不得改变未配置用户的语义。
- **记账点先定再写**：与 §13.7 开头同一问题——谁持有账本/在哪个 tick 记账。若 `budget_spend` 已有明确记账点，
  token 就挂在**同一点**上，不另开一处。

**记账点在哪里（2026-10-02 读出，不再"先定再写"）**：`budget_spend` 的记账点**不在 control 里的账本**，而在**agent 层的"运行暂停"**：

- `internal/control/turn_orchestrator.go` 的 `goalPauseFromRunError`：`agent.InspectRunPause(err)` 给出 `Kind == "task_budget" && HostOwned`
  ⇒ 映射成 `stopCauseBudgetSpend`（`goal.go:541` 落成停止原因）；
- ⇒ **token 复活的正确落点**是两处、且都在既有形状内：① `internal/agent` 侧让**token** 预算也产生同一种
  `task_budget` 运行暂停（spend 那条已有）；② `turn_orchestrator` 的这段映射按**是哪一种预算**分别给
  `budget_spend` / `budget_tokens`。不需要新机制，只需要让暂停信息里带上"哪个预算"。

**另一条必须说清的澄清**：这条 **Goal 预算（agent 层运行暂停）与 §S5 的 `agentbus.Ledger`（四级预算）是两回事**——

| | 粒度与层 | 现状 |
|---|---|---|
| Goal 预算（spend/token/time） | **一次无人值守 Run** 的累计，agent → control 的状态迁移 | spend 在写；token 待复活（本节） |
| §S5 `Ledger` | **一个板**上的 board/子树/节点/回合四级允许量，内核记账与拒绝（`IsBudgetReject`） | 机制就绪、**尚无宿主**（谁持有、哪个 tick 记账仍待定） |

两者**不是同一个账本、也不该合并**：前者决定"这次 Run 还跑不跑"，后者决定"这个板上的这一步能不能开工"。
上一段把二者写成一件事是错的，此处更正。

**token 复活的落点再收窄（2026-10-02 继续读代码）**：闸门**本来就是按轴通用的**，不需要照抄一段逻辑：

- `internal/agent/run_loop.go` 的回合前检查是 `if axis, detail := a.task.budget.exceeded(a.taskBudgetLimit(ctx)); axis != "" { … armFinalizationRound(… axis: axis …) }`
  ⇒ **谁超了就拿谁的名字起暂停**；暂停信息已经带 `axis`（`internal/agent/errors.go` 的
  `RunPauseInfo{Kind: "task_budget", Key: budget.axis, HostOwned: true}`），control 侧再把它映射成 Goal 停止原因。
- ⇒ **token 复活 = 给预算跟踪器补上 token 这条轴**：① `taskBudgetLimit(ctx)` 要能从 `GoalTokenBudget` 给出 token 上限
  （默认 0 = 不设 ⇒ 行为与现在一致）；② 让 token 用量落在该轴上（`run_loop.go` 每回合已有 `ObserveUsage(usage)`，
  需确认它喂的是同一条预算对象）；③ 于是 `exceeded` 会以 `axis == "tokens"` 返回，暂停与映射**自动**走通。
- **测试模板**：`internal/agent/task_budget_gate_test.go` 已经断言 `info.Kind == "task_budget" && info.Key == "cost"`
  ⇒ token 那条照它写（`Key == "tokens"`），再补 control 侧的映射分派（`budget_spend` / `budget_tokens`）。

**落点读到位（2026-10-02，最后一步收窄）**：token 轴的实现位置与模板都已确定，只剩一跳未读清 ✗：

- **轴的定义处**：`internal/agent/run_budget.go` 的 `func (a *Agent) taskBudgetLimit(ctx context.Context) TaskBudget`（:131）；
  调用点三处：`run_loop.go`（回合前）、`incomplete_read_runtime.go`、`sampling_recovery.go`——都走同一个 `exceeded`。
- **轴名已见**：`task_budget_gate_test.go` 里 `"cost"`（费用）与 `"time"`（墙钟），且有一条"未配置时什么轴都不该拦"的用例
  （`unconfigured budget crossed … nothing should stop by default`）⇒ **这正是"关配置时行为不变"那条验收的现成模板** ✓。
- **尚未读清的一跳** ✗：`GoalTokenBudget` 目前只到 `control.Options`（`controller.go` 的 `goalTokenBudget`），
  而 `taskBudgetLimit` 在 `internal/agent` ⇒ 需要确认 **Agent 是否已经能看到该值**（若不能，就得补一跳 control → agent 的装配），
  以及 `TaskBudget` 是否已有 token 字段。**这两点必须先读清再改**，不要凭"应该能拿到"动手。

**两点读清（2026-10-02）：token 轴其实是"死的"，但只死在**没有人填上限**这一处** —— 工作量因此缩到最小：

| 读到的 | 结论 |
|---|---|
| `internal/agent/run_budget.go` 的 `TaskBudget{Cost, Wall, Tokens}` | **token 字段早就存在**，且 `normalizeTaskBudget` 会把它归一（负值 = 不设）⇒ 与其它轴的语义规则一致 ✓ |
| 同文件的 `promptTokens` / `outputTokens` 与每回合累加 | **token 用量也一直在记**（`b.promptTokens += usage.PromptTokens` …）✓ |
| `grep -rn "GoalTokenBudget" internal/agent/` | **零命中** ⇒ agent 侧**看不到这个配置**，`taskBudgetLimit` 从不填 `Tokens` ⇒ 轴因此永远不触发 |

⇒ **token 复活的最小实现** = ① 补一跳装配（`Boot`/`control.Options` → agent 的预算配置）把 `GoalTokenBudget` 送到 agent；
② `taskBudgetLimit` 把 `TaskBudget.Tokens` 填上（0/负 = 不设 ⇒ 关配置时行为不变 ✓）；③ 用例照
`task_budget_gate_test.go` 的"cost/time/未配置"三态补 `tokens` 那一态，再补 control 侧按 axis 分派 `budget_spend` / `budget_tokens`。
**仍待确认**（下一刀第一件事）：`exceeded` 是否已经处理 `Tokens` 分支（该函数体本轮未读到 ✗）——若是，则实现只剩上述 ① ②。
**（紧接着的下一段就回答了它：`exceeded` 早已处理 token 轴 ⇒ 实现只剩装配那一跳；读者可直接跳到下一段。）**

**读全了（2026-10-02 收尾）：`exceeded` 早已处理 token 轴，缺的只是"注入时把上限填进去"** —— 并更正一处名字错误 ✗：

- `run_budget.go` 的 `exceeded` 第一个分支就是 `if limit.Tokens > 0 { used := promptTokens + outputTokens; if used >= limit.Tokens { return "token", … } }`
  ⇒ 轴的**名字是 `"token"`（单数）**，不是本文档前几段写的 `"tokens"` ✗——实现与用例都必须用 `"token"`；
- `taskBudgetLimit` 的注释写明"**a host-injected budget wins over the configured one**"，实现读 `taskBudgetFromContext(ctx)`
  ⇒ **装配跳已经存在**（上下文注入的形态）⇒ token 复活 = **在注入 `TaskBudget` 的地方把 `GoalTokenBudget` 也填上**（0 = 不设 ⇒ 行为不变）；
- 于是三步收敛为：①（唯一的代码改动）在注入处填 `Tokens`；② 照 `task_budget_gate_test.go` 的 `"cost"` 那一态补 `"token"`（含未配置不拦）；
  ③ control 侧 `goalPauseFromRunError` 目前把 `Kind == "task_budget"` 一律映射成 `budget_spend` ✗ ⇒ **按暂停的 `Key` 分派**（`"token"` ⇒ `budget_tokens`）。

**T9-7 落地，并当场抓到一处守卫失效（2026-10-02）**：整条链路已有端到端用例
（`internal/agentbus/capability_gap_e2e_test.go`），同时修正了"不能自己批自己"的判据 ✗：

- **错在哪**：`Authorized` 用**折叠出的 `Owner`** 认"产出者"，而 `Owner` 会被 `capability_gap`（以及 `done`）**清空**
  ⇒ 恰好在这一步**需要首次授权**的时刻，判据退化成"任何有证据的人都行" ⇒ **干活的人可以自己批自己** ✗。
  用例先写、旧代码下红（`TestACapabilityGapDoesNotLetTheWorkerAuthorizeItself`），再修 → 绿；四个既有 grant 用例无回归。
- **判据改从 op 日志派生**（与 `GrantsFor` 同一先例，也与 `applyDecide` 对"产出者"的定义一致）：
  **非授权类 `assert` 的 actor ∪ `claim` 的 actor**。授权类 `assert`（`Source == "agentbus-grant"`）**不计入**产出者，
  否则"谁批的"会把自己算成"干了活的人" ⇒ 谁也批不了 ✗。
- **`Authorized` 与 `AuthorizedGrants` 不再各写一份**：前者委托后者 ✓（两份重复逻辑正是它们能悄悄漂移的原因 ✗）。
- **仍缺（如实记，2026-10-04 复核）**：`AuthorizedGrants` 已被**读面**调用（`internal/agentbus/detail.go:55` 的节点明细），
  `Authorized` 仍只在内核内部用 ⇒ 授权记录能写、能读、能核，**但会话侧"开工前查授权"这道门仍未接**（与 §13.9 的槽位/队列
  同一形状：机制就绪、入口未接）。
  接线点 = 会话认领/开工之前那一处，与 S5 的"宿主接纳"是同一道门，**宜一并做** ✗。

### 13.9 账本宿主与记账时机（T4-8 之外的最后一处 S5 落地，2026-10-02 定规格）

**问题**：`agentbus.Ledger`（四级预算 + 本机槽位）机制就绪，但**生产里没有任何地方创建它** ⇒ 定"谁持有、哪个 tick 记账"。

**谁持有：宿主进程一份**。理由是 §13.4 已经定过——**槽位是本机 host 级**，而板/子树/节点/回合四级额度是
**按 board 记账**的（`Ledger` 的键形如 `board:<name>` / `subtree:…`）⇒ **一份账本可以服务同一进程里的多个板**。
所以它属于**宿主对象**（桌面 `App` / serve 服务），由宿主构造并**下发给各控制器**，形态照 `SetAgentBusWaker` 的先例
（`SetAgentBusLedger(ledger)` 之类的小接口），而不是让每个会话各自持一份（那会让"宿主全局槽位"名不副实）。

**哪个 tick 记账：两处，都在既有检查点上，不新增时钟**。

| 动作 | 位置 | 语义 |
|---|---|---|
| `Charge`（节点 + 回合额度 = **做工的额度**） | **既有每回合预算闸门**（`internal/agent/run_loop.go` 回合前那一处，`TaskBudget` 也在这里判） | "这一步还能不能开工"；与既有 `exceeded` 同一检查点，**不另开一处记账** |
| `Settle`（board + 子树额度 = **验收的额度**） | **板的写路径**（`control.ApplyAgentBusOp` 里 `decide(done)` 成功之后） | 只有**被验收**的节点消耗总预算（T7-4）；重复结算由 `Ledger.settled` 去重 |

**配置翻译**：§13.7 的操作者旋钮在**装配处翻译一次**成 `BudgetLimits`（`internal/boot` 正是 `GoalTokenBudget` 已有的流经之地），
拒绝发生在账本里（`IsBudgetReject` 命名到层级），超限的 Goal 语义按 §13.7 = `blocked`。

**残余（如实记）**：**墙钟轴（`time`）没有自己的停止原因**——`goalPauseFromPause` 目前把它也记成 `budget_spend`
（`internal/control` 的停止原因闭集里没有 `budget_time`）。要么补一个自有原因，要么在文档里明确"墙钟越限按 spend 记"，
**两者选一**，不要留着不说。

**墙钟残余：定案为"写明按 spend 记"（2026-10-02）**。

- **选择**：不新增 `budget_time` 停止原因，而是在文档里**明确写下**"墙钟轴（`time`）越限按 `budget_spend` 记录"，
  并保留"将来新增自有原因"作为待办。
- **为什么这次不顺手补一个** ✗：`StopCause` 是**会被消费的字符串**（`internal/acp/status.go` 等处按它分支、
  前端也会渲染），新增一个值属于**对外可见的枚举扩张**——而本会话已无余量把它的消费方逐个核实并验收 ✗。
  在"没核实的对外可见改动"与"先写明现状、留好待办"之间，选后者 ✓（这也是本稿一贯的纪律：不确定就不冒充已解决）。
- **待办**：若要给墙钟自有原因，最小改动 = 停止原因闭集加 `budget_time` + `goalPauseFromPause` 按 `Key == "time"` 分派
  + 核实 `acp/status.go` 与前端对未知停止原因的处理（**先核实再改**）。

**读代码后的缩小（2026-10-02）：宿主其实只有一处**。原写"桌面 `App` / serve 各一处" —— 实测**不是**：

- 生产里构造 controller 的地方**只有一处**：`internal/boot/boot.go` 的 `ctrl := control.New(ctrlOpts)`；
  桌面经 `buildTabControllerBoot` 走 `boot.Build`，cli / serve / bot 同样走 `boot.Build` ⇒ **所有前端共用同一处装配**。
- 因此"宿主持有并下发 `Ledger`"落成**一行注入**：`boot.Options` 加一个 `AgentBusLedger`（或构建后照 `SetAgentBusWaker` 形态
  装上去），由**各自的宿主二进制**在进程内创建**一份**并传入——**不需要在每个前端各写一套**。
  具体走 `Options` 还是"构建后再装"，取决于哪条更贴合 boot 现有装配顺序（`AgentBusDir`/`AgentBusID` 已经走 `Options` ⇒ 同路最省）。
- 两个检查点本轮之前已读过：`internal/agent/run_loop.go` 的每回合预算闸门；`control.ApplyAgentBusOp` 的 `decide(done)` 之后。
- ⇒ §13.9 的实现只剩：**一处 Options 注入 + 一处 boot 装配翻译 + 两个检查点各一行 + 用例**。

**最后一跳读清（2026-10-02）：`Charge` 也留在 control，`internal/agent` 一行都不用改** ✓✓。

- 问题：账本/额度怎么到达"每回合闸门"？读下来**不需要新通路**：
  `internal/agent` **并不引用** `agentbus`（实读为零命中）⇒ 不该把账本推进 agent；
  而 **control 自己就有每回合的绑定点**：`internal/control/turn_orchestrator.go` 的
  `ctx = c.withTurnContext(c.bindTurnScope(ctx, continuation), !turn.synthetic)`，
  它注入的正是 agent 层的 `TaskBudget`（`run_ceiling.go` 的 `bindTurnScope` → `agent.WithTaskBudget`）。
- ⇒ **`Charge` 就挂在这个绑定点上**（control 内，回合开始处），**`Settle` 挂在 `ApplyAgentBusOp` 的 `decide(done)` 之后**
  ——两个记账点、一处注入、一处翻译，**全部落在 `control` + `boot`**，`internal/agent` 与 `internal/agentbus` 都不动 ✓。
- 于是 §13.9 的实现收敛为**一个 control/boot 切片**（我这一场最熟的那类）：Options 注入 → boot 翻译 `BudgetLimits`
  → 绑定点 `Charge` → 验收点 `Settle` → 用例（装配值到达账本上限；`Settle` 只认验收节点；超限 Goal 为 `blocked`）。

**动手前又核出一处前提错误（2026-10-02）：§13.7 的"把旋钮翻译成 `BudgetLimits`"不成立** ✗。

- **作用域不同**：`GoalTokenBudget` / `TaskCostBudget` / `TaskTimeBudgetMinutes` 是**一次 Run 的累计**（agent 层，
  决定"这次 Run 还跑不跑"，§13.7 已澄清）；而 `agentbus.BudgetLimits` 是**一个板上的四级额度**
  （board / 子树 / 节点 / 回合，决定"这个板上这一步能不能开工"）。**一个是"跑"的账，一个是"板"的账** ⇒ 不存在 1:1 映射 ✗。
- ⇒ §13.9 第 2 步（`boot` 装配处翻译旋钮 → `BudgetLimits`）**应当删掉** ✗。`Ledger` 的四级上限需要**它自己的来源**
  （板上策略：谁定、从哪读——例如每板设置或板目录里的策略文件），这是一个**尚未决定**的问题 ✓，不能拿 Goal 旋钮冒充 ✗。
- ⇒ §13.9 的落地只剩三步（§13.9 里其余部分不变）：① `boot.Options` 注入 `AgentBusLedger`；② `Charge` 挂
  `turn_orchestrator.go` 的绑定点、`Settle` 挂 `ApplyAgentBusOp` 的 `decide(done)` 之后；③ 用例（装配值到达账本上限、
  只认验收节点、超限 Goal `blocked`）——**外加一件新待办**：决定板级四级额度的来源（本稿不替使用者发明 ✗）。
  - **落地结果（2026-10-04 核对）**：第 ① 步最终走的是本稿列的**备选形态**——方法注入 `SetAgentBusLedger`
    （`internal/control/agentbus_budget.go:16`），由宿主在装配后调用（`desktop/agentbus_waker.go:160`：
    `bus.SetAgentBusLedger(hostAgentBusBudget())`）。**`boot.Options` 里没有 `AgentBusLedger` 字段**（`internal/boot/boot.go` 零命中）
    ⇒ 读本节时不要把 ① 当成"已存在的 boot 选项"。

**板级额度来源：定案（2026-10-02 用户拍板）——先只启用 host 级槽位，四级额度保持"不设"**。

- **决定**：`board` / 子树 / 节点 / 回合四级上限**全部不设**（0 = 无限），只有**宿主槽位**（`BudgetLimits.Slots`）有上限。
  这与仓库既有哲学一致："**每条轴默认关闭，叫停是用户的事**"（`run_budget.go` 的注释原话），
  也避免在没有真实需求时替使用者发明"每板应该有多少额度" ✗。
- **这条决定的直接后果（让 §13.9 更小）**：`Ledger` 里**唯一"活"的上限就是宿主槽位**，而它的消费方**已经建好**——
  S5 的 `Take`（`internal/agentbus/schedule.go`）在领活前先 `AcquireSlot(holder)`，满了就一件不取、工作留在队列 ✓。
  ⇒ 让账本"真的有宿主"的最小落地 = **宿主创建一份并交给队列路径**；
  `Charge`/`Settle`（四级额度的记账）**可以先不接**——它们在不设上限时是空转 ✓，等哪天真的给某一级设了上限再接，
  且那时**只需要在既有绑定点与验收点各加一行**（缝已在 §13.9 记明）。
- **~~仍是待办~~ 已落（2026-10-04 复核）**：槽位上限由**配置面**给 —— `internal/config/config.go:51-53` 的
  `[agentbus] dispatch_slots`，桌面 `desktop/agentbus_waker.go` 的 `agentBusBudgetLimits` 把它与四级额度一起建账本、
  `SetAgentBusLedger` 装到每个 controller 上；`Charge`/`Settle` 的挂钩**也已落**（`internal/control/agentbus.go:180` 的
  `chargeClaim`、`:191` 的 `settleBudget`，见 T7-2/T7-4）。⇒ 本节"给账本找宿主"这件事已经完成，剩下的是 §13.8 那道门。

**动手前又核出一处前提（2026-10-02）：S5 的队列与账本在生产里没有任何驱动者** ✗。

- 实测：`agentbus.Take(` / `agentbus.OpenQueueLog(` / `Take(ctx` 在**非测试代码里零命中**；提到 `internal/agentbus/schedule.go`
  的非测试文件只有它自己 ⇒ **今天没有任何人把工作入队、也没有任何人领取** ✗。
- ⇒ "给账本找宿主"这一步**不能先做** ✗：账本的"活"上限是宿主槽位，而槽位的消费方 `AcquireSlot` 只被 `Take` 调用，
  `Take` 又只被测试调用 ⇒ **先要定的是"生产里谁驱动队列"**（谁 park、谁 take、在哪个 tick），
  然后才是"这份账本放哪"。顺序反了会做出一个没人跑的精美机制 ✗。
- **另一个必须记住的坑（同名异物）**：本仓库里 `Ledger` 至少指四样东西——`billing.NewLedger`（费用账）、
  `evidence.NewLedger`（证据账）、`capability.NewLedger`（能力账）、`agentbus.Ledger`（四级预算 + 宿主槽位）。
  接线时**必须写全限定名**，grep `NewLedger` 会把四条线混在一起 ✗。
- **待定（产品/架构选择，不替使用者发明）**：队列的生产驱动者是谁——唤醒路径（`WakeAgentBus` 同处）？
  宿主每 tick 扫描？还是编排回路自己？三者语义不同（事件驱动 / 轮询 / 内联），先定再写。

**队列的生产驱动者：取默认（2026-10-02，用户未指定）** —— 与 `WakeAgentBus` 同形，两个半场分开定：

| 半场 | 默认落点 | 依据 |
|---|---|---|
| **取（take）** | **宿主 tick**：与 `WakeAgentBus` 的兜底同处（宿主每 tick 一次，幂等） | 与既有先例同形，不新增时钟 ✓ |
| **停（park）** | **计算唤醒目标的同一处**（它已经知道"谁能开工、谁在等"——不能开工的那些正是要 park 的） | 复用同一份判断，避免第二套"谁该等"的逻辑 ✗ |

**明确标注为默认而非用户决定** ✓：两者都可推翻——若日后选择"编排回路内联"或"写者触发"，只需换调用点，
机制（`Park`/`Take` + `Ledger`）与语义（幂等、FIFO、槽满不取）**都不必改** ✓。

**仍未定（更小的一步）** ✗：宿主 tick 的具体宿主是谁（桌面 `App` 的心跳？serve 的 tick？）——按 §13.9 的注入形态，
账本与队列路径都随 `boot.Options` 走，真正的"每 tick 调一次"由各宿主二进制自己接（与它接 `WakeAgentBus` 的方式一致 ✓）。

**宿主 tick 找到了：桌面已有心跳，不必新造时钟（2026-10-02）**。

- `desktop/heartbeat.go` 已经有 `time.NewTicker(30 * time.Second)`（同一族文件里还有 `heartbeat_converge.go` 管无人值守那半）
  ⇒ **`Take` 就挂这个既有 tick** ✓（幂等，重复调用无副作用），与 §13.4"不引入新守护/新时钟"一致。
- serve 侧自己的 tick 未查（本轮余量所限 ✗）：注入形态（`boot.Options`）是统一的，serve 接线时按其既有 tick 照挂即可，
  **先找到它的 tick 再动手**（与桌面同理）。
- ⇒ 下一轮的施工面已经很窄：① `boot.Options` 注入 `Ledger`；② 桌面心跳里每 tick 调一次 `Take`（幂等）；
  ③ park 挂在"计算唤醒目标"处；④ 一条"槽满不取、工作留在队列"的真实装验收。

**施工前再想一层：③（park）在 v1 可能是多余的** ✗ —— 动手前应当先判定，别照着清单硬做。

- **"没开工的活"已经在板上**：一个依赖未满足的节点**本身就是持久的、可被任何人读到的"待办"**（`board.jsonl` 就是队列）
  ⇒ 把这类"还不能开工"的活**再抄一份进 `queue.jsonl`** 是重复存储 ✗，而且多一份需要保持一致的状态 ✗。
- **`queue.jsonl` 真正独有的场景只有一个**：某参与者**已经准备好开工**（拿到了唤醒、认领了节点），
  但**宿主槽位满了**——此时它是"想干而干不了"，需要一个持久的地方搁着、等槽位空出来 ⇒ 这**才是** §13.4 里
  "槽满**排队**而非失败"要的东西 ✓。
- ⇒ 因此 v1 的最小闭环可能是**两步而不是三步**：① 宿主注入 `Ledger`；② 宿主 tick 调一次 `Take` 给有空槽的参与者派活；
  **park 只在"准备开工却拿不到槽位"那一刻才需要**——而那一刻发生在宿主**接纳某个会话去干活**的入口上 ✗（尚未定，
  比 §13.4 设想的"计算唤醒目标处"更靠后 ✓）。
- **给下一轮的第一条动作**：先判定"**板是否已经就是队列**"——若是（我倾向是 ✓），则 `queue.jsonl` 在 v1 可以只服务
  "拿到唤醒但没槽位"这一种情形，甚至**先不做**（槽满时让参与者下一 tick 再来 ✓，与"幂等、不丢工作"并不冲突 ✓）。

**结论（2026-10-02）：S5 的队列与账本在 v1 保持"已测机制、暂不接生产"——接上去也是空转**。
**（2026-10-04 更新：这条"不接"的结论已被后来的接线推翻** —— 桌面已按配置建账本（`desktop/agentbus_waker.go:36` 的 `agentbus.NewLedger(agentBusBudgetLimits(cfg))`），派发走槽位闸门（`internal/control/agentbus_dispatch.go:51-61`），§13.4 已记"先读队列、后花槽"；下文 1029/1041/1089 一带的"不接 / 当前不存在"请按**历史**读，不再代表现状。）


把上一段的"收敛为两步"再推一步：

- **第一步（注入 `Ledger`）**：四级额度按用户决定**全部不设** ⇒ 账本里唯一"活"的是宿主槽位；
- **第二步（宿主机 tick 调 `Take`）**：`Take` 从**队列**取活，而 v1 **没有任何人 park** ⇒ 队列**恒空** ⇒ `Take` 每次都取不到东西 ⇒
  两次"收敛"之后仍然**不产生任何行为** ✗。
- 根因：**槽位与队列服务的是"宿主接纳某个会话去干活"这一道门**，而这道门**目前不存在**（会话自己认领、自己开干，
  没有谁向宿主申请开工许可）⇒ 在没有这道门的环境里，把账本挂上去只是让代码看起来更完整 ✗。

**因此定案（诚实收口）**：

1. **不接**：v1 不注入 `Ledger`、不在心跳里调 `Take`（避免"接线完成但零行为"的假象 ✗）；机制留在 `internal/agentbus`
   与测试里 ✓（`Take`/`Park`/`Ledger` 的语义与用例都已就位 ✓）；
2. **触发条件写明**：**当且仅当出现"会话必须向宿主申请开工许可"这道门时**再接线——那时槽位（本机并发上限）与
   队列（拿不到槽位时搁着）才真正有用 ✓，接线点也已备好：`boot.Options` 注入 + 宿主 tick（桌面 30s 心跳）调 `Take`；
3. **这条不是"跳过"**：它是"**先判定再动手**"得到的结论——本场第六次用这个方法拦下了基于错前提的施工 ✓
   （前五次：token 轴疑死、映射用例、翻译旋钮、队列无驱动者、park 是否多余）。

### 13.10 「验收节点」怎么认（T9-5 的前置，2026-10-02 读清后提出）

**读到的**：`board.NodeSpec` 只有 `{ID, Title}`，`Op` 与折叠状态里都**没有** kind/marker 字段
（实读 `internal/agentbus/board/op.go`）⇒ 本文档反复出现的"**验收节点**"（§1 失败模式、§8 规则、§11 S7⑤）
在内核里**无法表达** ✗。此时写 T9-5 的判定，就是在替使用者发明"哪些算验收" ✗——先定这个，再写判定。

| 选项 | 做法 | 代价 |
|---|---|---|
| **A 结构派生**（无新字段） | 验收节点 = 依赖图的**根**（没有别的节点依赖它），即交付物本身 | 零 schema 改动 ✓；但"任务有多个顶层节点、其中只有一个是真正验收标准"时表达不出来 ✗ |
| **B 显式标记** | `NodeSpec` 增 kind/marker（如 `acceptance`） | 语义最直白 ✓；属 **S1 op 结构变更**（写入 / 迁移 / 视图 / 面板都要跟着动），须单独一刀并写迁移口径 ✗ |
| **C 约定** | 用 Title 前缀、命名约定来认 | 零改动；但把语义藏在字符串里 ⇒ 核不上、易漂 ✗，与"可核验"纪律冲突 ✗ |

**建议**：v1 取 **A**（派生、零改动），等真出现"多顶层节点、只有一个算验收"的场景再升级到 **B**——
**升级点写明**：那是 S1 结构变更 + 迁移口径，不是补个判断就完事 ✗。

**与该决定无关、可先行的那半**（**2026-10-02 随 A 一并落地**：`landing.go` 的 blockers 里 `contested` / `deliberating` / `escalated` / `undecided` 就是它）：**无未决矛盾**（`contested` 节点 / 未决审议）。而"证据链完整"
**已由门槛保证**（`decide(done)` 要求可核对证据 + 非产出者复跑，见 §11.4 / T3-1 / T3-8 / T6-5），不必另写一条 ✓。

**并记一处形状**：这条决定与 §13.8 的"**开工前查授权**"、§13.9 的"**宿主接纳（槽位/队列）**"**是同一道门的两侧**——
三者都在问"谁、在什么时候、被允许推进哪一步"。这道门一旦立起来，三处应当**一并接**；分散接会出现三套半截判据 ✗。

**A 已实现（2026-10-02）**：`internal/agentbus/landing.go` 的 `AssessLanding(state, hearings) *Landing` ——
交付物 = **依赖图的根**（`rootsOf`，零 schema 改动 ✓）；逐条给出阻塞原因（节点 id + 状态/原因），同一节点只报
**最可操作的那条**（在争 `contested` > 悬念 `deliberating` / `escalated` / `undecided` > `missing` > `abandoned` > `not_done`；**实现是四级优先级表**：`contested`/`deliberating` = 0、`escalated`/`undecided` = 1、`missing` = 2、`abandoned` = 3、其余 = 4 —— `blockerPriority`，`landing.go:119-132`，`remember` 取优先级最高者 ⇒ 同一节点只留一条），
`Reason` 一行说清"没落地、因为什么"。用例 `landing_test.go` 六条：交付物未完成；**被 `refute` 后仍 `contested` ⇒
必须报"争议"而不是泛泛的未完成**；全 done 但审议未决（open / escalate / undecided-by-rule）⇒ 不落地，`stands` ⇒ 落地；
缺失依赖、被放弃依赖各报其类；空板不落地；两次读结果一致。
**仍未做**：把它接到 Goal（"宣告落地"对齐 `complete`）——与 §13.8、§13.9 同为那道门的一侧，宜一并接 ✗。

**实现中顺带发现（已挂 TODO T8-4）**：`observe.go` 的争议信号判据 `len(n.Refutes) > 0 && n.Outcome == ""`
**漏掉"先 `done` 后被 `refute`"的节点** ✗ —— `applyRefute` 只改状态、不动 `Outcome` ⇒ 该节点 `Outcome` 仍是 `done`，
状态却已是 `contested` ⇒ 那场争议在人侧看不见。本条判定改用状态机自己的答案 `StateContested` 才避开；信号那侧须单独修。

**更正：三者不是"同一道门"，别照这个口径施工（2026-10-02 自查）** ✗ —— 上一段把 §13.8 / §13.9 / §13.10 说成
"同一道门的两侧"是**错的**：按 T2-5 的定案，**能力授权与验收判定都是"事后"的**（把关在证据 + 审议 + 人读面），
**不是开工前的闸门** ✗；真正的事前闸门只有 §13.9 的**宿主接纳（槽位/队列）**这一处 ✓。所以三者的接法不同：

| 侧 | 何时起作用 | 接法 |
|---|---|---|
| §13.8 授权 | **事后**（T2-5：一律发新授权，把关在证据 + 审议） | 让授权**可读可核**（`AuthorizedGrants` 说出是谁批的）+ 缺能力必须有**派生节点与复跑**；**不做**"没授权就不许开工"的前置闸门 ✗ |
| §13.10 验收判定 | **事后**（宣告落地时） | 接到 Goal：`Landed` ⇒ 才允许 `complete`，否则停 `blocked` 并保留现场（本段已实现内核那一半 ✓） |
| §13.9 槽位/队列 | **事前**（开工前） | 只有在"会话要向宿主申请开工许可"这道门存在时才有意义；**当前不存在** ⇒ v1 不接（§13.9 已记）✗ |

⇒ **下一刀的顺序**：先接 §13.10 到 Goal（小、可测 ✓），§13.8 只做"可读可核"那一半（不新增前置闸门 ✗），
§13.9 维持"已就绪、待门" ✗。

**已接到 Goal（2026-10-02，按上条的"下一刀的顺序"做完第一步）**：`internal/control/agentbus_landing.go` 的
`withBoardLanding` 把 `AssessLanding` 的结果并进**宿主的 readiness**（注入点 = `turn_orchestrator.go` 里算完 readiness、
判决输入之前）⇒ 它**不是**另起一道闸门，而是**多一条缺失项**（id `agentbus_landing`，**故意不在**
`repeatedCompleteMayFinish` 可放行的集合里）⇒ "完整声明被拒 ⇒ 继续做工"与"重复声明也不放过"两条既有规则**自动适用** ✓。
入列但读不到板 ⇒ 按拒绝处理 ✗；未入列会话 ⇒ 原样不动 ✓（守护 T4-4 的"未接线即零变化"）。用例 2 条（含"被 `refute` ⇒
不许 complete 且原因点名争议"）。

### 13.11 会话面入口：工具与桌面（2026-10-02 落地，回应"看不到变化"）

**落地前的事实**：内核与读面早已就绪，但**会话侧没有任何入口** —— `boot.Options.AgentBusDir` 全仓无人赋值、
`ApplyAgentBusOp` 除 control 内部审议外无调用方、`internal/tool/` 下**零** agentbus 工具（而 §编排 假定"会话 ⇒ 有黑板工具"）。
⇒ 用户看到的是"面板永远未入列、看板永远空"。

| 入口 | 承载 | 说明 |
|---|---|---|
| 模型工具 `agent_bus` | `internal/tool/builtin/agentbus.go` + boot 的 `boardToolPort`（**按调用惰性解析**控制器，沿用扩展 UI hub 的缝） | 12 个动作；**常驻注册**（工具清单属 cache-stable 前缀，不随入列/离开增减）；内核 typed 拒绝以"该改什么"回给模型 |
| 人的面板操作 | `AgentBusControls.tsx` → `desktop.AgentBusApply` → **调用同一个工具** | 人类与模型不可能对动词理解不一致；回答就是看板自己的话（记录了什么 / 为何拒收 + 怎么改） |
| 入列与可见入口 | `desktop/agentbus_enrol.go` + 工作区面板「协作」节 + 状态栏常驻 chip（有异常带计数） | 默认板 `<state home>/agentbus/default`；未入列**无徽标也不轮询**；点开就是同一个协作面 |

**代价（如实记）**：`agent_bus` 常驻 ⇒ 每个会话的 provider 可见工具面变一次（golden 实测 `ToolSchemaTokens` 5150 → 5975，
`SystemHash` 不变、`ToolsHash`/`PrefixHash` 变）。若改成"只在入列的会话里出现"，代价是中途入列必须重启会话才生效 —— 未选。

**已修（2026-10-03）**：`assert` 的 op 现在也带**可选** `Title`（`board.Op.Title`，JSON 省略即零值；`DeriveID` 的 payload 用
`omitempty` ⇒ 对既有 op 的派生 id **逐字节不变**）⇒ `assert` 建的节点可以自带名字，`applyAssert` 只在节点尚无标题时写入
（后来的 `assert` 不改名：标题是标识，动词表里没有"改名"）。工具面 `agent_bus` 的 `title` 参数已同时适用于 `assert`。
用例：`TestAssertNamesANodeItCreates`（内核：带名/不改名/无名三态）、`TestAgentBusToolAssertNamesTheNodeItCreates`（工具面透传）。

### 13.12 审议边界旋钮：`RoundTTL` 是"沉默"的唯一来源（2026-10-03，G1 尾落地）

**为什么单列一节**：S4 的审议在**参数缺省时没有时钟** —— `HearingSilent` 第一行就是
`h == nil || !h.Open || lim.RoundTTL <= 0 ⇒ nil`，而 `RoundTTL` 只能来自 `HearingLimits`。于是
"未答者被唤醒"（`WakeTarget.Owes`）在**没人设边界**时**永远不会发生**：入口修好了、台阶高度是 0。
设施此前**没有生产调用方**（`SetAgentBusHearingLimits` 全仓零命中、无 `HearingLimits{...}` 字面量）。

| 旋钮（`[agentbus]`） | 字段 | 关掉什么（0 = 关掉这条边界） |
|---|---|---|
| `hearing_round_ttl_minutes` | `HearingRoundTTL` | 一轮等多久才把沉默算作 `no_answer`。**0 = 沉默永不被计数 ⇒ 没人会被唤醒**（本节的由来） |
| `hearing_max_rounds` | `HearingMaxRounds` | 每名必答者最多被问几轮 |
| `hearing_cooldown_minutes` | `HearingCooldown` | 收口后多久内不许重开同一议题 |
| `hearing_escalation_quota` | `EscalationQuota` | 等重议题可升级给人几次，之后按规则收口为 `undecided-by-rule`。**0 = 一次都不升级**（与其它旋钮"0 = 不限"相反，已写进字段注释） |

**接线点（唯一一处，覆盖三条宿主）**：`internal/boot/agentbus_wiring.go` 的 `enrolAgentBusController` —— `boot.Build`
在 `opts.AgentBusDir != ""` 时调用，因此桌面 / serve / cli 拿到的控制器带着同一份边界；映射是
`control.AgentBusHearingLimits(cfg.AgentBus)`（分钟 → `time.Duration`）。**缺省时说一句**：`RoundTTL == 0` 时每进程只警告一次
（`noticeAgentBusHearingWindow`），点名 `hearing_round_ttl_minutes` —— 看板本身显示不出"我的审议没有时钟"。

**证据**：`internal/boot/agentbus_wiring_test.go`（**先红后绿**：设 `hearing_round_ttl_minutes = 1` 且回合已过期 ⇒ 未答者被唤醒
且 `Owes` 含该节点；不设 ⇒ 唤醒 0 人）、`internal/agentbus/hearing_test.go` 的
`TestADeliberationWithNoRoundWindowNeverCountsSilence`（同一条审议：无窗口 ⇒ 空；`RoundTTL = 1h` ⇒ 两名必答者都在）、
`internal/config/agentbus_test.go`（9 个键都落到字段；缺 section = 全 0）、`internal/control` 的
`TestOperatorKnobsBecomeTheKernelsDeliberationBounds`（分钟不被读成秒）。

**同刀补上的契约缺口**：审议三件（open/answer/settle）此前**不在** `AgentBusControl` 契约里，于是**面板那条路**（`desktop/agentbus_apply.go` 的 `agentBusPanelPort`，人的操作走"同一个工具"）在编译期就没有实现可用 —— 一刀补进 `internal/control/port.go` 的 `AgentBusControl`，面板端口随之转调同一批方法。

### 13.13 重试耗尽的步：从"只留人读信号"到"自动唤醒请求者"（2026-10-03，G5）

**问题**：`agentBusDispatchTries = 2` 之外派活器**不再把这一步发出去**（`agentbus_dispatch.go` 的 `dispatchable`），
而**那之后没有任何东西会再提到它**：节点无主、可开工、状态不再变化 ⇒ 唤醒的 key 不变 ⇒ **连唤醒都不会再来一次**。
"没人看的信号 = 没有信号"。

**做法（复用既有面，不新增 op 词表）**：唤醒多一个分组 `WakeTarget.Stalled`，来源是**同一个判据** ——
`WakeInput.StallAfter`（由宿主注入，等于派活器自己的 `agentBusDispatchTries`；0 = 这个宿主没有这条预算，内核不自造），
命中者满足：`NoProgress >= StallAfter` **且无主**（有主的过期租约归 `Sweep` 管）**且未收口**（`done`/`abandoned` 不报，T8-2 的规则）。
被唤醒的是**它的 `Requesters`**（T5-6：谁要这个节点）——也就是"编排者"那个角色；文案带原因与出路
（"handed out N times with no progress: take one, replan it, or say why it cannot move"）。
**触发者是宿主 tick**（`AgentBusTick` → `WakeAgentBus`，桌面 30s / headless 30s），无需人点按。
**一个节点一次只报一种事实**：命中停滞后不再进 `Ready`/`Assigned` 那两组，否则同一节点会同时被说成"可开工"和"停住了"。

**证据**：`internal/agentbus/wake_test.go` 的 `TestWakeTargetsNameStalledWorkInsteadOfOfferingItAgain`（真折叠：认领 → 过期 → `Sweep`
两次 ⇒ `NoProgress = 2` ⇒ 请求者收到 `Stalled`；`StallAfter = 0` 或低于阈值 ⇒ 照旧是 `Ready`；两种事实的 key 必须不同）、
`TestWakeTargetsStayQuietAboutAStalledStepThatFinished`（收口后不再报）；`internal/control` 的
`TestAStalledStepIsWokenBackToWhoeverAskedForIt`（端到端：宿主 tick 把这一步报给请求者，文案含 `stopped moving` 与 `replan`，
第二次 tick 不重复）。
**测试踩到的内核坑（写下来免得下次再踩）**：`Sweep` 的回收 op 用 `SweepID(node, deadline)` 做幂等键 ⇒ **两次派活必须各有各的 deadline**，
否则第二次回收被判重放、`NoProgress` 只加一次（本刀 1/8 概率复现过）。

**已落（2026-10-04 复核，提交 `b36e8f3d1`）**：**指派超时回落 pool** —— 超时后宿主写一条 `unassign`（既有 op，
回到板级 pool），不再把活锁给不在场的受派人；`ObserveLimits.AssignedWait`（默认 10 分钟）仍是"何时算停滞"的窗口，
两件事各司其职。原条目记录的顾虑（哪个 tick 写这条 op、写几次、与租约过期如何错开）由那一笔一并处理，
用例（`internal/control` 的 stall 族）随之补上。

### 13.14 编排规范落在哪（G4，2026-10-03）

内核**没有自动 planner**：节点图是**写出来**的。缺件不是能力，而是"**怎么写**"那份可复用的规范 + 一个把它接到会话里的入口。
三件落点：

| 件 | 落点 | 说明 |
|---|---|---|
| 规范（节点粒度 / 子树划分 / 边界节点 / 交付物根 / 编排者被删后怎么继续长图） | `docs/agents/ORCHESTRATION.md` | 每条规则带**内核实现位置 + 用例**，只写内核真做得到的 |
| 会话入口 | 内置技能 `agentbus-orchestration`（`internal/skill/builtincontent/agentbus-orchestration/SKILL.md`，`runAs: inline`） | 编排者会话（任何会话）可 `run_skill` 取得打法清单；正文只在被调用时进上下文 |
| 演练 | `internal/agentbus/orchestration_drill_test.go` | 编排者写下交付物 + 一块工作后**消失** ⇒ 幸存者只读文件、**补出图上没有的第二块** ⇒ 交付物 `done` ⇒ `AssessLanding` 落地 ⇒ 新读者折出同一结局 |

**边界（已收口，2026-10-03）**：`assert` 建的节点原先**没有 Title**，规范因此要求"结构用 `require`/`split` 建、`assert` 只用于给已有节点落断言"
——那是**绕开**内核缺口的写法。现在 op 带**可选** `Title`（`board.Op.Title`，`omitempty` ⇒ 既有 op 派生 id 不变），
`applyAssert` 在节点尚无标题时写入，工具面 `title` 参数同时适用于 `assert` ⇒ 规范里那条绕行写法已退休。
**技能与缓存**：正文只在被调用时进上下文；新增内置技能给宿主生成的 `session-context` 的 Skills 目录**加一行**
（`internal/control/session_context.go`；会话上下文属回合尾部，不是 cache-stable 前缀），golden 基准本身不含技能目录
（`internal/boot/golden_baseline_test.go` 明记交由 session-context 测试覆盖）⇒ `TestGoldenBaseline` 实测未变。

### 13.15 宿主派发回环的四条真实规则（2026-10-04，`5578284b8` + 顺序半边补钉）

桌面宿主的派发回环 `agentBusDispatchTick`（`desktop/agentbus_waker.go`）此前没有用例；补用例时**实测问清了三条规则**
（前两条我的初稿写错过，跑出来才知道）；**同日又补上"发哪一步"那一条**（第四行，内核 tie-break 的显式契约），共四条：

| 规则 | 为什么 | 用例 |
|---|---|---|
| 每位参与者**每 tick 只发一步**，且**该参与者已有排队唤醒时按住**（`RuntimeStatus().PendingPrompt` 让它算"在做的事"） | 不把活堆在还没开工的会话上；派发本身留下的唤醒就是"它还没消化"的证据 | `TestADispatchTickHandsWorkToTheSessionOneStepPerTick`（`desktop/agentbus_dispatch_tick_test.go`） |
| **同秩时发最早要求的那一步**（一 tick 只发它） | 排序的末位 tie-break 是**到达顺序**，就写在内核里（`internal/agentbus/rank.go:56` 的原话：*"that tie-break is what keeps the queue starvation-free"*）；不钉住它，改成"随便发一步"也没人会发现 | `TestADispatchTickHandsOutTheEarliestStepFirst`（2026-10-04 补，同文件） |
| **投递不了就交回**：`deliver` 失败时把 claim 交回板上（"没人被告知的活不该留着"） | 否则活被一个**从没听说过它**的会话占着 | `TestADispatchTickLeavesNoClaimWhenTheSessionCannotBeTold` |
| **没入列的标签不是派发目标**（跳过，不替它猜参与者/板子） | 猜出来的身份等于把活发错人 | `TestADispatchTickSkipsATabWithoutABoard` |

**已知边界（记档，不做，2026-10-04 用户定调）**：会话摘要（recap）的后台 lane **自带它建立时的 provider/凭据** ⇒
轮换 API key 后它仍可能用旧 key 发请求（栈：`SendWithRetry ← openai.Stream ← boundedllm.Call ← recap.Generator ← recap.Runner`）。
判定为**可接受的边界**（摘要属辅助产物、lane 生命周期独立于会话运行时），不改代码，仅此记档。

### 13.16 无人值守贯通的可复跑清单（T9-4 的离线半边）

（这一类"宿主自己算出了状态、界面却不说"的缺陷及其核对法，见 §13.17。）

T9-4 要证的是"**杀掉进程 → 看门狗把树拉回来 → 板子接着往前走**"这一条链。契约与可观测量都在 `docs/UNATTENDED.md`
（§3 的启动印记、§6 的交接、§8 的验收、§10 的 `--watchdog-*`），**此处不复制**，只写演练怎么走、看哪里、以及一个必须先知道的陷阱。

| 步 | 动作 | 断言（都可复跑） |
|---|---|---|
| 0 | 开无人值守 → 起一个长任务（板上有交付物根 + 至少一块已 `claim` 的活） | `docs/UNATTENDED.md` §3 的 `<home>/desktop-host-state.json` 里 `unattended: true`、`phase: running`，`pid` 是当前进程 |
| 1 | **杀进程**（只杀这一个 pid；**不要按进程名通配**） | 印记文件**不会被删**（干净退出才删）⇒ 残留印记 + 死 pid = 上一次是**非干净退出** |
| 2 | 触发看门狗（调度器那条命令，或直接 `--watchdog`） | `reasonix-desktop.exe --watchdog-status` 报出 policy / 是否已注册 / **上次运行时间** / 入口 |
| 3 | 等新进程起来并自己恢复 | 新的 `pid`、`uncleanStreak` 计数；`agent_bus action=view` 里**那块活仍在原参与者名下**或已被重派，且交付物根的 `Ready`/`Assigned` 组**继续外移** |

**步 0 的只读检查（复制即用；三步都不写任何东西）**：

1. **读印记**：`<state home>/desktop-host-state.json` —— 出处 `desktop/host_state_marker.go:60-65`：
   `config.MemoryUserDir()`（= `REASONIX_STATE_HOME`，未设时回落到 home；Windows 通常即 `%APPDATA%\reasonix`）——
   看 `pid` / `phase` / `unattended` / `uncleanStreak`。判读：**文件还在、而它的 `pid` 已经死了 ⇒ 上次是非干净退出**
   （干净退出会删掉它）。确认 pid 死没死（Windows）：`tasklist /FI "PID eq <pid>"`（只读）。
2. **看策略与注册**：`reasonix-desktop.exe --watchdog-status`，输出**原样留档**（policy / 是否已注册 / 上次运行时间 / 入口）。
3. **先判降级、再判故障**：`uncleanStreak >= 3`（且上一次确实死了，见下）⇒ **本次启动的驱动器本来就是关的** ——
   这时"没在推进"不是故障，别当成自愈坏了。

**必须先知道的陷阱（真实，且代码可定位）**：`uncleanStreak` 统计 10 分钟内的连续非干净退出，**到 3 次即降级** ——
`desktop/host_state_marker.go:29` 就是 `hostCrashStreakLimit = 3`，而 `:200` 的 `hostCrashLoopDegraded()` 还要求**上一次确实死了**
（`out.Dead && out.UncleanStreak >= hostCrashStreakLimit`）；降级落地在 `desktop/heartbeat.go:196`：
`e.unattended = snapshot.cfg.Unattended && !hostCrashLoopDegraded()` —— 即"驱动器被开关之外的这一项关掉"，避免崩溃风暴。
⇒ 演练**不要连续杀三次**；每次之间让进程**干净退出一次**（清掉印记）再继续，否则第 4 次看到的是"驱动器本就不该起"，会被误读成"自愈坏了"。

**同一处的第二条微妙点**：印记里记的是**操作者的开关**，不是"这一跑打不打算驱动"（`host_state_marker.go:205-209` 的注释：
*a crash-degraded run drives nothing, and recording that as "off" is exactly what stops the watchdog from pulling the host back up*）。
⇒ 已经降级时印记的 `unattended` **仍是 `true`**；**不要**据它判"开关被关了"，要看驱动器是否在动。

**哪些必须真机、哪些可近似（如实标注）**：第 0/1/3 步的**印记与板面断言**可在单机复跑（印记是文件、板是共享目录）；
**必须真机**的是第 2 步里"调度器真的按时调用 `--watchdog`"（Windows 计划任务/`--watchdog-enable` 的注册结果只能真机看）。
**本会话未跑过这条链**（与 §5.1.1 同样是真机待验项），首次执行请把每步的印记文件原文、`--watchdog-status` 输出与
`view` 的原文+时间戳留档，作为 T9-4 的验收证据。

### 13.17 状态可见性：四段核对法（写→读→画→钉）与六类"看不见的刹车"的现状（2026-10-04）

**索引（2026-10-04 收口）：这一节已经核过哪些集群面、结论是什么。** 每条都留了证据或提交号；明细在下方对应小节。

| 面 | 结论 | 依据 |
|---|---|---|
| 状态可见性（六类刹车） | 四处曾悬空、本会话补齐；两处一直齐；两处经核实**自愈**（租约回收 / 会话保存） | 本节第一张表 + `11768809a`/`d2081d750`/`af5ade4f9`/`b9d2e038b`/`c68d7c450` |
| 规模（大板三问 + 每 tick 代价） | 三问都有答案；每 tick 是 **O(nodes)**，唯一上限 `maxSweepPerCall` 是**写入**节流 | 用例 `4a9c6d5fd`/`496689b9d` + 本节"每 tick 的代价" |
| 可重放（折出 / 时钟两半） | 折出确定早已钉密；**时钟那一半**补上；同一份日志**三种读法一致** | `cd591f643`/`5afd4079d` |
| 授权面 | 一条授权只命名一个节点、**不被子树继承**；**不设闸门是设计**（§13.6） | `29ae53356` + 本节"授权之后能干什么" |
| 听证 / 审议沉默 | 到点收成 `undecided-by-rule`，**可见**（`SignalUndecided` 一行），三处用例 | 本节"审议/听证面" |
| 预算五轴 | 五条轴**一致拒绝**；`Settle` 是文档写明的记账例外 | 本节"预算五条轴"表 |
| 令牌 / 身份（跨进程） | 鉴权**在寻址之前**；401/409/202 **分码**；不回显令牌与 session path | `c6752b896` |
| tick 三种形态 | 桌面 / headless / 派发**都接了**，没有漏 tick 的启动路径 | `a5340fbf1` |
| 地址簿 TTL 与撤回 | 过期并档是**设计**；退列**会撤回**（桌面关停 + serve 离场），残留只有硬杀 | `b9eff6b4a`/`b885dd61c` |
| 跨机时钟 | **写者本地钟权威**；偏差后果是回收早晚、不是错结论；容忍度 ≪ 租约量级 | `75a30c00a` |
| 板日志增长 | 无压缩、**只追加**；"读是 O(n)"仓里已有基准与 **T4-8** 靶子 | `9a4f11712` |
| **定调 ① 已结（2026-10-05）** | 速率/限流：**节点级迁移上限已补实现**（内核写侧守卫 + 旋钮 + 面板 `node_rate` 行）⇒ 文档不再"比代码说得多"；仍存的一半是 S4 ⑥ 的"连续 `refute` 触发冷却"（现实现是"关闭后冷却"）。收敛优先级**已决：不建单一收敛点**（四条今日各有读者，建它只是第三个零调用方导出 API；`d35cdf8a4`） | 本节"速率/限流面"与"第二例" + `internal/agentbus/board/rate_test.go` / `internal/boot/agentbus_node_rate_test.go` |
| **定调 ② 已做（2026-10-05：最小方向落地）** | 折叠快照 + 只折尾部（`<board>/snapshot.json`，写者每 256 op 落一次，边界指纹 + seq 接续验证）；**冷读仍是 O(ops)**（快照必须带幂等表），买到的是常数：真实形态 −19%~−38%、写路径 ≈0。仍**无压缩** | 本节「板日志的增长与压缩」 + `internal/agentbus/board/checkpoint_test.go` / `BenchmarkColdSnapshotAtLogSize` |


**为什么写这一节**：这套集群反复出现同一形状的缺陷 —— 宿主机自己**算出了**一个状态（拒绝、投递失败、没在驱动、
看门狗没生效…），却没有一处界面说出来；而"没在动"与"没活了"看起来一模一样。§13.16 引的 G3 原则说得更直白：
**a brake nobody can see is not a brake**。以下是可复用的核对法，以及逐条的当前答案。

**四段核对法**（给任何"host 自己记了个数"的新东西照做）

1. **写**：谁在哪一行记账/记错？除了 `slog`/`log.Printf`，还有别处吗？
2. **读**：有没有**导出读数**？读到的与写的是不是**同一份**（而不是各算一套）？
3. **画**：前端**真的渲染**那个字段吗？（**别只看类型声明** —— 类型在契约里 ≠ 有人在画；这个坑本轮踩到两次）
4. **钉**：把这一半删掉，**用例会不会变红**？（没有用例 ⇒ 那不是结论，只是阅读印象）
   **一句常被误判的**：授权/审计类事实**不一定该有闸门** —— 先看同一件事在文档里被声称成什么（§13.6 明说把关在"事后证据 + 审议"），
   别把"没有闸门"直接算缺口；只有当文档声称有闸门、而代码只是读数时，才是落差。

**谁 tick 谁（2026-10-04 追到底）**

| 宿主形态 | 定时器起在哪 | 间隔 | 覆盖 |
|---|---|---|---|
| 桌面 | 心跳引擎的 loop（`desktop/heartbeat.go:196` 的 ticker → `tick()` → `agentBusWakeTick()`） | 30s | **每个 tab 的 controller 都 tick**（`agentBusWakeTick` 遍历 tabs，每 tab 一次 sweep；多次 sweep 幂等）；`tick()` 里这一步**不受无人值守开关影响**（注释：The board is host-wide, not per task） |
| headless / serve | `internal/cli/cli.go:976` → `startAgentBusTick(ctrl, …)`（`internal/cli/agentbus_tick.go:24`） | `agentBusTickInterval`（30s） | 起在**一个** controller 上；但唤醒是**宿主级**的 —— serve 装的 waker 按参与者路由（本机每个会话 + 邻机地址簿），且任何写入路径自己也会 sweep（`internal/control/agentbus.go:176`） |
| 派发那一半 | 同上（`agentBusWakeTick` 末尾调 `agentBusDispatchTick`） | 随宿主 tick | 每个 tab 的 controller 都跑一次派发 |

⇒ 三种形态都接了，**没有哪个启动路径漏 tick**；健康时静默、失败才 slog（见上一节的判读）。

**（headless 特例）** `serve` 宿主**没有**状态面：它没有 `/agentbus` 之类的只读路由，"画"就是它自己的 stdout ——
拒绝（`recordBudgetRefusal`）与投递失败（`noteWakeFailure`）两处都保留着 `slog`，所以对 headless 操作者**够**；
桌面要面板，是因为它的用户从不读日志。判断"缺不缺画"时要先问**谁在看这台宿主的标准输出**。

**现状（写在哪 → 读数 → 画在哪 → 用例 → 结论）**

| 刹车 | 读数 | 画在哪 | 结论 |
|---|---|---|---|
| **槽位拒绝**（满槽把活停在队列） | `AgentBusBudgetRefusals()`（`internal/control/agentbus_budget.go`） | `AgentBusBriefing` 的拒绝行 → `AgentBusPanel` 的 host-signals | 曾有"计了没人画"：用例没断言 `slots`（`11768809a` 补） |
| **唤醒投递失败** | `AgentBusWakeFailures()`（`internal/control/agentbus_wake.go`） | `wakeFailureSignal` → 同上（`Kind: "wake_undelivered"`） | 曾只 `slog.Warn`（`d2081d750` 补） |
| **唤醒目标不在这儿**（板上留着的已下线参与者） | `AgentBusWakeUnreachable()`（**集合**，不是计数） | `wakeUnreachableSignal` → 同上（`Kind: "wake_unreachable"`） | **2026-10-05 真机查实并修**：见本节末"板外会话 vs 板内目标" |
| **无人值守没在驱动**（崩溃降级） | `HeartbeatConfigView.unattendedDriving/unattendedHold`（`desktop/heartbeat.go`） | 开关标签与提示（`unattendedPresentation`） | 曾只有一行日志、界面仍说"会持续推进"（`af5ade4f9` 补） |
| **OS 看门狗没生效** | `App.WatchdogStatus()`（`desktop/watchdog_control.go`：读数与用例一直都有） | 同上（`watchdogHold` + 三语文案） | 曾**没有任何组件读它**（`b9d2e038b` 补） |
| **限流**（429 被扛过） | `RateLimitRetries*` | `rateLimitSignal` → 同上 | 一直齐（分道计数与"未点名"都有用例） |
| **子树卡被裁掉** | `briefing.Hidden` / `HiddenCards` | 面板"还有 N 个子树 / M 条信号" | 一直齐（host 与前端各有用例） |

**板外会话 vs 板内目标：唤醒面不认地址簿（2026-10-05 真机查实并修）**

真机现象（dev.130，桌面日志 14:50–14:51）：宿主**每 30 秒 × 每个已加入的 tab**，对**四个早已不在的参与者**
（`bob`、两个**已撤回**会话、一个**从未公告**的会话）各喊一次，全部被拒：
`desktop: no tab and no address owns agentbus participant "…"`；两分钟 40 行、永不停。

- **不是投递到了错会话**：路由按 participant **精确匹配**（`desktop/agentbus_waker.go` 的 `agentBusWakeRecipient`
  找 `AgentBusParticipant()` 相同的 tab，两个 tab 同参与者直接报错；找不到才查地址簿）⇒ **拒绝**，不猜、不退化、不广播 ✓。
- **真正的缺陷是"下线只做了一半"**：`AgentBusWithdraw` 把参与者从**地址簿**撤了（桌面关停 `desktop/agentbus_shutdown.go:20`、
  serve 离场 `internal/serve/multisession.go:236`；加入/退出都被记住 —— `desktop/agentbus_enrol.go` 的 `opt-out`，
  用户点一次加入即清除），但**唤醒面读的是板**（`WakeTargets` 从折叠态派生"谁在等活"），而板比会话活得久
  ⇒ 已下线的参与者**仍是唤醒目标**；control 又把"没有路由"当成**投递失败** ⇒ 记一笔、**释放 key**、下一拍重试
  ⇒ 永续噪声 + 面板 `wake_undelivered` 无界增长。
- **修法（不掩盖真刹车）**：内核加 typed 拒绝 `agentbus.NoRoute(participant)`（`internal/agentbus/wake.go`），
  两条宿主的"查不到"分支返回它（`desktop/agentbus_waker.go`、`internal/serve/agentbus_waker.go`）；control 据此
  **不释放 key**（同一工作集不再每拍重试）、**不计入 `WakeFailures`**（没人会来不是刹车），改记进
  `AgentBusWakeUnreachable()` —— 一个**集合**（按参与者去重、条数有界），经 `wakeUnreachableSignal` 进面板。
  **投递真失败（对端拒连、令牌没了）照旧计数并重试**，那才是"看不见的刹车"要防的东西。
- **代价（如实）**：同一工作集下，一个"当时不在"的参与者**回来后不会立刻被这条唤醒叫醒**（key 已被认下）——
  但板上的活仍在**派发**路径里（`AgentBusDispatch` 按槽位把可开工的交给空闲会话），且它自己的 `view` 就列出属于它的活。
- **仍未做**：`ParticipantTTL` 仍 30 分钟（硬杀残留的窗口）；`default` 板跨任务累积、无归档，是这次噪声的**背景条件**，
  不是这条修复的目标（见上面"板日志的增长与压缩"）。
- 用例：`internal/control/agentbus_wake_unreachable_test.go`（同一块板上"已离开"与"暂时够不到"两种目标：前者只试一次、
  不计失败、进集合；后者照旧计数并重试）、`desktop/agentbus_wake_unreachable_test.go`。

**判为"自愈"、不是刹车的两处**（查实时一并核实，留在这里是因为它们长得像）

| 现象 | 为什么不是刹车 |
|---|---|
| **租约到点没人交还** | 投影自己就报"needs handoff: lease lapsed"（`SignalStalled` + `Stalled` 计数 ⇒ 面板可见），且任何写者开头都会 sweep、过期租约可被合法接管 |
| **会话保存不 durable**（快照没落地） | `in_flight_turn.go` 保留 in-flight 标记，下一次载入由 `resolveInterruptedTurnStart` 走恢复（`internal/control/in_flight_turn_dag_test.go`、`turn_orchestrator_test.go` 覆盖）⇒ 失败会被**修好**，不是停在那里 |
| **宿主 tick 停了**（两半：回收+唤醒、派发） | **没有"上次 tick 是什么时候"的读数**，但这不是刹车：① 健康时**静默是设计**（只有失败才 `slog`）；② 失败模式不真实 —— ticker 就是个 goroutine 定时器（`internal/cli/agentbus_tick.go` 的 `time.NewTicker`），它"停"只会因为你把进程杀了（进程死是可见的）；③ 后果**可见** —— 有活而没人接时，面板里它就是 Ready 且无主；④ 两半都有用例（`TestAgentBusTickReclaimsAndWakesWithNothingBeingWritten`、`TestAgentBusTickOffTheBoardIsANoOp`），headless 侧还有 `agentbus_tick_test.go` 用 20ms 间隔证明**定时器真的会触发** |

**至此这张清单没有未修项**：六类里**四处曾悬空、本会话补齐**（各带提交号），两处**一直齐**；另有两处"看着像刹车"
经核实是**自愈**的（上表）——它们只在"失败那一刻"静默，而失败会被下一次写/下一次载入消解，且迹象本身可见。
**"大板三问"（2026-10-04 收尾；规模用例此前只证明"收敛与有界"，不证明这三件事）**

| 问 | 现在的答案 |
|---|---|
| **卡被裁掉**（第一屏装不下时计数还准吗） | 新用例 `TestObserveTrimsALargeBoardToTheCap`（`internal/agentbus/observe_test.go`，`4a9c6d5fd`）：100 个无依赖节点各自成子树根 ⇒ `MaxCards: 5` 时画 5 张、`HiddenCards == 95`（**余数**）；信号上限自己的算术仍在原小用例 |
| **唤醒面**（大板 + 多人有待办时目标对不对） | 新用例 `TestWakeTargetsNameEveryWaitingParticipantOnceOnALargeBoard`（`internal/agentbus/wake_test.go`，`496689b9d`）：20 个参与者各要一份活 ⇒ 目标 = 20 个 worker（各 `Ready` 恰是自己那步）**+ `planner`**（§13.3 有意加宽：`assert` 也算"要"）；重算幂等（key 不随调用漂移）；"不唤醒自己"由 `internal/control` 的用例承担 |
| **派发不重复**（同一 tick 不会把同一步给两个人） | **已有用例，无需新增**：`internal/control/agentbus_dispatch_test.go`（注释即规则："one step per claimant per tick, taken out of the queue"）与 `TestWithoutASlotCeilingEachClaimantTakesAStep`（`agentbus_dispatch_slots_test.go:72`）⇒ 这是**逐认领者**的规则、与人数无关，再写"20 个认领者"的版本只会重复它 |

⇒ 三问都有答案：两条新用例补上"规模形状"这一维，第三问早就钉在规则发生的那一层（派发回环）。

**每 tick 的代价（2026-10-04 顺带核过一遍，不是缺口）**：把 `AgentBusTick` 那条链的每一步按"节点数增长时会怎样"看了一遍 ——
① 折出状态走 `Board.foldedState()`（**热缓存**，只有写入才重折）再 `st.Clone()`；② `WakeTargets` 与它的调用者
`parkStartableWork` 各做一次**全量派生**（O(nodes) + 一次 id 排序）；③ `Sweep` 是唯一**有显式上限**的地方
（`maxSweepPerCall = 256`，`internal/agentbus/board/board.go:17`）——因为扫一次**会写**，不是因为折得慢；
④ `TakeRanked` 只排**可领的那一小撮**（队列条目），`releaseIdleSlots` 只看持有者。
⇒ 整条链是 O(nodes)，n=1000 时是微秒到毫秒级，**没有超线性或无限上的动作**，所以没有"该加什么上限"这件事；
唯一的上限（sweep 256）是为**写入节流**而设，与规模无关。

**可重放面（2026-10-04 核实）**：同一份 op log ⇒ 同一份状态这件事，**早就钉得很密** ——
`internal/agentbus/board/fold_test.go` 有 `TestFoldIsDeterministic`、`TestFoldCountsDuplicateIDs`、
**`TestConcurrentApplyFoldsLikeSerial`**（并发写折叠等价于串行，最强的一档），`log_test.go` 另有"尾部半行不算 op / 中段坏行跳过并计数 /
空行计数 / seq 来自日志 / 撕裂尾被下一位写者修好 / 快照报告读损伤"一整族；`doc.go` 与 `fold.go` 都写明"**fold 不读时钟**"。
**唯一缺口是"时钟那一半"没有用例**：`validateFreshness`（`board.go:160-174`）只对 `VerbClaim`/`VerbHeartbeat` 判定
（缺 `Deadline` ⇒ `missing_deadline`；`Deadline` 不在未来 ⇒ `deadline_not_future`；其它 verb 不管），此前只有代码与注释、没有用例。
补 `TestAFreshnessLapseIsJudgedAtWriteAndNeverAtReplay`（`internal/agentbus/board/freshness_test.go`）：三种判定 + "assert 带旧 deadline 也放行" +
**重放侧**：把一条"写在当时就过期"的 claim 直接 `Fold` 进日志 ⇒ 它照样应用、节点保留记录里的 owner 与 deadline（重放没有钟可再判）。
**顶层一致性（同日补）**：新用例 `TestEveryWayOfReadingAFinishedLogAgrees`（`internal/agentbus/board/agreement_test.go`）——
同一份写完的日志**三种读法必须一致**：写出它的板（`Snapshot`）、把它按行重放（`Fold(lines.Ops)`）、以及**重新打开同一目录**再读
⇒ `Applied`/`Rejected`/`Seq`/节点数/`OpIDs` 数与每个节点的 State/Owner 逐项相等。上面几族用例分别钉"怎么写"（并发等价串行、撕裂/坏行），
这一条钉"读的人只有一个"。

**授权面（同日核实）**：`AuthorizedGrants(ops, state, node)` 天然是**逐节点**的（`internal/agentbus/grant.go:75-77`），
既有用例覆盖"自授权不算 / 别人有证据才算 / 节点失去立场就失去授权 / 无证据写不进"；缺的是**边界**：
加 `TestAnAuthorizationIsNotInheritedByTheNodesItSplitsInto`（`internal/agentbus/grant_test.go`）——
父节点被授权后 `split` 出两个子节点、再 `require` 引入一个新鲜节点 ⇒ 三者**都不**被授权（`Authorized` 与 `AuthorizedGrants` 双向都查），
而父自己的授权仍在 ⇒ 钉住"一条授权只命名一个节点"，不让它靠子树关系被继承。

**"授权之后能干什么"（2026-10-04 核实：与 §13.6 一致，无落差）**：`AuthorizedGrants` 在包内**只有一处生产调用** ——
`internal/agentbus/detail.go:55`，把授权折进 `NodeDetail.Authorizations`，也就是**呈给读者的视图事实**；
`Authorized`（`grant.go:48-49`，`len(AuthorizedGrants(...)) > 0`）目前**只有用例调用**，包外没有生产调用点。
这不是缺口，而正是 §13.6 的定案：**把关从"事前取交集"移到"事后证据 + 审议"** —— `done` 要带可核对证据并经非产出者复跑，
预算/速率是主闸门，授权链负责**可审计与可质疑**（可 `refute`、可 `revert`）。§13.6 还**明说**不声称等价于"事前最小权限"，
所以"`Authorized` 不拦任何动作"与文档一致，**不需要加闸门**（要收紧应先在派生处叠加显式授予，按新证据重新拍板）。

**审议/听证面：沉默会发生什么（2026-10-04 核实 —— 这一面本来就没问题）**

**板日志的增长与压缩（2026-10-04 核实无压缩；2026-10-05 补了"载入跳过已折叠前缀"，读**仍是 O(n)**，见下）**

- **没有压缩/归档/轮转**：`internal/agentbus/board` 与 `jsonl` 里唯一的 `Truncate` 是**修撕裂尾**（`jsonl.go:68,134-135`，只丢掉那条不完整的最后一行，不删历史）⇒ 日志**只追加**，一次任务一张板就永久长在盘上。
- **读是 O(n)，且已明写**：`snapshot_cost_test.go` 的 `BenchmarkSnapshotAtLogSize` 注释写着「T4-8 的剩余半边就是它 ——a read folds the whole log, so it is O(n) — **recorded here so the fix has a target**」，并给出写路径已优化的对照（1600 ops：**6.95 ms → 1.11 ms**，停止"每次写都全折"之后）。热态下 `foldedState()` 走缓存，但 `Snapshot` 仍要 `Clone` 一次（同样 O(n)）。
- **现状可接受的理由**：单次任务量级小（百级节点 ⇒ 数千 op ⇒ 折叠在 ms 级）；增长是**跨任务**才累积的，而每张板对应一棵任务树。
- **若要处理，最小方向**：不要在写路径上再动手（已优化），而是**让载入跳过已折叠的前缀** —— 持久化一份 fold 状态（或给日志做一次快照 + 保留一个只读尾部），因为缓存本身已经是热的、只有"冷启动重折"这一处值得投资。

**→ 已做（2026-10-05，定调②"最小方向"落地）：折叠快照 + 只折尾部。**
- **形态**：`<board>/snapshot.json`（原子替换，写者每 256 个 op 落一次）＝ 折叠到某个 `seq`/字节偏移的状态 + **幂等表 `opIds`**；载入时先用**边界窗口指纹（前缀末尾 256 字节的 sha256）+ 尾部第一条的 `seq` 必须接上**验证，任一条不成立就**当作没有快照**、退回全量折。读**从不写**快照（读持共享锁）；快照写失败**不让已落盘的 op 变失败**（它只是优化）。
- **为什么必须带上 `opIds`**：折叠态本身不序列化幂等表（`State.OpIDs` 是 `json:"-"`），而没有它，一条**重试投递**会被当成新 op 再应用一次 ⇒ **快照不是"状态的压缩"，它仍是 O(op) 的**。这条决定了下面量出来的性质。
- **实测（本机，`go test -bench 'ColdSnapshotAtLogSize' ./internal/agentbus/board/`，一对比就是同一负载的前后；"whole log" 先把快照删掉）**：

| 形状 | log=400 | log=1600 | log=6400 |
|---|---|---|---|
| **一节点一 op**（折叠态≈日志大小） | 2.43 vs 2.36 ms | 8.85 vs 8.78 ms | 33.8 vs 35.0 ms（**≈持平**） |
| **每节点 16 op**（真实形态） | 1.88 vs 2.07 ms | 5.95 vs 7.36 ms（**−19%**） | 18.7 vs 30.3 ms（**−38%**） |

- **结论（诚实）**：这**不是复杂度上的改进 —— 冷读仍是 O(ops)**，因为快照必须携带幂等表；它买到的是**常数**：真实形态下省 19–38%，而"一节点一 op"这种状态与日志同大的极端形状下几乎**没有收益**（还多一份与日志同量级的快照文件）。写路径代价 **≈0**：`BenchmarkApplyAtLogSize` log=1600 仍是 **1.13 ms**（与快照之前的 1.11 ms 同量级）。
- **仍然没有压缩**：没有归档/轮转，快照也不是压缩，日志继续永久增长；若要真正把冷启动变成 O(tail)，得让幂等表**不必**随快照走（例如给"过期 id"一个可由日志派生的裁剪规则），那是另一刀、且要单独立据。

**跨进程身份/令牌面（2026-10-04 核实：失败面可分辨、且不泄密）**

- **顺序**：中间件是 `log → gzip → auth → hostGuard → csrf → mux`（`internal/serve/serve.go:661`）⇒ 未带令牌的调用在**任何寻址之前**就被 401 拒掉，没人能靠探针问出"这台宿主有哪些会话"。
- **码**：鉴权失败 401（`internal/serve/auth.go:397`、`:597`）；**地址到不了**是 **409**（`inbox_addressing_test.go:33/88` 断言 409，报文 "session is not open in the desktop window; open it there first"）；成功是 202（`inbox.go:202`）⇒ 三种结果**不同码**，投递方（`agentbus.DeliverWake`）能据此区分"没人/地址错/收下了"。
- **不泄密**：`auth.go` 的拒绝报文只有 "Bad Request" / "Method Not Allowed" 一类的通用词（`403`/`401` 路径也一样），**不回显令牌**；地址失败报文只说"那个会话不在桌面窗口里"，**不回显 session path** ✓。
- 与文档一致：§11.5/§13.3 的"带令牌投递"就是 `Authorization: Bearer <announced token>` + `X-Reasonix-Session-Path` 寻址，令牌本身**只经文件引用**（`ReadAnnouncedToken`，地址簿里只有路径）。

**§13.10 与 `landing.go` 逐条对完（2026-10-04）**：文档声称的**六条用例**逐一对上 ——
`TestLandingNeedsEveryDeliverableDone`、`TestLandingStopsWhileADisputeIsUnresolved`、
`TestLandingStopsWhileAVerdictIsStillOut`（现开 / 已升级 / 按规则未决三态都在同一用例里）、
`TestLandingNamesWhatIsStillMissing`、`TestLandingOnAnEmptyBoard`、`TestLandingReadsTheSameTwice` ✓；
"同一节点只报最可操作的那条"由 `blockerPriority`（`landing.go:119-132`）+ `remember` 实现，与文档口径一致（唯一细化：
代码把 `contested` 与 `deliberating` 并列为最高、`escalated`/`undecided` 低一档，文档原句读起来像四级直链 ⇒ 已按代码事实补注）。：`RoundTTL` 到点是**按规则收口**，不是丢弃、也不会永远挂着 ——
`internal/agentbus/hearing.go:58-63` 写着"silence counts as no_answer … they start closing as `undecided-by-rule`"，
而 `VerdictUndecided = "undecided-by-rule"` 自己的注释就说明它是**真实结果、不是失败**（`hearing.go:27-34`）；
**并且它可见**：投影把它映成 `SignalUndecided` 一行，详情写着"**closed by rule: nobody may call it settled**"（`observe.go:209-213`），
节点详情同时给 `Deliberating`/`Verdict`（`detail.go:34-35,60`）；三处都有用例（`hearing_test.go:159/160/218-228`、`observe_test.go:187-198`、`detail_test.go:99-101`）。
⇒ "听证没人回应 ⇒ 结果不可被少数人当既定结论"这条设计，既**落地**又**看得见**。

**预算五条轴：拒绝还是仅记账（2026-10-04 核实 —— §13.7/§13.9 说的是前者，代码一致）**

| 轴 | 闸门在哪 | 拒绝面 |
|---|---|---|
| board / 子树 / 节点 / 回合 | `Ledger.Charge`（`internal/agentbus/budget.go:133-142`，逐层 `check` 后返回 error） | `internal/control/agentbus_budget.go:49` ⇒ `recordBudgetRefusal`（带 level/reason/limit/board/node） |
| 槽位 | `Ledger.AcquireSlot`（`budget.go:98-110`，满则 `BudgetReject{Level: "slots"}`） | `internal/control/agentbus_dispatch.go:60-61`（不 claim、只记账） |
| token | 不在账本里：agent 的 `exceeded`（`run_budget.go:110-115`）把回合落成**可恢复的 `task_budget` 暂停** | 由 `c209dc79f` 补的两条用例钉住 |

**`Settle`（验收后记账）是例外，而且是写在文档里的例外**：它的 doc 明说 "a refusal here is **bookkeeping and not a gate**: it says stop
starting new work, never undo the work that was accepted"（`internal/control/agentbus_budget.go:144-146`）；它能失败的情形是 nil/未知节点/节点未 `done`/
board 或子树超限（`budget.go:148-167`），失败时只 `slog.Warn`（`:162`）—— 而"停止新活"由**同一上限在下一次 `Charge` 上自然生效**，
所以这条 log-only 既不丢语义也不留缺口（前两类失败是防御路径，后两类是设计选择）。

**速率/限流面：节点级迁移上限已补实现（2026-10-05 落地；此前是"文档比代码说得多"）**

| 文档怎么声称 | 代码里实际有什么 |
|---|---|
| §13.7 前的"**每节点每分钟状态迁移上限**"（`:55`、`:59` 的四条硬上限、`:227`、S4 验收 ⑥"速率上限生效"） | **有（本轮补齐）**：内核写侧守卫 `(*Board).validateNodeRate`（`internal/agentbus/board/board.go`）——同一节点在一个 `nodeRateWindow`（1 分钟）内的迁移数 ≥ `Limits.NodeRatePerMinute` ⇒ 拒收 `rate_limited`；窗口只读**日志自带时间戳** ⇒ 重放不重判；`heartbeat`（续租）与 `actor=system` 的回收**不计入**；**零值 = 关**（内核不替使用者发明天花板） |
| "**主动限流**"、拒绝档 `rate_limited`（`:327`） | 有，但在 **talk 层**：`internal/agentbus/talk.go:235-245` 的 `rateLimited` 按 `lim.RateWindow`/`RateMax` 对**每话题的 talk 行**做滑动窗口，超限返回 `RefuseRate = "rate_limited"`（`talk.go:123`）；判定只读**日志自带的时间戳** ⇒ 重放同结论 |
| "连续 `refute` 触发**冷却**"（S4 验收 ⑥、`:55`） | 有，但语义是"**关闭后**的冷却"：`RefuseHearingCooldown = "hearing_cooldown"` + `Limits.Cooldown`（`hearing.go:60-61,177-180`，同样按日志时间戳），由 `HearingCooldownMinutes` 配置注入（`control/agentbus_hearing.go:32`）——限制的是**重新开启**已关闭的问题，不是"连续 refute" |

⇒ 三层现在**各就各位**：节点迁移在**内核写侧**（本轮补上），话题发言在 talk 层，听证冷却是裁决层 —— 文档与实现不再错位。
**处置（2026-10-05 已执行）**：走的是"**补一层节点级迁移速率闸门**"这条；另一条"收紧文档措辞"未采用，所以 **S4 验收 ⑥ 的原文一字未动**（这正是补实现的目的）。
旋钮 `[agentbus] node_rate_per_minute`（0 = 关，窗口固定 1 分钟），接线沿用 §13.12 的 `internal/boot/agentbus_wiring.go` → `control.AgentBusNodeRate`。
**可见性（G3）**：拒收由 `control.AgentBusNodeRateRefusals()` 计数、经 `desktop/agentbus_briefing.go` 的 `nodeRateSignal`
（kind **`node_rate`**，刻意不与 provider 429 那条已有的 `rate_limited` 混行）进面板。
**证据**：`internal/agentbus/board/rate_test.go`（六条：越限／窗口／heartbeat 豁免／零值／分节点／拒收不留痕且重放一致）、
`internal/control/agentbus_rate_test.go`（typed 透传 + 计数 + 映射）、`internal/boot/agentbus_node_rate_test.go`（真装配：设旋钮 ⇒ 第二动在工具面被拒且文案说 "not now"；不设 ⇒ 不拒）、
`desktop/agentbus_briefing_test.go` 的 `TestNodeRateSignalNamesTheCeilingThatRefused`。
**未做（如实）**：S4 验收 ⑥ 的另半句"**连续 `refute` 触发冷却**"仍只有"关闭后冷却"那一实现（见上表第三行），本轮不动语义。
**同族新缺陷的判据**：只有在"宿主机算出了一个状态、而**没有任何可见面**能让操作者看到它，且它**不会自愈**"时，才构成这一类问题。

**另一处已核实（2026-10-04）：地址簿 TTL 与两种投递失败的可见性。**
- **TTL**：`ParticipantTTL = 30min`（`internal/agentbus/directory.go:36-41`），由 headless tick **每 30s 续租**；
  `Lookup` 的态度是**设计上的并档**："A withdrawn participant is reported as absent, not as an address that no longer works."
  ⇒ 过期与"从未公告"合并成同一句 `no address owns it` —— 对操作者而言下一步动作相同（让那台宿主重新公告），不是缺陷。
- **两种失败可以分辨**：① 没人/没地址 ⇒ `serve: … no address owns it`；② **地址在、令牌文件没了** ⇒ `agentbus: the announced
  address carries no token file` / `read announced token: …`（`ReadAnnouncedToken`）。两者都经 waker 错误进 `noteWakeFailure`，
  面板那一行是 `Last` 的**原样转述** ⇒ 文本上分得清（差别在文案而不在结构，够用且不新增分类）。
- **TTL 行为已被钉**：`internal/agentbus/directory_ttl_test.go`（用 `-ParticipantTTL - time.Minute` 造过期公告，并断言
  `ParticipantTTL >= 10min` 以容纳 30s 续租节奏）⇒ 因此**没有必要**用两个真宿主等 30 分钟做近似读数（单测更强）。
- **退列会撤回（2026-10-04 核实，早已实现）**：`AgentBusAnnounce` 在 `internal/boot/agentbus_wiring.go:31`（每个入列宿主都公告）与
  `internal/cli/serve_multisession.go:71`（serve）调用；`AgentBusWithdraw` 在 **桌面关停**（`desktop/shutdown.go:102` 逐 tab 调
  `withdrawAgentBusOnShutdown`，其注释记着真机史：*a leftover headless serve stayed addressable for a day … a wake sent to it was
  accepted and dropped (2026-10-03)*）与 **serve 会话离场**（`internal/serve/multisession.go:236`）各调用一次；
  往返都有用例（`internal/control/agentbus_test.go:167`、`internal/serve/agentbus_withdraw_test.go:23`）。
  **残留只有硬杀**（关停钩子没跑到）：公告留到 30min TTL —— 而那段窗口里投递失败现在**看得见**
  （`AgentBusWakeFailures` 计数 + 面板 `wake_undelivered` 行，`d2081d750`）⇒ 连这条残留也不是"看不见"。

**同一族的第二例：":337 的收敛优先级没有单一实现点（2026-10-04 核实）**

文档那行是"**收敛**：预算 > 未决矛盾 > 轮次/速率上限 > 静默窗口"。代码里四个条件**都在**，但分散在三处、各答各的问题：

- **预算/轮次/速率**：都在 `talk.go` 的**逐行接受顺序**里（`:171` `rounds_exhausted` → `:174` `budget_exhausted` → `:177` `rate_limited`），
  答的是"**这一行能不能收**"——注意这个顺序与文档措辞**恰好相反**（先轮次后预算），但两者不是同一个问题；
- **未决矛盾**：在听证/裁决侧（`undecided-by-rule`、`SignalDisputed`，见上一节），答的是"**这个问题算不算定了**"；
- **静默窗口**：是**派生谓词** `TopicLapsed`（`talk.go:250-260`，只读日志时间戳）加一个**公开的收口** `CloseLapsed`/`CloseSilence`（`talklog.go:74`），
  注释写明 "Silence is visible work, never a quiet deletion"（`hearing.go:296-297`，T6-4）。

⇒ 与上一例同族：**不是"没实现"，而是没有一处代码把那条优先级写成判定顺序**。
**已决（2026-10-05，用户拍板，本刀 = `d35cdf8a4`）：不建单一收敛点。** 判据是把调用面查完 —— 那四个条件今天**各有各的读者**：
未决矛盾由 `landing.go` 与面板信号（`contested`/`undecided`）读，停滞由 `SignalStalled` 读，预算/轮次/速率由 `talk.go` 逐行拒收
（typed 回给调用者），没有一处需要"一个总结论" ⇒ 此时引入单一收敛点只会多出**第三个零调用方导出 API**
（前两个是 `Authorized` 与 `AgentBusTasks`，见 T9-7/T4-5 的诚实标注）。真要动，应等**"收敛后黑板冻结为结论"**那个机制本身落地时一起做，
而不是先造一个没人调的判定函数。
