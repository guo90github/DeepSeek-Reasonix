# STATE · 无人值守推进器（唯一状态真源 · 覆盖式更新）

> 接手方式：只读本文件 + 第 3 节点名的 1~2 个文件。不要重读历史会话转录与 `logs/2026-10-01-第*.md`。
> 第 1 轮的「第 N 轮日志」只作历史；`logs/journal.md` 为追加式流水。

## 1. 需求进度表（9 条 · 状态 / 证据）

| 编号 | 状态 | 证据（提交 / 命令 / 关键文件） |
|---|---|---|
| 第七 记忆分层 | 待验收（缺 1 个数字） | 代码已提交 `530f80d43`（`internal/memory/{activation,remember,remember_policy,store_v2,auto_recall}.go`、`internal/control/memory.go`）；设计 `docs/50` §六–§九。**缺**：常驻上下文记忆字节「改造前/后」对比数字 |
| 第八 前置/后置 | 待验收（BA2 留白） | `530f80d43` 含 BA1a/BA1b（`internal/agent/turn_outcome.go`、`internal/control/{turn_outcome_record,turn_progress}.go`），回合结束链路可见。BA2 前置块（U-4 已决：默认开、最多 3 轮、不设字符上限）未做 ⇒ 目前不加分也不扣判据 |
| 第九 异步/档位 | 待验收（缺真机） | `ce72be09a`（`internal/agent/shell_async.go`、`[agent] shell_async`）+ `internal/boot/loop_async_effect_test.go` 实测 **2137ms→18ms**；档位控件 `ShellAsyncTierField.tsx` + 11 测试（`cffe2df3a`）。**缺**：真机观感（档位默认 off，新建会话生效） |
| 第十 多开 | 部分（按 A-30 收窄） | `32ee36c82`（`lib/viewPlacement.ts`、`store/viewPlacements.ts`、`FloatingViewLayer.tsx` + 2 测试）+ 挂载点 `cffe2df3a`。**缺**：后台标签完整面板（需把 `ChatPaneRegion` region props 按 tabId 参数化）、真机 #2/#5 |
| 第十一 双主线 | 待验收（缺真机） | `530f80d43`：`internal/event/delivery.go` 的 `DeliveryClass` 四分类 + 唯一入口守卫；U-2 已决（**排队**） |
| 第十二 报错根治 | 待验收（缺降幅数字） | `759810f30`（20 文件，写入即证据 / 拦截即给下一步 / 作业回执闭环 / 起动≠跑过）+ `internal/boot/write_reread_effect_test.go`；根因图谱 `docs/10`。**缺**：按 `SESSION-65-FAILURES.md` 口径复跑，读证据门类拦截 97 → ≤20（`docs/10` §8.5/§9.1 自认未证） |
| 第十四 回顾展示 | 待验收（缺真机） | `cffe2df3a`：`RecapHeatmap.tsx/.css`、`RecapRecallStrip.tsx/.css`、`lib/recapHeatmap.ts`（测试 21+18+21+6 PASS）；U-3 已决（30 天 / 日格） |
| 第十六 召回记录 | 待验收（缺真机） | 内核 `530f80d43`；前端召回条并入既有记忆面板 `cffe2df3a`；`desktop/recall_record_view.go`(+test) |
| 第十七 待办队列 | 待验收（缺真机） | `014f3f71b`（19 文件：批次身份 S1、队列+归档 S3、归档界面+S2 重放，含 agent.go/boot.go 接线）；三类混乱回归测试由红转绿（`todo-queue-repro` 17/17、`TodoPanel` 35/35、`todo-archive-section` 11/11、宿主 `todo_history_replay_test.go`） |

## 2. 当前单元（2026-10-03 · 本轮）

**台账对齐现实并提交。** 实测：`git status --porcelain` **0 项**（工作台干净，别的会话的在制品也已落盘），
本任务 7 个提交全部在 HEAD 祖先链上（`12f093b85` / `759810f30` / `ce72be09a` / `014f3f71b` / `530f80d43` /
`32ee36c82` / `cffe2df3a`），产出目录 39 个文件已被 git 跟踪。
⇒ 上一版台账 §2 的「131 项未提交 / 至今零提交」是**过期描述**（写在收口之前），本轮改正，避免下个会话误判。
本轮提交哈希以 `git log -1 --format=%h -- docs/enhanced-product-experience/logs/` 为准（该提交只含本台账与流水两个文件）。

## 3. 下一步（下一轮直接照做）

**单元 A（首选）**：量出第七的常驻占用对比数字并固定下来。
- 口径：`Set.BackgroundDataBlock()`（改造后＝索引行 + 仅 pinned 正文，即 `internal/memory/memory.go:159`）
  vs 改造前（改造前渲染路径从 `git log -- internal/memory/memory.go` 的历史版本里取，不许凭印象）
  在同一份真实记忆库上的字节数；写进 `docs/50` 新增一节，并留可复跑命令。
- 完成标志：`docs/50` 有前后数字 + ≤1/2 的结论（或如未达标，如实记录并给方案）；台账第 1 行翻「已验收」。

**单元 B（并列候选）**：第十二降幅复跑，方法先定死再跑（`docs/10` §8.5/§9.1 的口径争议点：
「窗口未覆盖」与「写入产出行」各占多少 ⇒ 决定 A-1 能覆盖多少）。语料＝产出目录 `session-65-raw.json`
（`args` 被截断到 ~201 字符 ⇒ 精细复跑须用原转录 `.../sessions/20260930-053754.970846500-*.jsonl`，15.2MB，只读）。

任选其一即可，不要一轮做两件。

## 4. 待拍板（≤3 条）

- **B-1（真机验收 · 需人）**：第九 / 第十 #2#5 / 第十四观感 / 第十六 / 第十七三条 的「已验收」需人眼判定，
  且需先有含本任务改动的新构建（本机打包 SOP 见 `REASONIX.local.md`）。问题：是否由我按 SOP 打包一份
  并拉起桌面端供你复测？影响：这 5 条只能停在「待验收」。
- **U-1（第七，未答复）**：是否为「框架知识」新增独立 `type=framework`（现状取「不新增」，沿用 `project`+relevant）。
  **未答复期间按「不新增」推进**，不阻塞任何单元。

## 5. 未验证 / 暂缓

- 立项书验收 5（全量门禁）**本轮未复跑**：`go build ./...`、`go vet ./...`、`make frontend-check` 上次绿；
  `make lint` 18 条既有、`go test ./internal/... ./desktop/...` 失败全属既有类别（symlink 权限 / E2E / vision / packaging）。
- 真机未做：第九、第十 #2/#5、第十四、第十六、第十七三条。
- 第八 BA2 前置块未做（U-4 已决参数：默认开 / 最多 3 轮 / 不加字符上限）。
- 第十后台标签完整面板未做（A-30 收窄后属未做的原始需求 #5 部分）。

END-UNIT: 台账已对齐真实提交状态并提交；下一步＝单元 A（第七 常驻占用前后对比数字）。
