# 工具面彩蛋 + 面向智能体的可落地改进（2026-10-05，集群产出）

> 来源：agentbus 板 `default` 的 `seq 1–24`（交付物根 `tool-eggs-report`）。分工：`te-eggs-builtin`(会话 B)、
> `te-eggs-agentbus`(会话 B)、`te-agent-fit`(会话 A)；证据经**非产出者复跑**后收口。
> 本文分两半：**彩蛋** = 工具其实能做、但 `Description()`/schema/docs 都没写的能力；**改进** = 让工具更贴合智能体的可落地点。
> 所有 `文件:行` 都按 2026-10-04 的 dev-2 树逐行核对过；批量复跑命令见文末。

## 一、builtin 工具面（8 条）

| # | 能力（描述里没有的那部分） | 实现锚点 | 描述侧锚点 |
|---|---|---|---|
| A1 | `edit_file`/`multi_edit`：`old_string` 零命中时**静默降级为模糊匹配** —— 剥掉 read_file 的 `N→` 行号前缀、容忍 CRLF/LF、忽略行尾空白与 tab/空格差异，命中后回执标 `(fuzzy match)` | `internal/tool/builtin/encoding_helpers.go:126-141`（`fuzzyEditRanges` `:227`、`stripReadFileLinePrefix` `:354`） | `editfile.go:31` 只写 "Replace an exact string; old_string must occur exactly once" |
| A2 | `write_file`：目标内容与现有**字节完全相同**时 no-op（回 "already contains the exact content; no changes made"），并**保留原文件编码**（GBK/UTF-16/BOM），不会一律写成 UTF-8 | `writefile.go:111-113`、编码保留 `:107-110` | `writefile.go:41-43` 只说覆盖/建父目录；schema `:45-47` 只有 path/content/source |
| A3 | `glob`：**裸文件名**（如 `main.go`）无法命中时自动降级为递归 `**/<pattern>`；结果有 `globMaxResults = 1000` 硬上限与截断标记 | `glob.go:113-116`、上限 `:52`、截断 `:121-123` | `glob.go:35-37` 只讲 `* ? [] **` 语法 |
| A4 | `read_file`：自动识别并剥离 BOM（UTF-8 / UTF-16 LE/BE），**带 BOM 的 UTF-16** 也能读；NUL 判二进制并以 `binary file (NUL byte detected)` 拒 | `readfile.go:342-368`、`readfile.go:356-359` 与 `:362-364` | `readfile.go:137-139` 只讲行号前缀、offset/limit、intent |
| A5 | `ls recursive=true`：内置排除表（`.git`、`node_modules`、`.DS_Store`、`__pycache__`、`.idea`、`.vscode`），深路径分支 >50 时截断 | `ls.go:105-107`、`ls.go:118-120` | `ls.go:29` 只写 "(skips .git/node_modules)" |
| A6 | `web_fetch`：**SSRF 闸门** —— 拒 RFC1918、IPv6 ULA、link-local、`169.254/16`、元数据地址、CGNAT `100.64/10` 等，且 dial 时对解析出的 IP 再校验一次（抗 DNS rebind）；**故意放行 loopback**（"the agent can already reach localhost via bash"） | `webfetch.go:224-230`、`webfetch.go:61-66` | `webfetch.go:39-41` 完全没写 |
| A7 | `bash`：失败提示**自动追加进 stdout** —— git-bash `.bat` shim 与 `/tmp` 提示、batch trailer `goto :error`、PowerShell 提示、沙箱写拒绝文案（operation not permitted / read-only file system） | `bash.go:291`、`bash_gitbash_hint.go:26-47`、`bash_write.go:84-85` | `bash.go` 的 Description 未提"会自动附提示" |
| A8 | `view_image`：同一次调用可携带**结构化图片通道**（`ExecuteWithImages` 给 `data:<mime>;base64,`）；无结构化通道时 `Execute` 只追加一句固定说明 | `viewimage.go:42-46`、`viewimage.go:100` | `viewimage.go:33-35` 只说"通过原生视觉或配置的图片理解模型" |

## 二、agent_bus / 集群面（10 条）

