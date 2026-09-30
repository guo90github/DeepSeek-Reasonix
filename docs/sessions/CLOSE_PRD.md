# 会话回顾功能产品需求文档（PRD）

> 版本：v2（基于当前工作树源码事实重写） · 落位：`docs/sessions/CLOSE_PRD.md`
> 本版所有"现状"陈述均可溯源到源码，事实索引见 §12。v1 中与源码冲突的现状判断已作废（见 §0）。

## 0. 本版相对 v1 的实质变化

| v1 的说法 | 源码事实 | 本版处置 |
| --- | --- | --- |
| 会话"只能靠翻原始会话文件逐条看"，原始文件"不可检索" | 已有完整检索子系统：SQLite FTS5/BM25 投影 + Agent `history` 工具 + 桌面项目树与回收站页（§12-1、§12-2） | §1 现状与问题重写 |
| 立项理由 = 新建检索能力 | 检索能力已在运行（本机投影 52.6 MB），真正缺的是 CLI 入口与会话级回顾 | 重心改为"补入口 + 建会话回顾层" |
| 检索入口 = 新做终端命令 | 能力已有，CLI 命令面无检索命令（`docs/CLI.md` 仅有维护入口 `catalogs reindex history`） | 保留 CLI 命令，定位于补入口（§4.3） |
| R1"关闭语义务必在 P0 确认" | 已有三种语义 + 两套挂钩机制（§12-4 至 §12-8） | **结案**，移入 §4.2 触发点表 |
| 一个会话 = 一个本地文件，可直接当"轨迹快照" | jsonl + 多个 sidecar + 追加式事件日志 + schema-2 DAG（head 可切换，非当前 head 不可检索）+ 多根目录 | §4.4 存储模型重写 |
| 新建"归档目录" | `ArchiveDir()` 已存在（**压缩历史归档**，compaction 产物，且已是检索读根之一） | 不新建归档概念；回顾正文放既有可弃投影（`cache/`），`ArchiveDir` 维持原语义（O1） |
| 健壮性靠自己设计（后台执行、pending 队列、启动补偿） | 已有"权威文件 + 可弃投影"模型、非阻塞持久化提示、quarantine/degraded read、`doctor catalogs`/`catalogs reindex` | 对齐既有模型（§4.5） |
| 脱敏要从零实现 | `internal/secrets` 已有 `Redact*` 家族与隐私安全读路径先例 | 改为复用 + 补模式（§4.2、§5） |
| 非目标"先验证关键词检索是否够用" | 关键词检索已上线并在服务真实数据，"验证"阶段已越过 | §2.2 非目标修正 |
| 未来方向"索引升级为 SQLite" | 现状已是 SQLite（FTS5） | 删除该条，改为排序/会话回顾层升级路线（§11） |
| §1.1"约 2000 个以内" | 本机实测 `<state root>` 下 `*.jsonl` 共 1002 个（含事件日志与 subagent 转录），且是多根布局 | §1.1 数字化并修正单根表述 |

## 1. 背景与问题

### 1.1 现状（实测）

- 会话以 `<state root>` 下的 JSONL 为主，**多根**存放：用户全局 `sessions/`、项目域 `projects/<slug>/sessions/`、`sessions/subagents/`，压缩历史归档 `archive/`。本机实测 `*.jsonl` 共 **1002** 个。
- 每个会话除主 `.jsonl` 外还有旁路 sidecar（`.context.json`、`.recovery.json`、`.pinned-context.json`）与追加式事件日志（`.events.jsonl`、`.turns.jsonl`、`.conflicts.jsonl`、`.guardian.jsonl`）。
- 会话可被 resume（续写同一文件）；schema-2 会话是 DAG，检索只覆盖**当前所选 head** 的派生 transcript。
- 已有多份可弃投影：`cache/history-search/`（本机 `v1.sqlite` = 52,649,984 字节）、`cache/session-catalog/`、`cache/task-catalog/`、`cache/usage-catalog/`。它们可删可重建，权威始终是 JSONL/事件日志。
- 检索已可达：Agent `history` 工具（`search`/`around`、project/global 双 scope、按 user_text/assistant_text/tool_input/tool_error 过滤）、桌面项目树与回收站页（与工具共用同一投影）。
- 既有会话目录（`session-catalog`）已记录会话/话题的轮次数、起止与最近活动时间、健康度、标题与标题来源；`list_sessions`/`read_session` 工具已提供清单、预览与隐私安全读视图。

### 1.2 核心问题（重新定义）

会话关闭后"做过什么就忘了"依然成立，但**原因不是不可检索**，而是三层缺口：

1. **入口缺口**：检索能力虽已存在（Agent `history` 工具），但**人的入口不可靠**——顶栏「历史」抽屉在代码层已无可达入口（未实机确认）、项目树的搜索是本地过滤而非全文检索、终端没有一条随手可用的命令。
2. **会话回顾缺口**：会话级"目标 / 关键动作 / 结论 / 待办"不存在。现有摘要都绑在别的生命周期上——会话内 compaction 摘要只服务上下文压缩，标题只服务列表扫读，都不回答"当时做了什么、结论是什么"。
3. **沉淀缺口**：跨会话的稳定结论没有稳定落点。`internal/memory`（`remember` 工具 + `MEMORY.md` 索引 + 自动召回）提供了机制，但依赖模型在会话中的判断与人工触发，缺少以会话为单位的沉淀输入。

### 1.3 问题本质

会话的价值没有被**收敛**成可扫读的一层。原始 JSONL 是过程记录：长、无结构、对人不友好；检索能定位到片段，却不回答"这次会话解决了什么"。本版本要补的是**会话级回顾层**，并把它的入口收成一条命令。

## 2. 目标与非目标

### 2.1 目标

| 编号 | 目标 | 衡量标准（口径可判定，见 §8） |
| --- | --- | --- |
| G1 | 会话关闭后自动生成四要素**会话回顾** | 有效会话的回顾覆盖率 ≥ 95%（"有效会话"定义见 §8） |
| G2 | 一条命令找回历史会话 | 终端 `reasonix history <关键词>` 返回含会话回顾的命中列表；目标会话 30 秒内定位 |
| G3 | 回顾生成对使用体验零干扰 | 关闭路径上无同步 LLM 调用；关闭操作无可感知延迟（< 100ms） |
| G4 | 存量会话可一次性补录 | 全量补录可中断可续跑，跑完给出 成功/跳过/失败 统计 |
| G5（新增） | 不改动权威数据与缓存面 | 权威会话文件格式、resume 行为、Agent 工具（provider 可见面）均不变；投影可删可重建 |

### 2.2 非目标（本版本明确不做）

- 不做多设备同步、团队共享。
- **不做语义/向量检索**（关键词检索已上线并在用，本版本只做入口收敛与会话回顾层；语义检索是 §11 的独立议题）。
- 不改动权威会话存储格式、sidecar 布局与 resume 行为；任何需要改权威格式的方案必须单独立项。
- **不新增 Agent 工具**（检索工具已存在，新增会改写 provider 可见的工具清单，破坏系统提示前缀缓存；见 §7）。
- 会话回顾不自动注入模型上下文；跨会话记忆的自动注入由既有 `internal/memory` 机制负责，与本功能解耦。

## 3. 用户与场景

### 3.1 用户

单一用户：我自己（工具的主要使用者与开发者）。

### 3.2 核心场景

**场景 A：找回"上次怎么做"（两条路）**

> 路 1（新增，终端）：输入 `reasonix history 连接池`，返回命中会话的标题 + 四要素会话回顾 + 日期；选中展开完整回顾，再进一步打开原始会话（`read_session` 式隐私安全视图）。
> 路 2（已有，会话内）：直接对 Agent 说"上次那个连接池报错是怎么解的"，由 `history` 工具检索。

**场景 B：日常无感生成**

> 用户正常使用、正常 `/new` 或关掉标签页，不做任何额外操作。会话回顾在后台生成；唯一感知是下次检索时它已经在了。

**场景 C：存量补录**

> 功能上线后跑一次全量补录命令，历史会话一次性入库；中途中断可续跑，已生成回顾的自动跳过。

**场景 D（反场景，明确要避免）**

> 回顾抓错重点；把纯寒暄/误开的会话也强行生成回顾（浪费）；回顾失败但用户不知情（静默丢数据）；关闭会话变卡；为了生成回顾去改权威会话文件或写坏 resume。

## 4. 产品方案

### 4.1 功能总览

```
┌──────────────────────────────────────────────────────┐
│  会话回顾（本版：回顾层 + 入口收敛）                    │
│                                                      │
│  ① 回顾层（会话关闭时生成）                            │
│  ② 检索入口（复用既有检索，新增 CLI 命令）              │
│  ③ 存量补录（复用既有重建/游标机制）                    │
│  ④ 健壮性保障（对齐既有"权威文件 + 可弃投影"模型）        │
│                                                      │
│  已有、本版不动：BM25/FTS5 投影、Agent history 工具、   │
│  侧栏项目树与回收站页、session-catalog、memory          │
└──────────────────────────────────────────────────────┘
```

### 4.2 功能 ①：会话回顾层（关闭 / 轮转时生成）

**触发点（v1 的 R1 在此结案）**

会话生命周期已有三种语义，且两套挂钩机制都已存在：`SessionEnd` hook 事件（reason = `clear` / `other`，不可阻塞）与 extension 拦截点 `session.end` / `session.rotate`（阶段 End / Rotate）。

