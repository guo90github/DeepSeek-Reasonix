# 分批提交计划（每批＝一个可提交单元）

> 建立于 2026-10-01。依据：24 轮进度日志 + 设计文档里各自登记的「本轮产出文件」清单（131 个脏文件里 102 个可归属）+ `git status`。
> **通用纪律（每批都适用）**
> 1. 只 `git add` 本批**点名**的具体文件（**禁止** `git add -A` / `git add .`）；提交前 `git diff --cached --stat` 复核。
> 2. 提交信息用中文 + 机器字段 `Verified:` / `Documentation-impact:` / `Cache-impact:`（分隔符必须是 ASCII `-` 或 `:`）。
> 3. 提交后把短哈希回填 `logs/STATE.md` 第 1 节「证据」列，并往 `journal.md` 追加一行。
> 4. 每批先跑该批的验证命令，**失败不许提交**；既有红（见第三节）不算本批失败，但必须在本批提交信息里如实写明。

## 零、共享文件（不属于任何一批）

这些文件同时被**别的会话**改动，整文件提交会连带别人的在制品，因此本计划**一律不提交**，并在台账登记「未归属改动」：

`desktop/app.go`、`desktop/app_test.go`、`desktop/topic_activation_test.go`、
`desktop/frontend/src/components/SettingsPanel.tsx`、`desktop/frontend/src/lib/useController.ts`、
`desktop/frontend/src/styles.css`、`desktop/frontend/src/locales/{en,zh,zh-TW}.ts`、
`internal/control/inbox_test.go`、`internal/control/inbox_dispatch_test.go`、`internal/boot/effect_test.go`、
`internal/control/controller.go`（含别的会话的 readiness / skill recorder / nextTurn 重构）。

**例外手续**：若某批确实需要其中某个文件的改动（例如第九的档位控件落在 `SettingsPanel.tsx`），
先 `git diff <file>` **逐 hunk** 对照本任务文档点名的标识符（如 `ShellAsyncTierField`）；只把认识的那些 hunk 计入，
其余留在工作树；说不清归属就**不提交**该文件（宁少提交，不误带别人的改动）。

## 一、批次表（建议顺序）

