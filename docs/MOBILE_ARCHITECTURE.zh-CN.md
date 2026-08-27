# 移动端架构设计（手机复刻桌面能力）

> 状态：草案 v0.1（2026-08-25） · 范围：路径 A（手机 → 桌面 `serve` 桥，薄客户端）优先，路径 B（常驻云）作为演进方向
> 愿景：让用户在手机上获得与桌面端一致的全部 agent 能力（会话、工具、MCP、审批、记忆），与桌面端丝滑同步，从工位解放。

---

## 1. 结论先行

**架构上 80% 已就绪。** 本项目把 agent 内核做成传输无关的 `control.Controller`，所有前端（`cli` / `serve` / `acp` / `bot` / 桌面端）共享同一内核与同一会话存储。`internal/serve` 已经是一个**几乎完整的薄客户端后端**：SSE 事件流 + 全量 REST 命令面 + token/password 认证 + CSRF 防护。

因此手机端的主要工作量不是"再造一个后端"，而是：

1. **一个 Expo/React Native 移动前端**，消费 serve 的 REST + SSE 面（桌面的 transcript/审批/工具渲染是 WebView2 特化的，移动端需重做渲染层，但数据模型与事件模型同源可借鉴）；
2. **远程安全通道**（token 认证 + SSH 隧道/TLS，可复用 `internal/remote` 的端口转发）；
3. **推送通知**（回合完成 / 需要审批）；
4. **双端并发会话模型**（手机与桌面同时操作同一会话）。

---

## 2. 现状盘点（基于当前代码）

### 2.1 内核与前端分层

```
            ┌────────────── control.Controller（传输无关内核）──────────────┐
            │   会话 / 回合 / 审批 / 上下文压缩 / 记忆 / 工具 / MCP          │
            └──────┬──────────┬──────────┬──────────┬──────────┬──────────┘
                   │          │          │          │          │
                cli/      serve/     acp/      bot/      desktop/
               (TUI)   (HTTP+SSE)  (ACP协议) (IM渠道)  (Wails)
                   └──────────┴──────────┴──────────┴──────────┘
                              mobile/（本设计：新的前端）
```

分层规则（`REASONIX.md` 强制）：只有前端（`cli`/`serve`/`acp`/`bot`/`botruntime`/`boot`）与宿主（`cmd/`/`desktop/`）允许 import `control`。**移动端作为 `serve` 的 HTTP 客户端，不直接 import 内核**——它通过 serve 的协议面访问，天然符合分层。

### 2.2 `internal/serve` 已暴露的协议面（`serve.go` `handler()`）

| 面 | 端点 | 用途 |
|---|---|---|
| 事件流 | `GET /events` | SSE，广播 `event.Event`（一次 marshal、多订阅者扇出，见 `broadcaster.go`） |
| 会话 | `GET /sessions`、`GET /sessions/{id}`、`GET /history`、`GET /checkpoints`、`GET /branches` | 多会话列表 / 历史 / 检查点 / 分叉 |
| 上下文 | `GET /context` | 上下文窗口占用（prompt vs window） |
| 命令 | `POST /submit`、`/cancel`、`/approve`、`/plan`、`/compact`、`/new`、`/rewind`、`/fork`、`/summarize`、`/resume`、`/forget`、`/goal`、`/answer`、`/tool-approval-mode`、`/auto-approve-tools`、`/bypass` | 提交 / 取消 / 审批 / 计划 / 压缩 / 新会话 / 回滚 / 分叉 / 摘要 / 恢复 / 多会话命令 |
| 收件箱 | `registerInboxRoutes` | 排队式跟进（类似桌面 composer inbox） |
| 杂项 | `GET /status`、`/models`、`/skills`、`/todos`、`/provider-setup` | 运行时状态 / 模型 / 技能 / todo / 提供商安装 |

**结论：薄客户端的"后端"已经存在且相当完整。** 移动端几乎不需要给 serve 加新端点（除推送注册、隧道协商等少量扩展）。

### 2.3 认证（`serve/auth.go`）

- 模式：`none` / `token` / `password`；token 模式为 256-bit 随机预共享令牌（未配置则自动生成），可经 URL 或 cookie 携带（`reasonix_token`）。
- CSRF：`csrfGuard` 要求所有 POST 为 `application/json`，阻断跨站表单驱动。
- 注意：默认绑定 localhost、无 CORS；`HandlerWithCORS` 仅限本地开发（无 auth 时勿用于生产）。

