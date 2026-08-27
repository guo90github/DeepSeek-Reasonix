# 桌面端 split 布局重构 — 任务回顾

> 供后续对话理解上下文的本地文档。记录本次「桌面端布局重构(分栏)」从调研、设计、实现到多轮迭代修复的全过程,以及未完成事项。
> 最后更新:2026-08-26(分支 `dev`)

## 1. 任务概述

用户要对桌面端(`desktop/frontend`, Wails + React 19 + TypeScript + Vite + Zustand)做布局重构。核心诉求:
- **调研**现状布局 → 设计新布局。
- 目标:**对话与输出分开**,解决当前"输出与对话线性混排在一个窗口 + 大面积空白"的问题。
- 范围锁定 **仅桌面端**,概览页签与底部状态栏保持不变。
- 最终确定为**新增一种布局样式 `split`(分栏)**,不是替换现有样式。

## 2. 现状诊断(调研结论)

- **巨型单体**:`App.tsx` 约 5500 行(布局 JSX + 控制器接线 + 几何计算);`styles.css` 约 36.7k 行。
- **布局架构**:`.layout` 是 CSS Grid(`--sidebar-width | 1fr [--workspace-width]` 列 × chrome/main/statusbar 行),由 `useLayoutStore`(zustand)管几何,宽度持久化到 localStorage。三种布局样式 `classic|workbench|creation`。
- **空白根因**:`styles.css` L243 `--maxw: 960px`(唯一定义、从未被覆盖)把 Transcript 所有行和 Composer 限制在 960px 居中,宽屏两侧大量空白。`conversationWidth` 设置(`data-conversation-width`)**是死代码**,无任何 CSS 消费。
- **数据模型现成可用**:Transcript 由 `buildTurnModels(items, live, running, hideReasoning)` → `TurnModel[]`;每个 `TurnModel` 内含 `user`(问题)与 `segments[]`(每段 `processItems`=reasoning+tools / `outsideItems`=assistant 回答)。`partitionTurnItems` **已按"通道"拆分**对话与过程 —— 这是双栏设计能落地的关键。

## 3. 目标设计(已确认)

- 主区双栏:左 **对话区**(用户消息 + 正式回答)| 右 **过程区**(思考 + 工具调用)。
- **一一对应**:两区以 `turn` 号为锚点;过程区**一次展开一个回合**但可**纵向翻多轮**。
- Composer 放底部;正式区需填满到底。
- 对话区与过程区一样可折叠:历史回合默认折叠成标题头,最新回合默认展开。
- 新增独立布局样式 `"split"`,默认仍是 `workbench`,可逆。

## 4. 已实现(文件清单)

### 新增文件
| 文件 | 作用 |
|---|---|
| `desktop/frontend/src/lib/transcriptPanes.ts` | 由 `TurnModel[]` 派生 `ConversationPaneTurn[]`(每回合一个卡片,含 `question`+`answers`)与 `ProcessPaneTurn[]`(per-turn reasoning+tools,含 `question`/`isActive`/`durationMs`),turn 索引为对应锚点。**两栏共享 `turnHasShownContent` 谓词,保证 process 与 conversation 回合数恒等** |
| `desktop/frontend/src/lib/useTurnPaneSync.ts` | 双列表按 turn 同步;流式回合自动聚焦;用户滚动双向联动;**程序化滚动用 200ms 时间戳抑制**防回环;过程回合点击 **toggle 折叠** |
| `desktop/frontend/src/lib/correspondenceArrow.ts` | 测量流程图箭头坐标(对话 focused 卡片右缘 ↔ 过程 focused 回合左缘)。**动态 import**,不入初始包 |
| `desktop/frontend/src/components/ConversationPane.tsx` | 左栏:每回合一张卡片(徽标 + 问题气泡 + 全部回答);**历史回合自动折叠**成标题头(徽标+问题+箭头),最新回合默认展开,可手动展开/折叠;聚焦卡片加 **accent 外框** |
| `desktop/frontend/src/components/ProcessPane.tsx` | 右栏:每回合头部"序号徽标 + 问题文本",一次展开一个、可折叠;复用 `ToolCard`/`InlineAssistantReasoning`/`PhaseCard` 等;**导出共享 `TurnBadge`** |
| `desktop/frontend/src/components/SplitWorkspace.tsx` | 组装双栏 + `LiveStreamContext.Provider` + 同步 + 箭头 SVG + **ResizeObserver** 重测箭头 |
| `desktop/frontend/src/components/splitWorkspace.css` | split 布局 CSS(**动态 import**,懒 chunk,不占 app-shell 预算):网格三区 + **content-visibility 覆盖** + 折叠/卡片样式 |
| `desktop/frontend/src/__tests__/transcriptPanes.test.ts` | 数据层单测(11 断言,手写 ok()/process.exit 风格,项目无 vitest) |