| # | 批次（需求） | 文件清单 | 提交前验证 | 备注 |
|---|---|---|---|---|
| 1 | **第十二** 报错根治：写入即证据 / shell 契约提示 / 作业回执闭环 / 起动≠跑过 | `internal/evidence/{write_effect.go,evidence.go}`；`internal/agent/{execute_batch.go,execute_batch_audit.go,execute_one.go,write_reread.go,tool_receipts.go,shell_job_receipts.go}`(+`shell_job_receipts_test.go`)；`internal/tool/builtin/completestep_receipts.go`；测试 `internal/evidence/{write_effect_test.go,shell_contract_test.go,verification_summary_test.go}`、`internal/agent/{write_evidence_lifecycle_test.go,stale_anchor_guard_test.go,write_evidence_same_turn_test.go,shell_contract_hint_test.go}`、`internal/boot/write_reread_effect_test.go` | ✅ **已提交 `759810f30`**（20 文件 +858/−36；`evidence`/`agent`/`tool/builtin`/`boot` 四包全绿） | A-1/A-3/A-23/A-24；价值判定见 §四 |
| 2 | **第九** 异步不阻塞 + 速度档位 | **Go 侧**：`internal/agent/shell_async.go`(+`_test.go`)、`internal/config/{config.go,edit.go}`(+`edit_shell_async_test.go`)、`internal/boot/loop_async_effect_test.go`；**前端并入批 6**（见备注） | `go vet ./internal/config ./internal/agent` → `go test -count=1 ./internal/config` → `go test -run 'TestShellAsync' -v ./internal/agent` → `go test -run 'TestEffectSlowShellCallDoesNotHoldTheLoop\|TestEffectBalancedTierOnlyLiftsChecks' -v ./internal/boot` | 默认档 `off`（A-22）。**实测修正**：`bridge.ts`/`types.ts`/`locales/*`/`SettingsPanel.tsx`/`settings_app.go` 的改动是**逐 hunk 混合**（第九 + 第十四 + 第十六 + 第十七 同处一文件），整文件提交必然跨批；若只提交第九自己的新组件，会出现"组件引用了未提交的 i18n 键"的断裂提交 ⇒ 前端这几处统一并入批 6 一起提交 |
| 3 | **第十七** 待办队列 + 归档 | `internal/agent/{todo_state.go,todo_board.go,todo_batch_identity.go}`(+tests)、`internal/agent/{todo_state_repro_test.go,sessionstate.go,sessionstate_test.go}`；`internal/control/{todo_settle.go,port.go}`；`desktop/{todo_batch_identity.go,todo_history_replay.go,todo_dismissal.go}`(+tests)；前端 `lib/todoVisibility.ts`、`app-runtime/useTodoPanelCommands.ts`、`components/TodoPanel.tsx`、`__tests__/{TodoPanel.test.tsx,todo-queue-repro.test.ts,todo-archive-section.test.tsx}` | `go test ./internal/agent ./internal/control`；前端 `npx tsc --noEmit` → `tsx src/__tests__/todo-queue-repro.test.ts`、`tsx src/__tests__/todo-archive-section.test.tsx` | `TodoPanel.test.tsx` 的两条旧断言（A-31 口径）在这批里一并改对 |
| 4 | **第十** 自由停靠标签页（A1 接线） | `lib/viewPlacement.ts`、`store/viewPlacements.ts`、`components/FloatingViewLayer.tsx`、`__tests__/{view-placement.test.ts,floating-view-layer.test.tsx}`、`app-shell/AppRuntimeView.tsx`、`components/TabBar.tsx`、`app-runtime/useAppSessionComposition.ts`、`app-shell/decisionFooterBuilders.ts` | `npx tsc --noEmit` → `tsx src/__tests__/view-placement.test.ts`、`tsx src/__tests__/floating-view-layer.test.tsx` | A-27 硬纪律（一个视图任一时刻只被一个可见 surface 承载）；`styles.css` 若需改动走零节 hunk 手续 |
| 5 | **第七 + 第十一 + 第十六** 记忆分层 / 双主线 / 召回记录 | `internal/memory/{activation.go,remember.go,remember_policy.go,store_v2.go,auto_recall.go}`(+tests)；`internal/agent/{memory_recall_record.go,skill_use_record.go,services.go}`(+tests)；`internal/control/{memory.go,input.go,skill_use_record.go}`(+tests)、`internal/control/turn_tail_sites_test.go`；`internal/skill/{tools.go,use_record.go}`(+tests)；`internal/event/{event.go,delivery.go}`；`internal/trajectory/recorder.go`；`desktop/recall_record_view.go`(+`_test.go`)、`components/RecapRecallStrip.{tsx,css}`、`__tests__/recap-recall-strip.test.tsx` | `go test ./internal/memory ./internal/agent ./internal/control ./internal/skill`；前端 `tsx src/__tests__/recap-recall-strip.test.tsx` | 第十六的读数接口（B2/B3）与第十四的召回条（B6d）同批，避免中间态断裂；U-2 已决＝排队 |
| 6 | **第十四** 回顾展示 + **前端共享文件收口**（第九/十四/十六/十七） | 本批自己的：`lib/recapHeatmap.ts`、`components/RecapHeatmap.{tsx,css}`、`components/SessionRecapPage.tsx`、`__tests__/{recap-heatmap.test.ts,recap-heatmap-component.test.tsx,session-recap-page.test.tsx}`、`desktop/frontend/package.json`；**并入**：`components/ShellAsyncTierField.tsx`(+测试)、`lib/{bridge.ts,types.ts}`、`components/SettingsPanel.tsx`、`locales/{en,zh,zh-TW}.ts`、`desktop/settings_app.go` | `npx tsc --noEmit` → `tsx src/__tests__/recap-heatmap.test.ts`、`…-component.test.tsx`、`session-recap-page.test.tsx`、`shell-async-tier.test.tsx`、`npm run check:css` | U-3 已决：维持 30 天窗口 / 日格。此批是"共享文件一次收口"，提交信息里要点明它同时带入第九/十六/十七 的接线与文案 hunk（无法按文件切分） |
| 7 | **第八** 前置/后置（**在飞**：BA1a/BA1b 已落地，BA2 未做） | 只含已落地部分：`internal/agent/{turn_outcome.go,run_loop.go,turnruntime.go}`(+`turn_outcome_test.go`)；`internal/control/{turn_outcome_record.go,turn_events.go,in_flight_turn.go,turn_progress.go}`(+tests) | `go vet ./internal/agent ./internal/control` → `go test ./internal/agent ./internal/control` | `controller.go` 是共享文件 ⇒ 只把 `turn_outcome`/`turn_progress` 相关 hunk 计入；**BA2 做完后另起一次提交** |
| 8 | **生成物**（跨批） | `desktop/frontend/src/generated/desktopContract.generated.{ts,json}`、`desktop/host_command_owners.generated.json` | 先用仓库自己的生成脚本重新生成（命令从 `desktop/frontend/package.json` 的 script 取，**不手改生成物**）→ `npx tsc --noEmit` | 重生成后若仍有差异，说明还有未提交的契约源改动 |
| 9 | **收口** | 产出目录 `docs/enhanced-product-experience/`（自己一次提交）+ `docs/99-总结报告.md` | 全量门禁：`go build ./...`、`go vet ./...`、`make lint`、`go test ./internal/... ./desktop/...`、`make frontend-check` | 全绿后写总结报告，`logs/STATE.md` 置终态 |

