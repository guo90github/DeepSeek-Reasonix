# 无人值守长任务：持续推进、保活与自愈

<a href="./UNATTENDED.md">English</a>

本文件说明「打开总开关之后，宿主自己把一条长任务一直推下去」的全部语义、
保活链路与已知边界。实现全部落在 `desktop/`（宿主 + Electron 壳 + 前端），
内核 `internal/control` 零改动。

## 1. 唯一闸门：总开关

- 位置：`<Reasonix home>/heartbeat-tasks.json` 顶层的 `unattended`（布尔）。
  桌面端在标签栏「新建会话」左侧有一个小药丸按钮可切换，tooltip 会写明生效时机。
- **生效时机：重启程序**。运行中的进程在 `HeartbeatEngine.Start()` 时快照一次
  （`e.unattended`），开关的读写都不会改变当前进程的行为。
- 开关关闭时：**一切照旧**——任务退回普通定时 prompt，没有任何无人逻辑运行。
- 开关打开时，无人值守**与任务有没有 `goal` 无关**：`goal` 只决定这条任务有没有契约要推进；
  停摆门清理、窗口耗尽接续会话、版本切换重启对**所有任务**生效。
- 总开关**同时持有 OS 级看门狗条目**（见 §10）：打开开关会注册看门狗，关掉会注销计划任务并
  删除桌面那份文件。条目是立刻写入的，但它和开关一样保护的是**下一次启动**——印记带的是
  启动时的值，所以那次运行才是崩溃后会被拉回来的那次。注册被系统拒绝只记日志，绝不回滚开关。
- 开关还会报告无人值守有没有**刹车**：`[agentbus]` 的四个预算层级一个都没设时（见
  `reasonix.example.toml`），它自己显示「已开启（无预算）」，宿主同时打一条同样的日志。
  宿主**不替使用者发明**默认额度：没人选择的额度会把**正在跑**的工作拦下来。
- 真要拦下来时，宿主会说清是**哪一层**拦的：每次拒绝留一条记录，带 `level` / `reason` / `limit` / `board` / `node`
  （node 层另附 `remaining`，因为被拒本身不花钱）以及逐进程的拒绝计数 ⇒ 不用面板也能回答"是哪条上限顶住了"。
  拒绝还会**按层计数**，协作面板为此单独给一行 ⇒ 这个回答连日志都不用翻。
  槽位上限是**宿主自己的**、不算成本刹车：被它拦下的活留在队列里**排队而不是失败**（节点状态不变），
  而手上已经没活的持有者会在下一次派发前把槽位交回来，不会永久占着。
- provider 的 429 也照同一个口径说清：进程计数回答"**有没有**在发生"，日志那一行再回答"**在哪条通道上**" ——
  provider 实例、协议，以及响应带了的话 provider 的 trace id；错误里没有的字段就**不写**，绝不猜。
  被吸收的 429 还会**按 provider 实例计数**，协作面板为此单独给一行 ⇒ 多 provider 的宿主不用翻日志就能说出是哪条通道被限。

## 2. 任务模型：`goal` 契约

`HeartbeatTask` 增加一个字段：

| 字段 | 含义 |
|---|---|
| `goal` | 非空 = 这条任务是无人值守长任务，其文本就是**目标契约**；空 = 今天的普通定时 prompt |
| `topicId` | 该任务唯一的会话绑定（驱动只碰这条会话） |
| `interval` | 唯一的限速：每次「看一眼」的间隔 |

判定表（`heartbeatGoalDecide`，纯函数）：

| 会话当前状态 | 动作 |
|---|---|
| 任务没有 `goal` | 正常提交 prompt（普通任务） |
| 会话 Goal 为空 | **锚定**任务契约，然后提交 |
| 会话 Goal ≠ 任务契约 | **重锚回契约**（并记一条日志），然后提交 |
| Goal 正在运行 | 让出本轮（Goal 自己的续轮在推进），下一个 interval 再看 |
| Goal 已完成 | 停手，不再投放（目标达成） |
| Goal blocked / stopped | **无上限续命**（每个 interval 一次），直到完成或开关关闭 |
| 总开关关闭 | 完全惰性，一行都不多走 |

### 「人已接管」不存在

