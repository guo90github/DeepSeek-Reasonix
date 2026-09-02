# Task 功能修复规划文档（Repair Plan）

- **文档状态**：v1.0（基于两轮深度调研整合）
- **适用范围**：Reasonix Task 功能生态 —— `internal/taskmonitor`、`internal/taskcatalog`、`internal/agent`（task / read_only_task / parallel_tasks）、`internal/jobs`、`internal/taskcontract`、`desktop/task_catalog.go`、前端 `TaskMonitorPanel` / `TaskTreeNode` / `TaskTreeView` / `TodoPanel`
- **目标**：消除成本失控、稳定性隐患、规划混乱、数据泄密与前端 UX 缺陷，建立可验证的迭代路径

---

## 1. 问题概述与背景

经过两轮代码级调研（第一轮：四大风险域 A/B/C/D；第二轮：数据增长与前端 UX 追加问题 N1–N13），共识别 **25 项问题**，可按严重度归为五类：

| 类别 | 问题编号 | 核心表述 |
|---|---|---|
| 成本失控 | A1–A5 | `parallel_tasks` 单次最多 64 个只读子代理 × 默认步数（父预算一半、下限 5 轮）= 单次至少 320 轮模型调用且无费用预算；`requeue` 无上限；后台任务跨回合存活；子代理写权限级联失败导致返工 |
| 稳定性 | B1–B5 | 任务目录为可丢弃投影（内存模式启动全量重建）；双读路径并存；recorder 尽力而为产生中间态；僵死判定 30s 盲窗；跨进程 CAS 冲突无自动对账 |
| 规划混乱 | C1–C4 | 写子代理 `task` 无规划门控；`parallel_tasks` 隐藏依赖顺序；plan todos / 任务树 / 交付契约三套体系互不相通；并行派发无统一契约 |
| 安全性 | D1–D4, N1–N3 | 写子代理缺省声明=整个工作区；任务数据明文落盘于项目目录且无全局 gitignore 保护；CLI 与桌面暴露策略分裂 |
| 前端 UX | N8–N13, N5–N6 | 5s 轮询整页替换导致"加载更多"分页每 5 秒回退；StaleCursor 静默截断；搜索双路径语义不一致且无防抖；计数误导；墓碑状态无 UI；单项目查询失败整页失败 |

同时确认了值得保留的既有设计：控制操作 `expectedVersion` CAS + `idempotencyKey` 幂等、跨进程锁 + snapshot 原子替换（tmp+fsync+rename）、投影 revision 绑定游标 + 墓碑机制、后台任务并发上限（`DefaultMaxParallelWriters`）。

---

## 2. 根因分析

### 2.1 成本失控
- **无费用预算体系**：子代理执行只受 `MaxSteps`（步数）约束，步数预算又是"父代理自己预算的一半"（模型自定）；`model`/`effort` 可逐任务覆盖升级档位，费用随档位翻倍。
- **乘法效应**：`parallel_tasks` 并行 × 每任务步数，缺少"单次调用总步数"聚合护栏（仅有 64 任务上限）。
- **重试无成本意识**：`requeue`（failed/stale 重跑）无次数限制与退避；后台任务（`run_in_background`）跨回合存活无时长告警。

### 2.2 稳定性
- **投影设计取舍**：catalog 为"可丢弃（disposable）投影"，SQLite 建于 CacheDir，`Shared()` 惰性打开；CacheDir 不可用退化为内存模式 → 每次启动全量重建，首帧必然 partial。
- **历史遗留双路径**：面板优先 catalog（`ListTaskPage`），未就绪时静默降级 `ListTasksForTab`（会话级）——两条路径内容、搜索、分页行为不一致。
- **文件存储非事务**：recorder 明确"尽力而为"，SaveTask 成功但事件落盘失败的组合留下"新状态 + 不完整审计日志"。

### 2.3 规划混乱
- **三套体系各自演进**：plan todos（`todo_write` → TodoPanel）、运行时任务树（taskmonitor）、交付契约（`taskcontract` obligation/acceptance）无联动契约——执行完成不推进计划。
- **并行工具信息隐藏**：`parallel_tasks` schema 刻意不暴露依赖/顺序（有测试钉死），任务树 `DependsOn/Position` 字段不接线。
- **门控缺失**：可写 `task` 无 PlanModeSafe，规划阶段仅 `read_only_task`/`parallel_tasks` 可用但可无契约并发。

