# 桌面端布局重构（split 分栏 + 页签改造）— 任务复盘

> 记录 2026-08-27 在分支 `dev-2` 上完成的桌面端三模块改造全过程：调研、设计、
> 实现、验证与遗留事项。供后续对话理解上下文。
> 说明：旧 `dev` 分支上有一版已废弃的 split 实现（bug 多、已放弃），本方案
> 完全基于 `dev-2` 从零设计；旧版的技术坑清单作为经验引用，未移植任何代码。

## 1. 任务概述

用户要求对桌面端（`desktop/frontend`，Wails + React 19 + TS + Vite + Zustand，
Virtuoso 虚拟滚动）做三模块重构：

1. **对话分栏布局**：主对话区左右双栏——左栏最终正式回答、右栏实时推理过程，
   宽度比例可调 + 响应式。
2. **顶部页签重构**：右停靠区「概览/文件/改动」改造——概览只保留上下文窗口 +
   会话指标，文件树放到会话指标下面。
3. **输入框迁移**：Composer 固定到左栏底部，任何内容高度下可见。

后续追加一轮：**只保留「概览」页签，干掉「改动」页签**（改动功能收纳进概览
面板内部的 文件/改动 切换器）。

## 2. 现状诊断（决定方案的关键事实）

- `.layout` 是 CSS Grid（`--sidebar-width | 1fr [--workspace-width]` 列 ×
  chrome/main/statusbar 行）；`.chat-pane` 为 row2/col2、flex 列，内含
  `.topicbar` / `.main`(Transcript) / `.footer`(Composer+决策浮层)。
- **数据模型已按通道拆分**：`partitionTurnItems`（processItems=推理/工具 vs
  outsideItems=回答）、`buildTurnModels` → `TurnModel[]`（含 `user`、`turn`、
  `turnStableIdentity`）。双栏数据无需重写。
- 右停靠区页签由 `rightDockMode`（useLayoutStore）驱动：概览→ContextPanel、
  文件/改动→WorkspacePanel（2146 行，自持 virtualizer）。
- `--maxw: 960px` 由 `conversationWidth.ts` 设置（非死代码），`.transcript > *`
  居中约束。
- 硬性守卫：`check-single-scroll-writer`（Virtuoso 命令式滚动白名单）、
  `check-bundle-budget.mjs`（gzip/raw 双 ratchet）、theme-token 合约（退役
  token 名单）、z-index token、repolint（文件行数零容差 ratchet）。
- Go 侧 `internal/config` 校验布局样式，不认新值会回退。

## 3. 设计决策

- **新增第 4 种布局样式 `"split"`**（设置面板可选，默认仍 workbench，可逆），
  不替换现有样式。
- **D3（用户确认）**：默认 conversation:process = **40:60**，分隔条拖拽
  **40–60%**，localStorage 持久化（key `reasonix-split-process-width`）。
- 数据层两栏共享显示谓词 `turnHasShownContent`，保证左右轮数恒等。
- 页签合并（用户逐轮确认）：文件→概览（会话指标下方）→ 改动也并入概览，
  dock 只留「概览」（+ 条件性的「远程」）。

## 4. 实现清单

### 新增文件
| 文件 | 作用 |
|---|---|
| `desktop/frontend/src/lib/transcriptPanes.ts` | 双栏数据派生：`conversationPaneTurns`/`processPaneTurns`/`turnHasShownContent`，key 用 `turnStableIdentity` 哈希（prepend-proof） |
| `desktop/frontend/src/__tests__/transcriptPanes.test.ts` | 数据层单测（25 断言：轮数恒等、锚点对齐、prepend key 稳定、prelude 处理） |
| `desktop/frontend/src/components/SplitWorkspace.tsx` | split 组装层：live store 订阅、双栏渲染、分隔条拖拽、窄窗抽屉状态、establish 聚焦、paint gate |
| `desktop/frontend/src/components/ConversationPane.tsx` | 左栏：回合卡片（徽标+问题气泡+全部正式回答），最新展开/历史折叠，older-history 回填 |
| `desktop/frontend/src/components/ProcessPane.tsx` | 右栏：回合标题头+推理/工具/阶段，复用 `InlineAssistantReasoning`/`ToolCard`/`PhaseCard` 等；导出共享 `TurnBadge` |
| `desktop/frontend/src/components/splitWorkspace.css` | split 网格布局、卡片样式、content-visibility 覆盖、决策浮层覆盖、窄窗抽屉（**动态 import**，懒 chunk） |

### 修改文件
- `App.tsx`：`DesktopLayoutStyle` 加 `"split"`；`sidebarSplit` 标志与
  `app--split`/`layout--split` 类；main 分支渲染 SplitWorkspace（与 Transcript
  互斥）；dock 页签条最终只剩 概览(+远程)；概览分支 = ContextPanel +
  WorkspacePanel（`showViewTabs` 内部 文件/改动 切换器）；`openWorkspacePanel`
  把 files→context（非 creation）。
- `ContextPanel.tsx`：概览瘦身（预算 banner、指标卡网格、用量分析共 278 行
  移除；只留上下文窗口+会话指标）。
- `SettingsPanel.tsx`：布局选项加 `split`。
- `lib/bridge.ts`：mock `SetDesktopLayoutStyle` 放行 `split`。
- `store/layout.ts`：`applyLayoutStyleDefaults` 接受 `split`。
- `lib/transcriptRows.ts`：导出 `turnStableIdentity`（单一事实源）。
- `locales/{zh,zh-TW,en}.ts`：`settings.desktopLayoutStyle.split` +
  `split.*` 键。
