# 待办面板（TodoPanel）功能实施落地计划

> **文档状态**：待实施（依据 `docs/TODO_PANEL_FRONTEND_DESIGN.md` 定稿方案 + 评审结论）
> **前置**：设计文档评审已完成，三处修订建议并入阶段 0（本计划已内嵌修订后的定义）
> **关联记忆**：`todo-loss-root-cause-and-fix-plan`（后端三处改动即该搁置方案的核心部分，本计划为其正式启动）
> **关联文档**：`docs/TODO_PANEL_FRONTEND_DESIGN.md`（验收标准 F1–F5 见其 §7.1）

---

## 0. 总览与依赖

```text
阶段 0 文档对齐（修订设计文档，产出权威规格）
   │
   ▼
阶段 A 后端前置（附录 A 三处改动）──────────────┐   ← F1–F3 验收的唯一硬依赖
   │                                              │
   ▼                                              ▼
阶段 B 前端纯函数（todoVisibility.ts）       阶段 E 验证与验收（F1–F3 需 A 落地后执行）
   ▼                                              ▲
阶段 C TodoPanel.tsx 渲染                        │
   ▼                                              │
阶段 D styles.css ──────────────────────────────┘
   ▼
阶段 F 收尾（PR 拆分、CI、记忆更新）
```

- **依赖关系**：阶段 A 与 B–D 可并行开发；**F1–F3 验收必须在阶段 A 落地后**才能端到端执行（当前只能验收 F4/F5 与回归，见 `TODO_PANEL_FRONTEND_DESIGN.md` §7.1）。
- **零改动层（全程冻结）**：Wire 类型、`parseTodos`、`useController`、`App.tsx`（挂载点/批次 key/Dismiss）、批次与折叠记忆逻辑。`internal/evidence` 的序列校验器**无需改动**——它正是"子完成→父翻转"原子性的强制者（见 A1 注）。

---

## 1. 阶段 0：设计文档对齐（0.5d）

评审确认设计文档与代码高度吻合，但需内嵌三处修订作为实施规格：

| # | 修订 | 内容 |
| :--- | :--- | :--- |
| 0-1 | 补充后端契约依据 | §3 增注：phase 状态原子性由 `ValidateSerialTodos`/`validateSerialSegment` 强制（evidence.go:99-101、169-188）——"pending 父行+子项全完成"（stale）、"in_progress/completed 父行+未完成子项"均为非法形态，最后子项完成的那次 todo_write 必须同步翻转父行。设计的"子完成→父感知"是契约而非模型配合。 |
| 0-2 | 修正分组规则 | §2.2 分组规则改为**镜像后端 `serialTodoSegments`**（evidence.go:116-129）：level-0 拥有紧随其后的连续 level-1 段；游离 level-1（前项非 level-0）是独立单行，不并入任何 phase。 |
| 0-3 | 定义纯拆分谓词 | 附录 A 项 3 补充精确谓词（见阶段 A-3），并注明"拆分+其他改动同调用"仍须送审。 |

**退出标准**：设计文档包含以上三处；代码锚点核对无误。

---

## 2. 阶段 A：后端前置（附录 A 三处改动）

### A-1 核心：放宽 `verifyTodoCurrentContinuity`（`internal/tool/builtin/todo.go:155-177`）

**现状**：172-174 行拒绝任何 in_progress→pending 回退——包括纯拆分形态。

**放宽谓词**（仅当以下全部成立才允许 pending，否则维持原拒绝）：
对 `previous` 中每个 in_progress 项 `prev`：
1. `MatchTodoIdentity(prev, next)` 找到同身份项 `match`（step_id 或文案，身份保留）；
2. `match.Status` 为 `pending`（或空）；
3. `match.Level == 0`（成为 phase 头，且 `prev.Level == 0` 或 freehand）；
4. `next` 中紧随 `match` 的下一项存在，且 `Level == 1` 且 `Status == "in_progress"`（首个子项为新当前项）。

> 注：`ValidateSerialTodos`（evidence.go:42-103）已先于本函数执行并保证全局单 in_progress、completed 前缀、段序（当前项必须是第一个未完成段），故"首个子项为 in_progress"之外的分叉形态已被其拒绝；本放宽只需守住"身份保留 + 纯拆分"边界。**evidence.go 不改**——它的序列规则正是前端 G2 原子性的强制来源。

