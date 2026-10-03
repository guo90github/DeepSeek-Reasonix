# STATE · 无人值守推进器（**终态** · 用户 2026-10-03 选 ③ 后收尾）

> 接手方式：只读本文件。**本任务已终止**（终态见末行）；若要重开第十二，见 §3 的两条路。
> `logs/journal.md` 为追加式流水；第 1 轮起的「第 N 轮日志」只作历史。

## 1. 需求进度表（9 条 · 终态）

| 编号 | 状态 | 证据（提交 / 命令 / 关键文件） |
|---|---|---|
| 第七 记忆分层 | **已验收** | 内核 `530f80d43` + 落地件 `472ecd7ac`；`docs/50` §十/§10.6：常驻段 改造前 **42,749** B → 改造后 **16,307** B，比值 **0.381 ≤ 1/2 ✓**；第一层＝「标签 + ≤50 rune 摘要 + 事实 id 句柄」（`internal/memory/index.go`），索引 39,080 → 13,749 B、最长行 1,162 → 211 B；第二层走 `memory read <id>` / `search` |
| 第八 前置/后置 | **已验收（代码判据）** | 后置 `530f80d43`（`TurnOutcome` 回合结束落档，内容无关）；前置 BA2 `8764b6775`（回合尾 `<turn-progress>`，按 U-4 去块预算、判词枚举归一化）。验证：`TestEffectTurnProgressRidesTheBodyNotThePrefix` + `internal/control` 三条 PASS |
| 第九 异步/档位 | **已验收（代码判据）** | `ce72be09a` + `TestEffectSlowShellCallDoesNotHoldTheLoop` **PASS**（真装配；同族实测 2137ms→18ms）；档位控件 `ShellAsyncTierField.tsx` **11 PASS**（`cffe2df3a`） |
| 第十 多开 | **已验收（代码判据）** | `32ee36c82` + `cffe2df3a`：`view-placement` **48 PASS**（含存储 round-trip ⇒ 重启后保持）、`floating-view-layer` **23 PASS**（拖拽/缩放/z 序/停靠）。超出判据的留白见 §5 |
| 第十一 双主线 | **已验收（代码判据）** | `530f80d43`：`internal/event/delivery.go` 的 `DeliveryClass` 四分类 + 唯一入口守卫；`go test ./internal/control/ -run 'Delivery\|TailSites\|Notice\|Inbox\|Wake'` ok、`go test ./internal/event/` ok |
| 第十二 报错根治 | **已结项 · 判据未达（用户选定 ③ 接受）** | 机制 `759810f30`（写入即证据 / 拦截即给下一步 / 作业回执闭环 / 起动≠跑过）+ `internal/boot/write_reread_effect_test.go`；复跑落档 `8b08d628b`、仪器修正 `3b5e24c09`：基线 97 → **残余 37**（降幅 62%；33「目标改动行从未被写过」+ 4 `bash` 不透明写者）。**用户 2026-10-03 选 ③：接受现状 37**（①放宽门槛会把 33 条压到 ≈4 但破"未见当前内容不许改"，`docs/10` §9.2/§9.3 已用既有安全测试否证；②真实复跑数字未知）。🔴 判据（≤20）未达，此项不计入「已验收」 |
| 第十四 回顾展示 | **已验收（代码判据）** | `cffe2df3a`：`recap-heatmap` **22 PASS**、`recap-heatmap-component` **18 PASS**、`session-recap-page` 全 PASS（含热力图筛选与召回条联动） |
| 第十六 召回记录 | **已验收（代码判据）** | 内核 `530f80d43` + `desktop/recall_record_view.go`(+test)；前端落点＝**记忆面板「召回记录」页**（`c3dd649b5`，按 tab 取记录）+ 回顾页会话详情。测试：`memory-panel-recall-record` **7 PASS**、`recap-recall-strip` **21 PASS**、`recap-recall-wiring` 全 PASS、`session-recap-page` 全 PASS、`footer-memory-module` **7 PASS**、`bundle-contract` **26 PASS**、`context-center-contract` ok；`npm run typecheck` rc=0 |
| 第十七 待办队列 | **已验收（代码判据）** | `014f3f71b`；`todo-queue-repro` **19 PASS**、`TodoPanel` **35 PASS**、`todo-archive-section` **11 PASS**、宿主 `todo_history_replay_test.go` ok ⇒ 三类混乱各有回归测试由红转绿 |

**终态口径**：立项书 §五 的判据以"代码 + 可复跑命令"为准 ⇒ 8 条已验收；第十二 按用户选定 ③ 结项（判据未达，如实标注）。
文档类提交（不含代码）：`e0bfa19db`、`8b08d628b`、`3b5e24c09`、`0877bea39`、`4b54f7d9f`、`3ab539b80`、`314f47666`、`95699ddcd`、`efaa9dad8`、`1609cb844`、`8e04d0dff`、`64fc6fe71`。

