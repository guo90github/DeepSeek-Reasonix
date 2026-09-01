// Run: tsx src/__tests__/task-tree.test.ts
// Pure-function tests for src/lib/taskTree.ts (docs/TASK_TREE_DESIGN.md §8 step 1).

import type { TaskCatalogItem, TaskNode, TaskSnapshot } from "../lib/taskCatalogTypes";
import {
  aggregateState,
  buildTaskTree,
  mergeTaskPages,
  nextPendingSibling,
} from "../lib/taskTree";

let passed = 0;
let failed = 0;

function ok(value: boolean, label: string) {
  if (value) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

function snap(overrides: Partial<TaskSnapshot> = {}): TaskSnapshot {
  return {
    schema_version: 1,
    task_id: "t-1",
    session_id: "s-1",
    state: "running",
    version: 1,
    created_at: "2025-01-01T00:00:00Z",
    updated_at: "2025-01-01T00:00:00Z",
    ...overrides,
  };
}

function item(projectKey: string, task: TaskSnapshot): TaskCatalogItem {
  return { projectKey, projectLabel: projectKey, task };
}

function ids(nodes: TaskNode[]): string[] {
  return nodes.map((n) => n.task.task_id);
}

// --- buildTaskTree ---------------------------------------------------------

ok(buildTaskTree([]).length === 0, "empty list builds an empty forest");

{
  const roots = buildTaskTree([
    item("p", snap({ task_id: "a", updated_at: "2025-01-01T00:00:01Z" })),
    item("p", snap({ task_id: "b", updated_at: "2025-01-01T00:00:02Z" })),
  ]);
  ok(ids(roots).join(",") === "b,a", "flat tasks are roots, newest updated_at first");
}

{
  const tree = buildTaskTree([
    item("p", snap({ task_id: "parent" })),
    item("p", snap({ task_id: "c1", parent_id: "parent", position: 1 })),
    item("p", snap({ task_id: "c2", parent_id: "parent", position: 2 })),
  ]);
  const parent = tree.find((n) => n.task.task_id === "parent");
  ok(tree.length === 1 && !!parent, "children attach under their parent");
  ok(ids(parent?.children ?? []).join(",") === "c1,c2", "children ordered by position");
}

{
  // Child arrives before its parent in the array: order must not matter.
  const tree = buildTaskTree([
    item("p", snap({ task_id: "child", parent_id: "parent", position: 1 })),
    item("p", snap({ task_id: "parent" })),
  ]);
  const parent = tree.find((n) => n.task.task_id === "parent");
  ok(tree.length === 1 && ids(parent?.children ?? [])[0] === "child", "out-of-order arrival still nests");
}

{
  const tree = buildTaskTree([
    item("p", snap({ task_id: "orphan", parent_id: "no-such-parent" })),
  ]);
  ok(tree.length === 1 && tree[0].task.task_id === "orphan", "dangling parent floats to root");
}

{
  // Same parent_id in another project is NOT the same parent: edges are
  // project-scoped.
  const tree = buildTaskTree([
    item("p1", snap({ task_id: "parent" })),
    item("p2", snap({ task_id: "child", parent_id: "parent" })),
  ]);
  ok(tree.length === 2, "cross-project parent id does not attach");
}

{
  const tree = buildTaskTree([
    item("p", snap({ task_id: "a", parent_id: "b" })),
    item("p", snap({ task_id: "b", parent_id: "a" })),
  ]);
  ok(tree.length === 2, "a parent cycle is broken: both float to root");
}

// --- aggregateState --------------------------------------------------------

{
  const agg = aggregateState(buildTaskTree([item("p", snap({ task_id: "leaf" }))])[0]);
  ok(agg.done === 0 && agg.total === 1 && !agg.failed && agg.skipped === 0, "running leaf: 0/1, not failed");
}

{
  const tree = buildTaskTree([
    item("p", snap({ task_id: "parent", state: "running" })),
    item("p", snap({ task_id: "c1", parent_id: "parent", state: "succeeded" })),
    item("p", snap({ task_id: "c2", parent_id: "parent", state: "succeeded" })),
  ]);
  const agg = aggregateState(tree[0]);
  ok(agg.done === 2 && agg.total === 3 && !agg.failed, "parent + 2 succeeded children: 2/3");
}

{
  const tree = buildTaskTree([
    item("p", snap({ task_id: "parent" })),
    item("p", snap({ task_id: "c1", parent_id: "parent", state: "failed" })),
  ]);
  const agg = aggregateState(tree[0]);
  ok(agg.failed === true, "any failed descendant marks the parent failed");
}

{
  const tree = buildTaskTree([
    item("p", snap({ task_id: "parent" })),
    item("p", snap({ task_id: "c1", parent_id: "parent", state: "skipped" })),
    item("p", snap({ task_id: "c2", parent_id: "parent", state: "succeeded" })),
  ]);
  const agg = aggregateState(tree[0]);
  ok(agg.done === 1 && agg.total === 3 && agg.skipped === 1 && !agg.failed,
    "skipped counts toward total, neither done nor failed");
}

{
  const tree = buildTaskTree([
    item("p", snap({ task_id: "gp" })),
    item("p", snap({ task_id: "p", parent_id: "gp" })),
    item("p", snap({ task_id: "c", parent_id: "p", state: "succeeded" })),
  ]);
  const agg = aggregateState(tree[0]);
  ok(agg.done === 1 && agg.total === 3, "aggregation is recursive across depth");
}

// --- mergeTaskPages --------------------------------------------------------

{
  // Page 1 has the parent only; page 2 brings its child. The parent row must
  // survive the merge and the child must attach under it.
  const prev = buildTaskTree([item("p", snap({ task_id: "parent", version: 1 }))]);
  const merged = mergeTaskPages(prev, [
    item("p", snap({ task_id: "parent", version: 1 })),
    item("p", snap({ task_id: "child", parent_id: "parent", version: 1, position: 1 })),
  ]);
  ok(merged.length === 1, "parent row does not vanish when a child page arrives");
  ok(ids(merged[0]?.children ?? []).join(",") === "child", "new child attaches under its parent");
}

{
  // A stale poll frame (lower version) must not overwrite a live patch.
  const prev = buildTaskTree([item("p", snap({ task_id: "t", state: "succeeded", version: 5 }))]);
  const merged = mergeTaskPages(prev, [item("p", snap({ task_id: "t", state: "running", version: 4 }))]);
  ok(merged[0]?.task.state === "succeeded", "lower version is ignored");
}

{
  // Equal version but older updated_at is also stale.
  const prev = buildTaskTree([item("p", snap({ task_id: "t", state: "succeeded", version: 5, updated_at: "2025-01-01T00:00:02Z" }))]);
  const merged = mergeTaskPages(prev, [item("p", snap({ task_id: "t", state: "running", version: 5, updated_at: "2025-01-01T00:00:01Z" }))]);
  ok(merged[0]?.task.state === "succeeded", "equal version with older updated_at is ignored");
}

{
  // A genuinely newer row replaces the old one.
  const prev = buildTaskTree([item("p", snap({ task_id: "t", state: "running", version: 1 }))]);
  const merged = mergeTaskPages(prev, [item("p", snap({ task_id: "t", state: "succeeded", version: 2 }))]);
  ok(merged[0]?.task.state === "succeeded", "higher version replaces the row");
}

{
  // Rows from a previous page that are absent from the next one are kept
  // (父行不消失 under partial pagination).
  const prev = buildTaskTree([item("p", snap({ task_id: "old", state: "succeeded" }))]);
  const merged = mergeTaskPages(prev, [item("p", snap({ task_id: "new", state: "running" }))]);
  ok(ids(merged).sort().join(",") === "new,old", "absent previous rows are retained");
}

// --- nextPendingSibling ----------------------------------------------------

{
  const tree = buildTaskTree([
    item("p", snap({ task_id: "parent" })),
    item("p", snap({ task_id: "a", parent_id: "parent", position: 1, state: "succeeded" })),
    item("p", snap({ task_id: "b", parent_id: "parent", position: 2, state: "running" })),
  ]);
  const parent = tree[0];
  const a = parent.children[0];
  const next = nextPendingSibling(a, tree);
  ok(next?.task.task_id === "b", "completed node advances to the next running sibling");
}

{
  const tree = buildTaskTree([
    item("p", snap({ task_id: "parent" })),
    item("p", snap({ task_id: "a", parent_id: "parent", position: 1, state: "succeeded" })),
    item("p", snap({ task_id: "b", parent_id: "parent", position: 2, state: "succeeded" })),
  ]);
  const parent = tree[0];
  ok(nextPendingSibling(parent.children[0], tree) === null, "no ready sibling after the last done one");
}

{
  const tree = buildTaskTree([
    item("p", snap({ task_id: "a", state: "running" })),
  ]);
  ok(nextPendingSibling(tree[0], tree) === null, "a running node has no next sibling");
}

{
  const tree = buildTaskTree([
    item("p", snap({ task_id: "parent" })),
    item("p", snap({ task_id: "a", parent_id: "parent", position: 1, state: "succeeded" })),
    item("p", snap({ task_id: "b", parent_id: "parent", position: 2, state: "skipped" })),
    item("p", snap({ task_id: "c", parent_id: "parent", position: 3, state: "queued" })),
  ]);
  const parent = tree[0];
  ok(nextPendingSibling(parent.children[0], tree)?.task.task_id === "c",
    "terminal and skipped siblings are skipped in cursor advance");
}

{
  const tree = buildTaskTree([
    item("p", snap({ task_id: "a", state: "succeeded" })),
    item("p", snap({ task_id: "b", state: "running" })),
  ]);
  ok(nextPendingSibling(tree[0], tree)?.task.task_id === "b", "root-level cursor advance works");
}

console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
