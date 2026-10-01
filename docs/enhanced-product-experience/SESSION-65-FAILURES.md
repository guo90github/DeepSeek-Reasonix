# 会话失败 / 未执行 调研（第一份）

> 数据来源：会话转录 `20260930-053754.970846500-deepseek-deepseek-flash.jsonl`（15.1 MB / 4631 行；**只读**分析，未改动任何会话文件）。
> 口径：一次「工具调用」= assistant 的 `tool_calls` 条目，与 `role=tool` 结果按 `tool_call_id` 配对；状态取 `tool_run_state`；拦截码取 `tool_diagnostic.code`。
> 规模：**2501 次工具调用**、2013 条模型工作记录、120 条 user 消息。

## 一、总览

| 状态 | 次数 | 占比 |
|---|---|---|
| completed（完成） | 2199 | 87.9% |
| **not_started（未执行）** | 201 | 8.0% |
| **failed（失败）** | 65 | 2.6% |
| 无状态字段（特殊路径/旧记录） | 36 | 1.4% |

失败 65 次中：`exitCode != 0` 的**命令失败 41 次**；其余 **24 次是门禁/写入类失败**（没有 exit code）。

## 二、失败（failed）

### 2.1 按工具

| 工具 | 失败次数 |
|---|---|
| `bash` | 41 |
| `todo_write` | 11 |
| `complete_step` | 6 |
| `edit_file` | 5 |
| `read_file` | 2 |

### 2.2 命令失败（exitCode != 0）

