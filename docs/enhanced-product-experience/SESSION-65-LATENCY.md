# 会话耗时调研（第二份）

> 数据来源：会话转录（**只读**）。口径：`tool_execution.durationMs`（shell 专有，1242/1324 条带值，单位确认为毫秒）；
> `assistant.workDurationMs`（模型工作侧，2013 条，**单位无法自证**，见第四节）。
> 进程内工具（`read_file` / `write_file` / `edit_file` / `grep` / `glob` / `todo_write` / `ask` 等）**不落毫秒字段**，无法量化。

## 一、总览

- 有毫秒记录的工具调用：**1242** 次，其中 **456** 次 > 1000ms（36.7%）。
- 模型工作记录：**2013** 条；数值区间 1,855 – 1,929,815（单位见第四节）。
- 结构判断：**shell 的绝对耗时很小**（中位数 792 ms），**时间主要花在模型侧**（思考 + 生成长答案 + 工具批往返）。

## 二、Shell 调用耗时（毫秒，可信）

| 区间 | 次数 | 占已计时 |
|---|---|---|
| <1s | 786 | 63.3% |
| 1–5s | 227 | 18.3% |
| 5–15s | 108 | 8.7% |
| 15–60s | 117 | 9.4% |
| >60s | 4 | 0.3% |

### 2.1 最慢的 20 次（> 1000 ms）

