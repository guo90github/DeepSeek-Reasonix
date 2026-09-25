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
- **状态**：未推——本条写入时 `dev-2` 领先 `origin/dev-2` **56 笔**、落后 0，工作树干净。
- **理由**：本轮没人要求推；而「未推」意味着房内给的 sha 在远端取不到（对端只能靠本机路径读）。
- **下一笔**：决定推，或明确记「不推」并替换本条。
- **可红判据**：`git rev-list --count @{u}..HEAD` 归零，或本条被一条明确「不推」的声明取代。

> 计数会随每次提交变化，所以这一条的载体是**命令 + 仓绝对路径**，不是那个数字。

## 5 `LINT-1` 本仓自检 `repolint` 在未推分支上红

- **来源**：`go run ./tools/repolint`（本机读数，与 §0 同一时点）。
- **状态**：未落——exit 1：`New standards violations` 共 7 条（`internal/config/config.go`、`internal/config/render_test.go`、`internal/control/controller.go`、`internal/control/turn_orchestrator.go`、`internal/plugin/plugin.go`、`internal/provider/openai/openai.go` 两条）。
- **理由（本笔已排除）**：本笔只加两份 `docs/*.md`（`git show --stat e6de6ec74`），而 size 规则不查 `.md`；其中 `internal/plugin/plugin.go` 的超支来自上一轮的 `9f8efce3f`（该提交给该文件加了 9 行注释，把 essay 顶到 15/14），其余来自更早的合并。
- **下一笔**：逐条对 `tools/repolint/baseline.json` 收窄——首选改代码（收窄注释 / 拆文件）；**不许静默放宽基线**，`-update` 只在重命名、抽取这类搬债场景用，并在 PR 里写明理由。
- **可红判据**：`go run ./tools/repolint` 退出 0；或违规清单被明确记为「已知并接受」并写出理由。
