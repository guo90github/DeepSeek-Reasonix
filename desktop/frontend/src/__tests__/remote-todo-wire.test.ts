// Run: tsx src/__tests__/remote-todo-wire.test.ts

// The remote wire shape (/todos and /todos/board) parsed into panel items, and
// the remote-tab mirror of todo-queue-repro's S3 assertions.

import { parseRemoteTodos } from "../lib/tools";
import { mergeTodoBoardQueue } from "../lib/todoVisibility";

let passed = 0;
let failed = 0;

function eq<T>(actual: T, expected: T, label: string) {
  if (Object.is(actual, expected)) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}: expected ${String(expected)}, got ${String(actual)}\n`);
    failed += 1;
  }
}

const wire = [
  { content: "改两条描述", status: "in_progress", activeForm: "改描述", level: 0, step_id: "plan_step_01" },
  { content: "串行验证并提交", status: "pending", level: 0, step_id: "plan_step_02" },
];
const parsed = parseRemoteTodos(wire);
eq(parsed.length, 2, "every well-formed wire item becomes a panel item");
eq(parsed[0]?.step_id, "plan_step_01", "the stable identity survives the wire");
eq(parsed[0]?.activeForm, "改描述", "activeForm survives the wire");

eq(parseRemoteTodos(undefined).length, 0, "an absent payload (older host) yields no items");
eq(
  parseRemoteTodos([{ content: "x" }, null, "junk", { status: "pending" }]).length,
  0,
  "items without content+status are dropped instead of rendered",
);

// The remote panel list is the same merge the local tab runs: the board's queue
// first, then whatever the current list adds, one row per step_id.
const queue = parseRemoteTodos([{ content: "改两条描述", status: "in_progress", step_id: "plan_step_01" }]);
eq(
  mergeTodoBoardQueue(queue, parsed)
    .map((todo) => todo.content)
    .join(","),
  "改两条描述,串行验证并提交",
  "a retitled step is one row on a remote tab too",
);
eq(
  mergeTodoBoardQueue(queue, []).map((todo) => todo.content).join(","),
  "改两条描述",
  "an empty current list still shows what the remote board owes",
);

console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
