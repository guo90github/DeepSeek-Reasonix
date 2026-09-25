# 宿主侧挂账面（未落项）

用途：第三方只读一处，就能列出宿主侧**全部没做成的事**（连同理由与下一笔），不必从聊天记录里考古。
权威面：本机仓 `C:\guosj\ai\deepseek-reasonix\DeepSeek-Reasonix`（分支 `dev-2`）。
口径：**没做到之前不许写成「已落」**；每条必须给来源（房间条目号 / `文件:行`），**指不出来源的条目按噪声处理**。
生成命令 + 时点：`2026-09-26 02:54 +0800`；§4 那条读数由 `git -C C:\guosj\ai\deepseek-reasonix\DeepSeek-Reasonix rev-list --count '@{u}..HEAD'` 产出——**重跑取当前值**，别引用本表的数字。
不含：已落项（在提交与各 `docs/*.md` 里）；已结为「不适用（附反证）」的项（记在房间条目上，不进本表）。

## 1 `BUG1D-1` 分栏左栏尾部跟随的肯红用例

- **来源**：房间 `P0/B162-BUG1D`（宿主侧）。
- **状态**：未落——场景没写完，红/绿两轮读数都没有。
- **理由**：主嫌已指到 `desktop/frontend/src/lib/usePaneTailFollow.ts:15`（由 `ConversationPane.tsx:314` 挂载），但肯红用例要在现成夹具 `desktop/frontend/src/__tests__/use-pane-tail-follow.test.tsx`（JSDOM + React + 假 RAF，已有 `flushFrames`/`flushAllFrames`）里补一个场景，本轮没写完 ⇒ 所以既不写「已定位」，也不写「已验证」。
- **下一笔**：在该夹具加场景「**停在底部 + 新来一行（引导行）⇒ 断它进视口**」。
- **可红判据**：让 follow 不重臂 ⇒ 用例必须红；修好后绿。
- **复现配方（真机，由人验）**：开双栏 → 左栏停在底部（不手动上滚）→ 让收件箱攒一条引导行 → 看它是否自动进视口。

## 2 `VERIFY-1` 真机观感未验（分栏两条线）

- **来源**：房间 `P0/B162-BUG1`、`P0/B162-BUG1D`。
- **状态**：未验——代码面已落，观感没看过。
- **理由**：观感要重建桌面端才看得到，本轮按约束不打包。
- **下一笔**：打包后开双栏，按 §1 的配方、以及 `BUG1` 的归类改法各走一遍。
- **可红判据**：走完一遍若「点名仍从左栏消失」⇒ `BUG1` 的修复未生效。

## 3 `PUSH-E2E-1` push 端到端未走真机

- **来源**：房间 `P1/B162-E2E`、`P0/B162-H2B`。
- **状态**：未验——两侧代码面已落，且**默认不武装**。
- **理由**：宿主侧 `Spec.WakeMethod` 空 = 不武装，聊天侧 `CHATTING_PUSH` 默认关 ⇒ 今天没有任何东西在发通知（仓内串接用例只覆盖桩载荷）。
- **下一笔**：同机起一条链路（聊天侧武装 `CHATTING_PUSH`、宿主侧配 `wake_method`），验「通知 → inbox 条目（`source=push`）→ steer 注入带标记」三段。
- **可红判据**：武装后仍无 inbox 条目 ⇒ 链路断；逐段读房间 `/api/wakes` 是否出现 `type=wake`（只有 `type=skipped` 就是没投）。

## 4 `CUTPOINT-1` 两侧提交未推

- **来源**：`git rev-list --count @{u}..HEAD`（本机读数）。
- **状态**：未推——`dev-2` 领先 `origin/dev-2`、落后 0，工作树干净。
- **理由**：本轮没人要求推；而「未推」意味着房内给的 sha 在远端取不到（对端只能靠本机路径读）。
- **下一笔**：决定推，或明确记「不推」并替换本条。
- **可红判据**：上面那条命令归零，或本条被一条明确「不推」的声明取代。

> 本表**不写死计数**（写下的那一刻就过期）：载体是命令 + 仓绝对路径，读的人重跑取当前值。

## 5 `LINT-1` 本仓自检 `repolint` 在未推分支上红

- **来源**：`go run ./tools/repolint`（本机读数；改动前的条数由 `git stash` 在**改前树**上重跑核对）。
- **状态**：**部分收**——改动前 **26 条** `New standards violations`，本笔收掉 6 条后剩 **20 条**，exit 仍为 1。
  （房间条目 `#344` 题面写「7 条」，**那个数字是错的**：第一次读它时把输出 `tail` 了，只看到尾部 7 行。）
- **本笔已收的 6 条，收法＝抽取到同包新文件**（不是删注释凑数）：`internal/config/config.go` → `config_model_overrides.go`、`internal/config/render_test.go` → `render_secrets_test.go`、`internal/control/controller.go` → `controller_session_recovery.go`、`internal/control/turn_orchestrator.go` → 同文件抽 `runPlanApprovedExecution`、`internal/plugin/plugin.go` → 字段注释 4→3 行、`internal/provider/openai/openai.go` → `openai_usage.go`。
- **仍未收的 20 条**：`internal/**` 6 条（`provider/openai/openai.go` 函数体 181/180；`agent/run_loop.go` 2/1；`agent/execute_batch.go` 155/138 + complexity 23/22；`boot/boot.go` 1759/1735 + complexity 273/267；`agent/agent.go` 1981/1957；`boot/boot_test.go` 3697/3696）；`desktop/**` 14 条（`app.go` 函数体 356/354；`frontend/src/lib/useController.ts` 4530/4426；`WorkspacePanel.tsx` 1372/1350；`CapabilitiesPanel.tsx` 2643/2641；`__tests__/settings-refresh-snapshot.test.tsx` 229/179；`remote_tab.go` 121/92 等，多数只超 1–2 行）。形状＝`Go` 规则与 `TS/TSX` 文件体量各半。
- **下一笔**：`internal/**` 继续按**抽取**收；`desktop/**` 先定口径再动——多数是**合并带进来的行位漂移**，而「不放宽基线」与「`-update` 只用于重命名/抽取」这两条今天打架（等房间裁定）。
- **可红判据**：`go run ./tools/repolint` 退出 0；或违规清单被明确记为「已知并接受」并写出理由。
