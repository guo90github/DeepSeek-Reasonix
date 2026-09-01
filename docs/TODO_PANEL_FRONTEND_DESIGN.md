# 待办面板（TodoPanel）前端交互设计文档

> **文档状态**：方案定稿（待实施）
> **适用范围**：仅前端渲染逻辑与样式（`desktop/frontend`）
> **核心约束**：零 Wire 变更、零数据模型变更、不引入新组件、保持模块独立
> **关联文档**：`docs/TASK_TREE_DESIGN.md`（任务树设计，概念对齐）；后端修复方案见附录 A

---

## 摘要

当前"待办（TodoPanel）"面板以平铺列表渲染 `todo_write` 提供的任务清单。当 agent 将某一任务拆解为子待办（生成 `level:1` 子项）后，父待办在视觉上消失、进度语义断裂（详见附录 A 的根因分析）。

本文档定义前端交互方案：**在不触碰 Wire 层、数据模型与既有模块边界的前提下**，仅通过修改 `TodoPanel.tsx` 的渲染逻辑与 `styles.css` 的样式，将"父待办保留 + 子完成 → 父推进"的业务语义可视化。方案复用现有 DOM 结构（嵌套 `<ul>`）与现有状态字段（`Todo.level`），预计改动量：`TodoPanel.tsx` 约 +40 行、`styles.css` 约 +30 行。

---

## 1. 设计目标

| 编号 | 目标 | 说明 |
| :--- | :--- | :--- |
| G1 | **父待办可见性** | 拆解后父待办以"阶段行（Phase 行）"形态保留，不再消失；Phase 行与子项形成可读的层级结构。 |
| G2 | **状态感知** | 用户能一眼识别"当前进度"及"父节点等待条件"；子项全部完成后，父行高亮并标记为当前项（In Progress）。 |
| G3 | **进度语义对齐** | Phase 签收前视为未完成，`done/total` 徽标计全部行，确保"直到所有待办做完"的逻辑一致性。 |

设计边界：**面板为只读展示**。列表的写入通道唯一由 agent 的 `todo_write` / `complete_step` 驱动，前端不新增任何用户写操作。

---

## 2. 渲染结构与 DOM 模型

### 2.1 外壳保持（零改动）

保留现有外壳结构（`TodoPanel.tsx:81-127`）：

- `PromptShelf` 折叠卡片（标题 `t("todo.title")` = "待办"、`done/total` 徽标、header summary）；
- 批次管理、Dismiss 机制、折叠记忆（localStorage）均沿用现状。

### 2.2 列表内部结构（本次改动）

- 使用**嵌套 `<ul>`** 实现 Phase 行包裹子 `<ul>`：Phase 行作为父 `<li>`，其 level-1 子项放入内层 `<ul>`；
- **分组规则镜像后端 `serialTodoSegments`**（`internal/evidence/evidence.go:116-129`）：level-0 拥有紧随其后的连续 level-1 段；**游离 level-1**（前项非 level-0，后端合法输入）**独立成行**，不并入任何 phase；
- 子项样式沿用现有 `todobar__item--sub`，新增 CSS `border-left` 实现左树线效果；
- **徽标计算**：`done/total` 统计**全部行**；Phase 仅在签收后计入 `done`。

### 2.3 视觉示例

```text
▸ 阶段A（pending，等子项）  [2/3]      ← phase 行 + 子进度 chip
   1. 子A1  in progress                ← 缩进 + 左树线
   2. 子A2  pending
▸ 阶段B（in_progress，子项已完） [3/3]  ← 高亮显示
   1. 子B1  completed
   2. 子B2  completed
   3. 子B3  completed
- 普通步骤（无子项，渲染现状不变）
```

要点：

- Phase 行的 `[n/m]` 为**子进度 Chip**（子项 `completed` 数 / 子项总数），是"子完成 → 父感知"的量化表达；
- 无 level-1 子项的普通行渲染**完全不变**，保证回归面最小。

---

## 3. 状态映射表（Phase 行四态）

