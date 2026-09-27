# 宿主侧契约指纹（F1 另半边）

用途：与聊天侧指纹并列读，第三方不读代码即可判定「两侧认得的封闭面是否一致、差在哪一项」。
权威面：本机仓 `C:\guosj\ai\deepseek-reasonix\DeepSeek-Reasonix`（分支 `dev-2`）。表内路径都是仓内相对路径。
读法：每行都给 `文件:行`，打开即得该取值集合本身；「反例」列写明怎么读才算出错。

## 0 生成命令 + 时点

时点：随本表最后一次改动刷新（`git log -1 --format=%cI -- docs/COLLAB-SURFACE.md`）；仓根 `C:\guosj\ai\deepseek-reasonix\DeepSeek-Reasonix`（`dev-2`）。
值会随代码变，所以**载体是下面这五条命令，不是本表的字面值**——读的人重跑一次即可：

```
sed -n '10,14p' internal/boot/inbox_wake_contract_test.go   # §1 载荷字段集（契约用例）
sed -n '12,40p' internal/control/inbox_wake_marker.go        # §2 来源具名集合
sed -n '11,23p' internal/serve/reject_class.go               # §3 分类头 + 三档取值
sed -n '31,125p' internal/sessioninbox/types.go               # §4 state 封闭集
sed -n '92,95p' internal/plugin/plugin.go                    # §5 武装声明处（空 = 不武装）
sed -n '14,34p' internal/control/inbox_query.go              # §7 按 seq 回答的字段集与封闭值
sed -n '104,114p' internal/serve/inbox.go                      # §7 「队列里没有」那一档怎么答
sed -n '232,242p' internal/sessioninbox/types.go              # §7 回执上的同一对
sed -n '6,10p' internal/sessioninbox/settled_gloss.go          # §7 收尾措辞的唯一出处
sed -n '16,22p' internal/serve/inbox.go                       # §8 全局那条只读出口挂在哪
sed -n '190,226p' internal/control/inbox_query.go             # §8 字段集 + 失败那句原话
sed -n '230,240p' internal/control/inbox_dispatch.go          # §8 与回执共用的那一份判定
```

## 1 唤醒载荷（冻结的跨仓契约）

| 项 | 取值 | 定义处 |
|---|---|---|
| 字段集 | `seq` `from` `text` `topic` `mentions[]` `kind` `origin` | `internal/boot/inbox_wake_contract_test.go:10`（用例即契约） |
| 宿主实际消费 | **只有 `text`**：逐字落进 inbox 正文，不重解析 `mentions` | `internal/boot/inbox_wake.go:157` |
| 无正文 / 非法载荷 | 丢弃 + 告警，**不收成空回合** | `internal/boot/inbox_wake.go:31`、`internal/boot/inbox_wake_test.go:47` |

反例：把 `mentions` 当投递判据（宿主再筛一遍）——与契约相反，发送侧已判定「这行是给你的」。

## 2 宿主认得的「外来引导」来源值域

| 项 | 取值 | 定义处 |
|---|---|---|
| 标记形状 | `[remote wake source=<source> item=<id>]` + 换行 + 正文 | `internal/control/inbox_wake_marker.go:20`、`:39` |
| 具名集合 | `http` `push` `acp` `bot` | `internal/control/inbox_wake_marker.go:12` |
| 未具名但非空 | `unknown`（原值不当信息抹掉，也绝不当成本地排队） | `internal/control/inbox_wake_marker.go:37` |
| 空 source | **不打标记** = 本进程内排队 | `internal/control/inbox_wake_marker.go:31` |
| 位置约束 | 只在回合正文；**绝不进系统提示前缀**（前缀要字节稳定，前缀缓存靠它） | `internal/control/inbox_wake_marker_test.go:64` |

## 3 宿主回执的拒绝分类

| 项 | 取值 | 定义处 |
|---|---|---|
| 头名 | `X-Reasonix-Reject-Class` | `internal/serve/reject_class.go:11` |
| 值域（封闭） | `target_unreachable` / `not_accepting` / `invalid_request` | `internal/serve/reject_class.go:16`、`:19`、`:22` |
| 读侧规则（**聊天侧**，不在本仓） | 缺头或取值不认识 ⇒ 退回正文记号 | 房间条目 `P0/B76-B99B` |

## 4 inbox 条目（宿主落盘面）

| 项 | 取值 | 定义处 |
|---|---|---|
| 字段集 | `id` `sessionId` `intent` `state` `revision` `blobName` `source` `createdAt` `updatedAt` `preview` `byteSize` `checksum` `idempotencyKey` `refs[]` `blockReason` `runId` | `internal/sessioninbox/types.go:118` |
| `state` 封闭集 | `queued` / `steer_accepted` / `steer_consumed` / `running` / `blocked` / `uncertain` | `internal/sessioninbox/types.go:32` |
| 唤醒落地时 | `intent=steer`、`source=push` | `internal/boot/inbox_wake.go:35` |

`rejected_*` 是**回执的 `disposition`**，不是 `state`：两者别互相当成对方那一档。

## 5 宿主什么时候「武装」（不武装 = 零行为变化）