| 用户动作 | 代码路径 | 生命周期事件 | 本版处置 |
| --- | --- | --- | --- |
| `/new` | `Controller.NewSession()` | 旧会话 Snapshot → `session.rotate` → `SessionEnd("clear")` → 新文件 `session.start` | **生成回顾**（旧会话保留在盘上） |
| `/clear` | 清空会话路径 | 销毁旧工件（`MarkCleanupPending(old,"clear")`）→ `session.rotate` → `SessionEnd("clear")` | **不生成回顾**（丢弃语义；销毁后也无内容可取） |
| 关闭标签页 / 进程退出 | `Controller.Close()` | `SessionEnd("other")` + `session.end`（`closeOnce` 幂等，只触发一次） | **生成回顾**（文件仍保留） |
| 换模型/换 profile 重建 controller | `ReleaseResources()` | **不触发** SessionEnd（同一逻辑会话） | 不生成（正确行为） |
| 进程被中途杀掉 | 无事件 | 只能靠启动补偿（§4.5 自愈） | 启动扫尾生成 |
| 归档到回收站（话题级 / 单会话） | `App.TrashTopic` / `App.DeleteSession` → `closeRemovedSessionRuntime` → `ctrl.Close()` | **会触发** `SessionEnd("other")` + `session.end`（话题归档路径先 `ctrl.SetSessionPath("")` 再 Close） | **不生成回顾**（丢弃语义），护栏见下 |
| 彻底删除（清空回收站 / 清理恢复副本） | `trashSessionArtifacts` / `deleteRecoveryCopy` | 无回顾语义 | **不生成回顾** |

约束与结论：

- 触发点挂在 `SessionEnd` / `session.end` / `session.rotate` 上，**不在这些钩子里做同步 LLM 调用**（钩子执行的是任意 shell，同步生成回顾会直接违反 G3）。投递方式沿用既有非阻塞范式：权威 commit 且释放文件锁之后再发提示，后台消费（§4.5）。
- **幂等键 = 会话路径 + 内容指纹**（轮次数或 transcript 哈希），不是"关闭次数"——resume 会让同一路径反复触发，指纹变化才重新生成回顾。
- **丢弃类动作一律不生成回顾**：`/clear`、归档到回收站（话题级 / 单会话）、彻底删除。护栏：触发点先判"该会话仍可见且未被移除"——`IsVisibleSession`（排除 `.cleanup-pending`）+ 移除窗口用 `IsDestroyingSession` 排除；`SessionPath()` 为空直接跳过（话题归档路径会先 `SetSessionPath("")` 再 `Close()`）。参照：`BeginDestroySession` 只做 job teardown、**不发** SessionEnd。
- 因此**回收站里的会话多数没有回顾**（回顾只在正常关闭时产生）；§10 的 O7"撤下"只覆盖"先正常关闭、随后才归档"的情形，实际工作量极小。

**回顾记录的内容（分层：能不新建的不新建）**

| 字段 | 来源 | 是否新增 |
| --- | --- | --- |
| 会话 ID / 路径 / 轮次数 / 起止与最近活动时间 / 标题与标题来源 | 既有 `session-catalog` 的 topic/session 行 | 复用，不重复存储 |
| 候选条目：事实 / 根因—修法 / 否证结论 / 交接与未解坑（四类，每类 0–N 条） | LLM 生成 | **新增** |
| 回顾的生成时间、模型、提示词版本、内容指纹 | 生成过程 | **新增** |
| 关联：原始会话路径（+ DAG 的 head 标识） | 既有会话路径 | 复用 |

**存放位置（O1 已定）**

回顾正文放**可弃投影**（`cache/` 下的投影，随 reindex 一起维护、可删可重建），**不放 `ArchiveDir()`**：那个目录是 **compaction 的压缩历史归档**（与 `cache/session-catalog`、`cache/task-catalog`、`cache/usage-catalog` 同类，只是它恰好也是检索的读根之一），语义不同。这样"删除投影 = 完全回滚"，与 §4.5 的可回滚承诺一致。

**回顾生成规则**