产品决定：**唯一闸门就是总开关**。即使有人在开关打开时对这条会话明确干预
（包括改了 Goal），也一律按无人状态处理——驱动会把会话 Goal **重锚回任务契约**，
而不是让位或停手。因此本实现里没有任何基于「人类活动 / 回合来源 / 最后活动时间」
的接管判定；不要为了「更礼貌」再把它加回来。

归属由绑定给出：驱动只操作 `topicId` 指向的那条会话，别的会话它从不触碰。

## 3. 常驻印记与异常退出判定

`<Reasonix home>/desktop-host-state.json`（`host_state_marker.go`）：

```json
{ "schemaVersion": 1, "pid": 1234, "runId": "…", "version": "v0.0.0-dev.N",
  "phase": "running", "unattended": true, "startedAt": "…", "updatedAt": "…",
  "uncleanStreak": 1 }
```

- 每次启动写入（**不依赖遥测开关**；`diagnostics/lifecycle/*` 在 dev 版或关闭遥测时
  根本不写，不能当判据）。
- **干净退出**由 `completeDesktopShutdown` 删除该文件。
- 因此：**文件残留 + PID 已死 = 上次是非正常退出**；`unattended` 同时是"这次还该不该
  无人值守"的期望态，供重启策略与外部看门狗读取。
- `uncleanStreak`：10 分钟内连续出现非正常退出时累加；**达到 3 次则本轮启动降级**——
  驱动保持关闭（`Start()` 里与总开关相与），先把程序稳定起来，而不是陷入崩溃风暴。

## 4. 保活链路

| 层 | 机制 | 行为 |
|---|---|---|
| 服务子进程 | Electron `ServiceSupervisor` | 人活：3 次 / 5 分钟预算；**无人值守：不限次数 + 指数退避**（1s→2s→…→60s 封顶） |
| 整个壳 | `app.relaunch()` | 无人值守下重试耗尽时自拉起并退出当前进程；另加 3 次 / 15 分钟预算，防止坏构建无限重启 |
| 开机 | login item（**opt-in**） | 只有存在 `<home>/desktop-autostart.json` 且 `enabled: true` 才注册；关掉即注销，默认绝不碰你的机器。有版本目录的安装启动的是 `versions/<当前版本>/reasonix-desktop.exe`，由它再拉起壳；**绝不指向 `app\Reasonix.exe`**（壳自己找不到服务）。版本目录是动态递增的，所以每次启动都按 `current.json` 重写这条登录项，换版本自动跟过去 |
| 会话状态 | 印记 + goal sidecar | 重启后会话与 Goal 自动恢复，驱动下个 interval 继续推进 |

「人主动退出绝不拉起」由印记裁决：干净退出会删掉印记，看门狗/重启策略因此看不到期望态。

## 5. 停摆点自愈

无人值守下，凡「需要人点一下」的门都会被清掉（仅当开关打开且该任务有 `goal`）：

- **plan mode**：强制关闭（它的 approval gate 没人应答）。
- **暂停的 inbox**：自动 `SetInboxPaused(false)`（inbox 恢复流程默认暂停队列，等人确认）。

## 6. 窗口耗尽 → 接续会话

- 判定：`ContextSnapshot()` 用量 ≥ 窗口的 **90%**（`heartbeatWindowSpent`），可用
  `heartbeat-tasks.json` 的 `handoffPercent` 改（自动夹在 50–99；缺省即 90）。另外，
  **回合已因窗口上溢失败**的会话同样算耗尽——即使模型从未报过窗口大小（`ContextExhausted`）。
- 该判据**优先于 Goal hold**：Goal 运行中会自己驱动续轮，判据放在 hold 之后则永远轮不到——
  而它正是为这种长无人值守任务准备的。
- 回合若静默超过内核自己的停滞阈值（10 分钟），无人值守下会被**取消**，好让下一个 tick 走到
  这个判定而不是一直跳过；人活的会话绝不动（那里"工具很长"和"卡死"从外面看是一样的）。
- 动作：开一条**新会话**、把 `topicId` 切过去、并给它一条一次性前言：
  目标契约 + **旧会话的完整记录路径**（让模型自己去读，而不是让驱动编一份摘要）。
- 新会话按同一份 `goal` 契约重新锚定并继续推进；旧会话原样留在盘上可查，并留下一条
  "已交接给哪个会话"的通知。