## 二、顺序理由

1. **先保命**：本任务至今零提交，131 个改动只活在工作树里。批次 1–3 先覆盖稳定性基线（第十二、第九）与改动面最大的交互（第十七）。
2. **按依赖**：立项书 §三 —— 第九/第十二 先行（稳定性基线优先）；第七/十一/十六 先统一设计再落地；第十 先于 第十七 的挂载点变动；第十四 依赖 第十六 的读数接口（故与批 5 相邻）。
3. **在飞的排最后**：第八（BA2 未做）放最后，避免把未完成单元混进历史。

## 三、影响推进的清单（按影响排序）

| 项 | 影响 | 状态 |
|---|---|---|
| **零提交**（131 项未提交） | 一次误操作即全丢；本计划就是处置 | 计划已建，待执行 |
| **既有红会污染验收** | 立项书验收 5 要求全量门禁全绿，下列不修则 9/9 不可达 | 见下 |
| ① `TestComparePortableVersionsOrdersNumericSegmentsAndPrerelease`（`dev.9` vs `dev.10` 排序反了） | **真 bug**：影响无人值守自我升级（P6）挑目标版本 | **已修 `12f093b85`**（新增 `comparePortablePrerelease`，段内数值比较） |
| ② `TestModelSettingsQueuedFollowupAppliesLatestBeforeDispatch`（"queued message used the retired connection"） | 跟随消息可能投给已退役连接 | **诊断中**：已否证三条（指纹含凭据 ✓、钩子已接线 ✓、机制存在 ✓）；剩"为何这次 admission 判定无需应用"未定，见下 |
| ③ `TestBrowserUpload*` / `TestDesktopPackages*`（symlink 权限、`desktop-build.sh missing packaging contract`） | 环境类，非本任务代码 | 记为环境失败，不修 |
| ④ `TodoPanel.test.tsx` 两条旧契约断言 | 与第十七的新队列口径冲突 | 批 3 内一并改对 |
| ⑤ theme-token 2 处、`SettingsPanel.tsx`/`useController.ts` 超 repolint 预算 | 全量门禁会红 | 前者待查；后者是别的会话在改的文件，**不碰**，只在台账登记 |
| **运营前置**（不在仓库内） | 任务跑不起来 | 见 `logs/STATE.md` §4 U-6：换含 `273221f73` 的构建 → 填目标契约 → 开总开关并**重启 App** → 启用任务 |

## 四、已提交批次的「价值判定」

### 批 1（第十二，`759810f30`）

判据来自 `docs/10` §一（2501 次调用语料）：读证据门「内容已变」55 次 +「从未读过」93 次（A+B 合计 97 次 = 验收分子），批量屏障 91 次。