- 走既有 LLM 通道与限界原语（`boundedllm`），**但不沿用标题的回退契约**：标题失败可以回退"首条消息预览"（那是列表展示层的事），回顾**没有可用回退**，因此失败一律记为「待补」并重试，绝不落盘半成品（§4.5 不丢数据）。
- 输出固定四类候选结构（事实 / 根因—修法 / 否证结论 / 交接与未解坑），降低跑偏概率；**每条都是候选**，采纳后才写入记忆（2026-09-30 修订：回顾从"给人读的四要素文档"改为"给下一个会话复用的经验蒸馏"）。**采纳/改/弃已落地**：回顾页逐条给 采纳 / 改后采纳 / 弃，采纳写进**当前项目**的记忆（事实与根因→`project`，否证→`feedback`），弃按内容哈希记住、该条不再重新生成，撤销可反悔。**交接与未解坑不进记忆**（不是经验类产出），走**独立通道**：回顾页把它「登记为未完成项」，项**属于该项目并长期挂着**直到标为已处理（第 1 个会话产出、第 N 个会话才接着干是常态，故不做"只交给下一个会话"）；投递需**同时**满足三个闸门——**同项目 ∧ 话题命中 ∧ 未处理**，且只在**会话第一轮**或该轮出现**续接意图**（"接着/继续/上次/未完成"）时才提示；提示只报"看起来是这件事，要不要继续"，绝不派活；判据是**本地匹配**（一个标识符、或两处中文短片段，都只是普通重合；需**三个标识符**，或**一个标识符加两处片段**，或**三处片段**），零模型调用；阈值 2026-09-30 用真实会话校准过两轮（13 条真笔记 × 598 条同项目无关开场白：误放行率 39.5% → 15.7% → 13.2%，召回 9/13 不变；实测天花板是"阈值到不了 ≤5%"——再严只剩 3/13 命中；复跑法与实测表见 QA 手册 §1 第 7 步）。**交付分两种形态**：**未完成项**当**提问**（上面那套闸门），**早前结论**当**背景**——同一批闸门、同一份投影（`PriorNotes` 只取 `refuted` / `root-cause`、**只取既未采纳也未弃**的条目，最多 2 条），注入块明说"这是你没要的背景、可能过时或错、以你现在看到的为准、不要回答它"。敢这么放的依据：校准已量出判据到不了 ≤5%，而背景形态下一句错话只花一行上下文，不会把活派错人（QA 手册第 10 步）。另有**本项目未完成项**常驻列表，防"没命中 = 静默丢了"；**自动提示只看最近 30 天内登记的项，且只权衡该项目最近 20 条**（更旧的仍在列表里、页面标注「太久没动 · 不再自动提示」，撤销后重新登记即回到可提示态）——这是防"列表只增不减 → 越攒越容易误提"的衰减闸，2026-09-30 与用户定稿。**超长会话不丢中段**：超预算时头 3/5、尾 2/5 原样保留，中段压成"每轮一行"（`## User (turn N)` 标记 + 该轮首句，≤80 字，总预算 ≤ 输入的 1/5），并在提示里标出省略与剩余轮数——原来只写「N bytes omitted」，把"顺带得出的结论"整段丢了。生成失败的可见性与手动生成见 §4.2 之后的落地记录（QA 手册第 8/9 步）。**条目带指针、也带落点提议，但落点仍由人决定**：每条候选可给 `refs`（`path` / `command` / `test` / `turn` 区间 / `config`，最多 4 条，复制自会话原文）与一个 `scope` 提议（`project` / `base` / `generic` + 一句理由）——**指针进记忆**（采纳后事实里带 `**Pointers:**`，让下一个会话能去核对而不是信一段话），**落点只展示不生效**：写记忆**只在人点「采纳」时发生**（内核 `internal/recap` 不 import `internal/memory`，全链路唯一写入口是 `desktop/recap_review.go` 的 `AcceptRecapEntry`），且**一律写当前项目**（`MemoryScopeFor` 已定义并测试，但**有意无人调用**，等一个显式开关）。**落点提议有可核对的依据**：`Recurrences` 用**与投递判据同一套证据与阈值**（两个标识符或三处片段）在投影里数"哪些桶也得出过同一结论"——**≥2 个桶独立得出 ⇒ 才可能是通用级**，一个项目独有的结论再怎么自称"in general"也不成立；这层是纯统计、零模型，页面把它读成「另有 N 个项目独立得出同一结论：…」。**条目可以变成可执行工件**：`root-cause` / `refuted` 两类（**只有procedure**——事实不是流程、未完成项是提醒）在页面上给「**起草为 skill**」，宿主动手写进**当前项目**的 `.reasonix/skills/<name>/SKILL.md`，**绝不覆盖**已存在的文件；草稿 frontmatter 写 `invocation: manual`，所以**它不会自己跑**——只有你读它、改它、把那一行改成 `auto` 之后，模型才可能取用。这是"越用越智能"的闭环入口：沉淀 → 采纳/起草 → 变成下一会话的默认行为（QA 手册第 11 步）。
- 一条可复用结论都榨不出来的会话，输出**空条目**并照常落盘（幂等：不再重复调用模型），页面显示「本次会话没有可沉淀的结论」。
- 短会话（少于 2 轮实质交互，纯寒暄、误开）**不生成回顾**，标记"已跳过"，避免浪费。
- **§4.3 端到端实测（2026-09-30，第一次有答案层的数字）**：语料=投影里 3 条可投递笔记（2 `root-cause` + 1 `refuted`），4 道题（含 1 道同项目无关题当反向对照），每题两组：**有沉淀**（投影副本）vs **无沉淀**（空 cache home），共 8 次 `reasonix run -p`（`deepseek-flash`），总计 **$0.052**（约 ¥0.37），单次基础开销 ≈6.9k prompt token。**投递层（本地、零模型）**：3 道定向题都算出命中、对照题算出 0 —— 端到端误放行 **0/1**，机制是通的。**答案层**：**只有 1/3 用上了**——`:only-child` 那题把笔记的结论原样说出来（"不数文本节点…误判为整段加粗" + 渲染期插件），而 `WorkspaceGitStatsForTab` 那题**拿到了笔记却按函数名另起炉灶**列了 5 类猜测、通篇没说"从不填充 Files"，`refuted` 那题直接回"无背景记忆"并给通用清单；**无沉淀组 3/3 全部答"证据不足、答不了"**，所以差异确实来自沉淀。**结论**：瓶颈不在"送不到"（已证送到），在"送到了不被用"；下一个旋钮是**背景块的措辞**（现在写的是"你没要的背景、不要回答它"，对 `refuted` 这种自成一体的结论显然被当成了噪音）与**条目正文先给结论**的写法。而 `--dir` 只决定**文件/命令工具的根**——两者不是一回事。⚠️ **这一轮 `--dir` 指向了空目录（2026-09-30 用户指出）**：模型**看不到代码、无法核对**笔记，所以「无沉淀组 3/3 答不了」有很大成分是"空工作区"噪声，不是"这个事实推不出来"；而且 `:only-child` 那题的"整段加粗"字样**本就出现在我的提问里**（判据泄漏），∴ 真实可信的痕迹只剩"**渲染期插件**"一处。结论方向不变（送到了、用得少），但**数字必须在修正 `--dir` 为当前项目后重测**。**2026-09-30 重测（`--dir` 已改为当前项目，8 组全部 exit 0，总花费 $0.0518）：3 道题全部作废**——A 两组都答对（repo + `git show 0a45842ca` 就够）、C 两组都答出同一结论（`docs/research/…§9.5` 里写着）、B 更糟：**基线组用 `git log -S'only-child'` 直接否掉了题面前提**（真正的旧选择器是 `:first-child`，`:only-child` 只活在注释与文档措辞里），而**有沉淀组照抄了笔记里那个不精确的措辞** ⇒ 这一轮沉淀**没有任何可归因的正收益**，反倒把模型锚在一个次准确的细节上（`Refs`/`Evidence` 都指不出这一层）。**判据因此改写**：题库的判据不是"答案在不在沉淀里"，而是"**答案在不在模型够得着的地方**"（repo / docs / git 历史都算够得着）——先用 `git grep` 预筛（本机筛查脚本：`Temp/recap-eval/screen-notes.py`，13 条里 12 条可复原），但**唯一可靠判据仍是基线组**：预筛会漏（C 那条 grep 搜不到，基线却从 docs 里答了出来）。下一版题库必须取自**只存在于会话里的事实**：用户的否证与判断（未写进 docs 的）、走过的失败路径、环境与操作事实、临时决策——这才是沉淀不可替代的价值区。**2026-09-30 第三轮（按上一句重出题：3 条会话独有笔记种进 `cache-eval` 投影，7 组 exit 0，$0.0491）结果分化**：**Q1 事故那题命中**——"v1–v3 旧记录怎么丢的"：有沉淀组说出**这次事故的成因**（校准探针把变量名写成 `RECAP_CACHE_HOME`，`DefaultPath()` 落回 live 投影并在其上跑了 v4 迁移，且 `RECAP_CALIB_ROOT` 已给、skip 拦不住），基线组只挖出**机制**（v4 是 `DROP TABLE` 重建式迁移、`Open()` 即迁移、`Prune`/`Delete`）并自己承认"本机这一次是哪条路径没继续查"，还给出了**反向定性**（"这是设计决策而非事故"）⇒ 沉淀在这里确实补上了模型没拿到的那一环。**Q2 凭据题作废**：两组给出**同样正确**的答案（key 只从 `<REASONIX_HOME>/.env` 取、不读 `os.Getenv`，连 `ResolveAPIKeyFromProcessEnvForProbe` 的注释都引了）——代码注释里写着，够得着。**Q3 版本/操作事实作废**：两组都答对，因为**读工具并不受 `--dir` 限制**（两组都读了 `Reasonix-portable\current.json`、包内 `build.json`，基线甚至读了真实 state home 的 `shell.log`）⇒ **磁盘上读得到的状态不算"会话独有"**。反向对照仍干净（0/1）。**这轮给出的真实定位**：通道机制成立且能命中（投递 3/3、对照 0、一处真命中），但**能用上它的场域比预想窄**——只存在于会话里、且 repo/docs/git/磁盘四处都拿不到的事实（用户自己的判断与否证、走过的死路、事故成因）。注意那一处命中的事实**其实也躺在修复提交的代码注释里**（`calibrate_test.go` 的 "a mistyped env var once resolved to the live cache…"），基线只是没去读 ⇒ 沉淀的价值形态更像"**指路 + 省一次搜索**"，不是"提供模型拿不到的真理"。**产品取向因此明确**：生成器的价值取向应是①**会话独有**（宁可少写，也不复述代码/文档）、②**措辞精度可核**（够不着 refs 的措辞要降级成不确定），③**运营事实别当根因写**（它们该走"采纳进记忆"，因为磁盘可读、T1 排不上用场）。**已按这三条落地为提示词 v8**（`PromptVersion = "recap-v8"`；老记录按新版本重生成），并顺手修掉一个会把整条路线架空的问题：`redactEntries` 过去只带 `Kind/Body/Evidence`，**`Refs` 与 `Scope` 在入库前就被丢掉** —— v7 起要求的指针与落点提议因此**从未真正落盘**（页面指针行、「采纳后带 Pointers」都拿不到数据）；现在 refs/scope 一并过脱敏入库，`MaxSystemBytes` 也从 4 KiB 提到 6 KiB（v8 提示词 3.7 KB，原上限只剩 300 字节余量）。复跑配方见 QA 手册。
- **T3 跨项目周期聚合已落地（2026-09-30）**：把 `Recurrences` 从"页面上的一句话"升成一份**报表**——内核 `Store.Insights(since, limit)` 取**近 30 天内 ≥2 个项目各自得出**的结论；同一结论被两个项目用不同措辞写下时，**同证据（共享标识符 ≥2）的副本折叠成一条**（两地说两话是常态，报表该说"什么复现了"而不是"被说过几次"），按最近出现倒序、上限 20；宿主 `ListRecapInsights` 出站前再过一次 `secrets.Redact`；回顾页顶部渲染「跨项目复现的结论」段，每条写明**哪几个项目**得出，所以这句断言是可核对的而不是自称的。**零模型调用**：判据与投递/落点提议同一套阈值。这是愿景"多维度沉淀"的第一个可见产物：项目内结论 → 跨项目复现 → 才谈得上通用级。 **T2 的第二遍读法已并入同一份报表（2026-09-30）**：同一个结论如果被**同一项目的 ≥2 条记录**独立得出（`occurrences ≥ 2`），也进这份报表——「在一个代码库里反复回来」与「在别处也出现」是两种同等分量的证据，而前者是单项目仓库唯一拿得到的那一种。这一层仍是**零模型调用的确定性读取**；真正的「模型第二遍洞察合成」（跨条目写一段综述）**仍未做**，本轮只把它需要的输入备齐了。
- **三级落点真正生效（2026-09-30，默认关闭）**：此前 `MemoryScopeFor` 只是"定义了没人调用"的规则、落点**只展示不生效**（一律写当前项目）。现在设置页「常规」多了一个显式开关 **「让回顾条目自己决定分档」**（`[desktop] session_recap_tier`，默认 **false**）：关闭时行为与原来逐字相同；打开后，采纳一条被提议为 `base`/`generic` 的条目会写成**全局记忆**（`internal/memory` 只有 `project`/`global` 两档，`MemoryScopeFor` 正是把这两级映到 `global`），并在事实正文里写明「(accepted into global memory)」——落点从提示变成可核对的陈述。**为什么默认关**：分档是模型的提议，只有看过依据的人（回顾页会显示"另有 N 个项目独立得出同一结论"）才该让它决定；这条与"写记忆只在人点采纳时发生"是同一条纪律的延伸。
- **未完成项的结局回流（2026-09-30）**：此前只有 登记/处理/撤销——「这条最后怎么解决的」无处可去，闭环缺最后一环。现在「标记已处理」先给一个**可选**的一行输入（占位「结局（可选）：这条最后是怎么解决的？」）再确认：写了就把它写进**项目记忆**（`recapOutcomeFact`：结局正文 + `**Item:**` 原条目 + `Evidence:` 出处），下一个会话从记忆里就能读到；**不经过模型**——那句话是人自己打的，等于把「人工放行」直接落成沉淀。没写结局则与旧行为逐字相同（仍然关闭，不发明文本）。Best effort：写记忆失败只记 `slog.Warn`，不回滚已经关掉的项。
- 超长会话分段处理，回顾不失真。
- 同一 transcript 只生成一次回顾：与 compaction 摘要、memory 事实的分工要在提示词与触发条件上写清（§10 决策记录 O2）。
- **恢复副本（`<id>-recovery-<16 位 hex>`）不生成回顾**（S2 已定）：它是同一对话的副本，生成等于重复花 LLM 钱。
- 回顾页的对象 = **已生成回顾的在档会话**；进行中的会话不会出现（设计使然，不是缺陷）。
- **回顾的存在性**：回顾只在"会话结束但 transcript 仍保留"时产生（`/new` 轮转、关标签页 / 退出 App、存量补录）；会话被销毁（`/clear`）或移入回收站时不产生。这是 O7"撤下"只影响少数情形的根本原因。

**模型与调用形态（O8，已定）**

先破一个反直觉前提：**"延续会话"并不等于"拿到完整上下文"**。会话内 provider 看到的是 `<id>.context.json` 里的 **model-visible projection**（`internal/store/session.go` 注释：投影归投影、**权威仍在事件日志**），而超预算的早期内容已被 compaction **折成摘要挤出提示**（`internal/agent/compact_fold_input.go`：`foldToSummary` / `foldSummary{Mode, Spans, FoldTokens}`）。因此延续会话拿到的是"当时被折叠后的视角"；**独立读权威转录反而更完整**。真正要取舍的不是"上下文 vs 干净"，而是"**成本 vs 保真**"。

三个可独立取舍的旋钮：

| 旋钮 | 取值 | 推荐默认 |
| --- | --- | --- |
| **上下文来源** | 权威（主 `.jsonl` + 事件日志）/ 会话投影（`<id>.context.json`）/ 两者混用 | **权威**；会话投影只作辅助（可拿已生成的 compaction digest 当"该段的二手输入"省钱，但须标明非权威） |
| **模型** | 该会话记录的模型（`agent.LoadSessionModel`，离线可读、不要求话题打开）/ 配置的**会话回顾模型**（`SessionRecapModel`，见下） / 组合 | **会话模型优先 → 配置回退 → 跳过待补** |
| **预算** | 全量分段 / 头尾优先 / 二级流水线 | **头尾优先**（首段定目标、末段定结论与待办，中段抽检关键工具调用） |

超长会话的分段能力**仓库已有实现可抄**：`internal/agent/session_extract.go` 的 fragment 机制（"fragment %d/%d of an over-length session …" 逐段压成 durable briefing）+ `compact_fold_input.go` 的 digest 原语与 token 记账（`FoldTokens` / `Spans`）。因此"独立模型要读完整个会话"不是新负担。