**新增测试**（`internal/tool/builtin/todo_test.go`）：
- `TestTodoWriteAllowsSplittingCurrentItemIntoPhaseAndChild`：`[X(in_progress)] → [X(phase,pending), X1(sub,in_progress)]` 合法；
- `TestTodoWriteRejectsSplittingWithNonInProgressFirstChild`：子项非 in_progress 仍拒；
- `TestTodoWriteRejectsPhaseWithoutChildRevertingToPending`：phase 无子项回退 pending 仍拒；
- 回归：现有 `TestTodoWriteAcceptsLevels` / `TestTodoWriteRejectsBadLevel` 全绿。

**验证**：`go test ./internal/tool/builtin/`

### A-2 加固：扩展 `verifyStepIDsPreserved`（`internal/tool/builtin/todo.go:133-153`）

**现状**：仅拦截"项还在但丢了 step_id"（`:146-150`）；`!found`（整体删除）直接 `continue` 放行。

**扩展规则**：
- `previous` 有未完成工作时（`len(evidence.IncompleteTodos(previous)) > 0`）；
- `prev.StepID != ""` 且既无 `MatchStepID` 命中、又无 `MatchTodoIdentity` 命中（**整体删除**）；
- → 报错（提示 re-send 该 step_id 项）。
- 保留现状豁免：无 step_id 的 freehand 项可自由增删（`TestTodoWriteKeepsFreehandListsWorkingWithoutIDs` 为回归护栏）；宿主授权替换时跳过（现有调用点已由 `HasPlanReplacementAuthorization` 包裹）。

> 覆盖缺口说明：in_progress 项删除已被 A-1 拦，completed 项删除已被 `verifyCompletedTodoPositions`（todo.go:179-197）拦——本加固净新增覆盖为 **pending 带 step_id 项的整体删除**。

**新增测试**（`internal/tool/builtin/step_identity_test.go`）：
- `TestTodoWriteRejectsRemovingStepIDWhileUnfinishedWork`：有未完成工作时删带 id 的 pending 项 → 报错；
- `TestTodoWriteAllowsRemovingStepIDWhenAllCompleted`：全完成后删除不再拦截（不阻碍收尾清理）。

**验证**：`go test ./internal/tool/builtin/`

### A-3 配套：`recoveryPlanTransition` 纯拆分豁免（`internal/agent/agent.go:2266-2290`）

**现状**：拆分后 `samePlanStructure`（len 不等）→ 返回 `planTransition=true` → Auto 评审阻断（execute_one.go:438、444-460）。

**豁免谓词**（`isPureCurrentItemSplit(before, after)`，插入在 `samePlanStructure` 检查之前；命中则返回 `false` 不送审）：
1. `len(after) == len(before) + 1`；
2. `h` = `before` 中 in_progress 项下标（串行契约保证唯一）；
3. 对 `i < h`：`before[i]` 与 `after[i]` 的 `Level` 相等、归一化 `Content` 相等（`samePlanStructure` 的逐位比较复用）；
4. `before[h]` 与 `after[h]` 身份/文案/Level 相等，且 `after[h].Status == "pending"`；
5. `after[h+1].Level == 1` 且 `after[h+1].Status == "in_progress"`；
6. 对 `i > h`：`before[i]` 与 `after[i+1]` 逐位相等（新增项必须是唯一改动）。

**保守性**（评审 P5）：拆分 + 顺带改文案/重排/多增项的同一调用不满足上述谓词 → 维持现状送 Auto 评审。谓词基于 baseline 对比，不只看自身形态。

**新增测试**（`internal/agent/recovery_gate_test.go`）：
- `TestRecoveryPlanTransitionAllowsPureSplitOfCurrentItem`：纯拆分 → 不触发评审；
- `TestRecoveryPlanTransitionReviewsSplitWithExtraReorder`：拆分+重排 → 仍触发评审；
- 回归：现有 `TestRecoveryPlanTransitionDetectsOnlyStructuralRewriteOfActivePlan` / `TestRecoveryPlanTransitionIgnoresCompletedPriorPlan` 全绿。

**验证**：`go test ./internal/agent/ -run RecoveryPlanTransition`

**阶段 A 退出标准**：三处改动 + 全部新增/回归测试绿；`go test ./internal/tool/builtin/ ./internal/agent/ ./internal/evidence/` 通过。

---

## 3. 阶段 B：前端纯函数（`desktop/frontend/src/lib/todoVisibility.ts`）

> 评审 P6：分组逻辑与汇总一起进 `todoVisibility.ts`（纯函数、可测试），`TodoPanel.tsx` 只做哑渲染。

### B-1 `groupTodos(todos: Todo[]): TodoGroup[]`

镜像后端 `serialTodoSegments`（evidence.go:116-129）：
```ts
export interface TodoGroup {
  phase?: Todo;        // level-0 父行；游离 level-1 / 普通行无 phase
  children: Todo[];    // phase 后紧随的连续 level-1；无子项时为空数组
}
```
规则：level-0 拥有紧随其后的连续 level-1 段；游离 level-1（前项非 level-0）与普通行各自成组（`phase: undefined`）。**不得**用"最近前置 phase"归属游离 level-1（与后端段模型分叉）。

