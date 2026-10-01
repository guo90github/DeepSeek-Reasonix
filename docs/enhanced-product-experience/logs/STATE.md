# STATE · 无人值守推进器（人工建立的起始台账，2026-10-01）

> 这是本任务的**唯一状态真源**（新提示词的「单一真源」约定）。每轮只更新本文件 + 追加 `journal.md` 一行。
> 上一份形态是 24 份「第 N 轮进度日志」（`logs/2026-10-01-第*.md`）——**只作历史，不要重读全量**。

## 1. 需求进度表（9 条 · 状态 / 证据）

| 编号 | 状态 | 证据（文档章节 / 文件 / 验证） |
|---|---|---|
| 第七 记忆分层 | 部分已提交 | `docs/50` §六–§九；**内核余量已提交 `530f80d43`**（B1 第18轮 type×scope 矩阵+写入门槛、B2 第20轮召回记录、B3 第21轮技能使用记录）：`internal/memory/{activation,remember,remember_policy,store_v2,auto_recall}.go`、`internal/agent/{memory_recall_record,skill_use_record}.go`、`internal/skill/use_record.go`、`internal/control/memory.go`。**缺**：常驻上下文占用的前后对比数字 |
| 第八 前置/后置 | 部分已提交 | `docs/70`；**BA1a/BA1b 已提交 `530f80d43`**（`internal/agent/turn_outcome.go`、`internal/control/{turn_outcome_record,turn_progress}.go`）。**缺**：BA2 前置块（U-4 已决：默认开、最多 3 轮、不设字符上限）+ `internal/boot` effect test |
| 第九 异步/档位 | 部分已提交 | `docs/20` §六/§八/§九；**批 2 已提交 Go 侧**（`shell_async.go`+`internal/config` 的 `[agent] shell_async` + `internal/boot/loop_async_effect_test.go`，effect test 实测 2137ms→18ms；作业回执闭环见批 1）。**缺**：档位前端控件（并入批 6 收口）、档位热切换（改档需新建会话）、宿主层 Go 测试、真机 |
| 第十 多开 | 部分已提交 | `docs/30` §九/§十；第11轮 A1 接线：**批 4 提交** `lib/viewPlacement.ts`、`store/viewPlacements.ts`、`components/FloatingViewLayer.tsx`(+2 测试)；挂载点 `AppRuntimeView.tsx`（浮层 11 处 / 待办·回顾 4 处＝混合）与 `TabBar.tsx`（归属未明）并入批 6 收口。**缺**：后台标签完整面板（region props 按 tabId 参数化）、吸附/上限/快捷键、真机 #2/#5 |
| 第十一 双主线 | 部分已提交 | `docs/50` §九 B4 第22轮：`internal/event/delivery.go` 的 `DeliveryClass` 四分类已提交 `530f80d43`；U-2 已决（**排队**），收口时按此调整 |
| 第十二 报错根治 | 部分已提交 | **批 1 已提交 `759810f30`**（20 文件 +858/−36，四包测试全绿）：A-1 写入即证据（`internal/evidence/write_effect.go`+`internal/agent/write_reread.go`+`internal/boot/write_reread_effect_test.go`）、A-3 shell 契约提示、A-23 回执闭环（`shell_job_receipts.go`）、A-24「起动≠跑过」。**缺**：降幅复跑（A+B 97→≤20，`docs/10` §8.5 自认未证）、新建文件后立刻再写（A-2 已裁定不落地） |
| 第十四 回顾展示 | 待提交 | `docs/60` §四：B6a–B6d 全部完成（`RecapHeatmap.tsx/.css`、`RecapRecallStrip.tsx/.css`、`lib/recapHeatmap.ts`；测试 21+18+21+6 PASS）。**前端件并入批 6 收口**（`SessionRecapPage.tsx` 等共享文件逐 hunk 混合）。**缺**：真机观感（U-3 已决：维持 30 天/日格） |
| 第十六 召回记录 | 部分已提交 | `docs/50` B2/B3 的内核侧已提交 `530f80d43`；**待提交**：`desktop/recall_record_view.go`(+test)、召回条组件（批 6）。**缺**：真机 |
| 第十七 待办队列 | 部分已提交 | **批 3 已提交 `014f3f71b`**（19 文件 +892：批次身份 S1、队列+归档 S3、归档界面+S2 重放，含 agent.go/boot.go 内核接线）；`TodoPanel.test.tsx` 两条旧契约断言已改对（既有红 ④ **关闭**）。**缺**：真机三条 |

## 2. 当前单元