硬约束（与档位无关）：**独立 one-shot，走 `boundedllm.Call`；不写入会话转录、不进入会话上下文、不影响系统提示前缀缓存**。理由：触发点即会话结束（此时多数已无活跃 controller）、回顾是派生投影绝不能反写权威、resume 会反复触发（写回会话会破坏幂等）、cache-first 要求前缀字节稳定。

其余同款护栏：使用**独立用量来源**（拟名 `UsageSourceSessionRecap`，类比 `event.UsageSourceTitle`；正式名字随实现定）；回顾元数据记 模型 / 提示词版本 / 内容指纹 / 生成时间；限额 = 超时 + 最大输出 token + `MaxOutputBytes` + 低思考档（`PreferredReasoning(prov, "low")`）。

先例对照：桌面 AI 改名是「会话模型 + 要求话题已打开」（`AIRenameSession`：`session is not open; open it before using AI rename`）；`serve.generateTitle` 是「专用轻量 provider」。回顾 = 前者的模型优先序 + 后者的**离线可解析**形态。

**回顾通道隔离（硬约束；用户 2026-09 追加）**

回顾的调用路径**不得与会话抢占或共用路径，不得影响会话链路**。逐条验收点：

1. **独立 provider 实例**：不复用会话 controller 的 provider/client（先例 = `serve.generateTitle` 注入 `s.titleProv`）。共享实例会带出请求作用域的尝试计数、用量 sink、流式缓冲。
2. **不碰会话的准入与租约**：不走会话/标签的 turn admission，不取会话租约（`workspacelease`）与 session 写锁；触发点在 `SessionEnd` 之后，绝不与在飞回合重叠。
3. **自带并发闸门（已落地）**：已核实 `internal/boundedllm` **只提供**超时 / `MaxTokens` / `MaxOutputBytes`，**没有并发控制** ⇒ 回顾通道自建了单飞闸门（同一时刻只一个调用），并带**让路上限**（超预算就记 `pending("session busy")` 让出车道，绝不无限自旋）。**回顾调用不再自设 token / 字节上限（2026-09-30，用户要求"一劳永逸"）**：长会话是常态，而推理模型一旦把补全预算花在思考上就会**一个字都不返回**——这既是「生成失败：unparseable answer」的真身，也是"限制越大越容易踩"的根因。现在车道**不设自己的上限**：`MaxTokens = -1`（`boundedllm` 中负数 = 不设限，转发 0，让**模型自己的输出预算**当上限）、`MaxOutputBytes = -1`（不再中途掐断流），仍然由车道的 `Timeout`（默认 90s）与单飞闸门兜住失控。`boundedllm` 的 `0` 语义未变（仍取默认值），所以其它调用方行为逐字不变。**让路判定已接线（2026-09-30）**：先前以为"boot 拿不到有会话在跑的视图"，其实视图就在手边——`bindRecapLane` 本来就收着 `ctrl *control.Controller`，而 `(*Controller).Running()` 已存在（`internal/control/turn_finishing_boundary.go:24`）。现在该谓词就是 `ctrl.Running()`，无需新增跨层管道；内核侧也补上了此前完全缺失的覆盖：`TestTheLaneYieldsToARunningSessionAndGivesUpAtItsBudget`（Busy 恒真 + 小让路预算 ⇒ `Skipped/"session busy"` 且**留下 pending 标记**，Busy 转假后同一车道立刻放行）。通道隔离仍靠"独立 provider + 独立用量来源 + 不取会话锁"成立。
4. **不共享账目**：独立用量来源 `event.UsageSourceSessionRecap`（已落地），与会话用量、标题用量分开统计。
5. **失败隔离**：回顾超时或失败不得影响会话结束时序、不得阻塞关闭路径；失败只进自己的重试 / 待补队列（§4.5）。
6. **可观测**：回顾通道的排队 / 超时 / 失败可单独统计（`doctor` / catalog 输出可分辨），不进会话用量。
7. **完全旁路权威**：不写会话转录、不改系统提示前缀（重申）。

**实现现状（2026-09）**：上述验收点的落地形态与"批量关闭"场景的加固：

- **车道是进程级**（按投影路径键控），与 controller 生命周期解绑：桌面"关闭标签页 / 关闭其他 / 关闭右侧 / 关闭全部"是**同一入口的 N 次调用**（`TabOverviewMenu.tsx` 逐标签循环同一个 `closeTab` → `App.CloseTab`），所以车道不能随单个 controller 消亡。
- **关闭不等模型**：`Runner` 分开 `Stop()`（放弃队列 + 取消在飞，关闭路径用）与 `Close()`（排空，批处理/补录用）。
- **溢出不静默**：队列满时写 `pending("queue full")`，由补录/巡检捡回。
- **投影不常驻打开**：用一次开一次（否则缓存目录/临时目录会被锁住）。
- **关闭必须先让结束事件带上路径（真机首验暴露并已修的缺陷）**：桌面关标签原本"先 `SetSessionPath("")` 再 `Close()`"（目的是让后续快照变 no-op），于是 `close()` 用 `c.SessionPath()` 拿到的**是空串**，而回顾通道丢弃空路径 ⇒ 桌面关标签**从不生成回顾**（投影里 0 记录 0 待补，页面自然空）。修法：`desktop/tabs.go` 两处普通关闭改为"先 `Close()` 再清空路径"（行数中性；返回前路径仍为空，`TestCloseTabNoResurrectionFromAutosave` 的两条断言不变）；丢弃类动作（`/clear`、新建会话轮转）改由 boot 侧按 reason=`clear` 拦下；删除/归档因文件被移走而取不到指纹、天然跳过。回归测试：`desktop/tabs_session_end_test.go` 断言关标签时结束事件带上该会话路径。
- **剩余待接线**：`Busy` 让路判定（见上第 3 条）、关闭路径"同步 LLM 调用 = 0"的计数式断言（现由耗时上界代理）。
- **恢复的会话也必须上报（真机第二次失败暴露并已修的缺陷）**：`close()` 的门是 `if fireSessionEnd && started`，而 `started = c.startedOnce` 只在本进程跑过回合（或 NewSession/ClearSession）后置位。于是"启动 App → 恢复会话 → 直接关标签"这条路上整块结束事件被跳过 ⇒ **一次模型调用都不会发生**（用量投影里从未出现 `session-recap`，这正是当时的定位仪表）。修法（`c2f6c67aa`）：把结束序列收进 `session_end_observer.go` 的 `closeSessionEndHooks(started)` —— 面向用户的 hooks 与 extension 事件仍按 `started` 把关（它们与 SessionStart 配对），**结束观察者只按 `fireSessionEnd` 把关**；controller.go 调用点由 5 行变 3 行，未放宽 repolint 棘轮。
- **决策留痕（`recap_activity`，投影 schema v2）**：车道在每个决策点写一行 —— `submit` 的 `rejected: empty path | discarded | already queued | queue full`，`generate` 的 `stored` / `skip: <原因>` / `error: …`。判据：**没有任何 `generate` 行 = 关闭没上报结束事件**（分离或旧包缺陷）；`submit + rejected: …` = 上报了但被车道丢；`skip: …` = 跑过但跳过。两次真机失败缺的正是这条仪表（当时只能靠推理 + 用量投影反推）。
- **提示词 `recap-v2`（由真实失真样例驱动）**：首条真机回顾把**被否掉的候选名**（「会话档案」）写成定案、把决策区间写成 `O1–O6`（正文里 `O1–O9` 连写出现 38 次）。v2 明确：名称/编号/路径/版本串**照抄原文**、**只写最终结论**（讨论中被否的候选不算决策）、转录被裁剪时**宁缺勿编**。提示词版本升级会让旧记录在下次关闭时自动刷新（指纹 + 版本双重幂等）。
- **用量可见性（U1a 已落地；计价 U3 未做）**：回顾调用的用途标签写在当日 `stats/<日期>.jsonl`（`"usage_source":"session-recap"`），而用量投影（rollup 目录）**丢掉了 `usage_source`** ⇒ 为不动投影 schema、不做强制重建，新增 `stats.QueryPurposes`（`internal/stats/purposes.go`）**直接读每日 stats 文件**（每行 ≤ 一次请求，开销可忽略），由 `App.UsageStats` 并入 `UsageStatsRange.Purposes`，面板底部渲染「按用途」分区（新组件 `PurposeTokenUsageSection.tsx`）。该分区与"入口点"筛选正交：`SourceFilter.Source` 仍只按入口点过滤。**仍未做（U3）**：回顾那次调用报价为 `no_price`（`display_status: unavailable`）⇒ 面板能显示它的 token 占比，但**费用不参与**；计价要动共享的 `boundedllm`（调用侧没有传定价）与定价装配，单独评估。
- **分离运行时不会被自动回收（未修，产品语义待拍板）**：`detachedSessions` 里的运行时只有"关闭项目工作区"或"退出 App"会 `Close()`，没有"作业跑完即关闭"的路径 ⇒ 在活跃时被关掉的标签基本等不到回顾，只能靠存量补录（`catalogs reindex session-recap`）。可选修法：a) 工作跑完即关闭该运行时；b) 启动时对"已结束但无回顾"的会话自动补录。
- **转录读取的快路径（2026-09-30 落地，含实测成本模型）**：实测 6.4 MB 会话一次读取的构成是 **DAG 重放 ~500 ms + 渲染 ~0**（早期以为渲染是瓶颈，实测推翻）；直读兼容转录（`.jsonl`）只要 **~50 ms**。因此 `FileTranscript.Read` 先试直读，但**只在"侧车摘要能证明它"时采用**：`.meta` 的 `content_digest` 必须等于"直读行按 `messageForSessionIdentity` 规范化后重算的摘要"（新增 `agent.TranscriptDigest` / `agent.SessionContentDigest`）。其余一律回退重放：无侧车（368 个 archive/global 老会话）、摘要不符（24 个）、无行、解析失败、**含 pinned context revision**（唯一可能被重放折成内容的行）。语料核对：492 个会话渲染文本**逐字节一致 492/492**；在用 project 会话里 87/111 可被摘要证明。实测效果：可证明会话 146–178 ms → **30–36 ms（~5×）**；不可证明会话把"不可证明"按 `路径+size+mtime` 记进进程内缓存，重复关闭只多 **+18 ms**（首次多付一次解析 ~260 ms）。
- **直读的现场自证（shadow check）**：每进程前 2 次快读仍跑一次重放对账，不一致则**采用重放文本**并把 `fastpath` 行写进 `recap_activity`（`verified` / `mismatch: took the replay` / `authoritative read failed`）⇒ 部署后的构建自己证明自己的快路径，无需依赖离线语料。
- **转录读取的快路径 · 阶段 2（2026-09-30 落地）**：上一次关闭已经读过的部分不再重读。投影新增 `recap_resume` 表（`resume_offset` / `prefix_hash` / `content_digest` / `head_text` / `user_turns`，migration v3）记住"读到哪、头部文本是什么"，`FileTranscript.ReadCovered`（新接口 `CoveringReader`，`from.Offset == 0` 表示整读）只解析**新增行**，生成器再把保存的头部与新增文本合并、按原口径裁剪（合并处用不带字数的省略标记：中间那段是上一次读丢的，给不出诚实字数）。增量读**无法**重算整文件的侧车摘要——那正是它便宜的原因——因此只接受三个更便宜的证据：**①** 文件前 `resume_offset` 字节的哈希仍等于 `prefix_hash`（实测 6 MB ≈ 11 ms）；**②** 侧车摘要相对 `content_digest` **已推进**（保存会更新摘要，所以"只追加行却没保存"退回整读）；**③** 新增行可解析。任一不成立即整段回退（先试一次整读重建基线，再退到重放），影子校验继续对最终文本与重放对账。收益：大会话从阶段 1 的 ~380 ms 再降到 **~50 ms**（前缀哈希 ~11 ms + 增量解析 ~40 ms）。等价性与两道拒绝证明见 `internal/recap/resume_test.go`，通道是否真走覆盖读见同文件的效果测试。

