# STATE · 无人值守推进器（唯一状态真源 · 覆盖式更新）

> 接手方式：只读本文件 + 第 3 节点名的 1~2 个文件。不要重读历史会话转录与 `logs/2026-10-01-第*.md`。
> 第 1 轮的「第 N 轮日志」只作历史；`logs/journal.md` 为追加式流水。

## 1. 需求进度表（9 条 · 状态 / 证据）

| 编号 | 状态 | 证据（提交 / 命令 / 关键文件） |
|---|---|---|
| 第七 记忆分层 | **已验收** | 代码 `530f80d43`；占用对比与落地见 `docs/50` §十：常驻段 改造前 **42,749** B → 改造后 **16,307** B，比值 **0.381 ≤ 1/2 ✓**；第一层改为「标签 + ≤50 rune 摘要 + 事实 id 句柄」（`internal/memory/index.go` 的 `maxFirstLayerRunes`/`firstLayerText`），索引 39,080 → 13,749 B、最长行 1,162 → 211 B；第二层走 `memory read <id>` / `search`。验证：`go test ./internal/memory/`、`go test ./internal/control/`（32.6s）、`go test -v ./internal/boot/`（150.1s）全绿 |
| 第八 前置/后置 | **已验收（代码侧）** | 后置：`530f80d43` 的 BA1a/BA1b（`internal/agent/turn_outcome.go`、`internal/control/{turn_outcome_record,turn_progress}.go`）——回合结束把 `TurnOutcome`（verdict/计数/义务）落进会话侧车，内容无关。前置：**BA2 已落地**（`internal/control/turn_progress.go` + 接线 `input.go:218`）——回合尾注入 `<turn-progress>`（最近 3 轮 + 未满足义务计数）；**已按 U-4 去掉块预算/行上限**，改判词枚举归一化。验证：`internal/boot` 的 `TestEffectTurnProgressRidesTheBodyNotThePrefix` PASS（前缀与工具面逐字节不变）+ `internal/control` 三条（内容只来自记录、窗口 3 轮、无上限）PASS。真机观感并入 B-1 |
| 第九 异步/档位 | 待验收（缺真机） | `ce72be09a`（`internal/agent/shell_async.go`、`[agent] shell_async`）+ `internal/boot/loop_async_effect_test.go` 实测 **2137ms→18ms**；档位控件 `ShellAsyncTierField.tsx` + 11 测试（`cffe2df3a`）。**缺**：真机观感（档位默认 off，新建会话生效） |
| 第十 多开 | 部分（按 A-30 收窄） | `32ee36c82`（`lib/viewPlacement.ts`、`store/viewPlacements.ts`、`FloatingViewLayer.tsx` + 2 测试）+ 挂载点 `cffe2df3a`。**缺**：后台标签完整面板（需把 `ChatPaneRegion` region props 按 tabId 参数化）、真机 #2/#5 |
| 第十一 双主线 | 待验收（缺真机） | `530f80d43`：`internal/event/delivery.go` 的 `DeliveryClass` 四分类 + 唯一入口守卫；U-2 已决（**排队**） |
| 第十二 报错根治 | 待验收（降幅**未达**判据） | `759810f30`（20 文件：写入即证据 / 拦截即给下一步 / 作业回执闭环 / 起动≠跑过）+ `internal/boot/write_reread_effect_test.go`；根因图谱 `docs/10`。**复跑已做**（`docs/10` §十一，同轨迹重放原转录）：基线 97（53 STALE + 44 MISSING，与 `SESSION-65-FAILURES.md` §3.2 逐项一致）→ **残余 42**（降幅 57%），判据要 ≤20 ⇒ **未达标**；残余 42 = 33「目标文本从未被模型写过」+ 4 `bash` 不透明写者 + 5「行段部分覆盖」。原因：设计期押的 A-2/A-4 两手已被 §9.2/§9.3 用安全理由否证，留下的 A-1 只覆盖 55 条。**待拍板 D-2** |
| 第十四 回顾展示 | 待验收（缺真机） | `cffe2df3a`：`RecapHeatmap.tsx/.css`、`RecapRecallStrip.tsx/.css`、`lib/recapHeatmap.ts`（测试 21+18+21+6 PASS）；U-3 已决（30 天 / 日格） |
| 第十六 召回记录 | 待验收（缺真机） | 内核 `530f80d43`；前端召回条并入既有记忆面板 `cffe2df3a`；`desktop/recall_record_view.go`(+test) |
| 第十七 待办队列 | 待验收（缺真机） | `014f3f71b`（19 文件：批次身份 S1、队列+归档 S3、归档界面+S2 重放，含 agent.go/boot.go 接线）；三类混乱回归测试由红转绿（`todo-queue-repro` 17/17、`TodoPanel` 35/35、`todo-archive-section` 11/11、宿主 `todo_history_replay_test.go`） |

