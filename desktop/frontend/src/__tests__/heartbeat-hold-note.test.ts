import assert from "node:assert/strict";
import { taskHoldNote } from "../custom/features/heartbeat/heartbeat.presentation";
import type { HeartbeatTranslator } from "../custom/features/heartbeat/heartbeat.i18n";

// 任务按计划"已到期"说明它没跑，但说不出为什么。宿主机把原因算在任务上（lastHold）：
// 可能是设计（正在跑的 Goal 自己驱动这一轮、有更新在等安装），也可能是故障（打不开话题）。
// 面板只转述它，不替它分类。
const t = ((key: string, params?: Record<string, string>) =>
  params ? key + ":" + Object.values(params).join("|") : key) as unknown as HeartbeatTranslator;

assert.equal(taskHoldNote({}, t), null, "a task nobody held says nothing");
assert.equal(taskHoldNote({ lastHold: "   " }, t), null, "whitespace is not a reason");
assert.match(taskHoldNote({ lastHold: "a running Goal drives this session's own turns" }, t) || "", /heartbeat.holdNote:/, "a held task names the host's reason");
const withTime = taskHoldNote({ lastHold: "could not open the task's tab", lastHoldAt: Date.now() - 3600000 }, t) || "";
assert.match(withTime, /heartbeat.holdNoteAt:/, "with a time it uses the relative form");
assert.match(withTime, /could not open the task/, "and still carries the reason");

console.log("  PASS  the task panel tells a hold apart from silence");