- **回顾页面对大量记录（2026-09-30 落地）**：页面此前把 `<ul>` 直接挂在 `ManagementPageShell` 的 `management-screen__content` 下，而后者是 `overflow: hidden` ⇒ **超出一屏的行被裁掉且滚不动**（回收站/历史页用的是既有 `.history-list { overflow-y: auto }`，本页缺这个容器）⇒ 已补：列表改挂在 `.history-list` 容器里（`flex: 1; min-height: 0`）。同时补上"可扫读"能力：每张卡显示**会话标题**（用 `ListSessions` 的元数据按 `path` 关联，缺元数据时退回文件名）、轮次数、本地化的生成时间，并提供**打开会话**（走既有的 `navigation.onResumeSession` ⇒ 定位到该会话并离开本页）；工具条加**搜索**（标题 + 四要素 + 模型，纯前端过滤）与**排序**（最新在前 / 最旧在前 / 按会话名）。有意保留的取舍：取数仍是"一次拉全量"（无 `LIMIT`/游标、无虚拟滚动，数据量真上千再上，属过早优化）；`recap_records` 行只增不减、`recap_resume.head_text` 每会话留一份头部（实测 9–48 KB）都还没设保留策略。

**验收（P1）**：构造"会话正在跑（有在飞回合）＋ 同时把另一会话移入回收站"的场景，断言会话侧的请求时序、延迟、用量均无变化（用 §8「体验干扰」的口径）；并对回顾通道加一条守卫测试，证明它不持有会话租约、不经过会话准入。

**配置项命名定案**：**「会话回顾模型」（en `Session recap model` / zh-TW `工作階段回顧模型`）**，配置键 `session_recap_model`；代码侧照既有 `WebSearchModel` 模式命名（`config.SessionRecapModel()` / `ResolveSessionRecapModel` / `SaveSessionRecapModelTo`、桌面 `SetSessionRecapModel`，沿用其 admission/locking/rebuild 生命周期）。

**命名标准（本版起全篇适用）**：**名字要自带对象，不能只给动作或属性**。据此：「摘要模型」弃用（与 compaction 的"摘要"撞概念、不指明对象、不直白）；单独的「回顾模型」也不采用（"回顾"一个词，读者不知回顾什么）。备选（未采纳）：「后台回顾模型」「回顾生成模型」。

**脱敏要求**

复用既有 `internal/secrets`：`Redact`、`RedactCredentials`、`RedactMessage`、`RedactError`（覆盖 api_key / access_key / private_key / secret / token / password / pwd 等命名模式）。落盘前对回顾与任何派生文本执行一次脱敏；**禁止**把未脱敏文本写进回顾投影或日志。

**已完成（2026-09）**：`internal/secrets` 已补齐**内网地址类**模式（私有 IPv4 段、`localhost`/`127.0.0.1`/`::1`、`.local|.internal|.corp|.lan`、指向上述主机的 http(s) URL），并对公网地址写了负例与幂等测试；回顾落盘前经 `Redact` 脱敏已按此生效。

### 4.3 功能 ②：检索入口

**入口三形态（两个已有，一个新增）**

| 形态 | 现状 | 本版动作 |
| --- | --- | --- |
| Agent `history` 工具（search / around） | 已有，provider 可见面已定型 | **不动**（见 G5：新增工具会改前缀缓存） |
| 桌面检索面（现状） | **顶栏「历史」抽屉在代码层已无可达入口（未实机确认）**；当前入口 = 侧栏项目树（浏览）+ 回收站页（管理） | 本版新增**独立管理页「会话回顾」**，只承载**会话回顾**的查阅与检索（与回收站解耦，**不合并入口**）。落地后的入口有**两个**：侧栏的「会话回顾」按钮（`SidebarRegion`），以及**命令面板里的同名命令**（`usePaletteCommands` 的 `cmd-recap`，与「自动化」「回收站」同组、同 chip 形态，点击即 `openPage({ kind: "recap" })`） |
| CLI 命令 | **缺** | **新增** `reasonix history <关键词>`（非交互与 TUI 两态） |

**关键约束：复用同一份检索投影，不建第二个索引。** 关键词检索的排序与边界行为已定型并已在用，本版不改：FTS5 + BM25、英文小写/代码符号与 CJK 重叠 bigram、默认搜索 user_text/assistant_text/tool_input/tool_error（普通工具输出默认排除）、命中后从**权威源**回读 snippet 与上下文。新增的只是"会话回顾如何参与展示与检索"，已定见 §10 决策记录 O4。

**CLI 命令行为**

1. `reasonix history <关键词>` → 命中列表（标题 + 会话回顾核心字段 + 日期），相关度优先、同分按时间倒序。
2. 选中某条 → 展开完整四要素回顾。
3. 再进一步 → 打开原始会话（沿用 `read_session` 的隐私安全视图口径：截断、去 reasoning、去 system prompt）。

**边界行为**

- 搜不到时明确"无相关记录"，不返回弱相关结果凑数（沿用既有相关度下限语义）。
- 大小写不敏感的模糊匹配（既有索引行为）。
- 与既有维护命令区分命名：检索命令是用户入口，`reasonix catalogs reindex history` 是维护入口，二者不可混同。
- 会话回顾尚未生成时，命中列表照常返回（显示"尚无回顾"），检索不因回顾缺失而阻塞（沿用投影"边建边用"的既有行为）。
- **恢复副本（`-recovery-<hex>`）从检索命中中排除**（S2 已定）：检索侧当前不认识该命名，同一对话会被主会话与副本重复命中——本版顺手修掉（只动投影侧 `visiblePath`）。

### 4.4 功能 ③：存量补录

**扫描范围（v1 的单根假设作废）**

| 维度 | 实际取值 | 补录口径 |
| --- | --- | --- |
| 根目录 | 用户全局 `sessions/`、项目域 `projects/<slug>/sessions/`、`sessions/subagents/`、`archive/` | 全部纳入；subagent 转录不纳入（O5 已定） |
| 文件类型 | 主 `.jsonl` + 4 类事件日志 + 多个 sidecar | **回顾的读取来源 = 权威**（主 `.jsonl` **与**事件日志，见 O8）；事件日志不单独成篇 |
| 会话格式 | legacy 与 schema-2 DAG | DAG 只对**当前所选 head** 的 transcript 生成回顾，并记录 head 标识；head 切换后按"重写"处理（与既有投影同语义） |

**要求**

- 可中断、可续跑（已生成回顾的、指纹未变的自动跳过）——复用既有重建/游标与队列机制，不另写扫描器。
- 输出进度与结果统计（成功 / 跳过 / 失败）。
- 脏数据统计**定义为既有 catalog 的输出**（会话健康度 / 恢复状态），不再临时写统计脚本；清单用于反哺工具侧修复。

### 4.5 功能 ④：健壮性保障

本功能不自造一致性模型，直接对齐仓库既有的"**权威文件 + 可弃投影**"模型：

| 保障项 | 既有实现范式（复用） | 本功能增量 |
| --- | --- | --- |
| 零干扰 | 持久化成功后发非阻塞、按路径合并的提示，且在释放文件锁之后；CLI 侧把 SessionEnd 收尾刻意推迟到 `tea.Cmd`，不阻塞界面 | 回顾生成走后台队列，钩子内不做 LLM 调用 |
| 不丢数据 | LLM 失败不影响权威会话保存；投影可从权威源重建 | 回顾失败只影响回顾本身，重试 + 留底待补 |
| 自愈 | 启动补偿：既有 reconcile / repair 机制对投影做补偿与修复 | 未完成的回顾任务进同一补偿路径 |
| 可回滚 | 删除投影/缓存即回滚；权威文件不动，旧版本仍读同一份权威文件 | 删除回顾投影即完全回滚 |
| 可观测 | `reasonix doctor catalogs [--json]`、`reasonix catalogs reindex history`；诊断不打印查询、token、片段、消息、工具参数 | 统计里增加 回顾有效/跳过/失败 三态 |

**硬约束**：回顾投影属于可弃数据，任何情况下不得成为"唯一副本"；关闭路径不得等待 LLM。

### 4.6 状态口径（回顾页 · 回收站 · 归档）

本版必须与既有的"归档 = 软删除"口径对齐，否则会出现两个名字指同一批数据。