### 2.4 可复用的远程基建

- `internal/remote`：SSH 传输 + 端口转发（`reasonix serve` 可在远端经转发的 loopback 端口被访问）——**手机 → 常驻桌面/家里机器 的通道可直接复用**。
- 桌面 bot 桥（`bot_connection_app.go` 微信/飞书）：已经是一种"手机随时触达"，但受限于 IM 文本交互；移动 App 是它的超集。
- `internal/notify`：桌面端已能把 `TurnDone` / `ApprovalRequest` / `AskRequest` 转成系统通知——移动端推送可复用同样的"事件 → 通知"判定逻辑（`notify.message`）。

---

## 3. 架构决策

### 3.1 路径选择：A（薄客户端）→ 演进 B

| | A. 手机 → 桌面 serve 桥 | B. 手机 → 常驻云 serve |
|---|---|---|
| 能力复用 | 100%（同一 controller / 同一会话存储 / 同一 MCP） | 100%（需会话迁移/同步） |
| 前置条件 | 桌面或家里机器常开 | 常驻服务器 + 域名/TLS |
| 同步复杂度 | 低（同一数据主目录即天然共享） | 高（双端并发 + 迁移） |
| "从工位解放" | 依赖桌面开机 | 彻底 |
| 起步成本 | 低（复用 serve 面） | 高 |

**决策：先做 A。** serve 的协议面就是桥；`mobile/` 是 Expo/RN 环境（已装 expo、react-native，跑过 export/start，尚无 app 骨架），从零搭一个薄客户端。B 留作演进：A 稳定后，把 serve 部署到常驻机器并补会话同步即可平移。

### 3.1.1 部署拓扑（已确认基线：手机直连桌面本机）

```
手机 App ──LAN/WiFi（开发）或 SSH 隧道/TLS（远程）──► 桌面机
                                                     ├─ Reasonix.exe（桌面端，Wails）
                                                     └─ reasonix serve（CLI 独立进程，--auth token）
                                                          │
                                                          ▼
                                                  同一 REASONIX_HOME
                                                  会话/历史/检查点/MCP/记忆 天然共享
```

- 桌面机同时运行两个进程：桌面 App（用户日常操作）与 `reasonix serve`（手机桥接）。二者通过**同一 `REASONIX_HOME`** 读写同一会话存储，因此手机看到的会话与桌面完全一致，**无需任何同步协议**——SSE 双向广播本身就是两端同步机制。
- 桌面端的 single-instance 锁（`desktop/app.go singleInstanceID`）是**桌面进程级**的，按数据主目录哈希——它只约束桌面 App 自身，不阻止 serve 独立进程共存。但两个进程同时写同一会话存储的**并发锁**需要验证（见第 5 节待验证项）。
- 手机仅作薄客户端：不持有会话，不落本地数据（除令牌与缓存）。

### 3.2 系统组件图（路径 A）

```
┌──────────────┐   HTTPS/WSS over    ┌──────────────────────┐   HTTP/SSE    ┌────────────────────────┐
│  手机端 App   │ ─────────────────► │  SSH 隧道/LAN + TLS   │ ───────────► │  reasonix serve        │
│  Expo/RN      │   (token 认证)     │  (internal/remote 复用│              │  token/password auth    │
│               │                    │   端口转发 + hostkey) │              │  CSRF + SSE broadcaster│
└──────┬───────┘                    └──────────────────────┘              └───────────┬────────────┘
       │                                                                            │
       │  UI 层（RN 渲染）                                                          ▼
       │  · transcript（FlatList + 行高估计，借鉴桌面 transcriptStore 思路）   control.Controller
       │  · composer / 审批卡片 / 工具结果 / 多会话 / todo / 上下文环          （前端无关内核）
       │  · 推送通知（回合完成/审批）                                                  │
       │  · Keychain 令牌存储                                           会话存储/历史/MCP/审批/记忆
       ▼
  数据层（REST + SSE 客户端）
   · /events SSE → 增量事件 → 驱动 UI
   · /submit /approve /cancel /plan … → 命令
```

### 3.3 传输与协议