### B-2 `phaseSummary(group: TodoGroup): { done: number; total: number } | null`

- 仅 `group.phase` 存在且 `children.length > 0` 时返回 `{ done: children 中 completed 数, total: children.length }`；
- 无子项 → `null`（不渲染 Chip）；
- 裸数字（`2/3`），无 i18n（设计 §5.3）。

### B-3 单测（`desktop/frontend/scripts/test-todo-visibility.mjs`）

在现有 import 列表（:19-31）追加 `groupTodos`、`phaseSummary`，新增断言：
- phase+子项分组正确；连续 phase 各归其段；游离 level-1 独立成行；lone level-0 无 chip；
- chip 计数：0/3、2/3、3/3；无子项返回 null；
- 回归：现有全部断言不动。

**验证**：`pnpm test:todo-visibility`（含性能断言 200k 次 < 500ms——分组需保持 O(n) 摊还，勿在渲染路径内重复分组）。

---

## 4. 阶段 C：`TodoPanel.tsx` 渲染（约 +50 行）

| 步骤 | 改动 | 要点 |
| :--- | :--- | :--- |
| C-1 | 分组渲染嵌套 `<ul>` | `todos` → `groupTodos(todos)`（`useMemo`，单次分组）；phase 组渲染 `<li class="todobar__item todobar__item--phase …">` 内含子 `<ul class="todobar__sublist">`；游离/普通行平铺，**渲染与现状完全一致**（回归面最小，设计 §2.3）。 |
| C-2 | Chevron 折叠 | 仅 phase 且有子项时渲染；hover 显示（CSS 配合）；局部 `useState` 按组索引记录折叠；**默认展开**；批次重挂载（`key={scopedTodoBatch}`，App.tsx:4969）自动重置，不持久化（设计 §4.1）。 |
| C-3 | 子进度 Chip | phase 行 status 徽标旁渲染 `phaseSummary` 结果 `n/m`（设计 §2.2/§3）。 |
| C-4 | currentRef 复用 | 归一化状态为 `in_progress` 的 li（phase 或子项）挂 `currentRef`；依赖数组保持 `[open, current?.content, current?.activeForm]`——评审已确认翻转时 `current` 从子项变父行、content 必变、effect 触发滚动（设计 §4.3）。 |
| C-5 | 保持不动 | `done/total` 徽标（全行 raw completed 计数）、`allDone` 判定、Dismiss、整架 open 状态持久化。 |

**验证**：`pnpm typecheck`（tsc --noEmit）。

---

## 5. 阶段 D：`styles.css`（约 +35 行，todobar 区 :21661-21738 之后追加）

| 步骤 | 样式 | 要点 |
| :--- | :--- | :--- |
| D-1 | `.todobar__sublist` | 嵌套 ul 重置：`list-style:none; margin:0; padding:0; display:grid; gap:4px;`（外层 `.todobar__list` 的 reset 不继承到嵌套 ul）。 |
| D-2 | 左树线 | `.todobar__item--sub` 现有 `padding-left:18px` 基础上加 `border-left:2px solid var(--border); margin-left:8px;`。 |
| D-3 | phase 行四态 | 复用现有 `--pending/--in_progress/--completed` 状态色（:21725-21735）；补 `.todobar__item--phase` 布局（grid 列容纳 chevron/status/chip/text）。 |
| D-4 | Chip | 小号胶囊：复用 status 徽标骨架，弱化边框/前景（`--fg-dim`）。 |
| D-5 | Chevron | `opacity:0` → `.todobar__item--phase:hover` 显示；折叠态旋转（`▸/▾`）。 |

**验证**：`pnpm check:css`（含 `check-css-syntax.mjs src/styles.css`，构建 gate 之一）。

---

## 6. 阶段 E：验证与验收

| 项 | 命令/方式 | 对应验收 |
| :--- | :--- | :--- |
| E-1 后端单测 | `go test ./internal/tool/builtin/ ./internal/agent/ ./internal/evidence/` | A-1/A-2/A-3 新增 + 回归 |
| E-2 前端类型 | `pnpm typecheck` | 设计 §7.3 |
| E-3 前端单测 | `pnpm test:todo-visibility` | B-3 新增 + 回归 |
| E-4 CSS gate | `pnpm check:css` | D 改动语法/令牌合规 |
| E-5 手测 F4/F5（可先行） | 桌面 App：全部完成自动折叠 / 无子项列表与现状一致 | 设计 §7.1 |
| E-6 手测 F1–F3（需 A 落地） | 拆分当前项→嵌套+树线；子项逐个完成→chip 递增、父行翻转高亮+自动滚动；父行签收→下一节点推进 | 设计 §7.1 |
| E-7 回归 | 批次重挂载/折叠记忆、Dismiss、localStorage 行为与改动前一致 | 设计 §7.2 |
| E-8 预推送 CI | `gofmt -w .` → `go vet ./...` → `make lint` → `go test ./internal/tool/builtin/ ./internal/boot/` | REASONIX.md 预推送清单 |

