import type { TaskEvent, TaskSnapshot } from "./types";

export interface TaskPageRequest {
  scope: "session" | "project" | "all" | string; tabId: string; projectKey: string; states: string[]; query: string; cursor: string; limit: number;
}
export interface TaskCatalogStatus {
  state: string; mode: "disk" | "memory" | string; path?: string; revision: number;
  indexed: number; total: number; pending: number; failed: number; lastError?: string;
  warnings?: string[];
}
export interface TaskCatalogItem { projectKey: string; projectLabel: string; task: TaskSnapshot }
// Task tree node (docs/TASK_TREE_DESIGN.md §4.1/§5.4 ②). projectKey is carried
// per node: parent edges are scoped per project and actions need it for
// StopTaskByKey/RequeueTaskByKey bindings.
export interface TaskNode { projectKey: string; projectLabel: string; task: TaskSnapshot; children: TaskNode[] }
export interface TaskPage { items: TaskCatalogItem[]; nextCursor: string; revision: number; partial: boolean; staleCursor: boolean; status: TaskCatalogStatus }
export interface TaskEventPageRequest { projectKey: string; taskId: string; after: number; limit: number }
export interface TaskEventPage { items: TaskEvent[]; nextSequence: number; partial: boolean }
export interface TaskActionRequest { projectKey: string; taskId: string; expectedVersion: number; reason: string; idempotencyKey: string }
// Result of PruneTasks: terminal tasks beyond maxRetained are archived
// (moved to archivedDir) oldest-first; total = terminal tasks considered.
export interface PruneResult { archived: number; archivedDir?: string; total: number }