- **事件面**：`GET /events`（SSE）。客户端维护连接，按 `event.Event` 的 kind 增量更新 transcript 与状态栏。事件模型与桌面 `event.WireEvent` / `tabEventSink` 同源——移动端只消费同一协议的 HTTP 变体。
  - **重连**（已确认：serve 当前不发送 `Last-Event-ID`/事件序号）：断线重连后以 `GET /history` 全量重放当前会话 + 本地按 `submissionId`/回合序号去重；这是 MVP 基线。serve 侧可选增强：为事件流加单调序号，支持增量续传。
- **命令面**：REST JSON。`POST /submit`（提交回合）、`POST /approve`（应答审批，`approval_request` 事件后的闭环）、`POST /cancel`、`/plan`（计划确认）、`/compact` 等。
- **认证**：token 模式为主。令牌在手机端入 Keychain（iOS）/ SecureStore（Android）；serve 端 `--auth token`。所有请求带 `Authorization: Bearer <token>` 或 cookie；POST 一律 `Content-Type: application/json`（满足 CSRF 守卫）。
- **通道**：局域网直连（开发/演示）→ SSH 隧道（复用 `internal/remote`：手机端如果无法跑 SSH 客户端，由 serve 宿主侧 `ssh -L` 建隧道，或手机 App 内嵌 SSH 客户端库）→ TLS 反向代理（生产）。

### 3.4 移动端应用分层

```
┌───────────────────────────────────────────────┐
│ 交互层  composer / 审批卡片 / 工具折叠 / 多会话   │
├───────────────────────────────────────────────┤
│ 渲染层  FlatList transcript（行类型 → 组件映射） │
│        · 消息 / 思考 / 工具调用 / 计划 / 压缩卡片  │
│        · 行高估计 + 实测校正（借鉴桌面几何思路，    │
│          但 RN 用 onLayout，无 Virtuoso/WebView2）│
├───────────────────────────────────────────────┤
│ 状态层  会话状态机（queued/running/reasoning/    │
│        responding/tool/retrying/…，对齐 useController）│
├───────────────────────────────────────────────┤
│ 数据层  REST 客户端 + SSE 订阅 + 本地缓存/离线队列 │
│        · 提交乐观入队（submissionId 幂等，对齐    │
│          桌面的 submissionID 关联）              │
│        · SSE 事件 → reducer（对齐 useController）│
├───────────────────────────────────────────────┤
│ 平台层  Keychain 令牌 / 推送注册 / 后台任务       │
└───────────────────────────────────────────────┘
```

关键移植点：桌面的 `useController` reducer 与事件驱动模型、`submissionId` 幂等关联、审批/ask 闭环——这些是**协议级概念**，可平移；Virtuoso 虚拟滚动、WebView2 测量冻结是 WebView2 特化的，RN 用 FlatList + `onLayout` 重做。

### 3.5 推送通知

复用桌面 `internal/notify` 的"事件 → 通知"判定（`TurnDone` / `ApprovalRequest` / `AskRequest`）：

- **方案 1（最小）**：serve 侧加 `POST /push-register`（设备令牌），serve 进程内嵌一个"事件 → APNs/FCM"桥，把 `notify.message` 的判定逻辑搬到 serve。成本低、闭环完整。
- **方案 2（零后端）**：手机 App 在后台保持 SSE 长连接（或系统级后台刷新），事件到达本地再发本地通知。实现最简，但有电池/存活代价。
- **方案 3（桌面代发）**：桌面端已能发系统通知；若手机与桌面在同一局域网，桌面把通知转发到手机。仅限局域网。

**建议：先方案 2（本地通知，零后端）跑通 MVP，M3 再上方案 1（FCM/APNs）。**

### 3.6 双端并发与会话同步（已确认部署：同机共享主目录）

- **数据主目录**：serve 与桌面跑在同一机器、共用同一 `REASONIX_HOME`（已确认基线），会话存储天然共享（serve 的 `/resume` `/new` `/fork` 在同一存储上工作）。桌面与手机"同时看同一会话"即达成——无需额外同步协议。
- **并发写**：`control.Controller` 每控制器单回合（`admission_guard.go`：running 时新提交丢弃/排队）；手机与桌面同时提交 → 一方 `ErrTurnRunning` 或进入 inbox 排队（serve 已有 inbox 路由）。UX 约定：手机端显示"桌面端正在运行该会话"状态（事件流中 `turn_started`/`turn_done` 天然可推）。
- **会话切换**：桌面切换会话不影响 serve 侧（各自独立的 controller 实例，同一存储）；同一会话被两前端同时打开时，SSE 事件流天然广播，两端都会收到对方的增量——**这本身就是同步机制**，无需额外拉取。
- **双进程写锁**：桌面 App 与 serve 同时写会话存储的并发锁待验证（见 §5 待验证项 1）。

