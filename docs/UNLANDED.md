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
- **状态**：**部分收**——`internal/**` 已**清零**；`desktop/**` 12 条仍在（见 §6）。改动前共 **26 条**，现 **12 条**，exit 仍为 1。
  （房间条目 `#344` 题面写「7 条」，**那个数字是错的**：第一次读它时把输出 `tail` 了，只看到尾部 7 行 ⇒ 按 `COLLAB.md` §12「报数不许截断」重算。）
- **收法＝抽取**（同包新文件 / 同文件抽函数，不是删注释凑数）：`provider/openai/openai.go` → `openai_usage.go` + `resolveWireEffort`；`agent/run_loop.go` → `abandonStreamingAttempt`；`agent/execute_batch.go` → `prepareBatchCall` + `runBatchCall`；`boot/boot.go` → `reviewers.go`（guardian / recovery reviewer 各一个 helper）；`agent/agent.go` → `agent_tool_output.go`；`boot/boot_test.go` → `boot_mcp_wait_test.go`；`config/config.go` → `config_model_overrides.go`；`config/render_test.go` → `render_secrets_test.go`；`control/controller.go` → `controller_session_recovery.go`；`control/turn_orchestrator.go` → `runPlanApprovedExecution`；`plugin/plugin.go` → 字段注释 4→3 行。
- **下一笔**：只剩 §6 的 `desktop/**`。
- **可红判据**：读法＝`go run ./tools/repolint 2>&1 | grep '^repolint: internal'` **无输出**（已达成）；全绿须 §6 一并落。

## 6 `LINT-2` `desktop/**` 的 12 条：合并带进来的存量漂移（房间已裁定不在 `#344` 里做）

- **来源**：`go run ./tools/repolint`；裁定＝房间 `#344` 讨论（fusion-root：**新债现场清零；存量债入账可核**）。
- **状态**：未收，**在账**——12 条全在 `desktop/**`：`app.go` 函数体 356/354、`frontend/src/lib/useController.ts` 4530/4426、`frontend/src/components/WorkspacePanel.tsx` 1372/1350、`__tests__/settings-refresh-snapshot.test.tsx` 229/179、`__tests__/use-controller-meta.test.ts` 4/0、`CapabilitiesPanel.tsx` 2643/2641、`UsageStatsPanel.tsx` 277/276、`MemoryPanel.tsx` 1089/1088、`ThemeGallery.tsx` 490/489、`ThemeLibrary.tsx` 159/158、`remote_tab.go` 121/92、`audit_settings_app.go` essay 1/0。
- **理由**：多数只超 1–2 行，形状是**合并（`origin/main-v2`）带进来的行位漂移**，不是本笔新债；拆桌面壳与前端组件是另一摊工作量。
- **下一笔**：另开一条按抽取收，顺序建议——先纯文件体量（`remote_tab.go`、`settings-refresh-snapshot.test.tsx`），再动大件（`useController.ts`、`app.go`）。
- **口径（先写死，免得日后扯皮）**：若最终走 `go run ./tools/repolint -update`，**提交信息必须写明「合并漂移 + 理由」**并把 diff 摆给评审看；**不许静默放宽基线**。
- **可红判据**：`go run ./tools/repolint 2>&1 | grep '^repolint: desktop'` 输出为空；或本条被一条「已知并接受」的声明取代（附理由）。
