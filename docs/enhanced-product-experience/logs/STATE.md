# STATE · 无人值守推进器（唯一状态真源 · 覆盖式更新）

> 接手方式：只读本文件 + 第 3 节点名的 1~2 个文件。不要重读历史会话转录与 `logs/2026-10-01-第*.md`。
> 第 1 轮的「第 N 轮日志」只作历史；`logs/journal.md` 为追加式流水。

## 1. 需求进度表（9 条 · 状态 / 证据）

| 编号 | 状态 | 证据（提交 / 命令 / 关键文件） |
|---|---|---|
| 第七 记忆分层 | **已验收** | 代码 `530f80d43`；占用对比与落地见 `docs/50` §十：常驻段 改造前 **42,749** B → 改造后 **16,307** B，比值 **0.381 ≤ 1/2 ✓**；第一层改为「标签 + ≤50 rune 摘要 + 事实 id 句柄」（`internal/memory/index.go` 的 `maxFirstLayerRunes`/`firstLayerText`），索引 39,080 → 13,749 B、最长行 1,162 → 211 B；第二层走 `memory read <id>` / `search`。验证：`go test ./internal/memory/`、`go test ./internal/control/`、`go test -v ./internal/boot/` 全绿 |
| 第八 前置/后置 | **已验收（代码判据）** | 后置：`530f80d43` 的 BA1a/BA1b——回合结束把 `TurnOutcome`（verdict/计数/义务）落进会话侧车，内容无关。前置：BA2（`internal/control/turn_progress.go` + 接线 `input.go:218`）回合尾注入 `<turn-progress>`（最近 3 轮 + 未满足义务计数），按 U-4 去掉块预算、改判词枚举归一化（`8764b6775`）。验证：`TestEffectTurnProgressRidesTheBodyNotThePrefix` + `internal/control` 三条 PASS |
| 第九 异步/档位 | **已验收（代码判据）** | `ce72be09a`（`internal/agent/shell_async.go`、`[agent] shell_async`）+ `internal/boot` 的 `TestEffectSlowShellCallDoesNotHoldTheLoop` **PASS**（真装配：慢 shell 不占住循环；同族实测 2137ms→18ms）；档位控件 `ShellAsyncTierField.tsx` **11 PASS**（三档/保存/禁用/未知值回退，`cffe2df3a`） |
| 第十 多开 | **已验收（代码判据）** | `32ee36c82` + `cffe2df3a`：`lib/viewPlacement.ts`（拖拽/缩放/边界夹取/**存储 round-trip**）+ `FloatingViewLayer.tsx`（浮层拖拽/缩放/z 序/停靠）——`view-placement` **48 PASS**、`floating-view-layer` **23 PASS**（含 "both placements round-trip through storage" ⇒ 重启后保持）。超出判据的留白见 §5 |
| 第十一 双主线 | **已验收（代码判据）** | `530f80d43`：`internal/event/delivery.go` 的 `DeliveryClass` 四分类 + 唯一入口守卫（结构性声明表）；验证 `go test ./internal/control/ -run 'Delivery\|TailSites\|Notice\|Inbox\|Wake'` ok、`go test ./internal/event/` ok；U-2 已决（**排队**） |
| 第十二 报错根治 | 待验收（降幅**未达**判据） | `759810f30`（写入即证据 / 拦截即给下一步 / 作业回执闭环 / 起动≠跑过）+ `internal/boot/write_reread_effect_test.go`；`docs/10` §十一 复跑：基线 97 → **残余 37**（降幅 62%；仪器已按门禁真实规则修正，见 §11.2.1），判据要 ≤20 ⇒ **未达标**；残余 = 33「目标**改动行**从未被写过」+ 4 `bash` 不透明写者（A-2/A-4 已被 §9.2/§9.3 用安全理由否证，A-1 覆盖 60 条）。**待拍板 D-2** |
| 第十四 回顾展示 | **已验收（代码判据）** | `cffe2df3a`：`RecapHeatmap.tsx/.css` + `lib/recapHeatmap.ts` + `SessionRecapPage` 接线——`recap-heatmap` **22 PASS**、`recap-heatmap-component` **18 PASS**（含主题 token/退役变量检查）、`session-recap-page` 全 PASS（含热力图筛选与召回条联动）；U-3 已决（30 天/日格） |
| 第十六 召回记录 | **已验收（代码判据）** | 内核 `530f80d43`（记录含 turnSeq/injected/omitted，正文不入）+ `desktop/recall_record_view.go`(+test)；前端 `RecapRecallStrip.tsx` **21 PASS**（折叠计数/展开列事实 id 与技能名/不渲染正文）+ `recap-recall-wiring` 全 PASS（controller→前端→页面整条接线）+ 并入既有记忆面板（`footer-memory-module` **7 PASS**） |
| 第十七 待办队列 | **已验收（代码判据）** | `014f3f71b`（批次身份 S1、队列+归档 S3、归档界面+S2 重放，含 agent.go/boot.go 接线）；**本轮复跑**：`todo-queue-repro` **19 PASS**、`TodoPanel` **35 PASS**、`todo-archive-section` **11 PASS**，宿主 `todo_history_replay_test.go` ok ⇒ 三类混乱各有回归测试由红转绿 |

## 2. 当前单元（2026-10-03 · 本轮）

**把余下判据逐条对到可执行证据 ⇒ 第九/第十/第十一/第十四/第十六/第十七 翻「已验收（代码判据）」。**
本轮实跑：12 个前端套件（`view-placement` 48、`floating-view-layer` 23、`recap-heatmap` 22、`recap-heatmap-component` 18、
`session-recap-page` 全绿、`recap-recall-strip` 21、`recap-recall-wiring` 全绿、`shell-async-tier` 11、`todo-queue-repro` 19、
`TodoPanel` 35、`todo-archive-section` 11、`footer-memory-module` 7）＋ 三组 Go 用例（`internal/boot -run 'Async|Loop'`、
`internal/control -run 'Delivery|TailSites|Notice|Inbox|Wake'`、`internal/event`）全部 rc=0。
口径：立项书 §五 判据以**代码与可复跑命令**为准 ⇒ 这些条目按"代码判据"验收；真机观感降级为 B-1 的可选确认（不再是阻塞项）。

## 3. 下一步（下一轮直接照做）

**自主队列已空。** 8/9 已验收（代码判据），只剩：第十二 的 ≤20 需你选路（§4 D-2）；真机可选确认（§4 B-1）。
若 B-1 选了"打包复测"，则顺手做第十「后台标签完整面板」（§5 的超出判据留白）——它是唯一剩下的结构性候选。

不要一轮做两件。

## 4. 待拍板（≤3 条）

- **D-2（第十二 · 主问题）**：复跑 **97 → 37**（判据 ≤20）。①放宽证据门槛（`old_string` 唯一匹配放行 / 拦截时宿主代读并登记）
  ——**已被 `docs/10` §9.2/§9.3 的既有安全测试否证**；②真实复跑新轨迹（与同轨迹口径不可比）；③接受现状（每条残余＝一次读往返）。
  **未答复期间按 ③ 推进**（该口径下 60/97 原始失败面已消除）。
- **B-1（真机确认 · 可选）**：是否由我按 `REASONIX.local.md` 打包并拉起桌面端，供你确认第八前置块观感、第九档位、
  第十多开、第十四热力图、第十六召回条、第十七待办的**实际观感**？影响：不确认也不改验收状态（代码判据已成立）。
- **U-1（第七 · 未答复）**：是否为「框架知识」新增独立 `type=framework`。**未答复期间按「不新增」推进**，不阻塞任何单元。

## 5. 未验证 / 暂缓

- 立项书验收 5（全量门禁）**已复跑**（见 §2 上一轮与 `docs/99` §一标准 5）：三项绿；lint 30 条 / repolint 7 条 / 测试失败全部落在别的会话文件。
- 超出验收判据的留白：第十「后台标签多面板并行」（A-30 收窄，需把 `ChatPaneRegion` 的 region props 按 tabId 参数化，且需真机验证）。
- 第七 的**召回质量**（索引变短后模型选事实的准确度）属真机观察项，并入 B-1 可选确认。
- 第十二 复跑**未建模**外部改动与模型自身轨迹变化，两者都只会让残余 ≥42；真实复合降幅要一次真跑才看得到。

BLOCKED: 第十二 的 ≤20 需你在 D-2 里选路（放宽证据门槛 / 真实复跑 / 接受现状 42）；
真机确认（B-1）可选，不影响已成立的 8 条代码判据。自主队列已空，等答复。
