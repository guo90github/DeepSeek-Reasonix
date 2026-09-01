# TASK_TREE_DESIGN — 多层级任务树：布局升级与父子执行语义

> 范围限定：仅任务 4（任务拆解展示 + 父子执行语义）。其余三个方向经评估暂不实施。
> 本文档结论全部基于对当前代码库的逐行核实（见附录 A 证据索引），非推断。
>
> **v2 修订（评审后定稿）**：评审发现 subagent 的父（工具调用）在 taskcatalog 无对应行等
> 阻断性缺口。经取舍定案：① subagent 父 = **合成父 snapshot**（§5.2.2）；② 新增
> `TaskStateSkipped` 终态（§5.1）；③ `nested:true` 返回 **partial tree**，前端按
> `version/updated_at` 仲裁增量合并（§5.4 ①）。其余修订见 §5/§6/§7/§9 与附录 A 修正。

## 1. 背景与目标

当前任务体系存在两个断层：

1. **展示断层**：任务列表是平铺卡片（`TaskMonitorPanel`），`TaskSnapshot` 数据模型无父子字段；
   子任务生成后无法挂在父任务下，视觉上"覆盖"顶层任务。
2. **语义断层**：子任务完成后父任务如何感知、父任务如何继续推进相邻节点，没有统一模型
   （fleet 有依赖图调度，但任务持久层与实时进度层互不相通）。

目标：

- 提供用户友好的三层任务视图（快速查看 / 停靠面板 / 会话内卡片），统一展示任务树。
- 建立"子完成 → 父感知 → 父推进相邻节点"的完整执行语义，并与现有引擎（fleet/driveFleet）对齐。
- 全栈落地：`TaskSnapshot` schema v1→v2、taskcatalog SQLite 迁移、新 API、前端树形交互。

## 2. 现状核实结论（摸排结果）

### 2.1 数据源与存储

- 持久任务存储：`internal/taskmonitor`（`TaskSnapshot`，`internal/taskmonitor/model.go:158`，schema v1）+ `internal/taskcatalog`（SQLite 投影，`catalog.go:143`）。
- SQLite 位置（本机）：`C:\Users\guosj\AppData\Local\reasonix\task-catalog\v1.sqlite`
  （解析链：`taskcatalog.DefaultPath()` = `config.CacheDir()/task-catalog/v1.sqlite`，
  `config.CacheDir()` = `userCacheDir()` = `REASONIX_CACHE_HOME` → `REASONIX_HOME/cache` → `%LOCALAPPDATA%/reasonix`，`catalog.go:120`、`paths.go:168,515`）。
  WAL 模式（存在 `v1.sqlite-wal` ~4MB / `-shm`），拷贝需三件套或先 checkpoint。
- 实测内容：`task_snapshots` 277 条、`task_projects` 11、`task_events` 554；
  状态分布 succeeded 196 / failed 41 / cancelled 40；抽样为后台 bash 任务，schema v1 无父子字段。
- **有利事实**：`task_snapshots` 表权威数据在 `snapshot_json`（BLOB）列，
  `TaskSnapshot` 加字段无需 `ALTER` 即可投影；表列只是反范式副本。迁移走现成 `schema_migrations` 表。

### 2.2 现有执行机制（可复用的语义资产）

