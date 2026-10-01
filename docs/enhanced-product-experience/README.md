# 会话调研（为「两个基础功能重大改造」准备）

来源：**会话的转录**（`%APPDATA%
easonix\projects\c--guosj-ai-deepseek-reasonix-deepseek-reasonix\sessions60930-053754.970846500-deepseek-deepseek-flash.jsonl`，15.1 MB / 4631 行）。
全过程**只读**：没有修改任何会话文件（宿主对会话文件的外部改动会生成冲突副本，也不允许）。

| 文件 | 内容 |
|---|---|
| `SESSION-65-FAILURES.md` | **第一份**：失败（`failed` 65 次）与未执行（`not_started` 201 次）的分类、样本与背景分析 |
| `SESSION-65-LATENCY.md` | **第二份**：耗时调研——shell 毫秒分布（456/1242 次 > 1s）、模型工作侧的相对排序，以及两处**数据缺口** |
| `session-65-raw.json` | 明细：全部 2501 次工具调用的状态/拦截码/耗时/结果开头 + 2013 条模型工作记录 |

## 口径（复现方法）

1. 转录每行一条消息：`role=assistant` 带 `tool_calls`，`role=tool` 带 `tool_call_id`；
   两者按 id 配对即得一次「工具调用」，状态取 `tool_run_state`（`completed` / `not_started` / `failed`）。
2. 拦截原因取 `tool_diagnostic.code`（`READ_PARTIAL` / `WRITE_EVIDENCE_STALE` / `WRITE_EVIDENCE_MISSING` / `VERIFICATION_RECEIPT_*` …）。
3. shell 耗时取 `tool_execution.durationMs`（毫秒，可信）；模型侧取 `assistant.workDurationMs`（**单位存疑**，见第二份报告第四节）。

## 三条最重要的结论（给改造用）

1. **时间花在模型侧，不在 shell**：shell 中位 792 ms；真正的时间是「模型 → 工具批 → 模型」的往返，轮次越多越慢。
2. **最大的一类摩擦是「读证据门禁」**：`WRITE_EVIDENCE_STALE` 57 + `WRITE_EVIDENCE_MISSING` 44 + 回执类 5，全部来自「同一轮里第二次改同一文件 / 行号漂移」，每次都要重读再重试。
3. **数据缺口本身要修**：进程内工具没有耗时字段；`workDurationMs` 的名字与数量级不符 —— 不修，下一次调研还是只能猜。