### 时间戳口径

`lastRunAt` **只由真实运行推进**；驱动消耗掉却没有跑的 tick（Goal hold、拒绝恢复、
打不开话题）只记 `lastAttemptAt`。调度基准取两者较晚者——所以 hold 仍能把下一次尝试推迟一个
间隔，而任务列表里显示的"上次运行"是真运行；到期却没跑的会显示**已到期（晚了多久）**，
而不是永远停在"即将触发"（这正是被占用跳过的任务以前的样子）。

## 7. 代码地图

| 文件 | 职责 |
|---|---|
| `desktop/heartbeat.go` | 任务模型（`goal`/`unattended`）、引擎接线、提交前的一次性前言 |
| `desktop/heartbeat_converge.go` | 驱动本体：判定表、锚定/续命/让位、清停摆门 |
| `desktop/heartbeat_handoff.go` | 窗口耗尽判定与接续会话 |
| `desktop/host_state_marker.go` | 常驻印记与崩溃连击判定 |
| `desktop/heartbeat_store.go` | 配置读写（总开关由人工持有，整表保存不得丢） |
| `desktop/electron/src/main/{hostState,restartPolicy,autostart}.ts` | 印记读取、重启退避、opt-in 自启 |
| `desktop/frontend/src/custom/features/heartbeat/UnattendedToggle.tsx` | 标签栏总开关 |
| `desktop/portable_upgrade.go` | 版本切换：就绪门、指针交换、重启进新版本 |
| `desktop/electron/src/main/upgradeRollback.ts` | 切了但起不来时把指针写回上一个版本 |
| `docs/UNATTENDED*.md` | 本文件 |

## 8. 验证

```bash
cd desktop && go test -run 'TestHeartbeat|TestHostState' -count=1 .   # 驱动/印记/交接
cd desktop && go test -count=1 .                                     # 全量（见下方既有噪声）
cd desktop/electron && npm test && npm run typecheck                 # 壳侧策略
go run ./tools/repolint                                              # 注释/文件尺寸闸门
```

已知既有噪声（与本次无关）：`TestDesktopBuildScriptCompilesAndPackagesWindowsUpdateHelper`
断言 `scripts/desktop-build.sh` 的内容；electron 单测有 5 条 Windows 路径分隔符失败。

## 9. 无人值守下的自我升级切换（把长任务换成新版本继续跑）

任务的产物落在 `versions/<tag>/`，**版本号递增**；一个版本的**唯一启动入口是它自己的内层
`versions/<tag>/reasonix-desktop.exe`**。切换规则：

1. **首选信号是 SOP 自己维护的 `current.json`**：打包流程由本机 SOP（`REASONIX.local.md`）
   权威控制，它会把指针指向新 tag；只要指针指向另一个**完整**版本，就在**轮次边界**切换。
2. **兜底信号**：指针没动时，取 `versions/` 下**最新的、带就绪标记的**版本
   （`versions/<tag>/.reasonix-upgrade-ready.json`，内容 `{"ready":true}`——由任务在
   `verify-windows-portable.sh` exit 0 之后写入）。没有标记的目录一律不切：半解压的树绝不能
   变成活动版本。
3. **动作**：写 `current.json`（`installlayout.WriteCurrent` 会校验 `versions/<version>`、
   拒绝穿越与符号链接）→ 记下回滚目标（`desktop-upgrade.json`，phase=`pending`）→
   请求壳重启到**新版内层 exe**。
4. **只在回合边界做**：切换发生在驱动本轮开始、还没提交回合之前，不会打断在飞的回合；
   新进程下个 interval 继续推进同一个 Goal。
5. **失败回滚**：新版本起来后由它自己把 phase 标成 `healthy`；若壳在宽限期（10 分钟）内
   连不上服务，就把 `current.json` 写回上一个版本并重启回旧版内层 exe（3 次/15 分钟预算）。
6. **绝不删改已装版本**（与 SOP 的禁忌一致）：只新增指针与标记，不覆盖、不清理旧目录。

## 10. OS 级看门狗（整棵树被杀 / 断电后自动拉起）

进程自己死了就没有代码在跑，所以这一层只能由 OS 常驻者负责。看门狗 = 宿主二进制的一个
**shell 之前的模式**，由系统计划器周期调用：