| 机制 | 位置 | 语义 |
|---|---|---|
| 子→父实时状态 | `internal/agent/subagent_progress.go`（`subagentProgressTracker`/`Merger`）+ `internal/event/subagent_progress.go:19-25` 保留通道 `reasonix.subagent.status/reasoning/text/notice` | 子任务 phase（queued/running/…/completed/failed/cancelled）实时推前端；组级 terminal 由父 merger 发。**内存态，不持久**（`useController.ts:120` 注释明示） |
| 依赖图调度 | `internal/agent/fleet_graph.go`（`newFleetPlan`/`driveFleet`/`skipDependents`，`:27/:203/:177`）+ `fleet.go:39` 工具描述 | `depends_on` 依赖排序；失败→下游 skipped、独立分支继续；`fail_fast` 首败即停。**这就是"父推进相邻节点"的现成实现** |
| 任务快照写入 | `internal/taskmonitor/recorder.go`（`TaskRecorder.RecordStart/Done`，实现 `jobs.TaskRecorder`） | 只接 `jobs.Manager` 生命周期；**subagent（task/fleet/parallel_tasks）不写 TaskSnapshot** —— 最大缺口 |
| 子代理父子链 | `internal/agent/subagent_store.go:43-44`（`SubagentMeta.ParentSession`/`ParentToolCallID`）；tool 事件层 `ParentID`（`parallel_tasks.go:171`）；调用汇聚点 `TaskTool.RunProfileSpec`（`task.go:732`，task/fleet/parallel_tasks 唯一执行入口） | 父子关系在子代理工件层已存在，未落到 TaskSnapshot；**observer 挂 RunProfileSpec 一处覆盖三类** |
| 前端实时预览 | `desktop/frontend/src/components/ToolCard.tsx:24`（`subagentPhaseLabel`，`:22` 为 SUBAGENT_TOOLS）；Transcript `subcallsByParent`（`Transcript.tsx:356`） | 工具卡片已能嵌套渲染子代理调用 —— 树形渲染能力已具备 |
| 持久投影接线 | desktop recorder 经 `taskcatalog.ObservedStore()` 写入（`desktop/app.go:11531`）；运行时 overlay `overlayTaskCatalogRuntime`（`desktop/task_catalog.go:164-217`）把 controller 内 job 状态覆盖到 catalog 行 | 新 snapshot 自动投影进 SQLite；**运行时状态存在第三写源**（§9 风险） |

### 2.3 前端现状

- `TaskMonitorPanel` 在 App.tsx 仅一个挂载点：话题栏更多菜单 → `openSessionSummary` 切换 `tasksOpen`（App.tsx:1195）→ `taskmonitor-popover`（App.tsx:4794-4800，样式 `styles.css:39609`，440px 宽、72vh 高）。
- 数据：5s 轮询 `ListTaskPage`（`POLL_INTERVAL_MS=5000`，limit 50 + cursor，`TaskMonitorPanel.tsx:114,175,237-243`）；行内展开拉事件流 `fetchEvents`（`:190`，按 sequence 增量去重）。
- 可停靠面板先例：`MemoryPanel` 用 `ResizableDrawer`（`MemoryPanel.tsx:440`）——任务中心停靠面板直接复用该模式。

## 3. 方案架构总览

```
                ┌───────────────────────── 任务树（parent_id + position + depends_on）────────────┐
                │                                                                                  │
 实时通道(ToolProgress)    持久通道(taskcatalog TaskSnapshot v2)        执行引擎(fleet/driveFleet)
 子→父状态实时跳动          父聚合(agg_*)写回 + 5s 轮询校正              推进游标 + 失败跳过
                │                                                                                  │
                └───────────────┬───────────────────────────────┬────────────────────────┘
                                ▼                               ▼
                    L1 popover 折叠树(会话摘要)       L2 停靠面板任务中心(ResizableDrawer)
                                ▼
                    L3 Transcript 内 ToolCard 树(对话流上下文)
```

三条支柱：

1. **双通道状态感知**：实时（ToolProgress，turn 内跳动）为准、持久（snapshot 聚合，重启后可感知）兜底。
2. **推进语义复用引擎**：前端只读 `position/depends_on` + 状态渲染"当前在哪、下一步是谁"，
   调度推进由 `fleetPlan`/`driveFleet` 承担，不双实现。
3. **三层视图共享一棵树**：L1 折叠看全局、L2 操作看细节、L3 对话流中看上下文。

## 4. 前端实施细节

### 4.1 组件结构

```
TaskMonitorPanel.tsx（外壳：轮询/错误/空态，保留）
├── TaskTreeView（新文件 TaskTreeView.tsx，替换平铺渲染；L1/L2 共用）
│   ├── TaskTreeNode（递归组件，新文件 TaskTreeNode.tsx）
│   │   ├── 展开箭头（有子任务时显示；箭头=展开子树，点行=展开事件流，两者已分离）
│   │   ├── 聚合徽标（x/y 完成；任一 failed 父标红）
│   │   ├── 当前游标（★ 执行中 / → 下一相邻节点 / ⊘ 跳过）
│   │   └── 操作菜单（stop/cancel/requeue，复用 TaskActionRequest）
│   └── 子树过滤（Focus subtree / Show all / 面包屑）
└── （L2 额外）搜索 + 状态筛选 + 实时事件订阅
```

纯函数层（与 React 解耦、可单测）：`desktop/frontend/src/lib/taskTree.ts`

