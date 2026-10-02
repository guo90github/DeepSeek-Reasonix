# 远端任务板（远端 tab 也显示跨清单欠账）

状态：立项。基线 `941356d38`（含 todo 身份修复 `df524907a` 与 `/todos` 带 `step_id`）。

## 1. 目标

桌面开着**远端 tab**（连另一台 Reasonix 宿主）时，任务面板与本地 tab 同口径：
既显示该会话的当前清单，也显示**跨清单仍未完成的欠账**（板子的 queue），并显示已完成历史（archive）。

一句话：把本地已经一致的那套（canonical + queue + archive 三合一）补齐到远端路径上。

## 2. 不包含

- 不改本地 tab 的行为（已经一致）。
- 不改板子的身份与合并语义（`todoBoardKey` 按 `step_id` 认账已落地，不在本立项范围）。
- 不为远端引入"关闭批次"的远端写接口：远端 tab 的关闭只影响本地显示（见 §7 未决 1）。
- 不在本仓库改手机端（`android/` 是独立仓库），只把它列为消费方。

## 3. 现状与证据

| 环节 | 现状 | 位置 |
|---|---|---|
| 控制层端口 | **已提供** `Todos()` 与 `TodoBoard()` | `internal/control/port.go:237-240`；实现 `internal/control/todo_settle.go:46` |
| 本地 tab meta | canonical + batchId + queue + archive + supervised 全带 | `desktop/app.go:6565-6575`、`6718-6722`、`6734` |
| serve | 只有 `GET /todos`（**裸数组**、canonical、已带 `step_id`） | `internal/serve/serve.go:657`、`1659-1672` |
| 远端客户端（桌面） | `RemoteTabSnapshot` 并行取 `/history /context /todos /checkpoints /models /commands /status` | `desktop/remote_tab_snapshot.go:73-121` |
| 契约 | `RemoteTabSnapshot` 是 Go RPC，`todos?: unknown[]` | `generated/desktopContract.generated.ts:3389,4992`；`lib/remoteTypes.ts:77-86` |
| 前端消费 | 远端**整块忽略** meta：canonical / queue / archive 三处全 `remote ? undefined :` | `app-runtime/useTodoPanelCommands.ts:81,91,99` |

结论：唯一缺的是「板子过线」。控制层早就有，serve 与远端客户端没有；前端因此只能靠转录里最后一条 `todo_write` 显示当前清单。

## 4. 设计

1. **serve 新端点** `GET /todos/board` → `{"queue":[item…],"archive":[item…]}`；item 与 `/todos` 用**同一个投影函数**（含 `step_id`），空侧输出 `[]` 而不是 `null`。
   不改 `/todos` 的形状：它今天是裸数组，仓库外客户端（手机）已在解析，改成对象属破坏性变更。
2. **桌面远端客户端**：把 `/todos/board` 加进 `remote_tab_snapshot.go:94-102` 的取数 map，`RemoteTabSnapshot` 增 `todoQueue` / `todoArchive` 两个 `json.RawMessage` 成员；**保留现有容错**——除 `/history` 外取数失败只留空，不让整个快照失败。
3. **契约**：重生成 `desktopContract.generated.{json,ts}`（Go 结构体是源头），并把 `todos` / `todoQueue` / `todoArchive` 从 `unknown[]` 收紧为带 `step_id` 的 wire 类型。
4. **前端**：`useTodoPanelCommands` 去掉三处 `remote ? undefined :`，改成「有就消费、没有退回 canonical」。合并继续走 `mergeTodoBoardQueue`（按 `step_id` 键）——与本地同一函数、同一语义，不新写一套。

## 5. 分阶段

- **P0**：serve 端点 + 边界用例；桌面取数与结构体成员 + 契约重生成。（可独立合并）
- **P1**：前端消费 + 远端回归用例（把 `todo-queue-repro.test.ts` 的 S3 断言按 `remote: true` 再跑一遍）。
- **P2（本仓库外）**：手机端解析 `step_id`，必要时再消费 board。

## 6. 验收

- serve：`/todos/board` 返回 queue/archive，每项带 `step_id`；空板子输出 `[]`。
- 桌面：远端快照含两个新成员；`/todos/board` 失败时快照仍成功（只少这两项）。
- 前端：远端 tab 面板 = queue 在前 + 当前清单，按 `step_id` 去重（同 id 改标题只出一行）。
- 真机：桌面开一个远端 tab，面板应与本地 tab 同口径。
- Cache：**none** —— 只动 serve HTTP 与桌面/前端 RPC，不碰 provider 可见前缀（golden 与工具 schema 不变）。

## 7. 未决（附默认取向）

1. **远端 tab 的"关闭批次"**：`todoBatchId` 不在远端快照里。默认：远端 tab 的关闭只写本地（dismissal 存储），不向远端宿主发写请求；日后若要远端持久化，另立写接口。
2. **快照成员并行取**：canonical 与 board 可能来自不同瞬间。默认接受：合并按 `step_id` 幂等，陈旧的 queue 只会多显示欠账、不会误删；不做跨请求一致性。
3. **旧宿主兼容**：老版本远端宿主没有 `/todos/board` → 取数留空 → 远端 tab 退回"只显示当前清单"（等于今天行为）。不做版本协商。

## 8. 风险

- 契约重生成会动 `desktopContract.generated.*`：那是桌面 RPC 面、不是模型前缀，但容易与本地其它在建改动撞文件，建议单独 PR。
- `/todos` 与 `/todos/board` 的 item 投影必须一致：抽同一个函数，避免两处漂移（`internal/serve/todos_test.go` 的断言可复用）。
