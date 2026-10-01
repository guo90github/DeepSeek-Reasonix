# TODO — 多智能体集群落地执行清单

> 用途：**落地阶段的唯一执行清单**。每完成一项就在本文件里把 `[ ]` 改成 `[x]`，并在同一行末尾补证据
> （文件路径 / 命令 / 提交号）。条目**只给指针，不复制出口条件**——出口条件一律以
> `docs/agents/AGENT_BUS.md` §11（S1 的实现规格见 §11.1）为准，避免两份真相漂移。
> 规则：本目录（`docs/agents/`）是这批文档的落点；新增文档一律落在这里；推进中发现缺失的节点**追加到本文件**。

## T0 文档收敛（落地前）

- [x] T0-1 契约落盘并迁入本目录 → `docs/agents/AGENT_BUS.md`
- [x] T0-2 两份旧文档加状态块指向契约 → `docs/agents/multi-agent-collaboration-design.md`、`...-development-plan.md`
- [x] T0-3 提交文档基线 → `c414f6e55`（中文信息 + `Documentation-impact: updated - …`；纯文档）
- [ ] T0-4 确认是否需要 `*.zh-CN.md` 英文对照（`docs/COLLAB-SURFACE.md` 是中文单份先例，倾向不需要）

## T1 对抗评审（已跑一轮；结论已并入契约）

- [x] T1-1 一轮对抗评审（独立只读子智能体，任务是「挑错」而非补全）→ 结论：**不能直接开工**，须先补 S1 规格 + 消解范围冲突
- [x] T1-2 结论逐条并入契约（改判留痕）→ `docs/agents/AGENT_BUS.md` §11.1（S1 规格）、§11.2（评审留痕）、§10（3 处处置补正）、§跨工作区与 §9.3（3 处锚点修正）、§集群规模（单机边界）
- [ ] T1-3 第二轮对抗评审（**在 S1 实现完成、§11.1 有代码后**再跑一次，对着代码挑错而不是对着文字）

## T2 补规格（**必须在对应阶段开工前完成**；本轮评审未消解的三条）

- [ ] T2-1 视图/摘要规格：谁生成、多大、多久刷新、字段集 + **如何廉价物化「我的子树」**（O(N²) 闸的实际落点；评审指出 S2 成本模型无法验证）
- [ ] T2-2 「就绪」事件的发起者：完成者 / 宿主 / 编排者扫描——定死一个，否则退回轮询（S3/S5 接缝）
- [ ] T2-3 预算与既有旋钮对齐：`GoalTokenBudget`（`internal/config/config.go:1301`）/ spend budget / `REASONIX_SKIP_BUDGET` 谁统谁，超限后 Goal 变 `blocked` 还是 `complete`
- [ ] T2-4 **并发槽的持久队列落点**（评审新增）：槽满「排队而非失败」需要一个持久队列，而既有 inbox 是**会话级**且嵌套 fail-fast —— 定死排队落点后再做 S5
- [ ] T2-5 **能力的授权链**（本轮自查新增）：缺能力派生「获取能力」子节点时，谁批装依赖/改仓库（`CapabilityGrant` 交集 vs 新授权）

## T3 S1 黑板内核（可独立开工；不接线）

产物：`internal/agentbus/board`（op 日志 + 确定性 fold + 节点状态机 + 逐节点租约 + 心跳/回收）。
规格：`docs/agents/AGENT_BUS.md` **§11.1**（实现以它为准）；出口条件：§11 的 S1 行。逐条勾：

- [ ] T3-1 非法迁移拒收（带原因；不在 §11.1 迁移表内的组合）
- [ ] T3-2 op 重放幂等（同 `op.id` 重复提交不追加第二条）
- [ ] T3-3 并发 op 折叠结果与串行一致
- [ ] T3-4 杀掉推 op 的进程后，剩余 op 仍能折出完整状态
- [ ] T3-5 两个不同进程并发写同一作用域不丢 op（board 级 `internal/filelock`）
- [ ] T3-6 认领者被杀后节点在 deadline 内被回收并记 `no_progress`（同 `(node, deadline)` 幂等）
- [ ] T3-7 `revert` 后传递下游（谁依赖它）中的 `done` 正确标 `stale`
- [ ] T3-8 无证据的 `abandon` 请求被拒；无证据的 `decide(done)` 被拒
- [ ] T3-9 阶段闸门：`gofmt -w .` / `go vet ./...` / `make lint`（含 repolint，**不许放宽 baseline**）/ `go test ./internal/tool/builtin/ ./internal/boot/`
- [ ] T3-10 结构体按生命周期分组（`struct-state` 上限 12 个标量字段，`tools/repolint/structstate.go:13`）
- [ ] T3-11 `seq` 跨进程单调不重号，且 `<board>/#seq` 可作稳定地址
- [ ] T3-12 截断尾行（无结尾 `\n`）不计入 op；中段坏行跳过且**计数可见**
- [ ] T3-13 成环的 `split` / `require` 被拒
- [ ] T3-14 `agentbus` 的分层自证：不 import `internal/agent` / `internal/control`（`go list -deps` 或分层测试）