## 2. 当前单元（2026-10-03 · 本轮）

**阶段收口：全量门禁复跑（2026-10-03 实测）。** ✓ `go build ./...`(0) ✓ `go vet ./...`(0) ✓ `make frontend-check`(0)；
`make lint` 30 条既有（intrange 21 / modernize 8 / staticcheck 1）与 `repolint` 7 条（`desktop/tabs.go`、`desktop/unattended_*.go`、`internal/boot/boot.go`、`internal/serve/serve.go`）**逐条落在别的会话文件，本任务改动面 0 条**；
`go test ./internal/...` 124 包 ok / 7 失败（symlink 权限 5 + `acp`/`agentbus` 各 1，均别的改动面）；`desktop` 模块 8 包 ok / 1 失败（browser upload ×2、packaging ×2、ModelSettings 既有红②、ResumeUnattended ×1）。结果逐条写进 `docs/99-总结报告.md`。

## 3. 下一步（下一轮直接照做）

**自主队列已空 —— 等答复（见 §4）。** 剩下两项都不是"没人也能做"：
- 真机验收（B-1）：第八前置块观感、第九档位、第十 #2#5、第十四观感、第十六面板、第十七三条 —— 需人眼判定，且需先有含本任务改动的新构建；
- 第十二是否放宽证据门槛（D-2）：复跑已到 42，判据 ≤20 只能靠放宽门禁或真实复跑新轨迹。

唯一剩下的自主候选是第十「后台标签完整面板」（把 `ChatPaneRegion` 的 region props 按 tabId 参数化），
但它是结构性前端改动、且**没有真机就无法验证** ⇒ 建议等 B-1 反馈后再动（在台账里留着，不算丢）。

不要一轮做两件。

## 4. 待拍板（≤3 条）

- **B-1（真机验收 · 需人）**：第九 / 第十 #2#5 / 第十四观感 / 第十六 / 第十七三条 的「已验收」需人眼判定，
  且需先有含本任务改动的新构建（本机打包 SOP 见 `REASONIX.local.md`）。问题：是否由我按 SOP 打包一份
  并拉起桌面端供你复测？影响：这 5 条只能停在「待验收」。
- **U-1（第七，未答复）**：是否为「框架知识」新增独立 `type=framework`（现状取「不新增」，沿用 `project`+relevant）。
  **未答复期间按「不新增」推进**，不阻塞任何单元。
- **D-2（第十二，需拍板）**：复跑 **97 → 42**（判据 ≤20）。残余八成是「目标文本从未被模型写过」；要压到 ≤20 只有三条路：
  ①放宽证据门槛（`old_string` 唯一匹配放行，或拦截时宿主代读并登记）——**已在 `docs/10` §9.2/§9.3 被既有安全测试否证**；
  ②真实复跑新轨迹（与同轨迹口径不可比）；③接受现状（每条残余＝一次读往返）。
  问题：是否放宽？**未答复期间按 ③ 推进**（该口径下 55/97 原始失败面已消除）。

## 5. 未验证 / 暂缓

- 立项书验收 5（全量门禁）**已复跑**，逐条结果见 §2 与 `docs/99` §一标准 5；**五项里三项绿**（build/vet/frontend-check），lint 与测试的红全部落在别的会话文件。
- 真机未做：第八前置块观感、第九、第十 #2/#5、第十四、第十六、第十七三条。
- 第十后台标签完整面板未做（A-30 收窄后属未做的原始需求 #5 部分；需真机才能验证，建议 B-1 之后再做）。
- 第七 达标已复测（`docs/50` §10.6，0.381 ≤ 1/2）；**召回质量抽查未做**——索引变短后模型选事实的准确度属真机观察项，并入 B-1。
- 第十二 复跑**未建模**外部改动（格式化/别的会话改动）与模型自身轨迹变化，两者都只会让残余 ≥42；真实复合降幅要一次真跑才看得到。

BLOCKED: 第八/第九/第十/第十四/第十六/第十七 的「已验收」需真机人眼判定（B-1：是否由我按 `REASONIX.local.md` 打包并拉起桌面端供你复测）；
第十二 的 ≤20 需你在 D-2 里选路（放宽证据门槛 / 真实复跑 / 接受现状 42）。自主队列已空，等答复。