| # | 语义（容易读错的那一面） | 锚点 |
|---|---|---|
| B1 | **`refute` 落在已 `done` 的节点上会把它拉回 `contested`**：`applyRefute` 的 switch 只拒 Abandoned/Stale，没有 Done 分支，随后无条件 `n.State = StateContested` | `internal/agentbus/board/node.go:316-328`；schema 只写 "challenge a result with a reason"（`internal/tool/builtin/agentbus.go:78`） |
| B2 | `hearing_settle` 的称重把**双方的** `hearing_answer` 都算成**反证方**：`refuteSupport` 收所有 `HearingAnswer` 记录、**不看阵营** | `internal/control/agentbus_hearing.go:110`、`internal/agentbus/hearing.go:186-197` |
| B3 | `view` 行里的 `evidence=` 是 **assert 条数**，不是证据量 —— 一条不带 evidence 的 assert 也 +1 | `internal/agentbus/view.go:165` |
| B4 | `hearing_open` 的 `required` 为空：**内核**会拒（`RefuseHearingNoRequired`），但**工具路径上传空集不会被拒** —— control 先把它解析成 owner ∪ 反证者（`agentbus_hearing.go:44-46`），于是审议**打开成功**并点名解析出的那一方。⇒ 内核那条分支在工具路径上确实不可达，但"传空必被拒"这个读法**被真机否定**（2026-10-04：我作为反证方传 `required=[]`，回执是 `hearing opened; <我> must answer`）。另一个未经记录的拒绝面：同一节点上重复开审议给的是 `hearing_already_open` | `internal/agentbus/hearing.go:173-176` + `internal/control/agentbus_hearing.go:44-46`；真机读数：板 `seq 58-59`、`hearings.jsonl` seq=3 |
| B5 | `decide` 三条 outcome 各有**未写明的前置**：`done` 需 ≥1 条带证据的 assert + `reproducedBy` 非空且**不在 assert 作者集合**；`blocked` 仅允许 open/claimed/contested；`abandoned` 需先有带证据的 abandon 请求。且 `done` 在注释里明写**故意不查依赖**（装配在零件未完成时也可判 done，由 landing 兜）。**2026-10-05 起这三条已写明**：进了 `agent_bus` 的描述（`internal/tool/builtin/agentbus.go` 的 `Description()`，与 §四 I9/I10 同处），智能体不必再从回执反推（`go test -count=1 ./internal/tool/builtin/` → `ok 19.645s`） | `internal/agentbus/board/node.go:512-568` |
| B6 | **`capability_gap` 与 `abandon` 都不是终态**：前者只清 owner/deadline 而 `claimable()` 仍返回 true；后者只记 `AbandonReq`、**不改 state**，要等有人 `decide(outcome=abandoned)` | `internal/agentbus/board/node.go:111-118`、`:484-490`、`:494-508` |
| B7 | 内核里有**工具发不出的动词**与**够不到的通道**：`VerbYield` 被 validate 并按 release 折叠，但全仓无发出者、动作枚举里也没有 `yield`；`VerbNoProgress` 只由宿主 sweep 以 `ActorSystem` 写；`control.AgentBusSay` 与 `PostAgentBusResult` 只有定义 + 测试 | `internal/agentbus/board/op.go:22`、`:30`；`internal/control/agentbus_talk.go:33`、`:68` |
| B8 | `claim` 的 `title` 落进**没人读的** `op.Reason`（`applyClaim` 不读 Reason）；`tokens`/`output` 是**死参数** —— 唯一扣费处只用 `Bounds.Steps` | `internal/tool/builtin/agentbus.go:303-305`、`internal/agentbus/board/op.go:57-63`、`internal/control/agentbus_budget.go` |
| B9 | `evidence` 数组里 **`ref` 为空的项被静默丢弃** ⇒ 整个 `assert` 随后以 `missing_evidence` 被拒；模型只能从"没证据"反推，而不是得到参数校验错误 | `internal/tool/builtin/agentbus.go:361-374` |
| B10 | **真机命中**：预算拒绝以 **`error`** 形态回传（`agentbus budget: turn (turn:default/) refused: budget_turn`），绕过了该工具自称的 "a refusal is the board's answer, not a tool failure: it comes back as text" | 报文出自 `internal/agentbus/budget.go:31,60-68,137`；对照 `internal/tool/builtin/agentbus.go:344-347` |