```bash
reasonix-desktop.exe --watchdog          # 计划器每 5 分钟调一次（宿主日志留痕）
reasonix-desktop.exe --watchdog-status   # 查：策略 / 是否已注册 / 上次运行时间（从未运行会写明）/ 入口 exe
reasonix-desktop.exe --watchdog-enable   # 开：写策略 + 立刻注册（不等重启）
reasonix-desktop.exe --watchdog-disable  # 关：写策略 + 立刻注销 + 删除桌面那份文件
```

- **驱动者：总开关持有这个条目**。开总开关=注册看门狗并写出桌面文件；关总开关=注销计划任务并
  删掉文件，不需要人再记第二条命令。上面四条命令保留作巡检与人工覆盖用；每次启动都会把条目
  与配置文件里的开关对齐（旧宿主写下的开关、手改过的政策都会在此收敛）。非版本化安装
  （开发态运行）从不碰 OS 条目。
- **判据（只有这一条）**：印记存在 ∧ `unattended=true` ∧ 记录里的进程已不在 ∧ 崩溃在 24
  小时以内 → 用 `current.json` 的 active 版本内层 exe 拉起。干净退出会删掉印记，所以**人主动
  关掉永远不会被拉回来**。
- **注册**：Windows 计划任务 `ReasonixDesktopWatchdog`（每 5 分钟）；macOS
  `~/Library/LaunchAgents/io.reasonix.desktop.watchdog.plist`（RunAtLoad + StartInterval
  300）。两者都**只指向那个脚本，从不指向某个版本**。
- **策略**：`<home>/desktop-autostart.json`（`enabled` + `watchdog`，默认跟随 enabled），
  它是总开关的**镜像**，同时管登录项（Electron 的 login item，只在登录时拉起）——
  **开总开关会连登录项一起打开**，关掉则一起注销。
- **唯一那一个管理文件**：`<state home>\watchdog\watchdog.cmd`（Windows 上即
  `%APPDATA%\reasonix\watchdog\watchdog.cmd`）——看门狗的可见副本：
  双击=巡检一次、打开=看清它干什么、删掉=停用。它**不做版本判断**，只启动**稳定 launcher**
  并把模式用 `REASONIX_WATCHDOG=1` 传下去；launcher 每次运行自己解析 `current.json`，所以
  **打完包/切版本后拉起的必然是新版本**，不需要重写这个文件。应用每次启动与开关仍会重写它
  （路径可能变），关掉看门狗会删掉它；该目录里**只有这一个文件**，巡检记录写
  `<home>/desktop-watchdog.log`（每次一行：时间/动作/原因）。
  它原先放在 `…\Desktop\$`——**那是用户自己的目录**（106 项），迁移时被整目录强删（2026-10-03 复查，
  见 `docs/agents/TODO.md` 的"更正"）。现在发布与删除都只限本应用自己的目录，且拒绝**装有他人文件的目录**。
  注册条件也已改成显式要求
  `-AllowStartIfOnBatteries` / `-DontStopIfGoingOnBatteries` / `-StartWhenAvailable`：缺了它们，
  Windows 会接受任务、状态显示"已注册"，却**从不运行**。

## 11. 边界与未做

- **不迁移**旧会话的 `scopeID` / DeliveryCheckpoint / todos：新会话以同一 `goal` 文本
  重新锚定，交付证据链会断一截。
- macOS 用 LaunchAgent 的**周期巡检**（RunAtLoad + StartInterval），不用 `KeepAlive`；
  Electron main 自身崩溃且 relaunch 预算耗尽时会停在失败页（不会无限循环，这是有意的）——
  下一轮巡检会把它拉回来。
- 面板尚未显示总开关的当前值（配置文件的 `unattended` 是权威位）。看门狗没有自己的开关：
  它由总开关驱动（见 §10），CLI 留着做巡检。**注册为什么被拒**目前不在面板或看门狗状态里
  显示，只落在 `<home>/desktop-watchdog.log` 与宿主日志。
- 「异常杀进程 → 自主拉起 → 继续长任务」与「打包后自动切版本 → 继续推进」两条链都还没在
  真机跑过一次完整验收（均只做过单元级验证）。
