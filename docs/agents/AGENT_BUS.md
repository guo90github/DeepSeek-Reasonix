# 多智能体契约（AGENT_BUS · 黑板为骨）

> **状态：目标契约（尚未实现）。** 本仓当前**没有** `internal/agentbus`，因此下表「定义处」一栏凡标 *待建* 的
> 都是落地后必须回填的 `文件:行`，不是现有代码的指针。别把本稿当已实现面读。
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
| 真相与协议在内核 | 黑板与消息信封放 `internal/agentbus`（*待建*）；命令面加在 `control.Controller`；传输面加在 `serve` | 按 `REASONIX.md` 约定，CLI / serve / 桌面**自动继承**（`tools/repolint/layers.go:71-74`） |
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

三件里**只有黑板是待建**的，另两件已在仓里。

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

## 2 消息信封（*待建* `internal/agentbus/message.go`）

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

`assert` / `refute` / `claim` / `release` / `split` / `require` / `decide` / `abandon` / `revert` / `yield`

| 动词 | 语义 |
|---|---|
| `assert` | 在节点上落一个断言或产出，**须带证据引用**（证据须可核对，否则只算 `assert`） |
| `refute` | 反驳某条断言/产出，**须带理由**；反驳与断言同级，不是「下级打小报告」 |
| `claim` / `release` | 认领 / 放回一个节点（认领 = 承诺，**带 deadline 且须心跳续租**；超时被回收） |
| `split` / `require` | 拆分节点 / 声明对新节点的依赖——**DAG 由此在运行时长出来**；跨子树时生成**边界节点** |
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

## 5 存储（*待建*；落地前按落点确认）

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
| S1 | 黑板内核：op 日志 + 确定性 fold + **节点状态机（含 `capability_gap` / `abandon` / `revert`）** + 逐节点租约 + **心跳/回收**（*待建* `internal/agentbus/board`；规格见 §11.1） | 非法迁移拒收（**带原因**）；op 重放幂等；并发 op 折叠结果与串行一致；**杀掉推 op 的进程后，剩余 op 仍能折出完整状态**；**两个不同进程**并发写同一作用域不丢 op；**认领者被杀后节点在 deadline 内被回收并记 `no_progress`**；`revert` 后下游正确标 `stale`；**无证据的 `abandon`（与无证据的 `decide(done)`）被拒**；`seq` 跨进程单调不重号；**截断尾行不计入 op 且计数可见**；成环的 `split`/`require` 被拒；`agentbus` 不 import `agent`/`control` |
| S2 | 写侧接线 + 读侧投影（**含跨工作区与子树分片**）：多会话可写；**视图裁剪**（每轮只读我的子树+我的节点+订阅摘要）；读侧复用 `internal/taskcatalog` / `internal/taskmonitor` / 桌面任务树 | **两个不同工作区**的会话对同一黑板 `claim`/`assert`/`refute` 全部可见；**跨子树只经边界节点**；某 agent 的一轮请求里**不含别人的子树**（§8 第三条守卫）；视图**增量读**（cursor/seq）而非每轮全量；`make frontend-check` 过 |
| S3 | **交流三档**（§3.2）：① 自由对话 `say` + 话题边界；② 有界点对点 `ask`/`answer` + 回执（§7 缺口）；③ `results/<correlation>.json` | 话题轮数/预算/静默窗口任一触顶即收口；**自由对话默认不灌上下文**（只投点名与摘要）；`correlation` 的预算/TTL/hop 生效；跨进程目标带令牌；超速被拒时返回 `rate_limited`；**就绪节点用事件唤醒（`interval` 只兜底）** |
| S4 | **审议**与裁决：审议状态（参与者/轮次/必答/权重/冷却）+ `decide` 规则 + 人作为参与者 + **升级配额** | ① 一次 `refute` **改变**结论（可回放：同一 op log 折叠出不同结局）；② **等重升级给人**，不被规则拍死；③ 弃答以 `no_answer` 可见；④ 争议节点可由人在桌面下结论；⑤ **无可核对证据时 `done` 被拒**（只允许留 `assert`）；⑥ **连续 `refute` 触发冷却**，速率上限生效；⑦ 超升级配额时**自动降级为 `undecided-by-rule`** 并记账；⑧ **票数不改变权重**（consensus ≠ evidence） |
| S5 | **集群调度与预算**：**本机 host 级**并发槽 + 排队 + 四级预算（board → 子树 → 节点 → 回合）+ 触顶即暂停（不杀会话）+ **调度四则**（关键路径优先 / 批量领取 / 同子树亲和 / 最难优先） | 并发槽满时新回合**排队而非失败**（依赖 T2-4 定死的持久队列落点）；槽与预算在**本机**被 host 级共享（多会话/多进程同算一池；**跨机不在 v1**）；触顶后**现场保留**、重启可续；`rate_limited` 与 provider 429 不再互相放大；**关键路径上的节点先跑**；**最难节点不被饿死**；**预算只被验收节点消耗**（做工不消耗总预算） |
| S6 | **观测聚合**（人读）：按子树折叠、只显异常/争议/停滞/孤儿、可下钻到节点 | 100 节点规模下首屏不画 >N 张卡片（阈值可配）；孤儿节点与停滞节点**必现**；下钻到节点能看到参与者、证据与 `no_progress` 记录 |
| S7 | e2e + **无人值守贯通** + 完成判定 + **N≈100 压测** | ① 杀掉编排者，未完成节点仍被其他参与者认领并推进；② 一次 `refute` 改变结果；③ 运行时依赖：节点/边数在运行后增加，且由参与者而非派发方新增；④ **无人值守贯通**：杀掉桌面进程 → 既有看门狗拉起 → 黑板继续被推进（`docs/UNATTENDED.md` §11 自认这条链还没真机验收，所以第一次必须端到端跑一次）；⑤ **任务级落地**：验收节点全 `done` + 无未决矛盾才宣告完成，否则停在 `blocked` 并保留现场；⑥ **百级压测**：N≈100 并发参与、任意杀掉 20% 后仍收敛；总支出不超预算；无 429 风暴；**存在一次"缺能力→派生获取能力节点→完成"的真实链路** |
| S8 | PR 元数据门 + 打包 | `verify-windows-portable.sh` exit 0；`Cache-impact`/`Cache-guard`/`System-prompt-review`/`Documentation-impact` 齐全 |