### 2.4 安全性
- **存储位置选择**：任务数据放 `<projectRoot>/.reasonix/tasks/` 便于 CLI/Desktop/脚本多端共享，但 repo `.gitignore` 只覆盖 `/.reasonix/*`、`internal/**`、`desktop/**` —— 用户任意工作目录不受保护，含 `error_summary`/`session_id` 的数据可被意外 git 提交。
- **暴露边界不统一**：CLI 输出刻意 `contentFree` 抹掉 ErrorSummary（"cannot disclose paths or commands"），桌面面板完整展示同一字段。

### 2.5 数据增长
- **无保留策略**：全仓无 retention/prune/delete/archive；每任务 2 文件（`snapshot.json` + `events.jsonl`），事件日志只增不减；已删除会话/主题的任务成孤儿数据。

### 2.6 前端 UX
- **刷新模型与分页模型冲突**：5s 轮询以 `cursor=""` 全量替换首屏，与"加载更多"追加分页互斥——已加载页每 5 秒被丢弃。
- **条件降级放大不一致**：catalog 未就绪时整条路径切换，搜索字段、计数、分页语义全部漂移。

---

## 3. 详细修复步骤（分阶段、模块化）

> 每个步骤标注：**[改动文件]** 做法 → 验收点。阶段间无强依赖，可独立合入。

### 阶段 P0：数据安全与增长治理（最高优先，先止血）

**P0.1 任务数据移出项目树或全局 gitignore（N1）**
- [internal/taskmonitor/jsonstore.go, internal/control/controller.go:791, internal/cli/task.go] 方案 A（首选）：`FileStore` baseDir 改为项目外应用数据目录（`%APPDATA%/reasonix/tasks/<project-hash>`），项目内仅保留内容无关索引；方案 B（低改动）：在 `RegisterProject`/`OpenProject` 时向 `<projectRoot>/.git/info/exclude` 写入 `.reasonix/`，并随项目模板下发 `.gitignore`。
- 验收点：新项目目录下 `git status` 不出现 `.reasonix/`；既有任务数据可通过一次性迁移脚本搬移并校验 `events.jsonl` 序列连续性。

**P0.2 暴露策略对齐（N3）**
- [desktop/task_catalog.go, 前端 TaskTreeNode.tsx] 桌面面板对 `error_summary` 默认截断展示、点击展开，与 CLI `contentFreeTaskSnapshot` 的边界一致；catalog 搜索字段移除 `error_summary`。
- 验收点：CLI 与桌面在相同任务上展示同级别的敏感字段暴露。

**P0.3 保留/回收策略（N2）**
- [internal/taskmonitor（新增 PruneTasks）, desktop/task_catalog.go] 终态任务按数量上限（如保留最近 500）或保留时长归档（移动到 `.reasonix/tasks-archive/` 或直接删除）；topic/session 归档时按 `session_id` 前缀联动清理（`monitorTaskID` 已带 session 前缀，可精确匹配）。
- 验收点：达到上限后新增任务不触发投影异常；归档后 catalog 投影与文件系统一致（墓碑机制生效）。

**P0.4 events.jsonl 轮转/聚合（N7）**
- [internal/taskmonitor/jsonstore.go AppendAuditEvent] 事件文件超过阈值（如 1000 行）时压缩为聚合摘要事件（首/末/关键状态）；追加写补充 fsync。
- 验收点：长任务（数百事件）磁盘占用有界；`ListTaskEventPage` 返回的聚合事件可读。

### 阶段 P1：成本硬护栏

**P1.1 parallel_tasks 总量步数预算（A1）**
- [internal/agent/parallel_tasks.go Execute] 派发前计算 `Σ max_steps`，超过可配置上限（默认如 200 步/次调用）直接拒绝并提示拆分；经 `ListTaskPage` 状态或任务描述向面板暴露预估步数。
- 验收点：新增测试——超预算请求被拒且无任何子代理启动；预算内请求正常并行。

