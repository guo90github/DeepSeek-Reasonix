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

## 2. 当前单元（2026-10-03 · 用户点名三处：输入框档位开关 / 面板召回段 / 召回可读性；均已提交）

- **第九 · 档位控件挪到输入框底部并改滑动开关**（`89c58d096`）：`ShellAsyncTierSwitch.tsx`（轨随 `data-tier` 滑、三停位、写入中全禁、
  未知值回落 `off`、**读不到不渲染**）+ 同名 CSS（已登记 `check:css`），挂 `Composer.tsx` 底部 meta 行 `--shell-async` 位；**设置页原字段保留**。
- **第十六 · 召回记录并入桌面「面板」**（`72c88bb2e`）：`FooterRecallModule.tsx` 注册进 `footerPanelModules.tsx`（`id: "recall-record"`，记忆族之后）；
  `RecapRecallStrip` 加受控入口 `record`（调用方已有记录就不再问宿主）；卡片规矩 ⇒ 无记录整块不出现（含表头）。
- **第十六 · 召回记录要给人看懂**（`e50e02a13`/`b51a824d1`/`c70d701e1`，用户反馈"只有 id 看不懂"）：**记录自带短标签**——
  `MemoryRecallTurnHit`/`RecallHitView` 加 `name`/`title`（写侧车时手上就有 `hit.Memory.Name/Title`，正文仍不进记录；契约按仓库方式 `go run . -emit-contract` 重生成），
  `RecapRecallStrip` **记录自带标签优先**、`facts` 只作老记录兜底、都没有才显示 id。**根因**：记录读侧车文件（不需要控制器），而旧实现的名字要活着的控制器
  （`MemoryForTab` → `memoryForCtrl(..., false)`，`desktop/app.go:11147-11163` 在 `ctrl==nil` 时直接返回空视图）⇒ 重启后记录在、名字空 ⇒ 只剩 id。三个面（面板卡片/记忆面板召回页/回顾页）同源修好。
- **打包：默认用本机缓存的 Electron zip**（`f537fecdc`，用户建议）：packager 的 `SHASUMS256.txt` 校验每次联网、绕过缓存 ⇒ 本机两次 `ECONNRESET` 打包失败（zip 早在盘上）。
  `lib.mjs` 新增 `defaultElectronCacheRoot`/`electronZipNames`/`findElectronZipDir`/`resolveElectronZipDir`（显式变量优先，否则本机缓存；找不到仍回落下载，`universal` 缺 zip 不误用半套），`package.mjs` 默认调用并打印所用目录。
- **真机**：dev.118/119/120 均 rc 0 + 装机并列新增 + verifier **exit 0**；最新装机 `v0.0.0-dev.120`（commit `b51a824d1d9c`）。**召回名字的修复尚未打包**（阻塞见 §3/§4）。

## 3. 下一步（唯一阻塞项是别人的在飞改动；其编译通过后即可出包）

**打包被别人的未提交改动挡住**：`desktop/agentbus_apply.go`(123/127)、`agentbus_waker.go`、`heartbeat.go` 引用 `control.AgentBusControl.AgentBusAsk/AgentBusAnswer`（该接口尚无这两个方法）
⇒ `scripts/desktop-build.sh` 编不出 desktop 模块（规则上我不碰别人的文件）。**一旦它编译通过，直接跑**
`bash scripts/desktop-build.sh windows/amd64 v0.0.0-dev.121`（三个 SKIP 变量）→ 装机 → `verify-windows-portable.sh`，本单元即收口。

## 4. 待拍板（收尾后剩 ≤2 条）

- **B-2（等你定）**：**召回名字的修法已提交但还没打包**（见 §3 阻塞）。可选：① 等别的会话把 agentbus WIP 提交/修好，我再出 dev.121；② 你允许我在**临时 worktree**（HEAD+我的提交，不含他们的在飞改动）出包——该法可行但要给 worktree 接 node_modules，脆。
- **U-1（第七 · 未答复）**：是否为「框架知识」新增独立 `type=framework`。**按「不新增」结项**。
- 已决（2026-10-03）：**D-2 选 ③**（接受 37）；U-2 排队、U-3 热力图 30 天/日格、U-4 前置块默认开最多 3 轮无字符上限、U-5 建议不自动写；
  晚：**「面板」＝底部面板带卡片（`footerPanel.title`）**（召回记录按此落点，若指别处说一声即挪）。

## 5. 未验证 / 暂缓

- 立项书验收 5（全量门禁）：Go 侧三项绿、`make lint` 30 条 / `repolint` 7 条 / 失败**全在别的会话文件**（本任务 0 条）；前端全量：`72c88bb2e` 前 **401/401**，此后一次 **401/403**（唯二红＝那条 `TypeError`），修复后只复跑受影响套件，**未再跑全量**。
- **既有抖动（非本次回归）**：满负载下 `go test ./internal/agent/` 全量报 2 条 `loop_e2e_test.go`（推理循环）失败，但**干净 HEAD worktree 与我的树里单独跑都 PASS** ⇒ 判为负载抖动，必要时复跑。
- **未验证**：召回名字修复的**真机 UI**（等出包 + 重启 App）；**老记录**（本次改动前写入的侧车）仍只有 id，除非 `facts` 能解析到。
- 超出验收判据的留白：第十「后台标签多面板并行」；第八 BA4（按 verdict 触发记忆建议——现有候选机制已满足 U-5 实质）；第十二 复跑**未建模**外部轨迹变化，37 是同轨迹上界。

END-UNIT：五处用户点名（档位开关 `89c58d096`、面板召回段 `72c88bb2e`、召回可读性 `e50e02a13`+`b51a824d1`+`c70d701e1`、打包默认用缓存 `f537fecdc`）均已改完、验证、提交；
**唯一真 blocker＝别的会话的 agentbus 未提交改动让 desktop 模块编译失败**（见 §3/§4），其通过后跑 `dev.121` 即收口。
