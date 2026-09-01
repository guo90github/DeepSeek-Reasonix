// Pure tree helpers for the task tree view (docs/TASK_TREE_DESIGN.md §4.1).
// No React and no side effects: every function is unit-testable against wire
// TaskCatalogItem arrays. Parent edges are scoped per project — a snapshot's
// parent_id is a session-namespaced monitor id, so the same id may exist in
// another project.

import type { TaskCatalogItem, TaskNode } from "./taskCatalogTypes";
import type { TaskSnapshot } from "./types";

const TERMINAL_STATES = new Set(["succeeded", "failed", "cancelled", "stale", "skipped"]);
const PENDING_STATES = new Set(["queued", "running", "waiting"]);

function isTerminal(state: string): boolean {
  return TERMINAL_STATES.has(state);
}

function isPending(state: string): boolean {
  return PENDING_STATES.has(state);
}

export function taskNodeKey(projectKey: string, taskId: string): string {
  return `${projectKey}:${taskId}`;
}

function nodeKeyOf(node: TaskNode): string {
  return taskNodeKey(node.projectKey, node.task.task_id);
}

function compareUpdatedAt(a: string, b: string): number {
  return a < b ? -1 : a > b ? 1 : 0;
}

// Roots sort newest-first, matching the flat list today; children sort in
// execution order: position when present, then creation time (jobs siblings
// carry no position yet — docs §5.2 B4 — so they fall back to created_at).
function compareRoot(a: TaskNode, b: TaskNode): number {
  return compareUpdatedAt(b.task.updated_at, a.task.updated_at);
}

function compareSibling(a: TaskNode, b: TaskNode): number {
  const pa = a.task.position ?? 0;
  const pb = b.task.position ?? 0;
  if (pa !== pb) return pa - pb;
  return compareUpdatedAt(a.task.created_at, b.task.created_at);
}

function sortTree(roots: TaskNode[]): void {
  roots.sort(compareRoot);
  for (const node of roots) {
    node.children.sort(compareSibling);
    sortTree(node.children);
  }
}

// buildTaskTree turns a flat (possibly partial, possibly out-of-order) item
// list into a forest. Two passes: attach by parent_id, then float any node
// that stayed unreachable from a root (missing parent or a cycle) back to the
// root level so no row silently disappears.
export function buildTaskTree(items: TaskCatalogItem[]): TaskNode[] {
  const byKey = new Map<string, TaskNode>();
  for (const item of items) {
    const key = taskNodeKey(item.projectKey, item.task.task_id);
    byKey.set(key, { projectKey: item.projectKey, projectLabel: item.projectLabel, task: item.task, children: [] });
  }
  const roots: TaskNode[] = [];
  const parentOf = new Map<string, string>(); // child key -> parent key
  for (const [key, node] of byKey) {
    const parentId = node.task.parent_id?.trim();
    if (!parentId) {
      roots.push(node);
      continue;
    }
    const parentKey = taskNodeKey(node.projectKey, parentId);
    const parent = byKey.get(parentKey);
    if (!parent) {
      roots.push(node); // dangling parent: tolerate out-of-order arrival
      continue;
    }
    parent.children.push(node);
    parentOf.set(key, parentKey);
  }
  const visited = new Set<string>();
  const visit = (node: TaskNode) => {
    const key = nodeKeyOf(node);
    if (visited.has(key)) return;
    visited.add(key);
    for (const child of node.children) visit(child);
  };
  for (const root of roots) visit(root);
  for (const [key, node] of byKey) {
    if (visited.has(key)) continue; // in a cycle: detach and float to roots
    const parentKey = parentOf.get(key);
    if (parentKey) {
      const parent = byKey.get(parentKey);
      if (parent) parent.children = parent.children.filter((c) => nodeKeyOf(c) !== key);
    }
    roots.push(node);
  }
  sortTree(roots);
  return roots;
}

export interface TaskAggregate {
  done: number;
  total: number;
  failed: boolean;
  skipped: number;
}

// Post-order DFS over the subtree including the node itself: the badge shows
// how many plan steps (children) plus the orchestration step itself finished.
// skipped counts toward total but neither done nor failed (docs §5.1).
export function aggregateState(node: TaskNode): TaskAggregate {
  const agg: TaskAggregate = { done: 0, total: 0, failed: false, skipped: 0 };
  const visit = (n: TaskNode) => {
    const state = n.task.state;
    agg.total += 1;
    if (state === "succeeded") {
      agg.done += 1;
    } else if (state === "failed") {
      agg.failed = true;
    } else if (state === "skipped") {
      agg.skipped += 1;
    }
    for (const child of n.children) visit(child);
  };
  visit(node);
  return agg;
}

// True when `candidate` is strictly newer than `current` — version first,
// updated_at as tie-break. A stale poll frame must never overwrite a live
// patch, so equal snapshots keep the existing row.
function newerWins(candidate: TaskSnapshot, current: TaskSnapshot): boolean {
  if (candidate.version !== current.version) return candidate.version > current.version;
  return compareUpdatedAt(candidate.updated_at, current.updated_at) > 0;
}

// mergeTaskPages merges one polled page into the current tree. It flattens
// the tree, keeps every previously-known row (a parent must not vanish
// because its children arrived in a later page), arbitrates by version, and
// re-builds so new children attach under their parent.
export function mergeTaskPages(prev: TaskNode[], next: TaskCatalogItem[]): TaskNode[] {
  const merged = new Map<string, { projectKey: string; task: TaskSnapshot }>();
  const collect = (nodes: TaskNode[]) => {
    for (const node of nodes) {
      merged.set(nodeKeyOf(node), { projectKey: node.projectKey, task: node.task });
      collect(node.children);
    }
  };
  collect(prev);
  for (const item of next) {
    const key = taskNodeKey(item.projectKey, item.task.task_id);
    const existing = merged.get(key);
    if (existing && !newerWins(item.task, existing.task)) continue;
    merged.set(key, { projectKey: item.projectKey, task: item.task });
  }
  const items: TaskCatalogItem[] = [];
  for (const entry of merged.values()) {
    items.push({ projectKey: entry.projectKey, projectLabel: "", task: entry.task });
  }
  return buildTaskTree(items);
}

// nextPendingSibling advances the display cursor: after `node` reaches a
// terminal state, the next ready sibling (same parent, execution order) is
// the candidate. Ready = queued/running/waiting; terminal or skipped siblings
// are skipped. Display-only — the engine (fleet driveFleet) does the real
// scheduling. A non-terminal node has no next sibling.
export function nextPendingSibling(node: TaskNode, roots: TaskNode[]): TaskNode | null {
  if (!isTerminal(node.task.state)) return null;
  const parent = findParent(node, roots);
  const siblings = parent ? parent.children : roots;
  const idx = siblings.findIndex((s) => nodeKeyOf(s) === nodeKeyOf(node));
  if (idx < 0) return null;
  for (let i = idx + 1; i < siblings.length; i++) {
    const sibling = siblings[i];
    if (isPending(sibling.task.state)) return sibling;
  }
  return null;
}

function findParent(node: TaskNode, roots: TaskNode[]): TaskNode | null {
  const parentId = node.task.parent_id?.trim();
  if (!parentId) return null;
  const parentKey = taskNodeKey(node.projectKey, parentId);
  const search = (nodes: TaskNode[]): TaskNode | null => {
    for (const n of nodes) {
      if (nodeKeyOf(n) === parentKey) return n;
      const found = search(n.children);
      if (found) return found;
    }
    return null;
  };
  return search(roots);
}
