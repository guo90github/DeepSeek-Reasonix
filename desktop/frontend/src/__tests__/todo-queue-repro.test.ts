// Run: tsx src/__tests__/todo-queue-repro.test.ts

// Requirement 17 reproductions, at the seam that decides what the shelf shows.
// The first block pins why the *derived* key resurrects a closed batch; the
// last block pins the fix: the host issues the identity, so an edit keeps it.

import { mergeTodoBoardQueue, shouldShowTodoPanel, todoBatchIdentity, todoBatchKey } from "../lib/todoVisibility";

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

type Todo = { content: string; status?: string; level?: number };

const finished: Todo[] = [
  { content: "定义数据模型", status: "completed", level: 0 },
  { content: "接线宿主", status: "completed", level: 0 },
];

// --- S1: a closed batch comes back after any edit --------------------------

const closedKey = todoBatchKey(finished);
eq(shouldShowTodoPanel(closedKey, closedKey, finished), false, "a closed completed batch stays hidden while the list is untouched");

// The model appends one more step to the same plan (one new item, same work):
const extended: Todo[] = [...finished, { content: "补回归测试", status: "pending", level: 0 }];
const extendedKey = todoBatchKey(extended);
eq(extendedKey !== closedKey, true, "one added step changes the derived batch key");
eq(
  shouldShowTodoPanel(extendedKey, closedKey, extended),
  true,
  "…so the finished part of the batch reappears: the user closed the same list, not a new one",
);

// Rewording one item resurrects it too: identity must not be the content set.
const reworded: Todo[] = [{ content: "定义数据模型（改）", status: "completed", level: 0 }, finished[1]];
eq(
  shouldShowTodoPanel(todoBatchKey(reworded), closedKey, reworded),
  true,
  "renaming one item also resurrects the batch",
);

// Flipping a status must NOT change identity; this is the one case that holds.
const flipped: Todo[] = [{ ...finished[0], status: "pending" }, finished[1]];
eq(todoBatchKey(flipped) === closedKey, true, "a status flip alone keeps the batch key (the only stable part today)");

// --- S3: only the newest list exists --------------------------------------

// The frontend reads one list; there is no archive to show what a later
// todo_write dropped. The predicate can only answer about that single list.
const replaced: Todo[] = [{ content: "另起一批", status: "pending", level: 0 }];
eq(
  shouldShowTodoPanel(todoBatchKey(replaced), closedKey, replaced),
  true,
  "a replacement list shows itself (and the dropped unfinished items are simply gone from this seam)",
);

// --- S2: the shelf's only source is the live controller --------------------

// MetaForTab fills canonicalTodos from the bound controller (desktop/app.go:6693
// -> ctrlTodos, which returns nil when the controller is not bound yet). A tab
// restored from history therefore hands the shelf an empty list, and this
// predicate can only answer false: the list that exists in the transcript stays
// invisible until the controller binds.
eq(shouldShowTodoPanel(undefined, null, []), false, "S2: with no list from the source, the shelf renders nothing");
eq(
  shouldShowTodoPanel(todoBatchKey(finished), null, finished),
  true,
  "…while the same list arriving from a bound controller renders (an all-completed batch is kept, not auto-hidden)",
);

// --- S1 fixed: the host issues the identity --------------------------------

// The shelf now keys on the batch id the host issues (desktop/todo_batch_
// identity.go), so the very edits that used to resurrect a closed batch keep
// it. The content-derived key survives only as the fallback for a backend that
// does not report an id yet.
eq(todoBatchIdentity("todos-1", extended), "todos-1", "S1 fix: an edited batch keeps the host-issued identity");
eq(todoBatchIdentity("todos-1", reworded), "todos-1", "S1 fix: a reworded batch keeps it too");
eq(todoBatchIdentity("todos-1", replaced), "todos-1", "S1 fix: a replacement list keeps the batch it belongs to");
eq(todoBatchIdentity(undefined, extended), extendedKey, "without a host id the derived key is still the fallback");
eq(todoBatchIdentity("  ", finished), closedKey, "a blank host id is not an identity");

// --- S3 fixed: the board's queue is what the shelf shows -------------------

const historical: Todo[] = [{ content: "旧的一步", status: "pending", level: 0 }];
eq(
  mergeTodoBoardQueue(historical, replaced)
    .map((todo) => todo.content)
    .join(","),
  "旧的一步,另起一批",
  "S3 fix: the shelf shows the board's queue first, then the current list",
);
eq(mergeTodoBoardQueue([], replaced), replaced, "an empty queue leaves the current list alone");
eq(mergeTodoBoardQueue(undefined, replaced), replaced, "an absent queue (older host) leaves it alone");
eq(mergeTodoBoardQueue(historical, historical).length, 1, "the same item is never listed twice");

console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
