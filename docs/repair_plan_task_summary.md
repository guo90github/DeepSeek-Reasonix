# 会话摘要：Reasonix Task 修复计划（docs/repair_plan_task.md）分阶段实施记录

> 本摘要基于一次长会话的技术决策、代码变更与遗留问题整理，供新会话作初始上下文。**所有"状态"均以磁盘实际为准，已在本会话末核验。**

## 1. 核心决策

- **总目标**：按 `docs/repair_plan_task.md`（25 项问题 A1–A5/B1–B5/C1–C4/D1–D4/N1–N13，P0–P4 五阶段）全阶段依次实施，每阶段独立回归。
- **用户使用场景（关键约束）**：完全自用、仅桌面端（Wails）、多数项目无 git 操作、代码不发布生产。此场景使部分按"多端共享/多人/CI"前提设计的修复项失去价值。
- **P0.1（N1 任务数据 git 排除）最终裁定：回退删除，不予保留。** 理由：任务数据含 `error_summary`/`session_id` 明文放项目树只在"会被 git 提交并推送"时构成问题；用户场景无此路径 → 改造无价值且增加复杂度。`EnsureTaskDataGitExcluded` 全套（helper + 5 测试 + 3 处挂载）已删除，`internal/control/controller.go` 与 `desktop/project_root_registration.go` 已回 HEAD 原状（P0.1 为其唯一改动）。
- **中途发生过一次误操作与纠正**：助手一度把"恢复"误读为"回退"又误重建过 `gitexclude.go/gitexclude_test.go`，用户纠正意图后两文件已删除；**代码不再改动**。
- **lint 立场**：用户自用不发布 → 不按生产门禁收敛 repolint 债务，也不运行 `repolint -update` 扩基线。
- **延后项（用户经 ask 确认下轮实施）**：P3.3（C3 todo→任务自动勾选）、P4.1（B1 投影持久化）、P4.2（B2 双读路径统一）——均系设计重/大改项，需独立回归窗口。

## 2. 代码变更状态

### 已删除（回退）

- **P0.1（N1）**：`internal/taskmonitor/gitexclude.go`、`gitexclude_test.go`；三处挂载点（`internal/control/controller.go` ~791、`internal/taskcatalog/catalog.go` RegisterProject、`desktop/project_root_registration.go` registerProjectRoot）全部移除。全仓 grep 无 `EnsureTaskDataGitExcluded`/`gitexclude`/`taskDataIgnorePattern` 残留。

### 已保留（实施完成并通过回归）

- **P0.2（N3）**：桌面面板 `error_summary` 默认 3 行截断 + 点击展开（`desktop/frontend/src/components/TaskTreeNode.tsx`、`styles.css`）；两端搜索字段统一 `task_id/session_id/error_code`。
- **P0.3（N2）**：`FileStore.PruneTasks`（`internal/taskmonitor/prune.go`）默认保留 500 终态任务、超限归档至 `<project>/.reasonix/tasks-archive`（移动不删除、同 ID 替换、跳过运行中、通知投影 sink）；`TaskRecorder.RecordDone` 后自动 prune；desktop 暴露 `PruneTasks` 绑定（`desktop/task_catalog.go`）。
- **P0.4（N7）**：events.jsonl 轮转——`events_rotate.go`（自 `jsonstore.go` 抽取以维持文件 <800 行）：≥1000 行压缩为首事件 + 尾 20 + `events_rotated` 标记；追加补 fsync；残缺尾行截断防拼接。`TaskEvent` 新增 `Detail` 字段。
- **P1.1（A1）**：`parallel_tasks` 派发前 `checkStepBudget`（`internal/agent/parallel_tasks.go`，默认 `DefaultParallelMaxStepsBudget=200`，`TaskTool.WithParallelMaxStepsBudget` 可配）；校验逻辑抽为 `parseAndValidate`。
- **P1.2（A2）**：cost 管线——`subagentProgressTracker` 捕获最后 Usage 事件与工具轮数（`subagent_progress.go`）→ `TaskTool.reportBackgroundUsage` 用父 `quoteContext` 计算 quote → `jobs.Manager.RecordTaskUsage`/`JobIDFromContext`（`internal/jobs/jobs.go`）→ `TaskRecorder.RecordUsage` 写 `cost` 事件 + 快照字段（`StepsUsed/StepsEstimated/CostTotal/CostStatus`，TaskSnapshot schema v3）；无定价降级 `unavailable`。仅覆盖后台任务。
- **P1.3（A3）**：`requeue` 上限 2 次 + 退避 30s/5min（`internal/taskmonitor/control.go`，抽为 `requeueGateResult`；错误码 `ErrTaskRequeueLimit`/`ErrTaskRequeueBackoff`）；TaskSnapshot 增 `RequeueCount`/`LastRequeueAt`。
- **P1.4（A4）**：前端 >10min 运行 ⚠ 告警（`taskLongRunning`）、停止确认带时长、步数/花费展示（三语 i18n）。
- **P2.1–P2.8（N8/N9/N10/N11/N5/N6/N13/N12）**：`TaskMonitorPanel.tsx` 轮询按 `__catalogKey` 增量合并；`StaleCursor` 提示+清游标+自动重取首页；搜索 150ms 防抖（抽为可测 hook `useDebouncedValue`）；计数 `已加载/总数`；**P2.5 模型扩展 `TaskStateRemoved`**（`model.go`，observer-only）+ catalog ListPage 对 `health='missing'` grace 窗口内返回 removed 态（`catalog.go` SELECT health + 覆盖 state，where 改为 `health IN ('ok','missing')`）；`Status.Warnings` 字段 + 未知 project key 跳过降级；open 统一经后端 `OpenTaskSessionByKey/ForTab` 解析后回调 `onOpenSession(tabID, sessionID)`（`App.tsx` 回调签名已改）；tmux CLI-only 文档化（`docs/CLI.md`、`docs/CLI.zh-CN.md`）。附带修复 `TaskMonitorPanel.test.tsx` 在 zh 机器因 Node `navigator.language` 的既有坏测试（固定 en-US）。
- **P3.1（C1）**：`TaskTool`/`FleetTool` 显式 `PlanModeSafe()=false`。
- **P3.2（C2）**：`parallel_tasks` schema 增可选 `depends_on`（0 基索引，越界/自引用校验拒绝）；`parallelDependsOnFor` 经 `TreeTaskID` 解析写入任务树 slot（`task_tree_ctx.go` withParallelChildSlot 增参）；执行仍并行（记录型依赖）。schema 既有"隐藏字段"测试已更新为新契约。
- **P3.4（N4）**：`sessionlessMonitorTaskID` 由随机 nonce 改 jobID 稳定哈希（跨 recorder 收敛同一行）。
- **P4.3（B3）**：`RecordDone` 审计事件 3 次有界重试（`appendAuditWithRetry`，`auditretry_test.go` 含瞬态/持久失败两测）。
- **P4.4（B5）**：前端 `task_version_conflict` 错误码 → 可读提示（三语 `summary.versionConflict`）+ 自动重取最新快照。
- **P4.5（D4）**：taskID 穿越变体矩阵（含 Windows 反斜杠、绝对路径）+ symlink 两级链测试（`jsonstore_test.go`）。