| # | 轮 | 耗时 | exitCode | 命令（截断） | 结果开头 |
|---|---|---|---|---|---|
| 2336 | 114 | **131088 ms** | 1 | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix && go test ./internal | error: command timed out (> 2m0s) |
| 97 | 4 | **123063 ms** | 1 | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix && go test ./internal | error: command timed out (> 2m0s) |
| 1983 | 48 | **89837 ms** | 0 | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix/desktop && go test ./ | --- FAIL: TestModelSettingsQueuedFollowupAppliesLatestBefore |
| 780 | 15 | **82026 ms** | 0 | {"command": "for d in \"C:/Users/guosj/AppData/Roaming/reasonix\" \"C:/Users/guosj/Ap | == C:/Users/guosj/AppData/Roaming/reasonix AGENTS.md agent_m |
| 1866 | 44 | **57384 ms** | 0 | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix/desktop && go test ./ | FAIL FAIL reasonix/desktop 42.198s FAIL [receipt r_051c1ee4] |
| 1868 | 44 | **47077 ms** | 0 | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix/desktop && go test ./ | --- FAIL: TestModelSettingsQueuedFollowupAppliesLatestBefore |
| 370 | 6 | **43271 ms** | 0 | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix/desktop/frontend && n | [receipt r_ce1eb4a4] |
| 1590 | 36 | **39763 ms** | 0 | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix && go vet ./internal/ | ok reasonix/internal/recap 0.977s ok reasonix/internal/contr |
| 1261 | 23 | **37450 ms** | 0 | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix && go test ./internal | ok reasonix/internal/recap 0.861s ok reasonix/internal/contr |
| 1307 | 24 | **37093 ms** | 0 | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix && go test ./internal | ok reasonix/internal/recap 0.831s ok reasonix/internal/contr |
| 1176 | 21 | **33821 ms** | 0 | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix && go test ./internal | ok reasonix/internal/recap 0.839s ok reasonix/internal/contr |
| 1092 | 21 | **33729 ms** | 0 | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix && go test ./internal | ok reasonix/internal/recap 0.828s ok reasonix/internal/contr |
| 561 | 11 | **32768 ms** | 0 | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix/desktop/frontend && n | [receipt r_9a583b29] |
| 511 | 11 | **32644 ms** | 0 | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix/desktop/frontend && n | src/components/SessionRecapPage.tsx(3,57): error TS2724: '". |
| 1703 | 37 | **32051 ms** | 0 | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix && make frontend-chec | cd desktop/frontend && npx tsc --noEmit npm warn Unknown pro |
| 2290 | 113 | **31710 ms** | 0 | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix/desktop/frontend && n | duplicate tool result omitted (identical to call_id=call_00_ |
| 520 | 11 | **31510 ms** | 0 | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix/desktop/frontend && n | [receipt r_e6c65e40] |
| 1519 | 33 | **31336 ms** | 0 | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix && python \"C:/Users/ | [fact] 要点跳转经 aimAt() 写入 transcriptScrollWriter 的 scrollTo+to |
| 1571 | 36 | **30187 ms** | 0 | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix && go vet ./internal/ | ok reasonix/internal/recap 0.809s ok reasonix/internal/boot  |
| 635 | 13 | **29739 ms** | 0 | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reasonix/desktop/frontend && n | [receipt r_c0c67eb1] |

**构成**：`go test` × 143、`npx` × 118、`其它` × 74、`go build` × 59、`go run` × 41、`python` × 8、`git` × 7、`make` × 3、`npm` × 3。

读数：>1s 的几乎全是**编译/测试/打包类命令**（`go test`、`go build`、`npx tsc`、`go run ./tools/repolint`、`bash scripts/...`），
这一类从零开始编译或全仓扫描，几秒到几十秒属正常；**可优化的是同一轮里重复跑同一条昂贵命令**，而不是命令本身。

## 三、最慢的 shell 调用示例与背景

| # | 耗时 | 背景（该次调用的作用） |
|---|---|---|
| 2336 | 131088 ms | 全量 Go 测试（internal/recap + boot） |
| 97 | 123063 ms | 桌面端 Go 测试（Recap|Contract|Owner） |
| 1983 | 89837 ms | 打包/构建脚本 |
| 780 | 82026 ms | repolint 全仓扫描 |
| 1866 | 57384 ms | 前端 tsc 类型检查 |
| 1868 | 47077 ms | 契约重生成（go run . -emit-contract） |
| 370 | 43271 ms | npx tsx 页面套件 |
| 1590 | 39763 ms | git 大仓检索 |

## 四、模型工作侧的时长（`workDurationMs`）

- 条数 **2013**，最小 1,855、中位 201,772、p90 758,487、最大 1,929,815。
- **单位不可自证**：按毫秒解释，合计约 **10,536 分钟（175 小时）**，明显不可能；
  按**微秒**解释，合计约 **10.5 分钟**、中位 0.20 s、最大 1.9 s —— 数量级合理，但字段名写着 `Ms`。
- 因此本报告**只把它当相对量**（排序/占比），不声称绝对时长。

### 4.1 相对最慢的 15 条（连同它那一批的工具）

| 轮 | 原始值 | 推理态 | 同批工具 | 可见文本开头 |
|---|---|---|---|---|
| 39 | 1,929,815 | empty | — | Strategy compliance: ✓（P0-2 只做还账这一件事；P2-6 本行） ## 账还完了：`repolint: clean (1208 bas |
| 39 | 1,923,652 | complete | `todo_write` | Committed `0c1694a9d` ✓ (13 files, +184/−245 net −61 lines). Working tree clean. |
| 39 | 1,916,821 | empty | `bash` |  |
| 39 | 1,872,379 | empty | `bash`、`bash`、`bash` |  |
| 39 | 1,867,181 | complete | `bash`、`bash` | All work restored ✓ (10 modified + 3 new files). Final verification before commi |
| 39 | 1,860,467 | complete | `bash` | At HEAD (my changes stashed): - `message-settle-hardening.test.ts`: **8 FAILED,  |
| 39 | 1,848,361 | complete | `bash` | `message-settle-hardening.test.ts` has 8 failures — it does NOT import my module |
| 39 | 1,840,334 | complete | `bash` | **repolint: clean (1208 baselined findings)** ✓ — the account is settled with a  |
| 39 | 1,831,004 | empty | `bash` |  |
| 39 | 1,826,223 | empty | `bash` |  |
| 39 | 1,821,079 | empty | `read_file` |  |
| 39 | 1,815,794 | complete | `bash` | ⓘ `-update` rewrote the **entire** baseline to the current state: 101 insertions |
| 39 | 1,806,406 | complete | `bash` | `browser-panel.test.tsx` fails with a genuine assertion failure (`the empty stat |
| 39 | 1,796,359 | complete | `bash` | The user says: **don't touch useController**; **widen that gate / mark it as don |
| 38 | 1,637,625 | complete | `wait` | 没卡住，是我这条命令的**输出方式**骗了眼睛：我把 runner 接到 `/ tail -25`，`tail` 只在进程结束时才吐东西；而 `scripts/ |

## 五、背景分析

1. **模型侧是主成本**：每条 assistant 记录 = 一次「模型 → 工具批 → 模型」往返；同一轮里往返越多，轮时间越长。
2. **长会话 prefill 抬升**：这与既有结论一致（上下文越长，每轮首反馈越慢，服务端成本，非本地缺陷）。
3. **昂贵命令重复执行**：`go test` / `tsc` / `repolint` 在同一轮里被反复调用，每次从零编译或全仓扫描。
4. **数据缺口本身就是发现**：进程内工具没有毫秒字段；`workDurationMs` 的名字与数量级不符 —— 想量化「慢在哪」，先补这两处。

## 六、对「重大改造」的启示

- **压轮次优先于压单轮**：模型侧占多数时间，把「读 → 改 → 验」的多次往返合成一次，收益最大。
- **昂贵命令加同轮缓存**：输入未变时复用上一次结果（`go test`/`tsc`/`repolint`）。
- **工具层补 `durationMs`**：读/写/检索的耗时目前不可见，补上后本报告可覆盖全部调用。
- **修 `workDurationMs` 的语义**：单位、起点、是否含 prefill 都要写清，否则监控与优化都建立在猜上。

---

配套失败报告见 `SESSION-65-FAILURES.md`；明细数据见 `session-65-raw.json`。