```typescript
export function buildTaskTree(items: TaskCatalogItem[]): TaskNode[] {
  // 一次 Map 归并 O(n)；两遍解析：父缺失/悬空 → 归为根（容错乱序到达）
}
export function aggregateState(node: TaskNode): { done: number; total: number; failed: boolean } {
  // 后序 DFS 聚合（含自身）；父徽标 = f(所有后代)
}
export function mergeTaskPages(prev: TaskNode[], next: TaskCatalogItem[]): TaskNode[] {
  // 轮询增量就地挂树；新子任务插入父下，父行不消失（解决"覆盖"）
}
export function nextPendingSibling(node: TaskNode, roots: TaskNode[]): TaskNode | null {
  // 当前游标推进：终态后按 position 序取下一个就绪兄弟（queued/running/waiting）
}
```

### 4.2 三层视图

- **L1 popover（保留，树化）**：默认全折叠 + 聚合徽标；summaryMode 定位"扫一眼进度"。
- **L2 停靠面板（新增，主视图）**：`ResizableDrawer` 模式；话题栏新增"任务中心"按钮；
  全宽树 + 搜索 + 状态筛选 + 实时 ToolProgress 订阅（5s 轮询只做持久校正）。
- **L3 Transcript 卡片（加强）**：`subcallsByParent` 已嵌套；加折叠箭头 + 聚合徽标 + 缩进线。

### 4.3 状态管理

```typescript
// TaskMonitorPanel 内新增（现有 expanded 保留给事件流展开）
const [collapsed, setCollapsed] = useState<Set<string>>(new Set());  // 子树折叠态
const tree = useMemo(() => buildTaskTree(tasks), [tasks]);           // 平铺数组 → 树
// L2 实时订阅：useController 事件流里过滤 ToolProgress 保留通道 → 就地 patch 节点 phase
```

- 默认展开策略：L1 全折叠；L2 展开前 2 层，更深惰性展开（`…+N`）。
- 深度上限 8，由遍历计算实际深度，`depth` 字段仅作展示冗余。
- 聚合以实时（ToolProgress）跳动、持久（agg_*）兜底校正。

## 5. 后端实施细节

### 5.1 TaskSnapshot v2（`internal/taskmonitor/model.go`）

```go
type TaskSnapshot struct {
    // ……现有 v1 字段不变……
    SchemaVersion int `json:"schema_version"`   // 1 → 2

    ParentID    string   `json:"parent_id,omitempty"`   // 父 monitor id；空 = 根
    Position    int      `json:"position,omitempty"`    // 同级相邻节点顺序
    DependsOn   []string `json:"depends_on,omitempty"`  // 依赖 id（对齐 fleetTaskItem）
    Title       string   `json:"title,omitempty"`       // 任务标签（jobs label / subagent description）

    // 父节点聚合（持久通道，供轮询视图感知）
    AggDone   int `json:"agg_done,omitempty"`
    AggTotal  int `json:"agg_total,omitempty"`
    AggFailed int `json:"agg_failed,omitempty"`
}
```

- **同步新增终态 `TaskStateSkipped`**（B2 定案）：fleet `skipDependents` 的产物
  （`fleet_graph.go:177`）。需同步 `ValidTaskStates`、`Terminal()`（`model.go:89-108`）、
  前端 `TaskState` union（`types.ts:2251`）、`TaskMonitorPanel` 的 `STATE_CONFIG`/`isTerminalState`
  （`TaskMonitorPanel.tsx:38,64`）。`task_snapshots.state` 列是 TEXT，新值无需迁移。
- **`Validate` 扩展**（`model.go:184-231`）：新字段纳入检查——`parent_id`/`title` 的
  `maxFieldLen`、`depends_on` 每项长度、`position` ≥ 0；协议校验实现 `protocolValidatable`
  同步（有既有测试）。
- 聚合语义：`agg_total` 含 skipped，`agg_done`/`agg_failed` 不含；前端徽标 = done/total 进度，
  任一 failed 标红，skipped 灰显（§7.3）。

### 5.2 写侧（父边 + 聚合）