| 用户可见词 | 底层事实 | 位置 |
| --- | --- | --- |
| 「归档到回收站」/「归档对话」/「归档所有聊天」 | 就是移入回收站（软删除）；Go 侧方法名即 trash | `desktop/frontend/src/locales/zh.ts`（`projectTree.archive*`）；`desktop/topic_archive.go`（`App.TrashTopic` → `markTopicArchiveCleanupPending`） |
| 回收站（软删除） | 会话文件仍在盘上，但打了 `<id>.cleanup-pending.json` 标记 | `internal/store/session.go`（`SessionCleanupPending`） |
| 该状态的定义 | "隐藏，等待延迟清理" | `internal/agent/save.go`（`IsCleanupPending`） |
| 该状态是否可见 / 可检索 | 否：不出现在正常列表 / 恢复 / 检索面 | `internal/agent/save.go`（`IsVisibleSession` 注释）；`internal/history/search.go`（`visiblePath`） |
| 恢复 / 彻底删除 | 恢复 = 清标记；彻底删除才删全部 sidecar（含权威事件日志） | `internal/agent/save.go`（`ClearCleanupPending`）；`internal/store/session.go`（`SessionSidecarFiles`） |

**由此锁定的口径（2026-09 修正：对象分离）**：

1. **对象分离**：回顾页只覆盖**在档会话的会话回顾**（会话关闭 / 轮转时生成的一条派生记录）；回收站覆盖**会话本身**（软删除 → 恢复 / 彻底删除 / 清空），两者入口独立、**不合并**。
2. **"已归档" = "已移入回收站"**（同一批数据），因此回顾页**不设**"已归档 / 回收站"筛选态——那属于回收站页。
3. **会话回顾随会话可见性**：会话进入回收站 → 其回顾从回顾页**撤下**；恢复 → 重新纳入（指纹未变则复用、不重算）；彻底删除 → 回顾一并清理。这保证"回顾页看得见的 = `history` 工具搜得到的"（同以 `IsVisibleSession` 为准）。**注意**：回收站里的会话**多数没有回顾**（回顾只在正常关闭时产生，见 §4.2「回顾的存在性」），本条的"撤下"只覆盖"先正常关闭、随后才归档"的情形；且丢弃类动作（`/clear`、归档到回收站、彻底删除）**不生成**回顾。**本条已接线（2026-09-30）**：三半都核实过——①「进回收站→撤下」由 `ListSessionRecaps` 的 `agent.IsVisibleSession` 过滤承担（`desktop/session_recap.go:124`），不是靠删记录；②「恢复→重新纳入」因此天然成立（记录从未被删，指纹与 `PromptVersion` 未变时生成器照旧跳过、不重算）；③「**彻底删除→一并清理**」此前确实缺接线（`internal/recap` 的 `Delete` 在 `desktop/`、`internal/boot/` 没有任何调用点），现已在**唯一永久删除路径** `App.purgeTrashedSession`（`PurgeTrashedSession` 与 `PurgeRecoveryCopy` 都汇入它）里，于会话文件真正删除成功之后调用 `deleteSessionRecap`——不等按龄 180 天清理，因为那行描述的会话再也打不开了。
4. **回顾页无破坏性操作**：恢复 / 彻底删除 / 清空都只在回收站页，沿用其既有确认流程（确认框初始焦点在取消；批量删除逐条继续并报总数）。
5. **恢复副本**：不生成回顾，且从 FTS 命中中排除（S2 已定）；它仍可在项目树里以「可恢复」形式被看到（既有行为，不变）。
6. **进行中的会话不出现**在回顾页（回顾只在关闭 / 轮转时生成）；设计使然，不是缺陷。

## 5. 数据与隐私

- 会话回顾与检索投影仅存本地，不上传任何远端。
- 脱敏为默认强制行为，无开关（宁可回顾缺信息，不可泄漏密钥）；复用 `internal/secrets.Redact*`，并补齐内网地址类模式后方可认为脱敏达标。
- 回顾展示沿用 `read_session` 的隐私安全口径（截断、去 reasoning、去 system prompt、工具结果 opt-in），避免回顾成为绕过隐私约束的新通道。
- 原始会话文件所有权不变：回顾系统只是"旁路读者 + 派生投影者"，不写、不改、不删权威文件（`/clear` 的销毁是既有行为，与本功能无关）。

## 6. 既有能力覆盖矩阵

| PRD 需求 | 既有实现 | 状态 | 本版动作 |
| --- | --- | --- | --- |
| 关键词检索（BM25/FTS5、CJK bigram、模糊、大小写、相关度排序） | `internal/history` + `internal/historycatalog` | 已有 | 复用，不改排序语义 |
| Agent 检索工具（search/around、双 scope、kind 过滤） | Agent `history` 工具 | 已有 | 不动（provider 可见面保持） |
| 桌面检索入口 | 侧栏项目树（浏览）+ 回收站页（管理）；顶栏「历史」抽屉无可达入口 | 已有 | 新增独立管理页「会话回顾」（会话回顾查阅 + 检索，与回收站解耦） |
| CLI 检索入口 | 无 | **缺** | 新增 `reasonix history` |
| 会话清单/预览/标题/轮次/健康度 | `session-catalog` + `list_sessions`/`read_session` | 已有 | 回顾记录直接引用，不重复存储 |
| LLM 短文本生成（标题） | `serve.generateTitle`（失败回退预览） | 已有先例 | 回顾复用同通道，**但不复用标题的"回退到预览"契约**（§4.2） |
| 会话内摘要（compaction） | `internal/agent/compact*`、`SummarizeFrom/UpTo` | 已有（生命周期绑定） | 明确分工，同一 transcript 不重复生成回顾 |
| 跨会话沉淀 | `internal/memory`（`remember` / `MEMORY.md` / 自动召回） | 已有 | 本版不重复；回顾可作其输入（未来） |
| 脱敏 | `internal/secrets.Redact*` | 部分（凭证类） | 复用 + 补内网地址类模式 |
| 隐私安全读路径 | `read_session`（截断/去 reasoning/去 system prompt） | 已有 | 回顾展示沿用同等约束 |
| 非阻塞持久化提示 | 持久化观察者（`history.PersistObserver`） | 已有范式 | 回顾生成沿用 |
| 投影诊断与重建 | `reasonix doctor catalogs`、`reasonix catalogs reindex history` | 已有 | 统计扩为三态 |
| 补录游标 / 可续跑 | 重建与协调队列 | 已有范式 | 复用，不新写扫描器 |
| 压缩历史归档目录 | `config.ArchiveDir()`（**compaction 产物**，也是检索读根之一） | 已有 | **不用于存放回顾**（O1 已定：放可弃投影） |

## 7. 受影响层与前端

- **行为落在内核 `control.Controller`**，不写进任何前端：CLI TUI、桌面、serve、ACP、bot 五端共享（仓库约定的分层要求）。**但回顾器另需一条不依赖活跃 controller 的离线路径**（模型解析用 `agent.LoadSessionModel`、内容读权威文件）——否则"会话已关闭"就做不了（O8）。
- 新增包职责：回顾生成器与它的投影；复用既有投影数据库策略（WAL、`synchronous=NORMAL`、私有权限、短 busy timeout、远端或不可用缓存目录回退内存、迁移失败 quarantine 后重建、更高 schema 版本降级只读）。
- **缓存面不变**：系统提示前缀（base prompt + 工具清单 + 记忆）必须字节稳定。本版不新增 Agent 工具、不改工具描述与 schema ⇒ provider 可见前缀无变化。
- 前端落点：CLI（新命令，TUI + 非交互）；桌面新增**独立管理页「会话回顾」**（只读查阅会话回顾 + 检索；与回收站、项目树入口各自独立），与「回收站」「自动化」「设置」同一管理页外壳。serve / ACP / bot 与手机端均**不暴露**检索（O6 已定）。

## 8. 成功指标

上线一个月后验收。口径必须可判定，判定源固定为既有命令输出：

| 指标 | 目标 | 判定源 |
| --- | --- | --- |
| 会话回顾覆盖率 | 有效会话中 ≥ 95%。**「有效会话」定义**：`IsVisibleSession` ∧ 非恢复副本 ∧ ≥ 2 轮实质交互（即分母排除"跳过"类） | 回顾投影统计 vs 会话目录统计，二者均可从 `doctor catalogs --json` 类输出取 |
| 会话回顾质量 | 抽查 20 条，≥ 80% 能凭回顾看懂"当时做了什么" | 人工抽查记录 |
| 找回效率 | "上次那个 XX 的事"类需求，30 秒内定位 | 人工计时（终端命令路径） |
| 静默失败数 | 0（月末对账：权威会话数 − 跳过数 ≤ 回顾有效数） | 同上统计；差额即静默失败 |
| 体验干扰 | 0（关闭路径无同步 LLM 调用，关闭无可感知延迟） | 关闭耗时观测 + 钩子内无 LLM 调用（代码约束） |

## 9. 里程碑

| 阶段 | 交付物 | 工期 | 验收方式 |
| --- | --- | --- | --- |
| P0 | **结案报告 + 覆盖矩阵**：把 §6 逐行核对到代码位置，产出"已有 / 部分 / 缺"清单与缺口设计（触发点、幂等键、存放位置、脱敏模式清单） | 0.5 天 | 触发语义不再有开放项（本版已结案）；覆盖矩阵每行有代码位置；决策记录 O1–O9 全部具结 |
| P1 | 会话回顾层闭环 + 存量补录 | 2 天 | 连续使用 3 天无静默丢失；补录跑完给出三态统计；关闭路径无同步 LLM |
| P2 | `reasonix history` 检索命令 + 桌面「会话回顾」管理页（独立页，不合并回收站） | 1 天 | 3 个月前任意一次会话 30 秒内找回；回顾缺失时不阻塞 |
| P3 | 会话回顾质量调优（提示词迭代） | 1 天 | 抽查质量达标；与 compaction/memory 分工无重复生成 |

## 10. 风险与开放问题