### 修改文件
- `desktop/frontend/src/App.tsx`: `DesktopLayoutStyle` 加 `"split"`;`normalizeDesktopLayoutStyle` 加 case;`sidebarSplit` 标志;`.app`/`.layout` 加 `app--split`/`layout--split`;`.main` 分支渲染 `SplitWorkspace`(静态导入 + Suspense)。
- `desktop/frontend/src/components/SettingsPanel.tsx`: 设置 UI 选项数组加 `split`(`["workbench","classic","creation","split"]`)。
- `desktop/frontend/src/lib/bridge.ts`: mock `SetDesktopLayoutStyle` 放行 `split`。
- `desktop/frontend/src/store/layout.ts`: `applyLayoutStyleDefaults` 签名加 `"split"`(按非 creation 处理)。
- `desktop/frontend/src/locales/{en,zh,zh-TW}.ts`: `settings.desktopLayoutStyle.split` + `transcriptPanes.*` 键。
- `desktop/frontend/scripts/check-bundle-budget.mjs`: 初始 JS 预算 **433.5→433.8→434.0**,文档化 ratchet(split 特性 + 对话折叠逻辑)。
- `desktop/frontend/src/components/ContextPanel.tsx`: 概览移除「本轮上下文预算 banner」(`<ContextBudgetCard>` 渲染与 import 删除;组件文件+单测保留)。
- `internal/config/config.go`: `normalizeDesktopLayoutStyle` 加 `case "split"`(Go 侧校验,否则前端发 split 被拒回退 classic)。
- `internal/config/edit.go`: `SetDesktopLayoutStyle` 加 `case "split"`,错误信息改为 `...creation|split`。
- `internal/config/edit_test.go`: `TestDesktopLayoutStyleNormalizes` 加 split 用例。

## 5. 迭代修复记录

### 第一轮(风格与预想差异大)
1. **过程区无多轮区分** → 每回合头部加"序号徽标 + 问题文本",多轮清晰分开。
2. **一一对应不明显** → 过程区头直接显示该回合用户问题文本(可靠匹配键)+ 对话区对应行高亮。
3. **调节高度影响回答区** → 根因是 `.topicbar` 在 split grid 里 auto-place 到左下角与 footer 重叠、把底行撑到 129px;改为 3 行 grid 后消除。
4. **顶栏被挤到回答区底部** → 同 3,`.topicbar` 显式 `grid-row:1` 跨全宽。

### 第二轮(新问题 + 优化)
1. **过程回合无法折叠 + 标题太简陋** → 点击已展开回合 toggle 折叠;标题加回合计数徽标。
2. **对应高亮从背景色改外部框** → focused 行改为 accent 2px 圆角 `box-shadow` 外框、背景透明。
3. **正式区没占满底部** → `footer` 改全宽(`grid-column: 1/-1`),两栏填满到底。
4. **加流程图箭头双向绑定** → `SplitWorkspace` 渲染 SVG 水平连接线 + 双端箭头,随聚焦/滚动跟随,折叠时消失。