## T4 S2 写侧接线 + 读侧投影（含跨工作区与子树分片）

- [ ] T4-1 多会话可写同一 `board`
- [ ] T4-2 视图裁剪（每轮只读我的子树 + 我的节点 + 订阅摘要）
- [ ] T4-3 跨子树只经边界节点
- [ ] T4-4 守卫三：一轮 provider 请求里只含自己的视图（`internal/boot/effect_test.go` 模式）
- [ ] T4-5 读侧复用 `internal/taskcatalog` / `internal/taskmonitor` / 桌面任务树
- [ ] T4-6 `make frontend-check` 过
- [ ] T4-7 视图的增量读（cursor/seq）与「头部稳定、尾部追加」实测命中前缀缓存（评审指出该论证需落地才有意义）

## T5 S3 交流三档

- [ ] T5-1 自由对话 `say` + 话题边界（轮数/预算/静默窗口触顶即收口）
- [ ] T5-2 默认不灌上下文（只投点名与摘要）
- [ ] T5-3 有界点对点 `ask`/`answer` + 回执通道（`results/<correlation>.json`）
- [ ] T5-4 跨进程目标带令牌；超速返回 `rate_limited`
- [ ] T5-5 就绪即事件唤醒（`interval` 只兜底）

## T6 S4 审议与裁决

- [ ] T6-1 审议状态（参与者/轮次/必答/权重/冷却）
- [ ] T6-2 一次 `refute` 改变结论（可回放：同一 op log 折叠出不同结局）
- [ ] T6-3 等重升级给人；超升级配额自动降级 `undecided-by-rule` 并记账
- [ ] T6-4 弃答以 `no_answer` 可见
- [ ] T6-5 无可核对证据时 `done` 被拒
- [ ] T6-6 票数不改变权重（consensus ≠ evidence）

## T7 S5 集群调度与预算

- [ ] T7-1 **本机 host 级**并发槽 + 排队（槽满排队而非失败；跨机不在 v1）
- [ ] T7-2 四级预算（board → 子树 → 节点 → 回合）+ 触顶即暂停、现场保留
- [ ] T7-3 调度四则：关键路径优先 / 批量领取 / 同子树亲和 / 最难优先
- [ ] T7-4 预算只被验收节点消耗（做工不消耗总预算）

## T8 S6 观测聚合（人读）

- [ ] T8-1 按子树折叠、只显异常/争议/停滞/孤儿，可下钻
- [ ] T8-2 首屏不画 >N 张卡片（阈值可配）；孤儿与停滞必现

## T9 S7 e2e + 无人值守贯通 + 百级压测

- [ ] T9-1 杀掉编排者，未完成节点仍被认领推进
- [ ] T9-2 一次 `refute` 改变结果
- [ ] T9-3 运行时依赖：节点/边数由参与者新增
- [ ] T9-4 无人值守贯通：杀掉桌面进程 → 既有看门狗拉起 → 黑板继续被推进（`docs/UNATTENDED.md` §11 自认此链未真机验收，**第一次必须端到端**）
- [ ] T9-5 任务级落地：验收节点全 `done` + 无未决矛盾才宣告完成
- [ ] T9-6 N≈100：任意杀掉 20% 后仍收敛；不超预算；无 429 风暴
- [ ] T9-7 存在一次真实的「缺能力 → 派生获取能力节点 → 完成」链路

## T10 S8 PR 元数据门 + 打包

- [ ] T10-1 `Cache-impact` / `Cache-guard` / `System-prompt-review` / `Documentation-impact` 齐全
- [ ] T10-2 按本机 SOP 打包并 `verify-windows-portable.sh` exit 0

## 输入材料（本目录内，非清单项）

- `docs/agents/AGENT_BUS.md` — 契约（真相源，含 §11.1 S1 规格、§11.2 评审留痕）
- `docs/agents/multi-agent-collaboration-design.md` / `...-development-plan.md` — 被取代但保留继承项的旧稿
- `docs/agents/agent-evaluation-whitepaper-01-overview.md` — Agent 评测体系（外部方法论蒸馏），供「可核验」纪律对齐语言

## 不做（明确排除，防止漂移）

- 跨机协作（v1 单机；协议保留可换目标）
- 自由对话承载真相（对话是一等能力，但结论只在黑板上）
- 新守护进程 / 第四个前端（唤醒与常驻复用既有机制）