## 三、真机运行中额外确认的语义（6 条，未进上述清单）

1. **`decide` 没有所有权检查** —— 任何参与者都能替别人收口一个节点（`board/node.go:512-568`）。
2. **`unassign` 在 op 日志里记成 verb `assign` 且没有 `assignee` 字段**（板 `seq 14/15` 原始记录）⇒ 从日志复原"撤了指派"不能只看 verb。
3. **空 assignee 的 `assign` = 放回池子 ⇒ 唤醒的是请求者**（assignee 分支被 `continue` 跳过）⇒ 被派活的人可以把活推回编排者，而板不拦（`internal/agentbus/wake.go:142-164`）。
4. **`Ready` 要求"还有人在等它"**（`len(waiting[depID]) > 0`）⇒ 无依赖、无主、可开工但**没有下游在等**的节点不会唤醒任何人（`wake.go:152-154`）。
5. **`view` 是游标增量**：唤醒后的第一眼常常只看得到 delta（实测 `cursor=0` 全量 vs `cursor=15 next=16` 只列 1 个节点）；被唤醒的会话因此可能以为"view 是空的"。
6. **唤醒文本在注入时重算**，不是冻结的（`internal/control/agentbus_wake_text.go:10-18`），每条都带 `[agentbus wake queued …; rebuilt against the board as it is now]`。

## 四、面向智能体的可落地改进（12 条）