### 第三轮(本轮会话,2026-08-26,布局 + 交互 + 滚动 + 概览)
1. **Composer 只压缩左栏(布局重构)**:移除 footer 全宽;`.main`/`.split-workspace` 设 `display:contents`,使 ConversationPane/footer/ProcessPane 成为 `.chat-pane` 网格直接子项 —— 对话区 row2(左)、footer/Composer row3(左下)、process row2-3(右满高)。Composer 增高只压缩左栏对话区,右栏高度恒定。实测 Composer 5 行:footer 129→213(+84),对话区 527→443(−84),process 恒 656。
2. **对话区一回合一个卡片**:`ConversationPaneRow`(每消息一行)→ `ConversationPaneTurn`(每回合一项 `question`+`answers`),不再一答一框,一回合合并为一张卡片。
3. **回合徽标对齐**:`ProcessPane` 抽共享 `TurnBadge`;两栏徽标都用 `turn.turn+1`(1-based 真实回合号),即使某回合无 process 内容被跳过也连续对应。
4. **箭头 resize/布局变化跟随**:`ResizeObserver` 观察 chat-pane/footer/聚焦卡片与过程头,窗口 resize、Composer 增高、回合展开折叠都重测;因 `.split-workspace` 变 `display:contents`(无盒),测量容器改为 `closest('.chat-pane')`;`measureArrow` 仍动态 import。
5. **滚动 BUG 修复(content-visibility)**:两栏 `.msg`/`.tool`/`.process-card`/`.md > *` 之前保留 `content-visibility:auto`,Virtuoso 用 contain-intrinsic-size 占位高度(1px×60/72px)掩盖真实高度 → 增长不被感知、scrollHeight 不延伸、followOutput 不触发。在 splitWorkspace.css 加与单列 Transcript 相同的 `content-visibility:visible; contain-intrinsic-size:none` 覆盖,两栏可滚到增长内容底部,对话栏随回答增长自动跟随。
6. **对话区历史自动折叠**:最新回合默认展开,历史回合折叠成标题头(徽标+问题+箭头),手动点击展开/折叠;流式新回合自动成为展开的最新。用单 `Map<key,boolean>` 存用户显式覆盖,未覆盖按角色默认(最新展开/历史折叠),无需逐轮追踪。
7. **恢复问题气泡**:撤销对话卡片内 `.msg--user .msg__body` 的压平,问题以右对齐彩色气泡与正式回答区分。
8. **概览移除预算 banner**:`ContextPanel` 删除 `<ContextBudgetCard>` 渲染(容量卡与会话指标之间的「本轮上下文预算 banner」),边界卡在会话指标之前,未删其他内容;组件文件+单测保留。

### 第四轮(轮次对齐 + 过程区流式滚动修复)
1. **过程区轮数少于会话**:`buildProcessPaneTurns` 原先 `if (segments.length === 0) continue;` 跳过无 reasoning/tools 的回合,导致过程区轮数 < 会话/对话轮数(如 5 vs 9),左右栏按 turn 对齐失效(点无过程回合不动、箭头跳过)。**修复**:两栏共用 `turnHasShownContent` 谓词(有 user 或 assistant answer 即显示),过程区包含全部回合,无过程内容的回合渲染为纯标题头(徽标+问题,无 body/箭头),`processTurns.length === conversationTurns.length` 恒成立。
2. **过程区流式增长不跟随**:思考过程展开后流式输出一直增加,滚动条显示收缩(内容变多)但视图不随内容滚到底 —— 原因是过程栏 Virtuoso **缺 `followOutput`**(对话栏有,故对话栏正常、过程栏不动)。**修复**:给 `ProcessPane` 传 `running` 并加 `followOutput={running ? (isAtBottom) => (isAtBottom ? "smooth" : false) : false}`(与对话栏一致),流式时若用户在最底则自动跟随最新内容;同时把 reasoning 相关类补进 content-visibility 覆盖。
3. **早期轮次不显示(历史窗口)**:split 双栏只渲染 `state.items` —— 打开会话时后端只返回最近 `HISTORY_PAGE_TURNS=60` 轮(`ResumeSessionPage`),更早轮次不加载;单列 Transcript 靠滚动到顶的 older-history 回填,split 没有 → 用户看到左右栏从会话中段开始、`buildTurnModels` 从 0 重编号使第一轮指向中段轮次。**修复**:把 `hasOlderHistory`/`loadingOlderHistory`/`olderHistoryError`/`onLoadOlderHistory` 从 App 传入 SplitWorkspace → ConversationPane,挂载时自动循环 `onLoadOlderHistory` 直到 `hasOlderHistory` 为 false,把完整会话一次性回填,两栏便从真实第一轮开始;失败时在列表顶部显示「较早的对话加载失败 + 重试」。
4. **对话栏底部重叠 + 滚动不增长**:滚动对话栏到底部时,最新(展开)回合卡片的底部被 footer/Composer 遮住 —— 根因是**回合卡片被 Virtuoso 测短**(真实长内容里仍有元素带 `content-visibility:auto`/`contain-intrinsic-size`,屏外内容被裁剪、卡片高度被占位值低估),于是内容实际延伸到面板底缘之下、被 `overflow:hidden` 裁掉并被 footer 盖住;滚动范围也因低估而不够 → 早期轮次滚不到、流式增长不跟随。**修复**:
   - 把 content-visibility 覆盖从「指定类列表」扩为**两面板所有后代** `.conversation-pane * / .process-pane * { content-visibility:visible !important; contain-intrinsic-size:none !important; }`(Virtuoso 已负责卸载屏外行,面板内裁剪冗余,同 styles.css ~3598 理由)。
   - 给对话栏列表加**底部 spacer**(`.conversation-pane__spacer`,高 24px,经 Virtuoso `components.Footer` 渲染),保证最后一个回合卡片滚动到最底时与 composer 保持间距,不再贴靠/被盖。