| # | 轮 | 工具 | 参数（截断） | exitCode | 结果开头 |
|---|---|---|---|---|---|
| 34 | 4 | `bash` | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reaso | 1 | error: command exited: exit status 1 === types.ts SessionRecap === |
| 36 | 4 | `bash` | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reaso | 1 | error: command exited: exit status 1 1026:export type { SessionMeta, SessionReca |
| 97 | 4 | `bash` | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reaso | 1 | error: command timed out (> 2m0s) |
| 783 | 15 | `bash` | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reaso | 1 | error: command exited: exit status 1 schema_migrations cols: ['version', 'applie |
| 791 | 15 | `bash` | {"command": "cd /c/Users/guosj/AppData/Local/Temp/recap-fore | 1 | error: command exited: exit status 1 file bytes 327680 runs>=12: 138 runs carryi |
| 850 | 18 | `bash` | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reaso | 1 | error: command exited: exit status 1 # reasonix/desktop .\session_recap.go:121:2 |
| 1025 | 20 | `bash` | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reaso | 1 | error: command exited: exit status 1 # reasonix/internal/boot internal\boot\sess |
| 1119 | 21 | `bash` | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reaso | 1 | error: command exited: exit status 1 catalog cols: [] Traceback (most recent cal |
| 1376 | 25 | `bash` | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reaso | 2 | error: command exited: exit status 2 npm warn Unknown project config "confirm-mo |
| 1428 | 27 | `bash` | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reaso | 1 | error: command exited: exit status 1 decisions: {} open items: [] Traceback (mos |
| 1470 | 31 | `bash` | {"command": "E=\"C:/Users/guosj/AppData/Local/Temp/recap-pro | 1 | error: command exited: exit status 1 --- 0sanity File "<string>", line 1 // goto |
| 1507 | 32 | `bash` | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reaso | 1 | error: command exited: exit status 1 error: pathspec '3/3' did not match any fil |
| 1636 | 37 | `bash` | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reaso | 1 | error: command exited: exit status 1 修好的 press(X?) 调用：['upgradeFailureButton', ' |
| 1662 | 37 | `bash` | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reaso | 1 | error: command exited: exit status 1 Traceback (most recent call last): File "<s |
| 1664 | 37 | `bash` | {"command": "cd /c/guosj/ai/deepseek-reasonix/DeepSeek-Reaso | 1 | error: command exited: exit status 1 Traceback (most recent call last): File "<s |

### 2.3 门禁 / 写入类失败

| # | 轮 | 工具 | 拦截码 | 参数（截断） | 结果开头 |
|---|---|---|---|---|---|
| 156 | 4 | `todo_write` | `-` | {"todos": [{"activeForm": "定义 Entry 与 Record 数据模型", "co | error: todo 7 "真机：用两条真会话跑 recap-v3，把候选贴给用户看质量" is a second in_progress |
| 158 | 4 | `todo_write` | `-` | {"todos": [{"activeForm": "定义 Entry 与 Record 数据模型", "co | error: todo 5 "测试：internal/recap、boot 效果、cli、前端 tsx 全绿" removed its st |
| 159 | 4 | `todo_write` | `-` | {"todos": [{"activeForm": "定义 Entry 与 Record 数据模型", "co | error: completed todo 5 "文档：QA 手册 / CLOSE_PRD / PLAN / MANAGEMENT_PAGE |
| 253 | 5 | `edit_file` | `WRITE_EVIDENCE_STALE` | {"new_string": "DROP TABLE recap_records;\nALTER TABLE  | error: read C:\guosj\ai\deepseek-reasonix\DeepSeek-Reasonix\internal\r |
| 272 | 5 | `edit_file` | `WRITE_EVIDENCE_STALE` | {"new_string": "\trecords, err := store.List(ctx)\n\tif | error: read C:\guosj\ai\deepseek-reasonix\DeepSeek-Reasonix\desktop\se |
| 329 | 6 | `edit_file` | `WRITE_EVIDENCE_STALE` | {"new_string": " </span>}\n </p>", "old_string": " </sp | error: read C:\guosj\ai\deepseek-reasonix\DeepSeek-Reasonix\desktop\fr |
| 384 | 6 | `todo_write` | `-` | {"todos": [{"activeForm": "定义 Entry 与 Record 数据模型", "co | error: serial task list has pending work but no in_progress item |
| 385 | 6 | `todo_write` | `-` | {"todos": [{"activeForm": "定义 Entry 与 Record 数据模型", "co | error: completed todo 8 "第二步：采纳/改/弃 + 采纳写当前项目记忆 + 弃不再重生成" cannot be in |
| 874 | 18 | `complete_step` | `VERIFICATION_RECEIPT_MISMATCH` | {"result": "内核 `Store.PendingMap` + 宿主三态（stored/empty/p | error: VERIFICATION_RECEIPT_MISMATCH: receipt "r_1ef08648" is not cove |
| 875 | 18 | `complete_step` | `VERIFICATION_RECEIPT_MISMATCH` | {"receipt_ids": ["r_18fe0485", "r_97317785", "r_688a387 | error: VERIFICATION_RECEIPT_MISMATCH: receipt "r_18fe0485" is not cove |
| 903 | 18 | `edit_file` | `WRITE_EVIDENCE_STALE` | {"new_string": " keep: sessionActions.keepRecapHandoff, | error: read C:\guosj\ai\deepseek-reasonix\DeepSeek-Reasonix\desktop\fr |
| 957 | 18 | `complete_step` | `VERIFICATION_RECEIPT_MISMATCH` | {"receipt_ids": ["r_a14186bc", "r_dc765340"], "result": | error: VERIFICATION_RECEIPT_MISMATCH: receipt "r_a14186bc" is not cove |
| 958 | 18 | `complete_step` | `-` | {"receipt_ids": ["r_83da5ede"], "result": "手动生成入口落地并已提交 | error: step "p2_generate" matches pending todo 4 "P0-2：宿主 GenerateSess |
| 959 | 18 | `todo_write` | `-` | {"todos": [{"activeForm": "加内核待补读取 API", "content": "P0 | error: todo 5 "门禁（gofmt/vet/测试/前端检查/元数据闸门）与中文提交" removed its step_id " |
| 989 | 18 | `complete_step` | `VERIFICATION_RECEIPT_MISSING` | {"evidence": [{"kind": "verification", "command": "PYTH | error: VERIFICATION_RECEIPT_MISSING: evidence 1: verification command  |
| 991 | 18 | `complete_step` | `VERIFICATION_RECEIPT_MISSING` | {"evidence": [{"command": "PYTHONIOENCODING=utf-8 pytho | error: VERIFICATION_RECEIPT_MISSING: evidence 1: verification command  |
| 1495 | 32 | `edit_file` | `WRITE_TARGET_ABSENT` | {"new_string": " ( REASONIX_CACHE_HOME=\"$cache\" \"$B\ | error: WRITE_TARGET_ABSENT: read C:\guosj\AppData\Local\Temp\recap-eva |
| 1990 | 49 | `read_file` | `WRITE_TARGET_ABSENT` | {"intent": "full", "path": "C:\\guosj\\ai\\deepseek-rea | error: WRITE_TARGET_ABSENT: open C:\guosj\ai\deepseek-reasonix\DeepSee |
| 2013 | 50 | `read_file` | `WRITE_TARGET_ABSENT` | {"intent": "range", "limit": 45, "offset": 76, "path":  | error: WRITE_TARGET_ABSENT: open C:\guosj\ai\deepseek-reasonix\DeepSee |
| 2267 | 112 | `todo_write` | `-` | {"todos": [{"activeForm": "接线批量技能（按话题合成）", "content": " | error: todo 2 "本钩子与透传：useController → useAppRuntimeAdapter → AppRuntim |
| 2268 | 112 | `todo_write` | `-` | {"todos": [{"activeForm": "接线批量技能（按话题合成）", "content": " | duplicate tool result omitted (identical to call_id=call_00_wIr5A9ay6W |
| 2277 | 113 | `todo_write` | `-` | {"todos": [{"activeForm": "接线批量技能（按话题合成）", "content": " | error: todo 2 "逐层透传：useController → adapter → runtimeView → overlayBui |

## 三、未执行（not_started）

### 3.1 按工具

| 工具 | 未执行次数 |
|---|---|
| `edit_file` | 98 |
| `bash` | 87 |
| `write_file` | 7 |
| `update_goal` | 4 |
| `use_capability` | 3 |
| `ask` | 1 |
| `complete_step` | 1 |

### 3.2 按拦截码（并对比它出现在 completed 上的次数）

| 拦截码 | 未执行 | 出现在 completed（提示而非拦截） |
|---|---|---|
| `WRITE_EVIDENCE_STALE` | 53 | 0 |
| `WRITE_EVIDENCE_MISSING` | 44 | 0 |
| `READ_PARTIAL` | 0 | 409 |

### 3.3 未执行的结果开头（原因归类）

| 结果开头 | 次数 |
|---|---|
| blocked: [evidence required] edit_file targets C:\guosj\ai\d | 83 |
| blocked: this command mixes a verification check with a segm | 32 |
| blocked: this command runs a verification check after a stat | 28 |
| blocked: skipped because an earlier modification (command ef | 14 |
| blocked: skipped because an earlier modification (workspace  | 12 |
| blocked: [evidence required] write_file targets C:\guosj\ai\ | 6 |
| blocked: skipped because an earlier modification (go subcomm | 5 |
| update_goal is only available while an active goal turn is r | 4 |
| blocked: [evidence required] bash cannot declare which files | 4 |
| blocked: the trailing echo/printf of $? masks the verifier's | 2 |
| error: WRITE_EVIDENCE_STALE: old_string not found in C:\guos | 2 |
| argument validation failed for "session_read_strategy_receip | 2 |

## 四、背景分析

三类成因机制不同，改造方向也不同：

1. **读证据门禁（绝大多数）**——`READ_PARTIAL` 在未执行里 0 次但作为提示出现 409 次；真正的拦截是
   `WRITE_EVIDENCE_STALE`、`WRITE_EVIDENCE_MISSING`、`VERIFICATION_RECEIPT_MISMATCH/MISSING`。
   成因：同一份文件在一次会话里被反复读、写后再读；宿主要求「写前必须见过当前内容」，
   而我常在**同一轮里第二次改同一文件**，或改了别处导致行号漂移，于是被要求重读再重试。
   代价：每次多一次读盘 + 多一次模型往返。
2. **我自己的调用姿势（未执行的主要来源）**——后台命令、幂等重试、同一批里既写又验等被工具层直接拦下，
   结果是 `not_started`（不消耗模型时间，但让轮次变多、可读性变差）。
3. **环境与命令本身**——`exitCode != 0` 里有相当一部分是**探针的正常失败**（文件不存在、grep 未命中、
   `git log` 范围为空），另一部分是**我的命令写错**（路径、参数、编码）。

## 五、对「重大改造」的启示

- **压掉读证据往返**：给「同一轮内对同一文件的第二次编辑」一条明路（例如把刚刚写入的内容作为证据、或把行号漂移纳入锚点匹配），可直接消掉 `WRITE_EVIDENCE_STALE` 的多数。
- **把「读」的粒度说清**：`READ_PARTIAL` 409 次说明大量读取只是窗口预览；若返回时显式标注未覆盖范围与「下一段起点」，可减少重复读。
- **拦截要给下一步模板**：`not_started` 的结果文本已有原因，但缺少「改成什么样就能跑」，模型只能再猜一轮。
- **命令失败要分类**：区分「探针正常失败」与「命令写错」，否则失败率被噪声淹没。

---

明细数据见 `session-65-raw.json`（含全部 2501 次调用的状态、拦截码、耗时与结果开头）；耗时报告见 `SESSION-65-LATENCY.md`。