| 状态 | 触发条件 | 视觉呈现 |
| :--- | :--- | :--- |
| **Pending（等子项）** | 子项未全部完成 | 弱化显示（`--pending`）+ 显示子进度 Chip（`n/m`） |
| **In Progress（当前）** | 子项全完成，Phase 升为 `in_progress` | 高亮显示（`--in_progress`）+ 自动滚动至可视区域 |
| **Completed（完成）** | 用户签收后 | 常规完成样式（`--completed`） |
| **Normal（普通行）** | 无 Level-1 子项 | 保持现状不变 |

映射规则说明：

- "子完成 → 父感知"的视觉落点 = **Chip 数字走满 + 父行从 `--pending` 翻转为 `--in_progress`**，无需动画或通知机制；
- **原子性契约（后端强制，非模型自律）**：`validateSerialSegment`（`internal/evidence/evidence.go:169-188`）拒绝 "pending 父行+子项全完成"（判定为 stale）、"in_progress 父行+未完成子项"、"completed 父行+未完成子项"三种形态；因此最后子项完成的那次 `todo_write` 必须同步把父行翻转为 `in_progress`，否则整份列表被 `ValidateSerialTodos`（`evidence.go:99-101`）拒绝。前端永远看不到"chip 走满但父行仍 pending"；
- 状态判定复用现有归一化逻辑（`normalizeTodoStatus`，`TodoPanel.tsx:129-138`）与 `level` 字段（`lib/tools.ts:127-132`），不引入新字段。

---

## 4. 交互逻辑

### 4.1 展开 / 收起

- Phase 行 Hover 时显示 Chevron 图标；
- **默认展开**：子项代表正在执行的工作，不应隐藏；
- **折叠状态**：使用局部 `useState` 管理（按索引键），**批次重挂载时重置，不持久化**（与现有折叠记忆语义隔离）。

### 4.2 写入权限

- 面板保持**只读展示**；
- 唯一写入通道为 agent 的 `todo_write` / `complete_step`；
- 用户操作仅保留现状能力：全完成后的 Dismiss（关闭待办列表）。

### 4.3 自动滚动

- 复用现有 `currentRef` 逻辑（`TodoPanel.tsx:60,74-77`），无需额外代码即可指向 `in_progress` 行；
- 当当前项恰为 Phase 行（子项全完成、待签收）时，`currentRef` 天然指向该父行，滚动行为自动正确。

---

## 5. 影响范围与改动清单

### 5.1 零改动层（Wire 契约冻结）

| 层 | 说明 |
| :--- | :--- |
| Wire 类型定义 | `TaskCatalog`/`Todo` 相关 Wire 类型不变 |
| `parseTodos` | 解析逻辑不变（`lib/tools.ts:135-142`） |
| `useController` | 事件流与状态派生不变 |
| `App.tsx` | 挂载点、批次 key、`showTodos`/`dismissTodos` 流程不变 |
| 批次处理与 Dismiss 逻辑 | 含 localStorage 折叠/关闭记忆，全部不变 |

### 5.2 前端改动清单

| 文件 | 改动 | 预估量 |
| :--- | :--- | :--- |
| `TodoPanel.tsx` | Phase 分组渲染（嵌套 `<ul>`）、Chevron 图标、子进度 Chip | 约 +40 行 |
| `styles.css` | Todobar 子树样式（左树线 `border-left`、Phase 行四态、Chip） | 约 +30 行 |
| `todoVisibility.ts`（可选） | 新增纯函数 `phaseSummary(todos)` 计算 Phase 子进度，便于测试复用 | 约 +20 行 |

### 5.3 国际化（i18n）

- **无新增文案**；
- Chip 直接使用裸数字格式（如 `2/3`），不引入 i18n key。

---

## 6. 默认决策与权衡

### 决策 1：拆分触发新批次

- **决策**：面板重挂载并折叠（沿用现有 batch 策略，`App.tsx:4968` `key={scopedTodoBatch}`）。
- **理由**：与现有批次策略保持一致，避免双套折叠语义并存；Header Summary 显示当前子项，折叠态下进度仍可见。
- **权衡备注**：暂不实施 mid-turn 新批次自动展开——需要区分"live 更新 vs 新批次"，实现成本高，非 v1 必需；作为后续迭代候选。