| # | 现状锚点 | 建议 | 消费者 | 最小验收 |
|---|---|---|---|---|
| I1 | `internal/tool/tool.go:21-35`（`Tool` 只有 5 方法）+ `:50-260`（13 个可选能力接口全靠类型断言发现：`BatchClassifier`/`ContextualTool`/`Previewer`/`ImageTool`/`PlanModeClassifier`/`ReadOnlyExecutionHostMutation`/`ReadOnlyExecutionBlockReason`/`MCPMetadata`/`MCPVisibleMetadata`/`MCPPackageMetadata`/`MCPAnnotations`/`MCPServerAuthorization`/`SnipHinter`） | ⏳ **一半已落地（2026-10-05）**：能力探测升为工具包**单一来源** `tool.CapabilityNames(t)`（新文件 `internal/tool/capabilities.go`），并让 `ContractEntry` 带上 `Capabilities []string`（`contract.go:14-21`，由 `contractEntriesFromTools` 填充）⇒ `Registry.ContractEntries()`/`AllContractEntries()` 的每一项都自带能力集合（`use_capability` 目录、`boot` 的 `Tools:` 面用的就是这份数据）。**未落：出屏到 `docs/TOOL_CONTRACT.md`**，原因是一个新查到的问题：`RenderContractMarkdown`（`contract.go:85-106`）**全仓零调用方** ⇒ 那份文档其实是**手工维护**的（`TestBuiltinToolContractDocumentation` 只查行存在，不比对渲染输出）；而本轮我手改过其中一行 ⇒ 该文件与渲染器输出已不一致。**两条路待决策**：(a) 接上渲染器（新增 `tools/toolcontract` 或 `go:generate`）+ 一条"提交的文档 == 渲染输出"的锁测试 ⇒ 文档变成生成并锁定；(b) 维持手工维护，只把能力写成文档里的一节 + 一条"该节与 registry 一致"的检查。**2026-10-05 用户决定：先走 (c) —— 不动文档，只保留已落的一半**（单一来源 + `ContractEntry` 携带能力）；生成机制以后再定，上文那处"手改行与渲染器输出不一致"如实留档，不动它 | 智能体、工具作者、生成文档 | ✅ 能力集合非空且与注册表一致：`internal/tool/contract_test.go::TestEveryBuiltinDeclaresItsCapabilities`（23 个内建逐条声明，并断言 `ContractEntry.Capabilities` 与工具本体一致）→ `ok 0.167s` |
| I2 | `tool.go:305-312`（`providerVisible`：能把工具从 provider schema 摘掉而 `Get/Execute` 照旧）+ `:322`（`SetProviderVisibleTools`）+ `contract.go:59`（`AllContractEntries`） | ✅ **已落地且是默认（2026-10-05 核对，无需写码）**：机制就是**统一工具面** —— `boot.UnifiedProviderToolNames()` = `CoreProviderToolNames()`（bash / bash_output / kill_shell / wait / read_file / view_image / edit_file / write_file / compress / use_capability / web_search）+ `HostControlToolNames()`（ask / update_goal / todo_write / complete_step / agent_bus），共 **16 个**；`applyUnifiedProviderToolSurface(reg)`（`internal/boot/agent_preset.go:101-118`）把它们设为可见面，其余工具只从 `Get` 可达 ⇒ 经 `use_capability(tool:<name>)` 进入。调用点是 `internal/boot/boot.go:2004` 且**无条件**（注释原文："Provider-visible tool surface is identical for every role setting before the extension snapshot freezes registry schemas for cache diagnostics"）；主会话用的就是这一条，子智能体预设另用 `agent_preset.go:117` 收窄 | token 预算、前缀缓存稳定性 | ✅ 断言已在（本轮未新增）：`internal/boot/boot_test.go::TestBuildTokenDeliverySharesUnifiedSurfaceAndExecutionPolicy` → `ok 1.413s`；`go test -count=1 -run 'Surface' ./internal/boot/` → `ok 13.676s`（含 `unifiedBootToolNames`（`:2299`）、"tools stay off the provider-visible surface and dispatch through use_capability"（`:2392`）、`boot_safe_mode_test.go:109` 的 "unified core for every role setting"） |
| I3 | `tool.go:29-34`（并行只在整批 ReadOnly 时发生）+ `:40-46`（`CallClass.ParallelSafe` 只活在调度器里） | ⏳ **按证据改口径（2026-10-05）**：① `ReadOnly` **早已**在智能体可见面 —— list 载荷 `read_only`（`internal/agent/usecapability_list.go:32`）与 inspect 载荷 `read_only`（`internal/agent/usecapability_inspect.go:104`）都有；② **`SchemasForContext`（`tool.go:674`）不能加字段** —— 它返回 `[]provider.ToolSchema`，也就是**provider 线上 schema**，多加的字段会直接进 API 请求（多数 provider 会拒未知字段；真要加就得按 schema 变更声明缓存影响）；③ **`ParallelSafe` 不是工具属性，而是按调用**的：`CallClass` 出自 `ClassifyCall(args)`，依参数而定（如 `internal/agent/path_bound_tools.go:62-69` 按路径参数分类）⇒ 静态声明就是撒谎，**不改**，调度器自己按调用分类即可。本轮实际落地的是把两条面拉齐：**inspect 也带上 `capabilities`** | 智能体自己排批 | ✅ 用例：`internal/agent/usecapability_list_test.go`（沿用原 MCP 汇总用例的夹具，尾部新增）—— `reg.Add(&fakeImageTool{})` 后 `inspect capability_id=tool:shot` 必须出现 `ImageTool` → `ok 0.090s`（该串只可能来自新增的 `capabilities` 字段） |
| I4 | `tool.go:90-102`（`ImageTool`；注释明写文本被固定字节预算截断会毁 base64） | ✅ **已落地（2026-10-05）**：能力自述一路走到**智能体可见面** —— `tool.ContractEntry.Capabilities`（I1）→ `capability.Entry.Capabilities`（`internal/capability/capability.go:48-51`，由 `ToolEntries` 填充）→ `use_capability` 的 `action=list` 载荷 `capabilities`（`internal/agent/usecapability_list.go:27-34` 定义、`:61-68` 填充）⇒ 会话能直接看到 `["ImageTool"]`。**缓存影响**：list 载荷是 `use_capability` 的**工具结果**（会话内按需读取），不是 provider 的工具 schema ⇒ `Cache-impact: none`（稳定前缀一字未动） | 任何返回图的工具 | ✅ 用例 `internal/capability/tool_capabilities_test.go::TestACatalogueEntryCarriesTheToolCapabilities`（`ToolEntries` 带进 `ImageTool` 且保留 `tool:view_image` 与 read-only）→ `ok 0.071s`；`view_image` 本体确实声明 `ImageTool` 由 I5 的棘轮钉住。**如实缺口**：图片**通道完整性**已由 `internal/agent/toolimages_test.go::TestToolResultImagesBypassTruncation` 钉住（图片绕开文本截断预算），但"**占位串数与图片数一致**"这一条我没有单独写用例，本行不声称它已被覆盖 |
| I5 | `tool.go:258-`（`SnipHinter` 零值无效 + 契约测试**强制**每个注册工具表态） | ✅ **已落地（2026-10-05）**：`internal/tool/contract_test.go` 新增 `declaredCapabilities`（23 个内建工具 × 13 个可选接口的**显式**声明）与 `TestEveryBuiltinDeclaresItsCapabilities`；缺行或与注册表不符都点名报出。**落点更正**：按指示放在 `contract_test.go` —— `contract_lock_test.go` 实为 `ContractEntries` 的死锁回归，不是能力锁。**I1 的能力汇总（Registry 侧 + 出屏到 `docs/TOOL_CONTRACT.md`）仍未落**，这里先用测试级清单顶上 | 防"新工具悄悄少能力" | ✅ 守卫**咬过**：把 `bash` 那行改成 `Previewer` ⇒ `contract_test.go:170: bash capabilities = [SnipHinter], but the table declares [Previewer]` + FAIL；恢复后三条契约用例 `ok 0.155s`。表内**实测**事实：`agent_bus`/`code_index`/`move_file`/`todo_write` 四个**零可选能力**；`view_image` 是唯一 `ImageTool`；**没有任何内建工具声明 MCP\* 能力**（那些属 MCP 适配层的工具） |
| I6 | `internal/agentbus/budget.go:228-236`（`turnKey` = `turn:<board>/<Turn>`）+ `internal/control/agentbus_budget.go:40-56`（claim **从不填 `Turn`**） | ✅ **已落地（2026-10-05）**：`chargeClaim` 现在传 `Turn: op.Actor` ⇒ 额度变成**每参与者**（键空间 = 参与者数），不再是"跨参与者、进程内只增不减"的一次性配额；用例 `internal/agentbus/budget_turn_test.go::TestTheTurnCeilingIsPerParticipant`，详见 `CLUSTER_REGRESSION.md` §一 F2(a) | 多智能体分工的成本模型 | ✅ 用例：一个参与者花满后，另一个仍可花，第一个再花被拒 |
| I7 | `internal/agentbus/talklog.go:74-96`（`CloseLapsed` 自己取锁；复核时唯一调用点在写一条 talk 行的路径 `internal/control/agentbus_talk.go:169`） | ✅ **已落地（2026-10-05）**：宿主 tick 就是 `AgentBusTick`（`internal/control/agentbus_wake.go:138`，接口 `control.AgentBusControl` 的一员、桌面 `desktop/heartbeat.go:221` 每 30s 调），现在它在**算唤醒之前**调用新的 `closeLapsedAgentBusTalk` ⇒ 没人写 talk 行时过期话题照样收口；同一 tick、同一条排序理由（收口会改变唤醒面点名的对象）。**残留**：`internal/serve`（无头）目前没有任何周期 tick ⇒ 那里仍要等写者 | 唤醒面噪声（真机实测：10-03 的 5 条 ask 到 10-04 仍在产生 `wake target has no route`） | ✅ 用例：`internal/control/agentbus_talk_close_test.go::TestTheHostTickClosesALapsedTopicWithoutAWriter`（`SilenceWindow=1ms`、问一句后只睡 5ms、**不写任何 talk 行** ⇒ tick 后 `topic.Closed=true`；再 tick 一次 talk 日志行数不变 ⇒ 收口是记录而非每次重写）→ PASS |
| I8 | `internal/control/agentbus_budget.go:39-41`（`NodeSpent(board,node) > 0` 时直接放行）+ `:152-170`（`settleBudget`） | ✅ **已核对并已用真用例钉住（2026-10-05）**，两半都在生产里：① `chargeClaim`（由 `ApplyAgentBusOp`（`internal/control/agentbus.go:185`）调用）只扣 **node + turn**，且**一个节点只扣一次** ⇒ `release`/`unassign` 交回活时**钱不退**、重领免费；② `settleBudget` 挂在 `decide(done)` 上（`agentbus.go:199-202`，且只在 `!receipt.Duplicate` 时做），按 `n.Bounds.Steps` 结算 **board + subtree** ⇒ 只有**被接受**的活才动总预算（T7-4），结算失败只记 WARN、不回滚已落的决定。**更正**：`Ledger.Settle` 一度被我误判为"生产零调用方"（grep 漏了包装名 `settleBudget`），实际早已接线 | 分工/成本模型 | ✅ 用例 `internal/control/agentbus_budget_cost_test.go::TestANodeIsChargedOnceAndOnlyAcceptedWorkSpendsTheTotal`（claim 3 步 ⇒ `NodeSpent=3`；release 后重领 ⇒ 仍 3；换第二个节点 3+3>5 ⇒ 被拒；claim 阶段 `BoardSpent=0`；`decide(done)` 后 `BoardSpent=3`）→ PASS |
| I9 | `board/node.go:316-328`（= B1） | ✅ **已落地（2026-10-05）**：`agent_bus` 的工具描述写明「refute also undoes a done step: the node goes back to contested for a verdict」（`internal/tool/builtin/agentbus.go:68`），并同步进 `docs/TOOL_CONTRACT.md` 的 `agent_bus` 行 | 智能体的正确预期 | ✅ 描述已改：`internal/tool/contract_test.go::TestBuiltinToolContractDocumentation` → `ok 0.149s`（该测试**不校验描述文本**，只查行存在/read-only/描述非空/schema 规范 ⇒ 无需重生成契约） |
| I10 | `board/node.go:512-568`（`decide` 无所有权检查） | ✅ **已落地（2026-10-05）**：同一描述补上「decide is not ownership-checked: any enrolled participant may close a step out, so close only what its evidence and a reproducer can back.」 | 防误替别人收口 | ✅ 同上。**缓存影响（I9+I10 合起来）**：`Description()` 是 provider 可见的工具 schema，这两句会改动稳定前缀一次 ⇒ `Cache-impact: low`（一次性固定文本、无逐轮漂移；`go test -count=1 ./internal/tool/builtin/ ./internal/tool/` → `ok 19.626s / ok 0.226s`） |
| I11 | `internal/tool/builtin/agentbus.go`（= B9；复核时 `evidenceOf` 在 `:361-374`） | ✅ **已落地（2026-10-05）**：`Execute` 在**进任何分派之前**校验每个 evidence 项的 `ref` 非空，报 `evidence[i] has no ref: every evidence item needs the ref that lets another participant check it` —— 覆盖 `assert`/`decide`/`refute`/`abandon`（走 `opFor`）与 `hearing_answer`（不走 `opFor`）两条路 | 少一次"没证据"的困惑 | ✅ 用例：`internal/tool/builtin/agentbus_test.go` 的 `TestAgentBusToolNamesWhatAnIncompleteCallIsMissing` 新增两行子用例（`evidence[{kind:test,ref:"  "}]` 与 `hearing_answer` 的空 ref），断言都报 `evidence[0]` 且**未到达板**（该表统一断言 `port.applied` 为空）；`go test -count=1 ./internal/tool/builtin/` → `ok 18.740s` |
| I12 | `internal/agentbus/budget.go`（= B10） | ✅ **已落地（2026-10-05）**：转换放在**工具边界**（`internal/tool/builtin/agentbus.go` 的 `apply`）—— 预算拒绝（`agentbus.IsBudgetReject`）现在与板拒绝一样回**文本**（形如 `refused (budget_turn): the spending ceiling turned claim on "build" down — …`），不再冒成工具失败；**宿主侧的 typed error 一字未动**，因为下发循环的终止性跳过（F2）正是靠它认出拒绝 ⇒ 两个口径各归其位：**对模型是文本、对调度器是错误**。工具自述那句 "A refusal comes back as a reason (…)" 本来就只列举例子（illegal transition / missing evidence / unknown node），故**未改描述** ⇒ `Cache-impact: none` | 前端与智能体读取口径一致 | ✅ 用例 `internal/tool/builtin/agentbus_budget_refusal_test.go::TestABudgetRefusalComesBackAsTextNotAFailedCall`（假端口返回 `*agentbus.BudgetReject` ⇒ 调用必须 `err==nil`，且文本含 `budget_turn` 与 `refused`）→ `--- PASS` |