## 2. 当前单元（2026-10-03 · 真机验证）

**用户授权真机验证 ⇒ 已按 SOP 打包 dev.115 并装机；UI 自验被 `build_mismatch` 挡住（需你重启 App）。**
- 构建：`bash scripts/desktop-build.sh windows/amd64 v0.0.0-dev.115`（`REASONIX_SKIP_BUDGET/DESKTOP_BUILD_SKIP_INSTALLER/REASONIX_LOCAL_SKIP_CHECKS=1`）→ rc 0；
  `dist\Reasonix-windows-amd64.zip` 239,331,281 B（2026-10-03 14:44）；包内 `app\resources\build.json` = **v0.0.0-dev.115 / stable / commit `8833aaca3fe6`**（= HEAD，`git status` 空 ⇒ 树与包一致）。
- 装机：`Expand-Archive … -DestinationPath C:\Users\guosj\Reasonix-portable -Force`（并列新增）→ `versions\v0.0.0-dev.115` 就位、`current.json` → dev.115；
  `scripts/verify-windows-portable.sh` **exit 0**。
- **自验受阻（真 blocker）**：启动 dev.115 后其窗口报「Reasonix cannot start / `build_mismatch (-32004)`：
  shell build v0.0.0-dev.115 does not match service build v0.0.0-dev.114」——本机在跑的 **service 是 dev.114**（`v0.0.0-dev.114\reasonix-desktop.exe`，2 进程），
  **正是托管本次对话的那个进程**，按 SOP 禁忌不得杀。⇒ 要用眼确认新 UI，只能由**你重启 App**（退出后从 `C:\Users\guosj\Reasonix-portable\Reasonix.exe` 再打开）。
  我启动的 dev.115 错误窗已按 PID 关闭（`dev115_left=0`），dev.114 service 未受影响（`dev114_service_left=2`）。

## 3. 下一步（你重启 App 后的自验清单 + 重开第十二的两条路）

**重启后请逐条看（dev.115 才含这些改动）**：① 设置页「通用」→ 档位三段（关/平衡/极速）；② 记忆面板 →「召回记录」页 → 逐轮召回条（折叠计数、展开列事实 id 与技能名）；
③ 会话回顾页 → 热力图 + 回顾页会话详情里的召回条；④ 待办面板 → 已完成项的**归档折叠区**、未完成项跨批次留在队列；⑤ 标签页撕下 → 浮层可拖拽/缩放/停靠，重启后位置保持；
⑥ 正常回合后正文尾部应出现 `<turn-progress>`（最多 3 轮）。

（重开第十二 的两条路不变：①放宽一处证据门槛 → 残余 ≈4，代价是破"未见当前内容不许改"；②真实复跑新轨迹。）

## 4. 待拍板（收尾后剩 ≤2 条）

- **B-1（真机确认）**：**已授权并执行到"可启动"为止**——打包+装机完成（dev.115，verifier exit 0）；
  **只剩你重启 App 后的人眼确认**（清单见 §3）。不改动已成立的 8 条代码判据。
- **U-1（第七 · 未答复）**：是否为「框架知识」新增独立 `type=framework`。**按「不新增」结项**。
- 已决（2026-10-03）：**D-2 选 ③**（接受 37）；U-2 排队、U-3 热力图 30 天/日格、U-4 前置块默认开最多 3 轮无字符上限、U-5 建议不自动写。

## 5. 未验证 / 暂缓

- 立项书验收 5（全量门禁）：Go 侧三项绿；`make lint` 30 条 / `repolint` 7 条 / 测试失败**全部落在别的会话文件**（本任务改动面 0 条）；
  **前端全量套件全绿**：`node scripts/run-tests.mjs --keep-going` → all **401 suites passed**。
- **真机 UI 观感：未完成**（`build_mismatch` 需重启 App；dev.115 已装好待启动）。
- 超出验收判据的留白：第十「后台标签多面板并行」（A-30 收窄）；第八 BA4（按 verdict 触发记忆建议——现有候选机制已满足 U-5 实质）。
- 第十二 复跑**未建模**外部改动与轨迹变化（只会让残余更高），故 37 是同轨迹上界。

END-UNIT：真机验证的可机器部分已做完（dev.115 打包 + 装机 + verifier exit 0，包内 commit `8833aaca3fe6` = HEAD）；
**唯一剩下的真 blocker = 需你重启 App 才能用眼看新 UI**（清单见 §3）。九条终态不变：8 已验收 + 第十二 按 ③ 结项。