### 涉及的关键文件/路径

- Go 内核：`internal/taskmonitor/{jsonstore.go, events_rotate.go, prune.go, recorder.go, model.go, control.go}`（新增 `prune.go`/`events_rotate.go`/`auditretry_test.go`/`prune_test.go`/`rotate_test.go`/`recordusage_test.go`，gitexclude 已删）；`internal/taskcatalog/catalog.go`；`internal/agent/{parallel_tasks.go, task.go, fleet.go, subagent_progress.go, task_tree_ctx.go}`；`internal/jobs/jobs.go`。
- 桌面/前端：`desktop/task_catalog.go`（PruneTasks 绑定、Warnings 降级）、`desktop/project_root_registration.go`（已回 HEAD）、`desktop/frontend/src/components/{TaskMonitorPanel.tsx, TaskTreeNode.tsx, TaskMonitorPanel.test.tsx}`、`src/lib/{types.ts, taskCatalogTypes.ts}`、`src/App.tsx`、`src/locales/{en,zh,zh-TW}.ts`、`src/styles.css`、`src/__tests__/task-monitor-summary-truncation.test.tsx`。
- 文档：`docs/CLI.md`、`docs/CLI.zh-CN.md`（tmux CLI-only 说明）。

### 已知测试异常

- `desktop` 包 `TestCredentialProxyReconnectRegistersTrackedWorkspaces` 失败为**环境依赖**（本机 provider 凭据未配置，`cred_proxy_test.go`），与任何改动无关；同轮 `-run 'Task|Catalog'` 均绿。

### 验证基线（本会话末全绿）

- `go test ./internal/taskmonitor/ ./internal/taskcatalog/`
- `go test ./internal/agent/ ./internal/jobs/ ./internal/control/`
- `desktop go test . -run 'Task|Catalog'`
- `go vet`（五个包）；前端 `npx tsc --noEmit` + `TaskMonitorPanel.test.tsx`（28 项）+ `task-monitor-summary-truncation.test.tsx`（20 项）+ `task-monitor-navigation.test.ts`（3 项）
- `make lint`：golangci-lint 0 issues；repolint 有 **HEAD 即存在的基线漂移**（`internal/agent/session_events.go` 等非本会话文件）与少量新增代码余量——用户裁定不收敛。

## 3. 上下文关联提示

- **注意：P0.1 方案 B 的代码已确认删除回退，不应恢复**（用户明确：对自己无价值、增复杂度）。若新会话看到任何残留或相关建议，以删除态为准。
- 任务数据仍存于项目树 `<project>/.reasonix/tasks/`（`FileStore` baseDir 未动，方案 A 存储重构从未实施）；`AppendAuditEvent` 现带轮转/fsync。
- 遗留三件事需在后续会话推进（独立合入）：
  - **P3.3（C3）**——先在设计上确定 task 描述 ↔ seeded `step_id` 身份契约再挂 job 终态观察器（防与模型 `complete_step` 互踩，参照 `hasTodoUpdateSince` 纪律）；
  - **P4.1（B1）**——投影持久化可复用 `projectiondb` 文件模式 + 面板 `opening→indexing→ready` 状态机；
  - **P4.2（B2）**——移除 `ListTasksForTab` legacy 面板路径与 `hasTaskCatalogBinding` 分支，无 catalog 场景行为需明确定义。P2 已消除两路径语义漂移为其铺路。
- 可选未决想法（用户未拍板）：桌面启动时对 `loadProjectsFile()` 已注册项目重跑 git 排除——因 P0.1 已删，此想法作废。
- 用户环境杂项：`git status` 中 `desktop/wails.json` 的改动与 `stash@{0}`（"dev split-layout WIP"）为环境既有，非本会话产物，勿误动。