## 五、批量复跑（一条命令打印全部锚点）

```bash
cd C:/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix
# builtin 8 条（A1–A8）：描述侧 + 实现侧
sed -n '29,36p' internal/tool/builtin/editfile.go; sed -n '118,145p;225,232p;350,360p' internal/tool/builtin/encoding_helpers.go
sed -n '39,46p;103,120p' internal/tool/builtin/writefile.go; sed -n '33,40p;48,56p;96,125p' internal/tool/builtin/glob.go
sed -n '135,142p;340,368p' internal/tool/builtin/readfile.go; sed -n '27,32p;100,122p' internal/tool/builtin/ls.go
sed -n '37,44p;58,68p;220,232p' internal/tool/builtin/webfetch.go; sed -n '105,130p;228,236p;286,296p' internal/tool/builtin/bash.go
sed -n '80,115p' internal/tool/builtin/bash_write.go; sed -n '20,50p' internal/tool/builtin/bash_gitbash_hint.go; sed -n '33,46p;96,101p' internal/tool/builtin/viewimage.go
# agent_bus 10 条（B1–B10）+ 零调用方
sed -n '74,95p;212,224p;298,310p;344,378p' internal/tool/builtin/agentbus.go
sed -n '104,122p;308,332p;392,422p;484,570p' internal/agentbus/board/node.go; sed -n '18,32p;55,66p' internal/agentbus/board/op.go
sed -n '156,182p' internal/agentbus/view.go; sed -n '166,180p;350,368p' internal/agentbus/hearing.go
sed -n '36,52p;104,116p;184,198p' internal/control/agentbus_hearing.go; sed -n '28,34p;60,68p;130,140p' internal/agentbus/budget.go
grep -rnE 'AgentBusSay|VerbYield|PostAgentBusResult' --include=*.go internal/
# 改进 12 条（I1–I12）
sed -n '21,46p;90,102p;240,260p;305,312p;655,669p' internal/tool/tool.go; sed -n '13,19p;46,51p;57,61p;82,90p' internal/tool/contract.go
sed -n '228,236p' internal/agentbus/budget.go; sed -n '40,56p' internal/control/agentbus_budget.go
sed -n '70,80p' internal/agentbus/talklog.go; sed -n '164,172p' internal/control/agentbus_talk.go; sed -n '512,568p' internal/agentbus/board/node.go
```

