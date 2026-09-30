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

## 9. 后续迭代：左栏回答的「重点信息」（2026-09）

此后又迭代了三个提交（`71fca09f3` → `c265b4a62` → `7d45a10c0`），目标是让回答里的
**判断**可扫读、可定位。它们都落在本节描述的文件上。

### 9.1 阶梯收敛为一处单源

原实现把同一套强调阶梯写了两份（`components/conversationPane.css` 给分栏、
`styles.css` 里自注 "mirror" 的那份给单列），两侧已开始漂移。现在阶梯只存在于
`desktop/frontend/src/styles.css`，两处作用域并列：分栏
`.conversation-pane .msg__body .md …`、单列 `.transcript .msg__body .md …`；
`components/conversationPane.css` 只留本栏的状态轨与动效。守卫
`scripts/check-emphasis-ladder-single-source.mjs`（4 个精确声明串锚点 × 25 张样式表）
接入 `pnpm build` 与 `pnpm check:css`：阶梯若在第二张样式表重现即失败。

作用域同时收窄到**答案正文** `.msg__body`。此前单列用
`.transcript .msg--assistant .md`，而推理面板也在 `.msg--assistant` 内
（`.reasoning__body` 里同样是 `.md`），于是推理的小标题轨、荧光带、整段加粗主张
都拿到"正式回答级"强调，答案的重点因此没有对比度。

### 9.2 重点判定移到渲染期

`src/components/rehypeEmphasisMarks.ts`（接入 `rehypeReasonixKatex.ts` 的
`reasonixRehypePlugins`）给两类节点打标记：

| 标记 | 条件 |
|---|---|
| `md-p--claim` | `strong` 是段落**唯一**有意义的子节点（整段加粗 = 标题性主张） |
| `md-li--label` | `strong` 是列表项**首个**有意义的子节点（标签式条目） |

这两件事 CSS 表达不了：`:only-child` / `:first-child` 只数元素、不数文本节点，
`**要点**：说明` 会被误判成整段加粗。CSS 只负责给标记上色。

### 9.3 要点条与跳转

| 落点 | 作用 |
|---|---|
| `src/lib/answerKeyPoints.ts` | 从渲染后的 DOM 读标记；**采集时**按文档顺序给每个标记写 `data-md-point="k"`，要点带 `ordinal`；去重与上限 6 只影响展示，`total` 报去重后的真实要点数 |
| `src/components/AnswerKeyPoints.tsx` | 呈现「N 个重点」；拿不到跳转能力时渲染纯文本（不做点了没用的按钮） |
| `src/components/Message.tsx` | 挂点 = `AssistantMessage`（单列与分栏左栏共用同一处实现）；`MutationObserver` 观察三条 markdown 路径的 DOM 落地；要点条在 `.msg__body` **之外**，复制回答不会重复带出 |
| `src/lib/answerJump.ts` + `ConversationPane.tsx` | 跳转意图的 context 与其注入点；落点闪 `md--landed` |
| `src/lib/usePaneTailFollow.ts` | 滚动写者提到 `writerRef`，新增 `aimAt(element)`：经 `createTranscriptScrollWriter` 以 `operation:"scrollTo"` + `top` 写入（generation / ownershipEpoch / geometryRevision 三重围栏），被接受才把跟随模式置 `manual` |

两条不可动摇的约束：

1. **编号写在采集时的 DOM 上**，不在解析层编：同一次回答会被 `Markdown.tsx` 的
   `sections` 分段多次解析，各段从 0 编号会在同一个正文里撞号。
2. **滚动只经该表面的受肯写者**（`check-single-scroll-writer`）：分栏写者由
   `usePaneTailFollow` 持有，单列走内核（`owner` 是联合类型 + `blockKey`）。

验证：`test:transcript` exit 0 · `test:split` 13 passed ·
`src/__tests__/answer-key-points.test.ts` 17 passed · `tsc --noEmit` 0 错 ·
app-shell CSS 120.6 KiB / 122.6 · 初始 JS 471.5 KiB / 475.5。

### 9.4 已知未做

1. **单列的要点条不可点**（纯文本）：单列滚动写入是内核那条路，本次未接。
2. 要点条超过 6 条时只列前 6，**没有 "+N" 提示**（表头报的是真实总数）。
3. 落点观感与要点条观感**未经人眼验收**。

### 9.5 上游输出风格的判定（关闭）

曾考虑让答案天然长成"结论先行 + 只加粗判断 + 标签式列表项"（`internal/outputstyle`
可加内置风格，或放 `<项目>/.reasonix/output-styles/*.md`）。**判定收益小、关闭**：
它是唯一一条需要模型配合的路（其余已是确定性机制），而它最易失守的恰是否定式与
计数式约束，失守还是静默的；代价却确定（进 system prompt、cache 影响 real、作用于
该用户所有会话）。若重启，先量三个数字：平均加粗标记数／回答、有 ≥2 标记的回答占比、
有 h2/h3 的回答占比。