**按 `logs/COMMIT-PLAN.md` 执行分批提交**（9 个批次，每批＝一个可提交单元；计划里另有「共享文件不提交…」清单与「既有红」清单，提交前必读）。
工作树现有 131 项未提交（~76 改 + ~55 新增），而本任务**至今零提交**——一次误操作或别的会话的 `git checkout` 就能全部丢失。
提交纪律：只 `git add` 该批点名的文件（**禁止 `git add -A`**）、中文提交信息 + `Verified:` / `Documentation-impact:` / `Cache-impact:`，短哈希回填本表「证据」列。
本轮已顺手修掉一条卡验收的既有红：`12f093b85`（版本比较 `dev.9` vs `dev.10`，影响自我升级挑版本）。

## 3. 下一步（按序）

1. 分批提交（建议批次：第十二 → 第九 → 第十七 → 第十 → 第七/十一/十六 → 第十四 → 第八），每批先跑相关包测试再提交。
2. **第八 BA2**（前置块 + `internal/boot` 的「前缀与工具面逐字节不变」effect test），随后 BA3/BA4 可选。
3. **第七 常驻占用对比数字**（改造前后各量一次，落进 `docs/50`；这是唯一缺量化数字的验收项）。
4. 真机验收：第十七三条 → 第十四观感 → 第十六 → 第九 → 第十 #2/#5。
5. **第十二 降幅复跑**（按 `SESSION-65-FAILURES.md` 口径：`WRITE_EVIDENCE_STALE`+`MISSING` 97 → ≤20）。
6. 全量门禁（立项书验收 5）：`go build ./...`、`go vet ./...`、`make lint`、`go test ./internal/... ./desktop/...`、`make frontend-check`。
7. 写 `docs/99-总结报告.md`，本台账置终态。

## 4. 决策记录（用户 2026-10-01 拍板）

已决：

- **U-2**（第十一）**已决：排队** —— 外来引导与用户当前输入同时到达时不插队，按到达顺序处理（`docs/50` §九.5）。
- **U-3**（第十四）**已决：维持现状** —— 热力图默认窗口 30 天、粒度日格（`docs/60` §五）。
- **U-4**（第八）**已决：前置块默认开、最多 3 轮**；用户未设字符预算 ⇒ 实现时不加字符上限（`docs/70` §五）。
- **U-5**（第八）**已决：落地** —— 后置在 `delivered` 后自动建议记忆候选，按 R-48 去重 + 每会话上限（`docs/70` §五）。
- **U-6**（运行机制，已决）：任务的 `goal` 契约字段原**无 UI 入口**，现状值是占位符。
  已决处置：编辑器新增「目标契约」字段（`273221f73`，需新构建才可见），用户填入按 STATE.md 第 1 节
  9 条需求写成的真契约；若改回纯 tick 驱动（每个 tick 都提交提示词、不做 Goal 锚定），把该框留空即可。
  生效前提：打包/换用含 `273221f73` 的构建 → App 重启 → 无人值守总开关打开（开关重启后生效）→ 任务启用。

仍未决（**唯一一项**，不阻塞当前单元）：

- **U-1**（第七）：是否给"框架知识"新增独立 `type=framework`？现状取"不新增"（沿用 `type=project`+relevant，A-33）。
  若要用索引一眼区分框架知识，才需要新增 type 并同步四处封闭值域。**未答复期间按"不新增"推进**。

## 5. 未验证 / 暂缓

- 立项书验收 5（全量门禁）**未跑**；`docs/99-总结报告.md` 未写。
- 真机验收未做：第九、第十 #2/#5、第十四、第十六、第十七。
- 既有欠账（与本任务无关但会污染全量）：`TodoPanel.test.tsx` 两条旧断言（批 3 修）、`TestModelSettingsQueuedFollowupAppliesLatestBeforeDispatch`（待处理）、symlink/打包契约类环境失败（不修）、theme-token 2 处（待查）、`SettingsPanel.tsx` 超预算（别的会话在改）。
- 已修：`TestComparePortableVersionsOrdersNumericSegmentsAndPrerelease`（`12f093b85`，段内数值比较）。
- 上轮遗留：`desktop/frontend/src/components/SettingsPanel.tsx` 与 `src/lib/useController.ts` 的 repolint 超预算（别的会话在改）。

END-UNIT: 九条需求代码侧全部提交完毕（7 个 commit，见 `docs/99-总结报告.md` §二）；tip 编译已复验。下一步＝真机与两个未证数字（第十二降幅复跑、第七常驻占用对比）+ 运营前置（换构建→填目标契约→开总开关重启）。
