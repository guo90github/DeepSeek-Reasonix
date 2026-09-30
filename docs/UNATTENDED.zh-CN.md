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
- 开关关闭时：**一切照旧**——带 `goal` 的任务退回普通定时 prompt，没有任何无人逻辑运行。

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
| 开机 | login item（**opt-in**） | 只有存在 `<home>/desktop-autostart.json` 且 `enabled: true` 才注册；关掉即注销，默认绝不碰你的机器 |
| 会话状态 | 印记 + goal sidecar | 重启后会话与 Goal 自动恢复，驱动下个 interval 继续推进 |

「人主动退出绝不拉起」由印记裁决：干净退出会删掉印记，看门狗/重启策略因此看不到期望态。

## 5. 停摆点自愈

无人值守下，凡「需要人点一下」的门都会被清掉（仅当开关打开且该任务有 `goal`）：

- **plan mode**：强制关闭（它的 approval gate 没人应答）。
- **暂停的 inbox**：自动 `SetInboxPaused(false)`（inbox 恢复流程默认暂停队列，等人确认）。

## 6. 窗口耗尽 → 接续会话

- 判定：`ContextSnapshot()` 用量 ≥ **96%**（`heartbeatWindowSpent`）。
- 动作：开一条**新会话**、把 `topicId` 切过去、并给它一条一次性前言：
  目标契约 + **旧会话的完整记录路径**（让模型自己去读，而不是让驱动编一份摘要）。
- 新会话按同一份 `goal` 契约重新锚定并继续推进；旧会话原样留在盘上可查。

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

## 9. 边界与未做

- **不迁移**旧会话的 `scopeID` / DeliveryCheckpoint / todos：新会话以同一 `goal` 文本
  重新锚定，交付证据链会断一截。
- 未做 macOS `LaunchAgent KeepAlive`：Electron main 自身崩溃且 relaunch 预算耗尽时会停在
  失败页（不会无限循环，这是有意的）。
- 面板尚未显示开关的当前值（配置文件的 `unattended` 是权威位）。
- 「异常杀进程 → 自主拉起 → 继续长任务」的整条链尚未在真机跑过一次验收。