## 六、边界（如实）

- **I9/I11/I12 出自 `te-eggs-agentbus`（会话 B）的断言**：我复跑了 B 给的锚点命令并逐条确认其中 4 条中心结论（工具 `enum` 里确实没有 `say`；`BudgetReject.Error()` 报文与实际报错逐字一致；`VerbYield`/`AgentBusSay`/`PostAgentBusResult` 生产零调用方；`decide(done)` 的三条前置属实且无所有权检查）。**I11 的窗口已在 2026-10-05 逐字复核并落地**（当时 `evidenceOf` 在 `builtin/agentbus.go:361-374`；校验点现加在 `Execute` 进分派之前，见 §四 I11 与 `internal/tool/builtin/agentbus_test.go` 的两条新子用例）。
- **未覆盖的面**：MCP 适配层、subagent 委派面、插件工具面，以及 `notebookedit`/`delete_symbol`/`delete_range`/`completestep`/`todo`/`bg*` 等未被逐个读取的内建工具。
- **复跑锚点对应 2026-10-04 的 dev-2 树**（`HEAD` = `c5546fd66` 前后）；行号会随之后的提交漂移，届时以函数名/字符串为准。
- 每一条改进的"最小验收"都写成**用例或断言形状**，不写具体的 `-run` 模式 —— 因为该模式此刻还不存在（写出来会变成一句无法复跑的承诺）。
