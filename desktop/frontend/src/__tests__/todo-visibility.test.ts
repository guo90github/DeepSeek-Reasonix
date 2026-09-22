// Run: tsx src/__tests__/todo-visibility.test.ts

import assert from "node:assert/strict";

import { shouldShowTodoPanel, todoBatchKey, todoDismissalKey } from "../lib/todoVisibility";

const note = [
  { content: "Sketch the fix", status: "in_progress" },
  { content: "Ship it", status: "pending" },
];

assert.equal(
  shouldShowTodoPanel(todoDismissalKey(note), todoDismissalKey(note), note, undefined, true),
  true,
  "an enforced incomplete list reappears even after a stale local dismissal",
);
assert.equal(
  shouldShowTodoPanel(todoDismissalKey(note), null, note, { batchKey: todoBatchKey(note), batches: [todoBatchKey(note)] }, true),
  true,
  "a persisted batch dismissal cannot hide an enforced incomplete list",
);
assert.equal(
  shouldShowTodoPanel(todoDismissalKey(note), todoDismissalKey(note), note, undefined, false),
  false,
  "a dismissed note stays dismissed",
);
assert.equal(
  shouldShowTodoPanel(todoDismissalKey(note), null, note, undefined, false),
  true,
  "an undismissed note still shows its progress",
);
assert.equal(
  shouldShowTodoPanel(todoDismissalKey(note), null, note, undefined),
  true,
  "omitting the flag keeps the previously pinned behaviour",
);
assert.equal(
  shouldShowTodoPanel(todoDismissalKey(note), null, note, { batchKey: todoBatchKey(note), batches: [todoBatchKey(note)] }, false),
  false,
  "a persisted dismissal still closes an unfinished note after remount",
);

const done = [
  { content: "Sketch the fix", status: "completed" },
  { content: "Ship it", status: "completed" },
];
assert.equal(
  shouldShowTodoPanel(todoDismissalKey(done), null, done, { batchKey: todoBatchKey(done), batches: [todoBatchKey(done)] }, false),
  false,
  "a finished batch stays closed whether or not it was a commitment",
);