### 决策 2：默认展开行为

- **决策**：默认展开所有子项。
- **理由**：降低用户认知负荷，避免隐藏关键工作流；子项是"正在执行的工作"，不应默认折叠。
- **权衡备注**：深层自动展开（仅展开当前链）留作后续迭代，v1 不做。

---

## 7. 验收标准（Acceptance Criteria）

### 7.1 功能测试（手测）

| 编号 | 场景 | 预期结果 |
| :--- | :--- | :--- |
| F1 | Agent 拆分当前项 | 父行保留且子行出现（嵌套渲染 + 左树线） |
| F2 | 子项逐个完成 | Chip 数字递增直至走满；父行状态翻转为 In Progress（高亮 + 自动滚动） |
| F3 | 父行签收 | 父行转 Completed；下一节点推进为当前项 |
| F4 | 全部完成 | 面板自动折叠（沿用现有 allDone 收起逻辑） |
| F5 | 普通列表（无子项） | 渲染与现状完全一致（回归） |

### 7.2 回归测试

- 现有批次管理、Dismiss 机制、折叠记忆行为保持不变；
- 单元测试覆盖：`use-controller-meta.test.ts` 及 `todoVisibility` 相关用例（含可选新增的 `phaseSummary` 单测）。

### 7.3 构建验证

- 执行 `npx tsc` 无类型错误；
- 确认 Wire 接口无变化（零 wire 变更约束满足）。

---

## 附录 A：后端前置依赖（本设计成立的前提）

本前端设计依赖后端修复方案将"拆分当前项"合法化。未落地前，agent 无法合法生成"当前项 → phase + 子项"的结构，Phase 行场景不会出现（不影响本方案其余渲染的回归）。后端三处改动：

1. **核心（必改）**：放宽 `verifyTodoCurrentContinuity`（`internal/tool/builtin/todo.go:155-177`）——允许当前 in_progress 项在"保留同一身份、成为 phase 头、其首个 level-1 子项为 in_progress"的纯拆分形态下回退 pending；
2. **加固（建议）**：扩展 `verifyStepIDsPreserved`（`internal/tool/builtin/todo.go:133-153`）——有未完成工作时，删除带 `step_id` 的项也报错，防静默丢失；
3. **配套（必需，与 1 耦合）**：豁免 `recoveryPlanTransition`（`internal/agent/agent.go:2266-2290`）对纯拆分形态的 Auto 评审（否则拆分通过 Execute 后仍被 `execute_one.go:438,444-460` 的 recovery gate 阻断）。豁免谓词 `isPureCurrentItemSplit(before, after)`：`len(after) == len(before)+1`；`h` = before 中 in_progress 项下标（串行契约保证唯一）；`i<h` 与 `i>h` 段与 after 逐位比较 `Level`+归一化 `Content` 相等（`i>h` 偏移 +1）；`after[h]` 与 `before[h]` 同身份且状态为 `pending`；`after[h+1]` 为 `in_progress` 的 level-1。**拆分 + 其他改动（改文案/重排/多增项）不满足谓词 → 仍送 Auto 评审**。

> 详细根因与风险评估见项目记忆 `todo-loss-root-cause-and-fix-plan`（方案搁置待启动）。

## 附录 B：相关代码锚点

| 锚点 | 位置 |
| :--- | :--- |
| TodoPanel 外壳/徽标/折叠 | `desktop/frontend/src/components/TodoPanel.tsx:81-127` |
| 状态归一化 | `TodoPanel.tsx:129-138` |
| currentRef 自动滚动 | `TodoPanel.tsx:60,74-77` |
| Todo 类型（含 `level`） | `desktop/frontend/src/lib/tools.ts:127-132` |
| `parseTodos` | `tools.ts:135-142` |
| 挂载点与批次 key | `desktop/frontend/src/App.tsx:4968` |
| 可见性/Dismiss/batch key | `desktop/frontend/src/lib/todoVisibility.ts` |
| 单元测试 | `desktop/frontend/src/__tests__/use-controller-meta.test.ts` |