| 项 | 取值 | 定义处 |
|---|---|---|
| 声明处 | `Spec.WakeMethod`（MCP 通知方法名） | `internal/plugin/plugin.go:95` |
| 配置面 | `[[plugins]].wake_method`（TOML）/ `wake_method`（`.mcp.json` 同名字段） | `internal/config/plugin_entry.go:36`、`internal/config/mcpjson.go:37` |
| 空值 | **不武装**：一条字节都到不了会话 | `internal/plugin/client_wake.go:10`、`internal/plugin/client_wake_test.go:56` |
| 共享 host | 不装处理器（一个 host 服务多会话，没有绑定对象） | `internal/boot/inbox_wake.go:14` |

## 7 房间行的按 seq 回答（查找方向）

`GET /inbox/room-line?seq=N`（桌面桥同名调用走同一条）是这条查找的唯一出口：推送唤醒没有回执通道，
发送方问「我第 N 句叫的人到底怎么了」就问这里。

| 项 | 取值 | 定义处 |
|---|---|---|
| 字段集 | `itemId` `state` `source` `preview` `gate` `reason` `refused` `resumable` `queuedForMs` `settled` `settledAt` | `internal/control/inbox_query.go:22` |
| 「宿主拒过没」 | `refused` 就是那一栏；`resumable` **只在 `refused` 在时才有意义**，缺 `refused` 时不许拿“有没有 `reason`”反推 | `internal/control/inbox_query.go:26` |
| 时长 | `queuedForMs` 只答「等了多久」，不答「为什么还没跑」；非排队态不带 | `internal/control/inbox_query.go:29` |
| 收尾三档（离开队列后怎么结束的） | `acknowledged`（跑完并确认）/ `discarded`（被丢弃）/ `deleted`（被删掉）——存储自己的处置原词；措辞只有一处声明，桌面那份被同词测试钉住 | `internal/sessioninbox/ops.go:448`、`internal/sessioninbox/ops.go:144`、`internal/sessioninbox/ops.go:44`、`internal/sessioninbox/settled_gloss.go:6` |
| 同一对在回执上 | `settled` / `settledAt`（按幂等键查回执时同义） | `internal/sessioninbox/types.go:237` |
| 读法 | `found=false` **不等于**「从没接过」：带 `settled` = 离开过队列、这么结束的；不带才只说没有。端点据此判「这一路到底知道点什么」 | `internal/serve/inbox.go:109` |

反例一（`refused` → `resumable`）：缺 `refused` 时把“`resumable` 不在”读成**提不起来**——那两种情形在线上同形，
只有宿主真的为这行给过答复（`refused`）时`resumable` 才在说话；拿“有没有原话”反推也是同一个错。
读法三（不缓存）：这条路每次问都答**当下**——出参里没有哪一栏是上一份答案的重放。谁想复用旧答案，必须把「这是哪一次」的
时刻一起给人：**旧的答案不许当新的**（房间侧限频那一档就按这条做：带 `cached` + 不变的时刻）。
反例二（`settled` → `found`）：把 `found=false` 一律读成“没接过”（把跑完/被丢弃抹成没这回事）、或把它读成“没送达”
（给读者的动作完全不同）；再或把 `settled` 当成“他跑得好不好”——它只说**怎么离开队列**；不认得的处置词照原样带出，不加注解。

## 6 与聊天侧指纹的差项

待 A1 并列后逐项确认；以下三项已可直接判：

- **MCP 工具清单**：宿主侧没有对等物——它不是 MCP server。对等面是 §5 的配置字段集。
- **条文号集合**：宿主侧不使用 `B` 编号；规则落在 `docs/REMOTE_SESSIONS.md` 的节名上（例：「三种形态都长得像『会话停了』」）。
- **`delivery.path` / `skipped.reason` 值域**：只在聊天侧（hub 出参）。宿主侧对应的是 §3 的分类头与 §4 的 `state` / 回执 `disposition`。

## 8 队列全局那条只读出口（「我是不是被握着」）

§7 是**按行**问：要知道某一句的下落，得先有那一句的 seq。房间真正要先知道的是「此刻这个会话会不会跑东西」——
它还没投任何东西时也得能问。`GET /inbox/gate` 就是这条路：只读、无参数、不改变它报告的那个持有状态。

| 项 | 取值 | 定义处 |
|---|---|---|
| 出口 | `GET /inbox/gate`，与其它读一样按会话围栏，用同一套围栏校验 | `internal/serve/inbox.go:18` |
| 字段集 | `gate` `gateReason` `resumable` `pendingPrompt` `paused` `readonly` `queued` `oldestQueuedForMs` `startFailure` | `internal/control/inbox_query.go:193` |
| 与回执同一个来源 | 闸名来自同一个判定，回执与这条路不许各判一份 | `internal/control/inbox_dispatch.go:234` |
| 「为什么还没跑」 | 起回合本身失败时，闸名取 `start_failed`，`gateReason` 就是那句失败原文；比默认的闸类模板句更该被照抄 | `internal/control/inbox_query.go:224` |

反例（把这条路当队列内容读）：它答的是**持有**，不是条目。要知道有哪几条、各排多久，用 `GET /inbox` 的快照；
把 `queued` 当条目清单（它只是个数）是第一处读错，拿 `gate` 反推「我的那条跑了没」是第二处（那是 §7 的活）。