1. **jobs 路径（现有）**：`TaskRecorder.RecordStart` 内写 `ParentID`（来源：job 元数据/`lookupMonitorID` 映射，`recorder.go:89-94`）。`jobs.TaskRecorder` 接口签名**不变**，父信息走可选元数据 carrier，兼容 CLI/Desktop 双端实现。`Title` 取 `RecordStart` 已携带的 `label` 参数（当前未落快照，`recorder.go:135`）。
   - **jobs 父 `Position`（B4 定案）**：`RecordStart(id, kind, label)` 无位置参数，本期按兄弟 `created_at` 排序兜底（前端展示序），不阻塞主流程；后续若需引擎序再让 `jobs.Manager` 暴露排队序（`jobs.go` 的 `order` 字段）。
2. **subagent 路径（新增，合成父 snapshot，B1 定案）**：observer 为 `taskmonitor.TaskTreeObserver`（`taskmonitor/task_tree.go`），agent 侧经 `task_tree_ctx.go` 助手接线，分两层：
   - **组行（fleet/parallel_tasks）**：前台 Execute 只印 observer 进 ctx；组行在 `runFleet` 内创建（`TaskID = treeTaskID(sessionID, groupCallID)`，沿用 jobs 命名空间但斜杠重编码，见下）；**组级 terminal 驱动父行终态**：对齐 `merger.directStatus` 收尾（`fleet.go:304-308`），判定复用 `fleetGroupTerminalPhase` 语义（cancel > failed > completed；`skipped` 只落子行，不升父）。
   - **子行**：`RunProfileSpec`（`task.go:732`）入口经 `beginTreeRecording` 创建（StartChild）、tracker `finish` 收尾（FinishChild）。fleet 子行 `TaskID = treeTaskID(sessionID, "groupCall/fleet-N")`，`ParentID`=组行，`Position=idx+1`，`DependsOn` = 图内 id → 子行 TaskID（经 `observer.TreeTaskID` 解析，`fleetPlan.deps`）；parallel 子行 `groupCall/sub-N`，无 depends_on。
   - **实现澄清（与 §5.2.2 原稿的差异）**：
     - **单 `task`/`read_only_task` = 叶子行**（无父子对）：其子代理卡片 ID 与工具调用 ID 相同（tracker `childID=parentID`），父+子双行会自引用；且其父是模型 turn 而非任务。`ParentID` 空、`Position=0`、`DependsOn` 空。
     - **`/` → `--` 重编码**：`treeTaskID` 将 callID 中的 `/` 换为 `--`——投影路径 `indexSnapshot`（`catalog.go:468`）防路径穿越拒绝含 `/` 的 task_id。
     - **background 模式整体跳过**：后台 task/fleet 是 jobs，`jobs.TaskRecorder` 已写 job 行（含 Title 语义待 recorder 补 label）；后台 fleet 子项经 ctx `suppress` marker 跳过，避免孤儿根行。
     - **聚合写回**：`TaskTreeRecorder` 进程内增量维护组聚合（done/total/failed/skipped），子 terminal/skipped 时写父 `agg_*`，组 terminal 时最终落盘（CAS 重试）。
3. **聚合写回**：父行存在后，子任务 terminal 时按 `ParentID` 对父 snapshot 重算 `agg_*`（与前端 `aggregateState` 同构的 Go 实现）；skipped 计入 total 不计 done/failed（§5.1 聚合语义）。
   - 父行/子行统一经 `taskcatalog.ObservedStore()`（`desktop/app.go:11531`）写入，自动投影进 SQLite，无需额外接线。写入走 `Catalog.queue`（1024 缓冲，`catalog.go:100`）异步落库，不阻塞工具执行路径。

### 5.3 SQLite 迁移（`internal/taskcatalog`）

```sql
-- 走现成 schema_migrations 表，启动时幂等执行
ALTER TABLE task_snapshots ADD COLUMN parent_id TEXT NOT NULL DEFAULT '';
ALTER TABLE task_snapshots ADD COLUMN agg_done INTEGER NOT NULL DEFAULT 0;
ALTER TABLE task_snapshots ADD COLUMN agg_total INTEGER NOT NULL DEFAULT 0;
ALTER TABLE task_snapshots ADD COLUMN agg_failed INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_task_parent ON task_snapshots(project_key, parent_id);
```

- v1 旧快照 `parent_id=''` 自动归为根——**已有任务不误挂子节点**。
- `snapshot_json` BLOB 自动携带新字段；reindex（`catalogs_tasks.go:43`）重建安全。
- `task_events` 表不加字段（事件流不变）。

### 5.4 API 定义