**P1.2 任务级费用估算与展示（A2）**
- [internal/agent/task.go RunProfileSpec, internal/taskmonitor/model.go] 子代理完成时复用 `billing.CostQuote` 计算用量，写入任务事件流（新增 `cost` 事件类型）；面板显示"已用步数/预估步数、已花费"。
- 验收点：任务详情可见费用；无 provider 定价时优雅降级（不显示）。

**P1.3 requeue 限次 + 退避（A3）**
- [internal/taskmonitor/model.go TaskSnapshot（新增 requeue_count）, desktop/task_catalog.go RequeueTaskByKey] 上限 2 次，超出后仅允许人工新建任务；重试间强制退避（30s / 5min）。
- 验收点：第三次 requeue 被拒且返回明确错误码；退避期间操作幂等（idempotencyKey 已具备）。

**P1.4 后台任务成本可见（A4）**
- [internal/jobs/jobs.go, 前端 TaskTreeNode.tsx] 面板展示后台任务已运行时长与步数；超过阈值（如 10 分钟）自动提醒；`CancelJob` 前提示已花费。
- 验收点：后台任务在面板可识别、可取消、有花费提示。

### 阶段 P2：前端体验修复

**P2.1 轮询改为增量合并（N8）**
- [前端 TaskMonitorPanel.tsx fetchTasks] 空游标轮询改为按 `__catalogKey` 合并（`setTasks(current => mergeByKey(current, decorated))`），保留"加载更多"累积页；或利用 `ListTaskPage` 的 revision 条件刷新（revision 未变则跳过）。
- 验收点：新增测试——模拟轮询两轮 + 加载更多，断言已加载页不丢失；滚动位置稳定。

**P2.2 StaleCursor 显式处理（N9）**
- [前端 TaskMonitorPanel.tsx] 收到 `StaleCursor` 时清空游标并显示"任务列表已更新，重新加载"提示 + 自动重取首页。
- 验收点：revision bump 后翻页不再静默截断。

**P2.3 搜索防抖 + 语义统一（N10）**
- [前端 TaskMonitorPanel.tsx, desktop/task_catalog.go] `query` 加 150ms debounce；两端搜索字段统一为 `task_id/session_id/error_code`（legacy 路径同步收窄，隐私利好）。
- 验收点：输入不逐键触发 Wails 调用；两端搜索结果一致。

**P2.4 计数修正（N11）**
- [前端 TaskMonitorPanel.tsx] count 改为显示 catalog `status.indexed/total`（或"已加载/总数"），不再用 `tasks.length`。
- 验收点：500 个任务时显示总数而非 50。

**P2.5 墓碑状态 UI（N5）**
- [internal/taskmonitor/model.go TaskState 扩展, 前端 TaskTreeNode.tsx] 投影墓碑（`health='missing'`）转 `removed`/`stale` 状态并呈现"任务已结束/被清理"；复用现有 `stateConfig`。
- 验收点：任务文件被外部删除后，面板在 30s grace 内显示过渡、之后显示终态。

**P2.6 查询降级（N6）**
- [desktop/task_catalog.go ListTaskPage] 未知 project key 时跳过该 key 而非整页失败，返回 `status.warnings` 由面板展示。
- 验收点：单项目异常不影响其他项目列表。

**P2.7 open 行为统一（N13）**
- [前端 TaskMonitorPanel.tsx controlTask, desktop/task_catalog.go] `scope==="session"` 的 `onOpenSession` 前端回调并入后端 `OpenTaskSessionByKey` 单一契约。
- 验收点：任何 scope 下"打开任务"行为一致。

**P2.8 tmux 收敛（N12）**
- [internal/cli/task.go] `/task tmux` 保留并标注 CLI-only；桌面不重复实现；文档明确两端口令差异。
- 验收点：无新桌面 tmux 依赖；CLI 行为不变。

### 阶段 P3：规划与一致性

**P3.1 写子代理规划门控（C1）**
- [internal/agent/task.go, internal/permission] 规划模式下 `task` 要求显式 `write_paths` 声明或先经 `approval`；缺省整工作区声明时显著告警（D1 联动）。
- 验收点：规划模式下的写子代理必须带声明或审批；`read_only_task`/`parallel_tasks` 不受影响。