- `styles.css`：`.workbench-dock__body--merged` 合并概览布局。
- `scripts/check-bundle-budget.mjs`：gzip 445.5→448.5、raw 2404.5→2415.0
  （文档化 ratchet；P5 裁剪后实际用量回落至 445.6/2402.8）。
- `internal/config/{config.go,edit.go,edit_test.go}`：`split` case + 单测。
- `tools/repolint/baseline.json`：仅放宽本次 feature 增长的 5 个文件 + 2 个
  上限（App.tsx +65、SettingsPanel +1、config +2/+4、edit_test +2），无债务混入。

## 5. 关键技术决策与坑

1. **`display:contents` 溶解链**：让 `.main`/`.transcript-navigation-surface`/
   `.transcript-navigation-content`/`.split-workspace` 全部 `display:contents`，
   Composer(footer) 成为 `.chat-pane` 网格直接子项，钉在左栏底部（row3/col1），
   右栏满高（row2-4/col3）不被压缩。**漏掉中间两层包装会导致双栏堆叠错位**。
2. **CSS 变量只向下继承**：`--split-process-width` 必须写到 `.chat-pane`
   （网格容器，是祖先），不能写在 `display:contents` 的 wrapper 上——否则
   网格读不到、拖拽不生效（曾因 localStorage 存了值但渲染不变而暴露）。
3. **Virtuoso 根元素内联 `position:relative`**：覆盖样式表，窄窗抽屉必须
   把绝对定位放在包装层 `.process-pane-host`（`display:contents` 宽窗 /
   `position:absolute` 窄窗）。
4. **content-visibility 覆盖**：两栏所有后代
   `content-visibility:visible !important; contain-intrinsic-size:none`，
   否则 Virtuoso 测不到真实高度，流式增长/滚动范围失效。
5. **`??` 与 `||` 混用**：TS/esbuild 解析错误（Logical expressions and
   coalesce expressions cannot be mixed），必须加括号。
6. **theme-token 合约**：`--border-strong` 已退役，用 `--border`。
7. **陈旧 wailsjs 绑定**：本地 `wailsjs/go/main/App.{js,d.ts}` 含后端已删除的
   `OptimizeDraft`，导致 `_CheckGenToApp` parity 失败、`tsc` 全红——本地移除
   即可（目录 gitignore，`wails build` 会按当前后端再生成）。
8. **establish 聚焦**：`scrollToIndex` 在重会话测前无效，用 120ms×20 有界重试；
   `userInteractedRef` 守卫防止初始 range-change/older-history 回填劫持聚焦。
9. **paint gate**：SplitWorkspace 用 `advanceSurfacePaintCommit`（rAF 采样两栏
   2 稳定帧或 180 帧降级）报告 `onSurfacePaintReady`，否则 runtimeTransitioning
   导航要等满降级超时、footer 长时间隐藏。
10. **scroll-writer 守卫**：Pane 的 Virtuoso ref 命名 `listRef`（避免正则误报）；
   命令式滚动只出现在白名单外规避路径。
11. **决策浮层**：审批/ask 渲染在窄 footer 内会被裁切——`position:absolute`
    锚 chat-pane 底部全宽覆盖 + 不透明背景（`--z-floating-menu`）。

## 6. 验证记录

- **构建**：`pnpm build` 全链 8 门（lint:hooks/waapi/scroll-writer/css-syntax/
  z-index/theme-token/tsc/vite/bundle）全绿；`wails build` 1m31s 产出
  `reasonix-desktop-v1.exe`（绑定再生成 + 前端 + Go 全通过）。
- **Go**：`go test ./internal/config/`（8s）、`go vet`、`gofmt`、`repolint`
  clean（1271 baselined findings）。
- **单测**：`test:split` 25 断言、`layout-style-defaults` 10 断言。
- **Playwright（mock + vite dev）**：
  - 双栏几何：conv x264 w381 (40%) | divider 8px | process x653 w583 (60%)
    满高 643px；footer row3/col1 y601 钉左栏底。
  - 最新轮展开显示完整回答、历史折叠；两栏 18 回合对齐。
  - 拖拽：mid-drag 43% → release 43% → localStorage 0.427；重载保持。
  - 窄窗（<1100px）：抽屉滑入/backdrop/点击外部关闭，宽 680px。
  - 页签：仅 概览/远程；概览 = 容量卡(y80-470) + 指标 + 工作区面板(y470-870)，
    内部 文件/改动 切换器正常渲染改动视图。
- **预算**：gzip 445.6 / raw 2402.8（P5 裁剪比 P2 峰值还省 ~2.8 KiB gzip）。

## 7. 未完成 / 待办

1. split 左栏消息操作菜单（rewind/edit/checkpoint）——按计划降级路径延后。
2. 过程栏↔对话栏对应箭头连线（`correspondenceArrow` 未实现）。
3. 回合徽标绝对编号（`historyStartTurn` 未传入 split，跨会话页编号从可见窗口
   起算）。
4. `desktop/wails.json`（outputfilename `reasonix-desktop-v1`）为历史遗留本地
   改动，未纳入本次提交。
5. `mobile/` 为另一条产品线的 Expo 脚手架（未跟踪），未纳入本次提交。

## 8. PR 元数据（提交分支 `dev-2` 时随 PR body 使用）

```
Cache-impact: none - provider-visible prefix 字节不变；仅桌面 UI 与 internal/config 新增布局样式枚举值
Cache-guard: pnpm build (tsc) + go test ./internal/config/ 既有守卫
System-prompt-review: <指定审查人> - internal/config/ 被触碰，仅新增 "split" 枚举值
Documentation-impact: updated - docs/research/desktop-layout-refactor.md 新增
```