| 编号 | 风险/问题 | 状态 | 应对 |
| --- | --- | --- | --- |
| R1 | "关闭会话"语义不等于会话终结 | **已结案**（§4.2，代码内明确） | 幂等键用 path + 内容指纹 |
| R2 | 会话格式混杂；schema-2 DAG 与旧构建互读失败（已有实证：出现全量"session history could not be loaded safely"） | **已知，非待确认** | 解析器防御式设计；DAG 只取当前 head；脏数据用 catalog 健康度/恢复状态呈现 |
| R3 | 回顾抓错重点 | 已知 | 结构化四要素；P3 提示词迭代；与 compaction 摘要对齐口径 |
| R4 | 回顾任务丢失（进程中途被杀） | 已设计兜底 | 复用启动补偿（reconcile/repair）+ 月末对账 |
| R5 | 敏感信息进回顾 | 部分兜底 | 复用 `secrets.Redact*` + 落盘前扫描；**内网地址类模式待补** |
| R6（新） | 关闭路径被回顾生成阻塞（违反 G3） | 已识别 | 钩子内禁同步 LLM；非阻塞提示 + 后台队列 |
| R7（新） | 回顾与 compaction/memory 重复劳动（同一条 transcript 被反复生成） | 已识别 | 回顾锚定内容指纹；分工写进提示词与触发条件 |
| R8（新） | 粒度歧义：回顾页（会话回顾）与回收站（整会话）若混用会歧义 | 已识别 | 对象与入口分离；回顾页副标题写明“这里只有会话关闭时生成的回顾，会话的删除/恢复在回收站”；`docs/MANAGEMENT_PAGES.md` 的 Trash 一节保持独立描述 |
| R9（新） | 恢复副本进 FTS 导致同一对话重复命中（既有行为，非本功能引入） | 已知 | S2 已定：副本不生成回顾 + 从检索命中排除（投影侧 `visiblePath` 改动） |

**决策记录（O1–O9）**

| 编号 | 问题 | 结论 |
| --- | --- | --- |
| O1 | 会话回顾与四要素数据放哪 | **已定**：回顾正文放可弃投影（`cache/`，可删可重建、随 reindex 一起维护）；**不放入** `archive/`（那是 compaction 的压缩历史归档） |
| O2 | 与 compaction 摘要、memory 事实的分工边界 | **已定**：三方互斥——会话级四要素走本功能；compaction 只管上下文压缩；memory 只收稳定偏好/决策 |
| O3 | 桌面是否新建"归档"页面 | **已定（2026-09 修正）**：新建**独立管理页「会话回顾」**，对象 = **会话回顾**；回收站（对象 = 整会话）保持**独立入口**、不再合并；页名 = **「会话回顾」**（en `Session recap` / zh-TW `工作階段回顧`） |
| O4 | 会话回顾是否参与检索排序 | **已定**：本版只作展示，不进 FTS 索引；P3 质量达标后再评估 |
| O5 | subagent 转录是否纳入回顾范围 | **已定**：不纳入本版 |
| O6 | serve/手机端是否暴露只读检索 | **已定**：不暴露（手机端不需要，只读也不暴露） |
| O7 | 回收站会话的会话回顾是否保留 | **已定（2026-09 确认：撤下）**：会话进回收站即从回顾页**撤下**其会话回顾（粒度分离后，回收站会话不属回顾页对象集）；彻底删除一并清理；恢复后指纹未变则复用。原先“保留”的暂定与本次粒度分离冲突，故反转为撤下 |
| O8 | 会话回顾的上下文来源 / 模型 / 预算是否等于延续会话 | **已定（2026-09 采纳建议）**：三个独立旋钮——① 来源 = **权威转录**（会话内是"当时被折叠后的投影"，见 §4.2）；② 模型 = 会话记录的模型（`agent.LoadSessionModel`）→ 配置回退 → 跳过待补；③ 预算 = **头尾优先**分段（抄 `session_extract` 的 fragment 机制）。**通道隔离为硬约束**：独立 provider 实例、不走会话准入/租约、自建并发闸门且可让路、独立用量来源；**不进会话、不写权威转录、不动前缀缓存** |
| O9 | 配置项命名（原"摘要模型"） | **已定（2026-09）**：**「会话回顾模型」**（en `Session recap model` / zh-TW `工作階段回顧模型`），配置键 `session_recap_model`，代码照 `WebSearchModel` 模式（`config.SessionRecapModel()` / `ResolveSessionRecapModel` / `SaveSessionRecapModelTo`）。命名标准 = **名字自带对象**，故"摘要模型"与单独的"回顾模型"均不采用 |

**O3 现状位置与结论**

桌面端现状（2026-09 查实，四条证据同向）：

- **顶栏「历史」抽屉已是不可达遗留**：`topbar.history` locale 键在 `desktop/frontend/src` 内**零引用**；前端没有任何把 `histView` 置为历史态的调用点（`setHistView` / `setHistory` 只出现在 close / refresh / delete / rename 路径）；宿主只 emit `history-index:changed-v1`（索引状态），没有"打开历史"事件；`HistoryPanel` 目前唯一的渲染点是 `TrashPage.tsx`（`presentation="page"`、`kind="trash"`）。
- **正常会话浏览入口 = 侧栏项目树**（`SidebarRegion.tsx` 懒加载 `ProjectTree`）：`projectTree.searchPlaceholder`「搜索项目或会话」、`newTopic`「新会话」、`archiveTopic`「归档到回收站」、`recoveryOnly`「可恢复」、`recovered`「已恢复」、`rebuildCatalog`「重建索引」。
- **回收站 = 独立整窗页**（`TrashPage`，复用 `HistoryPanel` 的 page 形态；入口在侧栏），与项目树入口并存。
- 面板层已有的能力（回顾页可复用为视图）：搜索框「搜索会话…」、历史筛选、按天分组、轮次徽标、当前 / 已打开 / 已删除标记、预览，以及 history-search 命中展示（宿主命令 `App.GetHistorySearchContext` / `App.SearchHistoryContent`）。

**O3 结论（已定 + 2026-09 修正）**：新建**独立管理页「会话回顾」**，与「回收站」「自动化」「设置」同一管理页外壳（整窗、惰性加载）。**对象与回收站彻底分离**：

- 回顾页的对象 = **会话回顾**（会话关闭 / 轮转时生成的一条派生记录）：只读查阅 + 检索；
- 回收站的对象 = **整个会话**（软删除 → 恢复 / 彻底删除 / 清空）：保持**独立入口**，`TrashPage` 不改名、不被吸收；
- 之前的"三态统一入口 / 与回收站合并入口"**作废**：粒度不同（文档 vs 会话），混在一页会让"这一行的操作作用于谁"变歧义。

视图层可复用 `HistoryPanel` 的 page 形态（列表 + 预览），但不因此合并入口。

命名约束（须同时满足）：① 不与顶栏「历史」抽屉、侧栏「会话」「回收站」「记忆与技能」混淆；② 三语都成立（`zh.ts` / `zh-TW.ts` / `en.ts`，管理页文案另在 `managementLocale.ts`）；③ 不暴露实现（不要“索引 / FTS”），要指向用途（找回“上次做过什么”）。

**O3 命名定案**：**「会话回顾」**（en: `Session recap` / zh-TW: `工作階段回顧`）。理由：① 桌面端「归档」已被软删除占用（`projectTree.archiveTopic` = 「归档到回收站」），"档案 / 档案馆"会被读成"回收站"，故弃用该路线；② **名字自带对象**（本版命名标准：「回顾」单独一个词，读者不知回顾什么）；③ 与顶栏「历史」、侧栏「会话」「回收站」「记忆与技能」都不撞；④ 不暴露实现。

**页面副标题（必须）**：写明"只含会话关闭时生成的会话回顾；会话本身的删除、恢复在回收站"，用以消除粒度歧义（R8）。

候选留档（供回溯；定案 = **会话回顾**）：

| 候选 | en / zh-TW | 优点 | 风险 |
| --- | --- | --- | --- |
| 档案馆 | Archive / 檔案館 | 与"归档"用词一致；与「回收站」并列自然 | en "Archive" 易被读成归档文件；zh-TW「檔案」日常义是 file |
| 会话档案 | Session archive / 工作階段典藏 | 限定"会话"，不与日志/文件混 | **撞名**：桌面“归档”= 归档到回收站；zh-TW 歧义更重 |
| 回顾 | Recap / 回顧 | 以用途命名，一看就知道干什么；与历史/记忆/回收站均不撞 | **未自带对象**（读者不知回顾什么）→ 最终定为「会话回顾」；偏口语 |
| 台账 | Ledger / 工作台帳 | 精确表达"我做过什么的逐条记录" | 偏企业内部用语；不含"可检索档案"之意 |

落选说明：「会话档案」与「档案馆」因与既有「归档到回收站」撞名而落选（口径见 §4.6）；「台账」偏企业内部用语；「回顾」因未自带对象，最终定为「**会话回顾**」（命名标准见 §13）。

**已查实（本机 2026-09）**：① 顶栏「历史」抽屉已无可达入口（四条证据见上），当前入口是侧栏项目树 + 回收站页；② 系统恢复副本**落在检索可见范围内**：命名约定 `<id>-recovery-<16 位 hex>`（`internal/agent/recovery_filename.go`），检索侧完全不识别 `recovered`（`internal/history/*`、`internal/historycatalog/*` 无相关代码），可见性只由 `IsVisibleSession` 决定 ⇒ 同一对话可能被主会话与副本重复命中。已按 S2 定案排除。

**仍未实机验证**：① 抽屉不可达（未运行桌面 App 点击确认）；② 检索重复命中（未实跑 `history` 检索或查 sqlite 验证，结论由代码路径推出）。

**O4 何时生效**

O4 与"回顾生成"无关，只在**输入关键词检索的那一刻**起作用，差别是会话回顾文本**参不参与匹配与排序**：

- 会话回顾进 FTS 索引：回顾里出现的词也能命中该会话，并影响相关度排名。
- 会话回顾只作展示（**本版采用**）：匹配与排序仍只看会话原文（user / assistant / tool 输入与错误），回顾只出现在结果行与展开处。

**O4 结论（已定）**：本版会话回顾**只作展示、不进 FTS 索引**。理由：排序语义已定型并在线上使用，回顾质量未达标前入索引会把"错回顾"变成"错命中"。P3 质量达标后再评估让其参与检索（§11）。