### 11.1 S1 规格（实现前定死；S1 实现以此为准）

**包与分层**：`internal/agentbus`（信封与投影，*待建*）+ `internal/agentbus/board`（S1 本体）。
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

**明记为边界（不在 S1 修，已在 TODO 挂节点）**：① 写路径每次全量读 + 全量 fold（O(n)/写，总 O(n²)）⇒ S2 用增量 fold/游标替换并带实测预算（TODO T4-8）；② `evidence` 只有"`Ref` 非空"这一条约束，形状与唯一性留给 S3/S4 的证据权重设计（TODO T5-6）；③ `Sweep` 单次上限 256，节点按 id 排序故无永久饥饿，但需在文档写明（TODO T7-5）。

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
| 智能体（机读） | `agentbus.View`：固定头（`schema` / `board` / `participant`）+ **自游标起的增量行** | 默认 ≤ 8 KiB/轮、行数 ≤ 200 | 每轮拉一次（游标 = `seq`），**不重发历史** |
| 人（桌面 / CLI） | 既有任务树事件（`agent.*` 事件族）+ 节点行 | 复用现有面板阈值 | 沿用既有 5 s 轮询，**不新增轮询** |
| 编排者 | 同智能体视图，scope 覆盖整棵子树 + 全局计数 | 同上，另加 ≤ 1 KiB 计数头 | 同上 |

**行字段集**（机读行；顺序固定、只追加）：`id` / `title`（截 120 字符）/ `state` / `outcome` / `owner` /
`deadline`（RFC3339 或空）/ `deps_done`(bool) / `deps_open`(int) / `evidence`(int) / `refuted`(bool) /
`no_progress`(int) / `last_seq`。

**「我的子树」怎么廉价物化**（T2-1 的关键）：**不做逐参与者索引**。每轮 fold 一次（一次 O(nodes) 遍历）后用
两条集合运算裁剪：① 我拥有、或我断言过的节点；② 反向可达闭包里**状态非 `done`** 的依赖（谁在等我）。
N≈100、节点数百量级时这是几百次访问的过滤，比维护逐参与者索引的失效成本低一个量级。超上限时：
按（状态优先级，`last_seq`）取前 K 行，头部给出**被截断的计数**与"用游标继续"的指针——**不许静默丢**
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