| 落地项 | 价值 | 理由 |
|---|---|---|
| A-1 写入即证据 | **高** | 命中最贵形态（改自己刚写的行）：代价从"一次模型往返"降为"一次本地读盘"，而本仓库已证明耗时主项在模型往返 |
| A-23 作业回执闭环 / A-24 起动≠跑过 | **高（正确性，非降幅）** | 此前后台作业一起动就算 `Success=true`，`complete_step` 会接受没跑完的验证（假绿）；这是凭据语义修复 |
| A-3 拦截即给下一步 | 低-中 | 纯文案，只省"后续重复犯同形状"的轮次，**不省被拦的那一轮**，且该类降幅未量化 |
| A-2 / A-4 / C 类自动补跑 | **不落地（沿用原裁定）** | 分别等于放宽门禁、策略变更、收益≈0 |

**诚实缺口**：第十二的验收判据「A+B 97 → ≤20」**尚未复跑**，本批只证明机制正确，不证明降幅。

### ② 的诊断边界（未修，别再重复走）

已否证：①`ModelRuntimeFingerprint` 不含凭据（含 `Key: entry.APIKey()`）；②钩子未接线（`BeforeInboxDispatch` 在 6 个 boot 点注册，派发链会调到）；③机制缺失（`beginRuntimeTurn` 的 admission 已实现"需应用则重建、运行时被换则返回可重试的 `ErrInboxRuntimeUnpublished`"）。
`applied` 初值来自 `internal/boot/boot.go` 的 `ModelSettingsRevision: cfg.ModelRuntimeFingerprint(modelRef)`，`desired` 来自 `runtimeModelSettingsReader` 的现算值——**下一步只剩一个探针**：在该测试路径里打印这一对值（或读完整 `beginRuntimeTurn` 循环确认 `revision` 取值），据此再决定改代码还是改测试接线。

## 五、结构性事实：早批的树单独编译不过（已实测，别再试图"补文件"修）

用 `git worktree add <tmp> HEAD` 干净检出 + `go build ./...`（根模块与 desktop 模块）实测，`014f3f71b` 编译失败：

    internal\agent\agent.go:165,574: undefined: skill.WithUseRecorder / a.svc.skillRecorder
    internal\agent\execute_one.go:594: undefined: skill.WithUseRecorder
    internal\agent\todo_batch_identity.go:100+: meta.TodoBatch undefined (BranchMeta)

两条原因（都不是"漏提交某个文件"能解决的）：

1. **Go 包是真正的切分边界**：`internal/agent` 一个包内，第九/第十二/第十六/第十七 的改动互相引用（`agent.go` 的 `shellAsync`/`shellJobs`/todo 折入、`branch.go` 的 `TodoBatch`/`MemoryRecall`/`TurnOutcome`、`services.go` 的 `skillRecorder`）⇒ 任何子集都可能引用同包另一子集里的符号。
2. **逐 hunk 不可切分**：`internal/agent/branch.go` 同时承载 第八/第十六/第十七 的字段；`agent.go` 同时承载 第九/第十二/第十七。

**更正（本节初稿的错判，必须照此）**：初稿把 `internal/control/controller.go` 判为"别的会话的未提交改动"，并据此说存在外因阻塞。
复核后**推翻**——该文件当前脏改动只有 **4 增 / 5 删**，新增行就是 `readiness`(2) + `nextTurn`(1)，是**本任务自己的**接线
（第八/第十六；第18/19轮日志一直把它记在任务名下，"净增 0 行"是它的硬约束）。所以**不存在外部阻塞**：
把内核余量（批 5/7 的 `internal/**` 文件，含 controller.go 这 9 行）提交齐，tip 就能编译。

**验收口径**：

- 每批必须跑该批自己的测试命令（各批"提交前验证"列，均已实跑）；
- 整树编译以 **tip** 为准：**已复验通过**——`530f80d43` 干净检出后根模块与 `desktop` 各跑 `go build ./...` 均 OK；
- 中间提交不承诺单独可编译（Go 包级互引所致），CI 同样只看 tip。

END-UNIT: 批 1–4 / 6 + 内核余量 + 既有红① 已提交；批 8 的生成物已在收口批用生成器复验（发现并修正了批 6 的陈旧 `lastAttemptAt` 字段）；收口批含生成物 + 2 处 lint 修复 + 产出目录。剩余：真机、两个未证数字、运营前置。
