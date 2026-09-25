# 宿主侧契约指纹（F1 另半边）

用途：与聊天侧指纹并列读，第三方不读代码即可判定「两侧认得的封闭面是否一致、差在哪一项」。
权威面：本机仓 `C:\guosj\ai\deepseek-reasonix\DeepSeek-Reasonix`（分支 `dev-2`）。表内路径都是仓内相对路径。
读法：每行都给 `文件:行`，打开即得该取值集合本身；「反例」列写明怎么读才算出错。

## 0 生成命令 + 时点

时点：`2026-09-26 02:54 +0800`；仓根 `C:\guosj\ai\deepseek-reasonix\DeepSeek-Reasonix`（`dev-2`）。
值会随代码变，所以**载体是下面这五条命令，不是本表的字面值**——读的人重跑一次即可：

```
sed -n '10,14p' internal/boot/inbox_wake_contract_test.go   # §1 载荷字段集（契约用例）
sed -n '12,40p' internal/control/inbox_wake_marker.go        # §2 来源具名集合
sed -n '11,23p' internal/serve/reject_class.go               # §3 分类头 + 三档取值
sed -n '31,120p' internal/sessioninbox/types.go               # §4 state 封闭集
sed -n '92,95p' internal/plugin/plugin.go                    # §5 武装声明处（空 = 不武装）
```

## 1 唤醒载荷（冻结的跨仓契约）

| 项 | 取值 | 定义处 |
|---|---|---|
| 字段集 | `seq` `from` `text` `topic` `mentions[]` `kind` `origin` | `internal/boot/inbox_wake_contract_test.go:10`（用例即契约） |
| 宿主实际消费 | **只有 `text`**：逐字落进 inbox 正文，不重解析 `mentions` | `internal/boot/inbox_wake.go:48` |
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
| 字段集 | `id` `sessionId` `intent` `state` `revision` `blobName` `source` `createdAt` `updatedAt` `preview` `byteSize` `checksum` `idempotencyKey` `refs[]` `blockReason` `runId` | `internal/sessioninbox/types.go:100` |
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

## 6 与聊天侧指纹的差项

待 A1 并列后逐项确认；以下三项已可直接判：

- **MCP 工具清单**：宿主侧没有对等物——它不是 MCP server。对等面是 §5 的配置字段集。
- **条文号集合**：宿主侧不使用 `B` 编号；规则落在 `docs/REMOTE_SESSIONS.md` 的节名上（例：「三种形态都长得像『会话停了』」）。
- **`delivery.path` / `skipped.reason` 值域**：只在聊天侧（hub 出参）。宿主侧对应的是 §3 的分类头与 §4 的 `state` / 回执 `disposition`。