**① 扩展 `ListTaskPage`（向后兼容，`nested:false` = 现有行为）**

```
请求: { scope, tabId, projectKey, states, query, cursor, limit,   // 现有
        nested?: boolean, parentId?: string }                      // 新增
返回: { ...现有 TaskPage 字段, tree?: TaskNode[] }
```

- **分页契约（B3 定案，partial tree）**：`nested:true` 仍走现有 `updated_at DESC` 键集分页
  （`catalog.go:563-591`），父子可能散落多页——返回的是 **partial tree**，由现有
  `page.partial`（索引未追平）与 `nextCursor` 标示"还有更多"。前端 `mergeTaskPages`
  按 `version`/`updated_at` 仲裁增量合并（解决轮询旧帧覆盖实时 patch）；父行不因
  子行未入页而消失（父子同页或后续页补入，前端以持久字段为准）。

**② 新增 Wire 类型**（`desktop/task_catalog.go` + `desktop/frontend/src/lib/taskCatalogTypes.ts`
双镜像；`TaskSnapshot` 前端镜像在 `types.ts:2263`，`TaskPage`/`TaskPageRequest` 在
`taskCatalogTypes.ts:3-11`，`taskCatalogBridge.ts:6-15` 同步加字段）：

```typescript
// projectKey 每节点携带：父边按项目作用域解析，且操作菜单需要它做 Stop/Requeue
export interface TaskNode { projectKey: string; task: TaskSnapshot; children: TaskNode[]; }
// TaskSnapshot 扩展：parent_id?, position?, depends_on?, title?, agg_done?, agg_total?, agg_failed?
```

**③ 不动**：`ListTaskEventPage`、`StopTaskByKey/CancelTaskByKey/RequeueTaskByKey`、`OpenTaskSessionByKey`。

**④ 后端建树**：`taskcatalog.ListTree`（`catalog.go:568` 现有查询后，Go 版 `BuildTree` 与前端 `buildTaskTree` 同构：两遍解析 + 悬空归根）。

## 6. 前后端交互时序

```
[打开任务中心(L2)]  ListTaskPage({nested:true}) → taskcatalog.ListTree → {items, tree: partial, revision}
  └→ buildTaskTree(items) 幂等 → TaskTreeView 渲染根节点 + 聚合徽标
  └→ nextCursor 续拉 → mergeTaskPages 按 version/updated_at 仲裁补全

[subagent 工具调用（task/fleet/parallel_tasks）]
  调用开始 → observer 合成父 snapshot（running）→ SQLite upsert（异步队列）
  子 agent terminal → ① ToolProgress status(completed) 事件 → 前端就地 patch 节点 phase（实时）
                    → ② observer 写子 snapshot（含 position/depends_on/终态，skipped→TaskStateSkipped）
                      + 父 agg_* 写回 → SQLite upsert
                    → ③ 5s 轮询下一帧 → mergeTaskPages 校正（持久兜底，version/updated_at 仲裁）
  组 terminal（ToolProgress 组级生命周期）→ 父 snapshot 终态（cancel > failed > completed）
  父节点徽标刷新 + 游标推进到下一就绪兄弟（nextPendingSibling，仅展示，不调度）

[用户操作]  节点菜单 → StopTaskByKey / RequeueTaskByKey（现有 binding，零改动；requeue 保持父边与聚合）
```

## 7. 父子执行语义（关系定义）

1. **子任务 = 父任务执行计划中的一个步骤**（plan step）。父任务 = 编排者（orchestrator）。
2. **相邻节点 = 同一父下的兄弟步骤**：顺序由 `position` 表达，依赖由 `depends_on` 表达。
   递归成立：A-1 的子任务 = A-1 自己的 plan step，与父 A 的兄弟无关。
3. **推进规则**（对齐 `driveFleet`，`fleet_graph.go:203`）：
   - 完成即推进：completed → 游标移到下一个**就绪**相邻节点（position 序或依赖已满足）；无依赖兄弟可并行（受 `SubagentScheduler` 并发上限约束）。
   - 失败默认策略（`fail_fast=false`）：失败节点标记，其下游被 `skipDependents` 跳过（`fleet_graph.go:177`）落 **`TaskStateSkipped`（终态）**，**独立分支继续执行**——父不空等；skipped 计入 `agg_total` 不计 `agg_done`/`agg_failed`，徽标灰显（§5.1 聚合语义）。
   - `fail_fast=true`：首败即停，跳过全部剩余。