**验收顺序建议**：E-2/E-3/E-4 每次前端改动即跑；E-5 于 B–D 完成后；E-6 于 A 落地后集中执行（F1–F3 依赖链见 §0）。

---

## 7. 阶段 F：收尾

### F-1 PR 拆分建议

| PR | 内容 | metadata 注意 |
| :--- | :--- | :--- |
| PR1（后端） | 阶段 A 三处改动 + 测试；设计文档三处修订（0-1/0-2/0-3 中后端契约部分） | 命中 cache 敏感列表（`internal/tool/`、`internal/agent/`）→ 必填 `Cache-impact:` / `Cache-guard:`（`none` 仅在 provider 可见前缀字节不变时可用）；docs 有改动 → `Documentation-impact: updated - <内容>`；分隔符用 ASCII `-`/`:`。 |
| PR2（前端） | 阶段 B–D + 单测 | user-visible diff；docs 未动 → `Documentation-impact: none - <理由>`。 |

- 每个 PR 独立可测：PR1 落地后 F1–F3 才可验收；PR2 独立保证 F4/F5 与回归。
- 一次评审一轮 force-push；评审反馈 amend 不追加 commit（REASONIX.md PR hygiene）。

### F-2 记忆更新

- 实施完成后更新 `todo-loss-root-cause-and-fix-plan`：标记后端三处改动已落地（方案启动→完成），前端 phase 渲染层为新增覆盖。

---

## 8. 风险与缓解

| # | 风险 | 缓解 |
| :--- | :--- | :--- |
| R1 | A-3 谓词过宽：真 replan 被误豁免 | 谓词要求"新增项为唯一改动"（逐位对比）；测试锚定"拆分+重排仍送审"。 |
| R2 | A-1 谓词过宽：非纯拆分回退 pending 放行 | 要求同身份 + level-0 + 紧随 in_progress level-1 子项三条件齐备；`ValidateSerialTodos` 先行兜底。 |
| R3 | 前端 live/canonical 双路径滞后：`complete_step` 经 `AdvanceSerialTodo`（evidence.go:284-322）推进 phase 时，live todo_write args 可能短暂滞后 | **现有行为，不在本方案范围**；设计依赖模型重发 todo_write（与后端强制原子翻转一致），面板下次 todo_write 事件即收敛。 |
| R4 | 行数预算偏紧（C +50 / D +35 vs 设计 +40/+30） | 已按评审上调预留；分组移入纯函数后组件内为哑渲染，超预算风险低。 |
| R5 | F1–F3 验收依赖 A 落地 | 阶段顺序固定；PR1 先行合入后集中验收，避免前端单独无法验证。 |
| R6 | 游离 level-1 分组与后端分叉 | B-1 明确镜像 `serialTodoSegments`，单测锚定；禁止"最近前置"实现。 |

---

## 9. 实施顺序总览（checklist）

- [ ] 阶段 0：设计文档三处修订
- [ ] A-1 `verifyTodoCurrentContinuity` 放宽 + 3 测试 + `go test ./internal/tool/builtin/`
- [ ] A-2 `verifyStepIDsPreserved` 加固 + 2 测试 + `go test ./internal/tool/builtin/`
- [ ] A-3 `recoveryPlanTransition` 豁免 + 2 测试 + `go test ./internal/agent/ -run RecoveryPlanTransition`
- [ ] B-1 `groupTodos` / B-2 `phaseSummary` + B-3 单测 + `pnpm test:todo-visibility`
- [ ] C-1~C-5 `TodoPanel.tsx` 渲染 + `pnpm typecheck`
- [ ] D-1~D-5 `styles.css` + `pnpm check:css`
- [ ] E-5 手测 F4/F5 + E-7 回归
- [ ] E-6 手测 F1–F3（PR1 落地后）
- [ ] E-8 预推送 CI（gofmt / vet / make lint / 重点测试）
- [ ] F-1 PR 拆分（metadata 字段齐全）
- [ ] F-2 更新记忆 `todo-loss-root-cause-and-fix-plan`