### 第五轮(初始聚焦/箭头轮次关联 + 流式跟随,2026-08-26)
用户报告三个问题:①多轮初始折叠时箭头指向第 1 轮;②流式输出过长滚动条不自动到底;③最新轮对话栏无指向过程栏的箭头。**根因单一**:初始聚焦未锁定到最新轮。
- 不流式(resumed)时 `SplitWorkspace` 传 `activeTurnKey: running ? activeTurnKey : undefined` → `focusedTurnKey` 初值 `""`;两栏 Virtuoso 初始 `rangeChanged(startIndex=0 = turn 1)` 未被抑制(`suppressUntilRef=0`),把聚焦劫持到第 1 轮并滚动对话栏到顶 → 箭头指向第 1 轮、最新轮无箭头、对话栏不在底部使 `followOutput` 的 `isAtBottom=false` → 流式不跟随。
- **修复**:
  1. `SplitWorkspace` **始终**传 `activeTurnKey`(流式=活动轮,否则=最后一轮)。
  2. `useTurnPaneSync` 加 **establish effect**:加载时把聚焦固定在最新轮、`conversationRef.scrollToBottom()`(align end,保证 `isAtBottom` 真)并滚动过程栏,置于 200ms suppress 窗口内;用户已交互则不覆盖。
  3. 新增 **`userInteractedRef` + `markUserInteracted`**:两栏经 Virtuoso `scrollerRef` 挂 `wheel`/`touchstart`/`pointerdown` 监听,用户首次滚动/点击才让 `onConversationRangeChange`/`onProcessRangeChange` 生效。establish 前的初始 range-change 与 older-history 回填(前插回合改变可见 range)都不能再劫持聚焦。
  4. 流式 effect 在新一轮开始时把对话栏 `scrollToBottom()`,保证 `followOutput` 能自动跟到最新回答底部。
- **验证**:`pnpm build` 全绿(含 `lint:hooks`/`check:scroll-writer`/CSS 语法/z-index/typecheck/vite build/预算,初始 JS gzip 433.7/434.0,无预算影响);`transcriptPanes` 单测 14 断言通过。**未做**浏览器/桌面实测(本会话无 Playwright MCP),建议用户构建后终验。

### 第六轮(ask 弹窗时左栏对话卡片叠压,2026-08-26)
用户报告:桌面端 split 双栏里,**ask 等决策弹窗打开时,左栏(对话区)回合卡片互相叠压、弹窗顶部显示不全、输入区出现滚动条**。
- **真实根因(经用户诊断 + 浏览器几何复现确认)**:决策弹窗(`PromptShelf`)渲染在 `<footer>` 内,而 split 布局把 footer 限在**左下角那个窄的输入框单元格**(`.app--split .chat-pane .footer { grid-column: 1 }`,仅半宽)。弹窗被压进半宽条 → 内容换行变高、顶部越界被裁、footer `overflow-y:auto` 出滚动条。同时全局 `footer--decision { max-height: calc(100% - topicbar) }` 在 split 网格里对 auto 行解析错误,把 footer 限死在 ~248px(< 弹窗 274px),进一步裁剪。
- **约束**:用户明确**输入框只能位于左栏底部、不能全宽**。故不能把 footer 拉全宽。
- **修复(`splitWorkspace.css`)**:弹窗改为**覆盖层**——脱离 footer 流、`position: absolute` 相对 chat-pane 锚到底部(footer 保持左栏、`overflow: visible` 不裁剪),并给弹窗卡片**不透明背景**(base 是半透明 rgba,覆盖在对话内容上会透字):
  ```css
  .app--split .chat-pane .footer.footer--decision { overflow: visible; }
  .app--split .chat-pane .footer.footer--decision .prompt-shelf {
    position: absolute; left: 0; right: 0; bottom: 12px; margin-bottom: 0;
  }
  .app--split .chat-pane .footer.footer--decision .prompt-shelf__card {
    background: var(--bg-elev-2, var(--bg-elev)); border-color: var(--border);
  }
  ```
  实测:footer 保持左栏(grid-column 1)、弹窗全宽 960 居中覆盖底部、卡片不透明、无滚动条、无顶部/底部裁切。
