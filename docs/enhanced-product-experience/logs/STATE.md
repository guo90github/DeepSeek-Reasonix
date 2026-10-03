# STATE · 无人值守推进器（**终态** · 2026-10-03 选 ③ 后收尾；此后只做用户当轮点名的小单元）

> 接手方式：只读本文件。**原 9 条任务已终止**（终态见末行）；此后用户每点名一处 UI 就单独收口一次。
> `logs/journal.md` 为追加式流水；第 1 轮起的「第 N 轮日志」只作历史。

## 1. 需求进度表（9 条 · 终态）

| 编号 | 状态 | 证据（提交 / 命令 / 关键文件） |
|---|---|---|
| 第七 记忆分层 | **已验收** | 内核 `530f80d43` + 落地件 `472ecd7ac`；`docs/50` §十/§10.6：常驻段 改造前 **42,749** B → 改造后 **16,307** B，比值 **0.381 ≤ 1/2 ✓**；第一层＝「标签 + ≤50 rune 摘要 + 事实 id 句柄」（`internal/memory/index.go`），索引 39,080 → 13,749 B、最长行 1,162 → 211 B；第二层走 `memory read <id>` / `search` |
| 第八 前置/后置 | **已验收（代码判据）** | 后置 `530f80d43`（`TurnOutcome` 回合结束落档，内容无关）；前置 BA2 `8764b6775`（回合尾 `<turn-progress>`，按 U-4 去块预算、判词枚举归一化）。验证：`TestEffectTurnProgressRidesTheBodyNotThePrefix` + `internal/control` 三条 PASS |
| 第九 异步/档位 | **已验收（代码判据）** | `ce72be09a` + `TestEffectSlowShellCallDoesNotHoldTheLoop` **PASS**（真装配；同族实测 2137ms→18ms）；档位控件 `cffe2df3a`（设置页字段，11 PASS）+ `89c58d096`（**输入框底部的滑动开关** `ShellAsyncTierSwitch.tsx`，15 PASS；设置页字段保留） |
| 第十 多开 | **已验收（代码判据）** | `32ee36c82` + `cffe2df3a`：`view-placement` **48 PASS**（含存储 round-trip ⇒ 重启后保持）、`floating-view-layer` **23 PASS**（拖拽/缩放/z 序/停靠）。超出判据的留白见 §5 |
| 第十一 双主线 | **已验收（代码判据）** | `530f80d43`：`internal/event/delivery.go` 的 `DeliveryClass` 四分类 + 唯一入口守卫；`go test ./internal/control/ -run 'Delivery\|TailSites\|Notice\|Inbox\|Wake'` ok、`go test ./internal/event/` ok |
| 第十二 报错根治 | **已结项 · 判据未达（用户选定 ③ 接受）** | 机制 `759810f30` + `internal/boot/write_reread_effect_test.go`；复跑落档 `8b08d628b`、仪器修正 `3b5e24c09`：基线 97 → **残余 37**（33「目标改动行从未被写过」+ 4 `bash` 不透明写者）。**用户 2026-10-03 选 ③：接受现状 37**。🔴 判据（≤20）未达，不计入「已验收」 |
| 第十四 回顾展示 | **已验收（代码判据）** | `cffe2df3a`：`recap-heatmap` **22 PASS**、`recap-heatmap-component` **18 PASS**、`session-recap-page` 全 PASS（含热力图筛选与召回条联动） |
| 第十六 召回记录 | **已验收（代码判据）** | 内核 `530f80d43` + `desktop/recall_record_view.go`(+test)；前端落点三处＝记忆面板「召回记录」页（`c3dd649b5`）、回顾页会话详情、**桌面「面板」模块**（`72c88bb2e`，`FooterRecallModule.tsx` + registry `id: "recall-record"`）。测试：`memory-panel-recall-record` 7 PASS、`recap-recall-strip` 21 PASS、**`footer-recall-module` 11 PASS**、`footer-memory-module` 7 PASS、`footer-panel-visibility` 8 PASS、`bundle-contract` 26 PASS；typecheck rc=0 |
| 第十七 待办队列 | **已验收（代码判据）** | `014f3f71b`；`todo-queue-repro` **19 PASS**、`TodoPanel` **35 PASS**、`todo-archive-section` **11 PASS**、宿主 `todo_history_replay_test.go` ok ⇒ 三类混乱各有回归测试由红转绿 |

**终态口径**：立项书 §五 判据以"代码 + 可复跑命令"为准 ⇒ 8 条已验收；第十二 按用户选定 ③ 结项（判据未达，如实标注）。
文档类提交（不含代码）：`e0bfa19db`、`8b08d628b`、`3b5e24c09`、`0877bea39`、`4b54f7d9f`、`3ab539b80`、`314f47666`、`95699ddcd`、`efaa9dad8`、`1609cb844`、`8e04d0dff`、`64fc6fe71`。