4. **展示层不实现调度**：前端只读 `position/depends_on` + 各子状态，渲染"当前在哪、下一步是谁"；
   游标由执行引擎驱动，避免两套调度打架。
5. **单 `task` vs `fleet`/`parallel_tasks` 澄清**：单 `task` 是阻塞式（父 agent 本轮模型调用等子返回）；
   组级工具是一次调用调度整组、按依赖图推进。两类都写入相同 `parent_id + position` 树——展示统一、调度语义不同，都符合"父感知 + 相邻推进"模型。
6. **父行生命周期（合成父 snapshot，B1 定案）**：subagent 的父 = 发起它的**工具调用**，不是
   monitor 任务。为让树在 catalog 层成立，每次 task/fleet/parallel_tasks 调用合成父行：
   running（调用开始）→ terminal（组级 terminal：cancel > failed > completed）；fleet 的
   `skipped` 只落子行不升父。父行 TaskID = `monitorTaskID(sessionID, callID)`，与 jobs 共享
   命名空间规则但 ID 空间本身不相交（jobs 是 `session--job`，subagent 是 `session--call`）。

## 8. 落地顺序（每步独立验收）

| 步 | 内容 | 验收 |
|---|---|---|
| 0 | 执行语义验证：真实 task + fleet 跑通，确认子状态与 `depends_on` 数据可达 | 无代码，观察记录 |
| 1 | L1 树化：`taskTree.ts` 纯函数 + popover 折叠树 + 聚合徽标 | 单测：乱序/悬空/聚合；UI 手测 |
| 2 | 持久层：`TaskSnapshot` v2（含 `TaskStateSkipped`）+ 合成父 snapshot + subagent observer + 聚合写回 + SQLite 迁移 | effect test：子完成后父 `agg_*` 到达 SQLite 行、父行 running→terminal、skipped 子行落库（`boot/effect_test.go` 模式） |
| 3 | L3 卡片加强：Transcript ToolCard 树加折叠/聚合 | UI 手测 |
| 4 | L2 停靠面板：`ResizableDrawer` 任务中心 + 实时 ToolProgress 订阅 | UI 手测 + 轮询/实时一致性（version/updated_at 仲裁） |
| 5 | API 收尾：`nested:true` 上线、`nested:false` 回退开关保留 | 回归 |

## 9. 风险与注意事项

| 风险 | 说明 | 对策 |
|---|---|---|
| subagent 父行生命周期（原"最大缺口"已定案） | 合成父行的写入点与终态驱动是新增机制 | observer 挂 `RunProfileSpec`（`task.go:732`）一处；父终态对齐 ToolProgress 组生命周期（`fleet.go:304-308`，含背景模式 handoff `:247-268`） |
| 三写源冲突 | ToolProgress 实时 / 5s 轮询持久帧 / `overlayTaskCatalogRuntime`（`desktop/task_catalog.go:164-217`，仅覆盖 jobs 行）同时写状态 | 前端 `mergeTaskPages` 按 `version`/`updated_at` 仲裁；overlay 只匹配 job key，subagent 行天然不命中 |
| skipped 状态贯穿 | `ValidTaskStates`/`Terminal()`/前端 union/`STATE_CONFIG`/`isTerminalState` 需同步，漏一处即渲染异常 | §5.1 已列改动点；落地时先跑 taskmonitor 既有测试 |
| 实时/持久双通道不一致 | ToolProgress 内存态，重启丢失 | 明确"实时为准、持久兜底"；聚合实时跳动、轮询校正（带版本仲裁） |
| 推进语义双实现风险 | 前端自实现调度会与引擎打架 | 展示层只读状态；游标由 `fleetPlan`/`driveFleet` 驱动 |
| monitor id 命名空间 | 父边必须用 `monitorTaskID`（`recorder.go:60`），非裸 job id / 裸 call id | jobs 父解析统一走 `lookupMonitorID`；subagent 父/子行一律 `monitorTaskID(sessionID, id)` 命名空间化 |
| Title 泄露 | 快照是 sanitised（`model.go:156-157`），`contentFreeTaskSnapshot` 只清 ErrorSummary（`cli/task.go:37`）；jobs label 可能含路径/命令 | 评估 title 纳入 scrubbing；fleet 默认 label（`fleet-N`）安全 |
| requeue 语义 | 子任务 requeue 后父徽标 x/y、position、depends_on 是否保持未定义 | 约定：requeue 保持父边与聚合原值，仅状态回退非终态（落实现时验证既有 `RequeueTaskByKey` 行为） |
| 深树/并行渲染 | `parallelTasksMaxTasks=64`、多叉深树 | L2 默认展开 2 层 + 惰性展开 + 聚合徽标；后续可虚拟滚动 |
| 高频入口回归 | popover→dock 改动影响日常使用 | 三层渐进落地，每步可验收；`nested:false` 回退 |
| 项目规范 | 800 行上限、effect test 落最终边界 | `taskTree.ts`/`TaskTreeNode.tsx` 拆文件（`TaskMonitorPanel.tsx` 现 616 行）；效果测试走 `boot/effect_test.go` 模式 |

