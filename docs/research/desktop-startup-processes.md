# Desktop 启动进程谱系与启动优化分析

> 分析对象：Wails 桌面客户端（`desktop/`）在"启动后未做任何操作"时的后台进程启动行为。
> 结论基于代码路径梳理（`main.go` → `App.startup` → `restoreOrBuildTabs` → `buildTabController` → `boot.Build`）与当前机器真实配置（`%APPDATA%\reasonix\config.toml`、`mcp-state/`、`desktop-tabs.json`）。
> 本报告只做分析与建议，未改动任何代码。

## 1. 启动路径

```
main()
 ├─ maybeRunMacUpdateHandoff / captureFatalCrash / installFatalCrashOutput（仅崩溃恢复）
 ├─ NewApp() + singleInstanceLock()
 ├─ wails.Run(App{ OnStartup: app.startup, OnDomReady: app.domReady, ... })
 │
 └─ App.startup(ctx)
     ├─ initializeLifecycleDiagnostics / startWindowsWebView2StartupFallback / webView2Recovery
     ├─ desktopShell.coordinator.start(ctx)
     ├─ startHistoryIndexMigration()            # 遍历所有历史会话目录建索引
     ├─ repairDesktopIconIntegration()          # 图标集成修复
     ├─ startMainThreadWatchdog()
     ├─ heartbeat.Start()                       # 心跳引擎
     ├─ go restoreOrBuildTabs()                 # 恢复上次会话的 tab
     │    └─ 每个 tab → startTabControllerBuild → boot.Build
     │         └─ 每个 workspace root 一个共享 plugin.Host（MCP 子进程）
     ├─ startSessionCatalog(false)              # 会话目录扫描
     ├─ go refreshBotRuntime()                  # bot 网关（config [bot] enabled=true）
     ├─ go sendStartupPing()                    # 启动 ping（telemetry 已关）
     ├─ go flushMetrics / flushPendingCrash
     └─ startRecoveryGC()
```

## 2. 启动时产生的进程（按来源归类）

### 2.1 WebView2 浏览器进程树 —— 最显眼、数量最多
Wails 外壳启动必然拉起一整棵 `msedgewebview2.exe` 进程树：
browser / GPU / renderer / network / utility / crashpad 等多个子进程。
这是 WebView2 固有行为，与业务逻辑无关，通常是用户感知"大量进程"的主要来源。

### 2.2 MCP 服务器子进程 —— 业务侧真正的"大量进程"
当前配置（`config.toml` `[[mcp.servers]]`）共 7 个 stdio MCP 服务器：

| 服务器 | 拉起进程 | 运行库 |
|---|---|---|
| codegraph | `codegraph serve --mcp` | node |
| playwright | `cli.js`（node_modules/@playwright/mcp） | node |
| loop-server | `gov-server/dist/index.js` | node |
| zhenhua-mysql | `mysql-server/main.py` | python |
| guotai-mysql | `mysql-server/main.py` | python |
| paddleocr | `mcp_final_server.py` | python |
| java-test-runner | `test_mcp_server.py` | python |

关键机制（`internal/boot/boot.go` `registerEnabledMCP`）：
- 启用但**缓存命中**的服务器 → 注册占位工具，**不启进程**，进程闲置到首次真实 tool call。
- **缓存未命中**（`cs == nil || len(cs.Tools) == 0`）→ **立即踢一个子进程做 catalog 发现**。

本机实测：`%APPDATA%\reasonix\mcp-state/*/…/cache/` **所有服务器均为 0 个缓存文件**，即**冷缓存**。
⇒ 每次冷启动都会把 7 个 MCP 服务器全部拉起握手一次（`defaultStartConcurrency=8`，并行）。

共享 host 粒度（`desktop/tabs.go`）：**每个 workspace root 一个 `plugin.Host`**（key 为 workspaceRoot，全局 tab 用 `__global__`）。
本机 `desktop-tabs.json` 有 3 个 tab，分属 2 个不同的 root（`DeepSeek-Reasonix` ×2、`C:\guosj\ai\agents\fusion\root` ×1）。
⇒ 两个 root 各自拉起一份 MCP 进程，**codegraph 等被重复拉起 2 份**（受 `maxCodeGraphInstances=4` 上限约束）。

### 2.3 git 子进程
- 每个 workspace 的 watcher 启动时执行 2 个探测：
  `git rev-parse --git-dir` 与 `git rev-parse --git-common-dir`（`workspace_watch.go:gitMetadataDirsForWorkspace`）。
  2 个 root ⇒ 4 个短生命周期 `git.exe`。
- 前端加载变更面板 / 状态栏时再触发 `git status`（`--porcelain=v1`）、`git branch` 等。

### 2.4 后台 goroutine（非 OS 进程，但启动即开始）
- `heartbeat` 心跳引擎。
- `sessionCatalog` 会话目录扫描。
- `historyIndexMigration` 遍历所有历史会话目录（本机有数百个 session 文件）。
- `refreshBotRuntime`（config `[bot] enabled=true`，QQ/Feishu/WeChat 网关）。
- `startRecoveryGC` / `sendStartupPing` / `flushMetrics` / `flushPendingCrash`。
- 每个 tab 一个 `buildTabController` goroutine（3 个）。

## 3. 优化建议（按性价比排序）