**P3.2 parallel_tasks 可选依赖接线（C2）**
- [internal/agent/parallel_tasks.go schema, internal/taskmonitor/task_tree.go] schema 增加可选 `depends_on`（数组索引），派发时写入任务树 `DependsOn/Position`；缺省仍独立（不破坏既有隐藏顺序测试语义）。
- 验收点：依赖声明后任务树呈现父子/依赖关系；未声明时行为与现状一致。

**P3.3 todo → 任务单向映射（C3）**
- [internal/control（todo 管线）, internal/taskmonitor/recorder.go] `todo_write` 的步骤生成可派发任务描述；子代理 `complete_step`/终态回写对应 todo 进度——执行完成自动勾选计划项。
- 验收点：计划项与任务树联动，任务完成推进 todo；无双向循环依赖。

**P3.4 sessionless 任务 ID 稳定化（N4）**
- [internal/taskmonitor/recorder.go sessionlessMonitorTaskID] 改为 job 生命周期内稳定的派生 ID（jobID 稳定哈希），recorder 复用；投影层按 jobID+label 去重。
- 验收点：跨 recorder 实例同一 job 不产生重复任务。

### 阶段 P4：稳定性加固与测试补全

**P4.1 投影持久化 + 启动预热（B1）**
- [internal/taskcatalog] 从"disposable memory"改为文件投影 + 增量 reconcile（`projectiondb` 已有文件模式）；启动时与 tab 恢复并行预热；面板状态机区分 `opening → indexing → ready` 并展示 ETA。
- 验收点：二次启动首帧不再 partial；`Shared()` 快速返回就绪投影。

**P4.2 双读路径统一（B2）**
- [desktop/task_catalog.go, 前端 TaskMonitorPanel.tsx] 移除/统一 legacy `ListTasksForTab` 降级路径，面板只走 catalog。
- 验收点：无 catalog 依赖场景（测试/极简环境）行为明确定义，无静默语义漂移。

**P4.3 recorder 状态一致性（B3）**
- [internal/taskmonitor/recorder.go] SaveTask 成功后事件落盘失败时，补发一条 `state_persisted_without_audit` 补偿事件；或 RecordDone 重试事件写入。
- 验收点：注入事件写失败后，任务终态仍可审计（补偿事件存在）。

**P4.4 CAS 冲突对账提示（B5）**
- [desktop/task_catalog.go, 前端] 冲突错误（`ErrStoreVersionConflict`）时返回可读提示 + 自动重取最新快照，不静默失败。
- 验收点：并发 CLI/Desktop 写同一任务时桌面操作有明确反馈。

**P4.5 路径安全测试补全（D4）**
- [internal/taskmonitor/jsonstore_test.go] 补 taskID 路径穿越、symlink 链（`rejectSymlinkChain`）、父目录逃逸（`rejectStoreParents`）的针对性测试。
- 验收点：恶意 taskID/projectDir 无法逃出 baseDir；既有防护被测试钉死。

---

## 4. 风险评估与回滚方案

### 4.1 风险矩阵

| 风险 | 影响 | 概率 | 缓解 |
|---|---|---|---|
| P0.1 数据路径迁移破坏既有任务可见性 | 高（既有任务丢失观感） | 中 | 迁移脚本先校验（events 序列连续 + 快照指纹一致）再搬移；迁移失败保留原路径只读回退 |
| P0.3 保留策略误删用户任务 | 高（不可逆） | 低 | 先归档后删除（两段式 + grace 期）；默认上限可配置；文档明示策略 |
| P1.1 步数预算过紧误伤合法并行 | 中（功能可用性） | 中 | 默认值从宽（200 步），提供配置开关与明确错误提示 |
| P1.3 requeue 限次改变既有工作流 | 低 | 中 | 仅对"自动重跑"限次；人工确认路径保留 |
| P2.1 轮询合并引入重复/顺序问题 | 低（显示层） | 低 | 按 `__catalogKey` 去重合并 + 现有 `mergeByKey` 语义测试 |
| 存储格式变更（requeue_count、cost 事件） | 低（跨版本兼容） | 低 | `TaskSnapshot.SchemaVersion` 已存在，新增字段向后兼容读取 |
| 前端行为变更回归 | 中 | 中 | 每个改动附边界测试（见 §5），回归命令全绿后合入 |