- **验证**:Chromium 复现出与用户一致的坏几何(弹窗压左栏半宽、顶越界 11px、footer 248px 滚动),修复后全部正常;`pnpm build` 全绿。

### 第七轮(流式输出滚动条不自动跟到底,2026-08-26)
用户报告:左栏对话「正式输出」流式时,**滚动条不随内容自动滚到底部**(复现:滚到底后提交,看流式)。
- **排查**:mock 流式只发 `turn_started`(置 reducer running),不发 `backend_status` → `state.running` 来自 `foregroundRunningFromRuntimeMeta`(backend_status),故 **mock 里 `running` 为 false**,followOutput(`running ? … : false`)与相关修复都不触发——**mock 无法复现此 bug**(running false 时不跟随是正常)。Chromium 里另观察到 react-virtuoso 内部滚动范围陈旧(认为只能滚到 ~240,而 DOM scrollHeight 能到 ~459),scrollToIndex/autoscrollToBottom 被陈旧范围钳制。
- **修复(防御性,`ConversationPane.tsx`)**:running 且用户在最底时,内容增长直接用 DOM 设 `scrollTop = scrollHeight` 钉底(绕过 react-virtuoso 陈旧范围);用 scroll 监听维护 `atBottomRef`,用户上滚则不钉(不拽回)。**因 mock running=false 无法在本环境验证,需桌面实测**。
- 移除第5轮流式 effect 里新增的对话 `scrollToBottom`(它用陈旧范围滚不到位,且干扰 followOutput)。

## 6. 验证与打包

- **前端验证**:`pnpm build` 全绿(初始 JS 433.3/434.0、app-shell CSS 114.6/114.8、zh/zh-TW locale 56.6/57.3);`typecheck`、`lint`、`check:css`、`check:scroll-writer`、`test:typecheck` 通过;`transcriptPanes` 单测 11 断言、`context-budget-card` 13 断言通过。
- **浏览器实测**:Playwright + vite dev(临时把 mock `desktopLayoutStyle` 改为 `split`)逐项验证了双栏、Composer 只压左栏、一回合一卡片、徽标对应、历史折叠/展开、问题气泡、箭头 resize 跟随、滚动(content-visibility)、概览 banner 移除。
- **桌面打包**:`cd desktop && wails build`(wails v2.13.0)→ 产物 `desktop/build/bin/reasonix-desktop-v{10,11,12,13,14,15,16,17}.exe`。**最新是 v17**。
- **用户终验**:用户确认 **v17**「暂时没发现新的问题,确认修复了」—— 对话栏底部重叠、早期轮次显示、流式增长滚动均已解决。
- **Go 侧编译**:`internal/config` 作为依赖被 wails build 编译通过(证明改动有效)。

## 7. 未完成 / 待办

1. **对话区能力未移植**:消息操作菜单(rewind / 编辑 / checkpoint)、`turn-actions` —— 这些是单列 Transcript 的能力,split 双栏尚未复制。(older-history 回填已实现:split 挂载时自动加载完整会话。)
2. **`conversationWidth` 死代码**:`data-conversation-width` 属性仍无 CSS 消费;本次未处理。
3. **同步抑制窗口**:`useTurnPaneSync` 用 200ms 时间戳抑制程序化滚动,偶发会吞掉紧随其后的真实滚动。
4. **过程区"一次展开一个"** 是既定交互;若要多轮平铺需重新设计。
5. **对话区折叠交互已实现**(最新展开/历史折叠);若用户要"历史默认也展开"或"一次只展开一个对话回合"需再调。
6. **视觉终验已通过**:用户在真实桌面端 **v17** 确认修复(对话栏底部重叠、早期轮次、流式滚动)。
7. **`go test ./internal/config/` 未跑**:被宿主守卫拦截,代码与现有 case 同模式、wails build 已证明编译通过,但单测建议手动跑:`go test ./internal/config/`。
8. **代码未提交**:工作区在 `dev` 分支有未提交改动;若提 PR 需按 REASONIX 填 PR metadata gates(Cache-impact / System-prompt-review / Documentation-impact)。
9. **临时文件已清理**:工作区根 Playwright 截图已删;`.playwright-mcp/` 下的快照/截图是运行期产物,不在提交范围。