## 附录 A：证据索引（文件:行号）

- `internal/taskmonitor/model.go:158-180` — TaskSnapshot v1 结构（含 `schema_version`，无父子字段）；`:89-96` `ValidTaskStates`（**无 skipped**）；`:184-231` `Validate`
- `internal/taskmonitor/recorder.go:17,134-135` — TaskRecorder 只接 jobs 生命周期；`monitorTaskID` `:60`；`rememberMonitorID` `:83` / `lookupMonitorID` `:89`；`RecordStart(id, kind, label)` `:135`（label 当前未落快照）
- `internal/taskcatalog/catalog.go:120-126` — DefaultPath（`cache/task-catalog/v1.sqlite`）；`:137-163` 表结构（snapshot_json BLOB）与 `migrations()`（仅 Version 1）；`:518-592` `ListPage`（updated_at DESC 键集分页）；`:192` `ObservedStore`
- `internal/taskcatalog/shared.go:233-236` — `ObservedStore()` 共享投影接线
- `internal/config/paths.go:168-180,515-521` — CacheDir 解析链
- `internal/agent/subagent_store.go:35-55` — SubagentMeta（`:43-44` ParentSession / ParentToolCallID）
- `internal/agent/agent.go:98-103,128-134` — `callContext.parentID` = **工具调用 ID**（非 monitor id）
- `internal/agent/fleet_graph.go:27,58-60,177,203` — newFleetPlan / deps 映射 / skipDependents / driveFleet（推进与跳过语义）
- `internal/agent/fleet.go:39,165` — 工具描述（失败跳过 + 独立分支继续）；fail_fast 参数；`:304-308` 组级 terminal；`:322` 子 ID `parentID/fleet-N`；`:340` RunProfileSpec 调用
- `internal/agent/task.go:732` — `RunProfileSpec`（task/fleet/parallel_tasks 唯一执行汇聚点）
- `internal/agent/subagent_progress.go` + `internal/event/subagent_progress.go:19-25` — 子→父实时状态通道
- `desktop/task_catalog.go:135-162` — `ListTaskPage` 后端实现；`:164-217` `overlayTaskCatalogRuntime`（第三写源，仅 jobs）
- `desktop/app.go:11531` — recorder 经 `taskcatalog.ObservedStore()` 写入
- `desktop/frontend/src/lib/useController.ts:120,1803-1808` — ToolProgress 内存态处理（`isSubagentProgressName`）
- `desktop/frontend/src/lib/taskCatalogTypes.ts:3-15` — TaskPage/TaskPageRequest/TaskCatalogItem wire 镜像；`types.ts:2263-2275` TaskSnapshot 镜像
- `desktop/frontend/src/components/TaskMonitorPanel.tsx:114,175,237-243` — 5s 轮询、ListTaskPage、事件流展开（`expanded` `:142`；现 616 行）
- `desktop/frontend/src/App.tsx:1195,4794-4800` — `tasksOpen` state 与 popover 唯一挂载点
- `desktop/frontend/src/components/MemoryPanel.tsx:440` — ResizableDrawer 停靠面板先例
- `desktop/frontend/src/components/ToolCard.tsx:24`（`subagentPhaseLabel`，`:22` SUBAGENT_TOOLS）、`Transcript.tsx:356,685` — 子代理嵌套渲染能力
- 本机 SQLite 实测：`%LOCALAPPDATA%\reasonix\task-catalog\v1.sqlite`（277 snapshots / 11 projects / 554 events）