### 4.2 回滚方案
- **文件级回滚**：所有改动均为独立文件变更，可逐项 `git revert`，无跨阶段耦合。
- **配置开关**：成本护栏（P1.1/P1.3）、保留策略（P0.3）、轮询合并（P2.1）均提供开关/阈值配置，紧急时一键回到旧行为。
- **数据可逆**：P0.1 迁移脚本支持反向恢复（搬回 + 校验）；归档（P0.3）在删除前有 grace 期可恢复。
- **版本兼容**：快照/事件模型变更遵循 `SchemaVersion` 递增，旧版本读取新字段忽略即可（Go JSON 宽松解码已具备）。

### 4.3 合入顺序
1. **P0**（数据止血）→ 2. **P1**（成本护栏）→ 3. **P2**（UX，每项独立可合）→ 4. **P3/P4**（规划与加固）。
每阶段独立合入、独立回归，避免大爆炸式变更。

---

## 5. 验证标准与测试用例

### 5.1 边界测试清单（映射到修复项）

| 修复项 | 测试用例 | 断言 |
|---|---|---|
| P0.1 | 项目目录 git 排除 / 存储迁移 | 新项目 `git status` 无 `.reasonix/`；迁移后任务可读、事件序列连续 |
| P0.2 | 桌面 vs CLI 暴露边界 | 同一任务两端口令展示同级别字段 |
| P0.3 | `PruneTasks` 上限/归档 | 超上限后归档目录存在、投影一致、不误删运行中任务 |
| P1.1 | parallel_tasks 步数预算 | Σmax_steps 超限被拒且零子代理启动；限内正常（沿用 `parallel_tasks_test.go` harness） |
| P1.3 | requeue 限次 | 第 3 次被拒、退避期幂等 |
| P2.1 | 轮询合并 | 两轮轮询 + 加载更多后已加载页不丢（`__tests__` 现有 harness） |
| P2.2 | StaleCursor | revision bump 后翻页有提示 + 自动重取 |
| P2.3 | 搜索防抖/语义 | 防抖生效（无逐键调用）；两端结果一致 |
| P3.2 | depends_on 接线 | 树呈现依赖；缺省行为与现状一致（既有隐藏顺序测试不破） |
| P3.3 | todo→任务联动 | 任务完成推进对应 todo；无循环 |
| P4.5 | 路径安全 | 穿越/符号链接/父目录逃逸全部被拒 |

### 5.2 回归命令
```bash
# Go 内核（task 生态）
go test ./internal/taskmonitor/ ./internal/taskcatalog/ -count=1
go test ./internal/agent/ -run 'Test.*(Task|Parallel|Subagent)' -count=1
go test ./internal/jobs/ ./internal/control/ -count=1

# 桌面（task_catalog 绑定 + 既有行为）
cd desktop && go test . -run 'Task|Catalog|HistorySlice' -count=1

# 前端
cd desktop/frontend && npx tsc --noEmit
npm test -- --run src/__tests__/task-monitor-*.test.tsx src/__tests__/todo-*.test.tsx  # 新增用例

# 提交前 CI 模拟（遵循 REASONIX.md）
gofmt -w . && go vet ./... && make lint
```

### 5.3 效果验收场景
- **启动**：二次启动打开任务面板首帧无 partial/Indexing 横幅（P4.1）；长任务列表"加载更多"在 5s 轮询后不丢失（P2.1）。
- **成本**：发起 100 个并行子代理被预算拒绝并提示拆分（P1.1）；失败任务 requeue 第 3 次被拒（P1.3）。
- **安全**：任意项目目录 git 仓库中无 `.reasonix/` 数据入库（P0.1）；桌面面板 error_summary 默认截断（P0.2）。
- **规划**：计划 todo 随子代理完成自动勾选（P3.3）；带依赖的 parallel_tasks 呈现依赖树（P3.2）。

---

*本计划基于两轮代码级调研的事实推导；所有问题编号（A1–A5/B1–B5/C1–C4/D1–D4/N1–N13）对应调研报告中的原始证据位置，改动前请复核对应源码。*