## 2. 当前单元（2026-10-03 · 用户点名的两处落点变更，均已提交）

- **第九 · 档位控件挪到输入框底部并改滑动开关**（提交 `89c58d096`）：新件 `ShellAsyncTierSwitch.tsx`（轨随 `data-tier` 滑、三停位 `aria-pressed`、
  写入中全禁、未知值回落 `off`、**读不到不渲染**）+ 同名 CSS（已登记进 `check:css`）；挂在 `Composer.tsx` 底部 meta 行的 `--shell-async` 位；
  **设置页原字段保留**（与"模型切换器在输入框 + 模型设置页在设置里"同一套做法）。写路径仍是 `app.SetShellAsyncSpeedTier`。
- **第十六 · 召回记录并入桌面「面板」**（提交 `72c88bb2e`）：`FooterRecallModule.tsx` 注册进 `footerPanelModules.tsx`（`id: "recall-record"`，记忆族之后）；
  `RecapRecallStrip` 加**受控**入口 `record`，调用方已持有记录时不再重复问宿主。卡片规矩"不适用的模块不许留空表头" ⇒ 无记录时整块不出现。
  修的坑：直接 `app.RecallRecordForTab(...)` 在无该命令的宿主/桩上同步抛 `TypeError`，崩掉 `footer-memory-module`/`footer-panel-visibility`（全量 403 套件里唯二红）→ 改走 promise 链。
- **真机**：`v0.0.0-dev.117` 打包 rc 0、装机并列新增、`verify-windows-portable.sh` **exit 0**；包内 `build.json` = `v0.0.0-dev.117 / stable / commit 72c88bb2ea30`（= HEAD）。
  `dist\Reasonix-windows-amd64.zip` 239,331,839 B（2026-10-03 15:40）。dev.115/116 保持并列不动。快照完整性：构建前/后 `git status` 均只有 `desktop/frontend/dist/.gitkeep`（构建副产物，未提交）⇒ 包＝HEAD 那棵树。
- **UI 观感仍待你重启 App**：在跑的 service 是 `v0.0.0-dev.114`（托管本对话，SOP 禁忌不得杀）⇒ shell dev.117 会报 `build_mismatch (-32004)`。

## 3. 下一步（你重启 App 后的自验清单 · dev.117 才含全部改动）

① 输入框**底部**应有一条滑动开关（关/平衡/极速），点档滑块跟着走；设置页「通用」里的三段控件仍在；
② 桌面「面板」（底部面板带的卡片）里应出现**召回记录**一段（有记录才出现）；③ 记忆面板 →「召回记录」页逐轮召回条；④ 会话回顾页热力图；
⑤ 待办面板 → 已完成项归档折叠区、未完成项跨批次留在队列；⑥ 标签页撕下 → 浮层可拖拽/缩放/停靠且重启保持；⑦ 正常回合正文尾部 `<turn-progress>`（最多 3 轮）。

## 4. 待拍板（收尾后剩 ≤2 条）

- **B-1（真机确认）**：打包+装机已完成（dev.117，verifier exit 0）；**只剩你重启 App 后的人眼确认**（清单见 §3）。
- **U-1（第七 · 未答复）**：是否为「框架知识」新增独立 `type=framework`。**按「不新增」结项**。
- 已决（2026-10-03）：**D-2 选 ③**（接受 37）；U-2 排队、U-3 热力图 30 天/日格、U-4 前置块默认开最多 3 轮无字符上限、U-5 建议不自动写。
- 已决（2026-10-03 晚）：**「面板」＝底部面板带卡片（`footerPanel.title`）**，召回记录按此落点；若你指的是别的面板，说一声即可挪。

## 5. 未验证 / 暂缓

- 立项书验收 5（全量门禁）：Go 侧三项绿；`make lint` 30 条 / `repolint` 7 条 / 测试失败**全部落在别的会话文件**（本任务改动面 0 条）。
- **前端全量套件**：`72c88bb2e` 之前 `run-tests.mjs --keep-going` 全量 **401/401**；本次改动后一次全量得 **401/403**（唯二红＝上面那条 `TypeError`），
  **修复后只单独复跑了那两个套件（7 PASS / 8 PASS），未再跑 403 全量**。
- **真机 UI 观感：未完成**（`build_mismatch` 需重启 App；dev.117 已装好待启动）。
- 超出验收判据的留白：第十「后台标签多面板并行」；第八 BA4（按 verdict 触发记忆建议——现有候选机制已满足 U-5 实质）。
- 第十二 复跑**未建模**外部改动与轨迹变化（只会让残余更高），故 37 是同轨迹上界。

END-UNIT：两处用户点名落点均已改完、验证、提交（`89c58d096`、`72c88bb2e`），并打包装机 dev.117（verifier exit 0，包内 commit `72c88bb2ea30` = HEAD）；
**唯一剩下的真 blocker = 需你重启 App 才能用眼看新 UI**（清单见 §3）。