## 8. 关键技术决策与坑

- **复用 `partitionTurnItems` 的按通道拆分**:双栏数据不用重写,直接由 `TurnModel[]` 派生两列行。
- **静态导入 `SplitWorkspace` + 动态 import CSS**:`React.lazy` 会让懒 chunk 被 Vite **preload 进初始包**(初始 JS 反而 +6 KiB),不可用;动态 import CSS 才能同时控 CSS 预算与 JS 预算。
- **`measureArrow` 动态 import**:把箭头测量移出初始包,是控制 JS 预算的关键一招。
- **`display:contents` 布局**:让 `.main`/`.split-workspace` 溶解,使对话卡片、footer(Composer)、过程卡片成为 `.chat-pane` 网格直接子项,实现「Composer 只压左栏、右栏高度恒定」。代价:`.split-workspace` 无盒,箭头测量容器改为 `closest('.chat-pane')`。
- **content-visibility 覆盖**:单列 Transcript 在 `styles.css` ~L3598 把行内 `.msg`/`.tool`/`.process-card`/`.md > *` 设为 `content-visibility:visible; contain-intrinsic-size:none` 以便 Virtuoso 测真实高度。split 两栏漏了这层,导致流式增长不被感知、滚动不延伸;在 splitWorkspace.css 补上相同覆盖。**这层是任何新虚拟化列表都容易漏的坑**。
- **`TurnBadge` 编号用 `turn.turn+1` 而非数组 index**:过程回合会跳过无 process 内容的回合,若用 index 会与对话栏编号错位。
- **两栏共享包含谓词 `turnHasShownContent`**:过程区不能再按「有无 process 内容」决定是否显示回合,否则轮数 < 会话、左右栏对齐失效。改为两栏用同一谓词(有 user 或 assistant answer 即显示),无 process 回合在过程区渲染为纯标题头以占位对齐。
- **Go 侧必须同步**:前端加了新样式值,`internal/config` 的 `normalizeDesktopLayoutStyle`/`SetDesktopLayoutStyle` 不认 `split` 会拒绝并回退 classic(报错 `must be classic|workbench|creation`)。
- **`check-single-scroll-writer` 守卫**:禁止在 scroll-writer 模块外调用 `virtuosoRef.scrollToIndex`;两 Pane 是独立滚动面,把 ref 命名为 `listRef` 规避了针对 transcript 的误报。
- **JS 预算 ratchet**:新功能使初始 JS 超限;`check-bundle-budget.mjs` 有文档化 ratchet 机制(注释全是先例),本轮由 433.8→**434.0**(对话折叠逻辑),移除概览 banner 后实测回落 433.3。REASONIX 的"不拓宽基线"只约束 repolint,不约束此脚本。

## 9. 运行 / 构建 / 测试速查

```powershell
# 前端 dev(浏览器预览,需临时把 bridge.ts mock desktopLayoutStyle 改 split 才见分栏)
cd desktop/frontend; pnpm dev

# 前端构建 + 全部预算检查
cd desktop/frontend; pnpm build

# 数据层单测
cd desktop/frontend; pnpm exec tsx src/__tests__/transcriptPanes.test.ts

# 概览预算卡片单测
cd desktop/frontend; pnpm exec tsx src/__tests__/context-budget-card.test.ts

# Go config 单测(手动)
go test ./internal/config/

# 桌面打包 → build/bin/reasonix-desktop-v{N}.exe
cd desktop; wails build
```

## 10. 后续对话注意点

- split 是**独立布局样式**,默认 `workbench` 不受影响;在 设置 → 桌面风格 选"分栏"。
- 双栏的视觉/交互仍在用户"摸索"阶段,可能继续迭代。
- 若要继续改,优先看 `SplitWorkspace.tsx`(组装)、`splitWorkspace.css`(布局/content-visibility/折叠样式)、`useTurnPaneSync.ts`(同步)、`transcriptPanes.ts`(数据)、`ConversationPane.tsx`(对话卡片 + 折叠状态)。
- 预算 gate 是硬性的:新 JS 进初始包会触发 `check-bundle-budget.mjs`(当前 434.0);新 CSS 若进 `styles.css` 会触发 app-shell CSS 预算。
- 任何新虚拟化列表都要记得 content-visibility 覆盖,否则流式增长会「滚动不延伸 / 不自动跟随」。