## 11. 未来方向（本期不承诺）

- 语义/向量检索（关键词检索到瓶颈时；本版明确不做）。
- 把"从历史中回忆"封装为 Agent 工具 `recall_memory`——**前提**是它带来的前缀缓存代价经评估可接受，且会话回顾质量经 P3 验证过关。
- 跨会话记忆巩固（从多次会话提炼稳定偏好/决策），与既有 `internal/memory` 合并考量，而非另建一套。
- 会话回顾参与检索排序（O4 的第二阶段）。
- 回顾投影的保留策略与体积治理（投影会随会话数增长；既有 `cache/` 治理手段可复用）。

## 12. 源码事实索引

| # | 事实 | 位置 |
| --- | --- | --- |
| 1 | 检索投影的权威说明（FTS5、索引范围、非阻塞提示、后台重建、DAG head 语义、投影可弃、doctor/reindex 命令） | `docs/HISTORY_SEARCH_CATALOG.md` |
| 2 | Agent 检索工具（`NewIndexedTool`）、工具描述/schema、可选范围与上限 | `internal/history/tool.go`（工具构造与 schema）；`internal/history/search.go`（kind、默认/上限、相关度下限）；`internal/history/indexed.go`（archive 作为读根） |
| 3 | CLI 命令面无检索命令，只有维护入口 `catalogs reindex history` | `docs/CLI.md` |
| 4 | `SessionStart` / `SessionEnd` 事件定义与"关闭或轮转"语义 | `internal/hook/hook.go`、`internal/hook/runner.go` |
| 5 | `/new` 轮转流程与 `SessionEnd("clear")` | `internal/control/controller.go`（`NewSession`） |
| 6 | `/clear` 销毁工件 + `SessionEnd("clear")` | `internal/control/controller.go`（清空路径，含 `MarkCleanupPending`） |
| 7 | `Close()` / `CloseAfterDestroy()` / `ReleaseResources()` 与 `SessionEnd("other")`、`closeOnce` 幂等、`ReleaseResources` 不触发；**归档 / 删除会话也会走 Close**（故回顾触发点必须设丢弃类护栏） | `internal/control/controller.go`（close 家族）；`desktop/app.go`（`closeRemovedSessionRuntime` → `ctrl.Close()` / `CloseAfterDestroy()`；`deleteSession`）；`desktop/topic_archive_runtime.go`（`finalizeRemovedTopicRuntimes` 先 `SetSessionPath("")` 再 Close）；`internal/control/session_destroy.go`（`BeginDestroySession` 只做 job teardown、不发 SessionEnd） |
| 8 | extension 拦截点 `session.end` / `session.rotate`（阶段 End / Rotate） | `internal/extension/intercept.go`、`internal/extension/dispatch/payloads.go`、`internal/control/controller.go` |
| 9 | 磁盘布局唯一权威、sidecar 命名、主 transcript 与 4 类事件日志的区分 | `internal/store/session.go` |
| 10 | 会话/话题目录字段（轮次、起止与最近活动、健康度、标题与标题来源、pin/排序） | `internal/sessioncatalog/schema.go`、`internal/sessioncatalog/types.go` |
| 11 | 移除（回收站）tombstone 语义（源码注释称 "archived paths"） | `internal/sessioncatalog/removal.go` |
| 12 | 压缩历史归档目录（**compaction 产物**，与"归档 = 移入回收站"不同义） | `internal/config/paths.go`（`ArchiveDir`）、`internal/config/paths.go`（`userSupportDir`） |
| 13 | 脱敏家族（`Redact` / `RedactCredentials` / `RedactMessage` / `RedactError`、凭证命名模式） | `internal/secrets/redact.go` |
| 14 | 隐私安全读路径（截断、去 reasoning、去 system prompt、工具结果 opt-in）与清单工具 | `internal/tool/sessiontool/sessiontool.go` |
| 15 | LLM 生成会话短文本的先例与失败回退契约 | `internal/serve/serve.go`（`generateTitle` 及标题来源） |
| 16 | 会话内摘要（compaction）能力 | `internal/agent/compact*.go`；`internal/control`（`SummarizeFrom` / `SummarizeUpTo`，`/rewind` 使用） |
| 17 | 跨会话沉淀机制 | `internal/memory/`（`remember.go`、`index.go`、`auto_recall.go`、`recall_index.go`、`doc.go`） |
| 18 | 非阻塞持久化提示的接入点 | `internal/boot/session_observer.go` |
| 19 | CLI 侧刻意延后 SessionEnd 收尾、避免阻塞界面 | `internal/cli/chat_tui.go`、`internal/cli/cli.go`、`internal/cli/model.go` |
| 20 | 本机实测规模与投影现状（`*.jsonl` 1002 个、多根布局、`history-search/v1.sqlite` 52,649,984 字节、`archive/`、`cache/{session-catalog,task-catalog,usage-catalog}`） | `<state root>` = `%APPDATA%\reasonix`（本机 2026-09 实测） |
| 21 | 「归档」= 移入回收站（软删除）：用户可见文案与 Go 方法名 | `desktop/frontend/src/locales/zh.ts`（`projectTree.archiveTopic` / `archiveConversation` / `archiveAllConversations`）；`desktop/topic_archive.go`（`App.TrashTopic` → `commitTopicArchive` / `markTopicArchiveCleanupPending`） |
| 22 | 回收站状态的标记、可见性判定与检索过滤 | `internal/store/session.go`（`SessionCleanupPending` = `<id>.cleanup-pending.json`；`SessionSidecarFiles`）；`internal/agent/save.go`（`IsCleanupPending` / `IsVisibleSession` / `ClearCleanupPending`）；`internal/history/search.go`（`visiblePath`） |
| 23 | 回收站页的既有形态（独立整窗、系统恢复副本单独折叠、恢复 / 彻底删除的确认流程） | `docs/MANAGEMENT_PAGES.md`（Trash 一节）；`desktop/frontend/src/components/HistoryPanel.tsx`（回收站态） |
| 24 | 顶栏「历史」抽屉已无可达入口；当前浏览入口 = 侧栏项目树；回收站 = 独立整窗页（复用 HistoryPanel 的 page 形态） | `desktop/frontend/src/locales/zh.ts`（`topbar.history` 零引用；`projectTree.*`）；`desktop/frontend/src/app-shell/SidebarRegion.tsx`（懒加载 `ProjectTree`）；`desktop/frontend/src/components/TrashPage.tsx`（`presentation="page"`、`kind="trash"`）；`desktop/frontend/src/app-shell/AppOverlayHost.tsx`（overlay 形态无 opener） |
| 25 | 恢复副本：命名约定、catalog 身份、检索侧不识别 | `internal/agent/recovery_filename.go`（`-recovery-` + 16 位 hex）；`internal/sessioncatalog/recovery_groups.go`（`recovered=1` / `recovery_group_id`）；`internal/history/*` 与 `internal/historycatalog/*`（无 `recover*` 相关代码）；`desktop/recovery_copy_sweep.go`（自动清理） |
| 26 | 回顾类生成的先例与模型来源：一次性限界调用 + 独立用量来源 + 会话模型解析 + 超长会话分段 | `internal/control/session_title.go`（`GenerateSessionTitle` → `boundedllm.Call`，`Provider` 来自 `selection.ref`，`UsageSource: event.UsageSourceTitle`，`EffortOverride: PreferredReasoning(prov, "low")`，`MaxOutputBytes`）；`desktop/session_ai_title.go`（`AIRenameSession` 要求话题已打开）；`internal/serve/serve.go`（`titleProv` 专用轻量 provider）；`internal/agent/branch.go`（`Model` 字段 + `LoadSessionModel`）；`internal/agent/session_extract.go`（fragment %d/%d 分段 briefing） |
| 27 | 会话内 provider 视角 = model-visible projection（权威在事件日志）；超预算早期内容被折叠为摘要挤出提示 | `internal/store/session.go`（`SessionContext` = `<id>.context.json` 注释）；`internal/agent/compact_fold_input.go`（`foldToSummary` / `foldSummary{Mode, Spans, FoldTokens}`） |
| 28 | 回顾通道隔离的既有先例与缺口：独立 provider 注入、独立用量来源、**无并发控制** | `internal/serve/serve.go`（`titleProv` 独立 provider）；`internal/boundedllm/bounded.go`（**只有**超时 / `MaxTokens` / `MaxOutputBytes`，无并发闸门 ⇒ 需自建）；`internal/control/session_title.go`（`UsageSourceTitle`）；`desktop/settings_web_search_model.go`（`WebSearchModel` 的 admission/locking/rebuild 生命周期，兼命名与设置接线先例） |

## 13. 术语表（本书用词 ↔ 仓库用词）

**命名标准（本版起全篇适用）：名字要自带对象，不能只给动作或属性**（"摘要"不能单独指承载物；"回顾"不能单独指页面或模型）。

| 本书用词 | 含义 | 不要与这些混 |
| --- | --- | --- |
| **会话回顾** | 本功能的产物：会话关闭 / 轮转时生成的四要素派生记录（目标 / 关键动作 / 结论 / 待办） | compaction 的"摘要 / digest"（服务上下文压缩，用会话模型） |
| **会话回顾页** | 桌面只读查阅入口（原拟「回顾」，按命名标准补全对象） | 回收站页（管整会话）、侧栏项目树（浏览会话） |
| **会话回顾模型** | 生成会话回顾所用的模型配置项（`session_recap_model`） | 会话模型（对话用的）、compaction 的摘要模型 |
| **归档** | 仓库既有义 = **移入回收站**（软删除，`.cleanup-pending.json`）。本书只在引用既有行为时使用 | 本功能的"生成会话回顾"（勿用"归档"指它） |
| **压缩历史归档** | `ArchiveDir()`（`<state root>/archive`）：compaction 产物，也是检索读根之一 | 会话回顾的存放地（不放这里，O1） |
| **权威转录** | 主 `.jsonl` + 事件日志（transcript authority） | `<id>.context.json` 的 model-visible projection |
| **检索投影** | `cache/history-search` 的 FTS5/BM25 索引（可弃、可重建） | 权威文件（不可弃） |