### 3.7 安全模型

| 威胁 | 缓解 |
|---|---|
| 令牌泄露 | 256-bit 随机 token；手机端 Keychain/SecureStore；serve 端支持轮换 |
| 中间人 | SSH 隧道（host key 校验，复用 `internal/remote`）；或 TLS 反向代理 |
| 跨站驱动 | serve `csrfGuard`（POST 必须 JSON）已覆盖 |
| 手机丢失 | 远程吊销 token（serve 配置变更 + 重启）；可选设备绑定 |
| 敏感内容 | transcript 含代码/密钥——手机端可加生物识别解锁（iOS LocalAuthentication / Android Biometric） |

---

## 4. 里程碑路线图

| 里程碑 | 内容 | 验收标准 |
|---|---|---|
| **M0 骨架** | Expo App 骨架 + 连本机 `reasonix serve --auth token` | 手机看到 SSE 事件流；transcript 只读渲染一个会话；token 认证通过 |
| **M1 交互** | 提交回复 / 审批卡片 / 取消 / 计划确认 | 手机上完成一次"提交 → 回合 → 审批 → 完成"闭环 |
| **M2 多会话** | /sessions /resume /new /fork + 历史 | 手机上切换会话、分叉、回滚 |
| **M3 远程** | SSH 隧道或 TLS + 推送通知（FCM/APNs） | 外网手机收到"需要审批"推送并可操作 |
| **M4 同步完善** | 双端并发 UX、离线队列、令牌轮换 | 手机与桌面同会话操作无数据丢失 |

## 5. 已确认 / 待验证 / 待决定

### 已确认（基于代码与用户决定）

- 部署基线：**手机直连桌面本机**，桌面机跑 `Reasonix.exe` + `reasonix serve`（CLI 独立进程，`--auth token`），同一 `REASONIX_HOME` → 会话/历史/检查点天然共享。
- 文档语言：**仅中文**（手机端自用，不发布，无需英文版）。
- serve 协议面已完整覆盖薄客户端（SSE + REST + 认证 + CSRF，见 §2.2/§2.3）。
- serve 当前**不发送 `Last-Event-ID`/事件序号** → 断线重连采用 `GET /history` 全量重放 + 本地去重（MVP 基线）。
- 桌面 single-instance 锁（`singleInstanceID`）按数据主目录哈希，是桌面进程级，不阻止 serve 共存。

### 待验证（动工前需实测/读码确认）

1. **双进程共享会话存储的并发锁**：桌面 App 与 `reasonix serve` 同时读写同一 `REASONIX_HOME` 下的会话/检查点文件，是否有进程级锁或租约（`single_instance` 锁是桌面进程级、serve 侧未知）。若 serve 的会话存储有自己的锁（如 `session_lease_guard`），则共存安全；否则需约定"同一时间只一端写"或为 serve 加锁。
2. **`/events` 广播是否含会话维度过滤**：多会话场景下，手机订阅到的是全部事件还是当前会话事件（`broadcaster.go` 扇出模型）；决定移动端是否需要按会话过滤。
3. **`POST /approve` 的应答语义**：审批请求的 id/上下文如何从 `approval_request` 事件映射到 `/approve` 请求体（对齐桌面 `control/approval.go` 的合同）。

### 待决定（用户拍板，不影响文档主体）

1. 是否给 serve 加**推送注册端点**（`POST /push-register` + FCM/APNs 桥），还是 MVP 用本地通知（§3.5 方案 2）。
2. 移动端 transcript 的虚拟滚动实现选型（FlatList 基线 vs 其他）。
3. 手机端安全增强（生物识别解锁、令牌轮换）是否进 M0。

## 6. 参考

- 分层规则与内核：`REASONIX.md`；`internal/control/controller.go`、`admission_guard.go`
- serve 面：`internal/serve/serve.go`（`handler()` 路由表）、`auth.go`、`broadcaster.go`
- 事件模型：`internal/event`；桌面侧 `desktop/tabs.go`（`tabEventSink` / `toWireTab`）
- 远程基建：`internal/remote/remote.go`（SSH + 端口转发）
- 通知判定：`internal/notify/sink.go`（`message()`）
- 移动端现状：`mobile/`（Expo 环境，无 app 骨架）