### A. MCP 冷缓存即拉起的开销最大 —— 建议改代码
`registerEnabledMCP` 的 `kick := cs == nil || len(cs.Tools) == 0` 在冷缓存时于启动阶段即握手全部 7 个服务器。
两个方向：
1. **首启统一懒加载**：冷缓存也不立即踢进程，注册占位工具，首次真实 tool call 才起进程（与缓存命中行为一致）。副作用：工具列表需首次调用后补齐，影响首轮规划的工具可用性感知。
2. **启动预热缓存**：冷缓存场景后台一次性顺序预热（降低并行风暴），之后命中缓存不再拉进程。

### B. 共享 host 改为全局粒度 —— 建议改代码
当前按 workspace root 一个 host，codegraph/playwright 等在多个 root 下重复拉起。
改为全局共享一个 host（MCP 进程全应用只起一份）可显著减少进程数；需权衡不同 root 下 MCP 的 `cwd` / 环境隔离。

### C. 用户侧配置（不改代码，可立即执行）
- 不常用的 MCP 服务器设 `auto_start = false`（当前全部默认启动）。
- 关闭不需要的 bot：`[bot] enabled = false`。
- 关闭 WebView2 GPU：`REASONIX_DISABLE_WEBVIEW2_GPU=1`（可减少 WebView2 进程/显存占用）。
- `check_updates=false` 已配好；`telemetry/metrics` 已关。

### D. 可选：减少启动时后台 I/O
- 若历史会话目录巨大，`historyIndexMigration` 可改为更轻量的分批/按需索引。

## 4. 进程回收 / 退出机制

**结论：启动的进程有回收机制，但只发生在边界（关 tab / 关应用 / 崩溃），正常空闲运行期间不回收。**

### 4.1 MCP 子进程 —— 回收最完善
- **生命周期绑定 ctx**：每个 stdio MCP 子进程用 `proc.CommandContext(ctx, ...)` 启动（`transport_stdio.go:101`），ctx 取消即杀进程。
- **Windows Job Object（`KILL_ON_JOB_CLOSE`）**：`proc.StartTracked` 把子进程放进独立 Job Object（`kill_windows.go:37-49`），句柄关闭时（`KillTracked` 或 reasonix 异常退出）**整棵进程树被杀**——包括 launcher 的脱离式孙进程（如 codegraph daemon 重挂父进程的情况）。子进程先挂起、加入 Job 再运行，杜绝"快速 shim 先退出导致孙进程逃逸"（#3747）。
- **优雅关闭**：`Client.close()` → `stdioTransport.close()` 先关 stdin 给协议感知型服务器 EOF 机会，750ms 内未自行退出则 `KillTracked` 硬杀（`transport_stdio.go:732-751`）。
- **释放时机**：
  - 关 tab / 重建 → `releaseTabSharedHost` → 共享 host 引用计数归零 → `host.Close()` 杀掉其全部 MCP 子进程（`tabs.go:3265`、`shared_host.go:207-224`）。
  - 应用退出 → `shutdown` → `cancelAllTabBuilds` + 各 host 关闭。
  - 崩溃重启 → `reapOrphanCodeGraph()` 清理上一轮泄漏的 codegraph 进程（`app.go:678`）。
- **唯一自退出**：codegraph 由 Reasonix 注入 `CODEGRAPH_DAEMON_IDLE_TIMEOUT_MS=5000`（`known_overrides.go`），其 daemon 空闲 5s 自退。**其他 6 个 MCP 服务器没有任何 idle 自退逻辑。**

### 4.2 关键缺口：空闲期间不回收
- 共享 host 存活条件 = "仍有 tab 引用其 workspace root"。桌面启动即恢复全部 tab（`desktop-tabs.json` 3 个），且空闲不会自动关 tab。
- ⇒ **只要应用开着，7 个 MCP 子进程（×2 root 的 codegraph 等）一直驻留到应用退出**，不存在周期性 idle 清扫。`startRecoveryGC` 是 no-op（只处理会话文件 lineage，不碰进程）。
- WebView2 进程树同理：随应用生命周期存活，无空闲回收。

### 4.3 git 子进程 —— 天然短命
`git rev-parse` 探测是 `cmd.Output()` 一次性调用（2s 超时），跑完即退；`git status` 等也是单次。**这类进程无需回收，自生自灭。**

### 4.4 后台 goroutine
`heartbeat`、`sessionCatalog`、`historyIndexMigration`、`refreshBotRuntime` 等在应用生命周期内常驻（含 bot 网关连接），随 shutdown 停止，无空闲回收。

### 4.5 对本机启动优化的意义
- 进程**会**被回收，但只到应用退出为止；空闲期进程数是"峰值即稳态"（启动时拉起多少，就驻留多少）。
- 因此上一节的优化建议仍成立：**C（配置减负）和 A（MCP 懒加载）的收益是"驻留进程数"的下降，不只是启动瞬态**。
- 若想进一步让"空闲 MCP 自动退出"，需新增"最后引用释放后 + idle 超时关闭 host"机制（现有代码只提供引用计数回收，无 idle 时钟）——这是一项新能力，可作为后续方向。

## 5. 建议的落地顺序
1. 先做 C（零代码，立即减进程）。
2. 再评估 A（MCP 懒加载，效果最直接，涉及工具可用性语义）。
3. B（共享 host 全局化）改动面最大，需回归测试验证多 root 隔离。
4. 可选新能力：空闲 host idle 超时回收（现状无此机制）。

---
生成于：桌面客户端启动优化分析。未修改代码。
