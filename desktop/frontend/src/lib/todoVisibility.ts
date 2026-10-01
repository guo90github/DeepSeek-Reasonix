import type { Todo } from "./tools";

export interface TodoPanelScopeInput {
  activeTabId?: string | null;
  activeTab?: {
    id?: string | null;
    scope?: string | null;
    workspaceRoot?: string | null;
    topicId?: string | null;
    sessionPath?: string | null;
  } | null;
  eventChannel?: string | null;
}

export type TodoPresentationStatus = "pending" | "in_progress" | "waiting" | "paused" | "completed";

export interface TodoRuntimePresentation {
  running: boolean;
  pendingPrompt: boolean;
}

export function todoPresentationStatus(
  status: Todo["status"],
  runtime: TodoRuntimePresentation,
): TodoPresentationStatus {
  switch (todoStatus(status)) {
    case "completed":
      return "completed";
    case "in_progress":
      if (runtime.pendingPrompt) return "waiting";
      return runtime.running ? "in_progress" : "paused";
    default:
      return "pending";
  }
}

export function todoContinueTarget(
  targetTabId: string | null | undefined,
  activeTabId: string | null | undefined,
  runtime: TodoRuntimePresentation & { ready: boolean; readOnly?: boolean },
): string | null {
  const target = String(targetTabId ?? "").trim();
  if (!target || target !== String(activeTabId ?? "").trim()) return null;
  if (!runtime.ready || runtime.readOnly || runtime.running || runtime.pendingPrompt) return null;
  return target;
}

export function resolveTodoPanelTodos(
  canonical: Todo[] | null | undefined,
  live?: Todo[] | null,
): Todo[] {
  // `live` is set only when the transcript has a completed top-level todo_write.
  // Prefer it over MetaForTab — meta only refreshes on turn_done / focus change,
  // so mid-turn status flips otherwise freeze until the user switches tabs (#7642).
  if (live !== undefined && live !== null) return live;
  return Array.isArray(canonical) ? canonical : [];
}

/**
 * What the shelf shows: the board's queue (unfinished work across lists) first,
 * then whatever the current list adds. Identity and dismissal never come from
 * here — they stay on the canonical list — so showing history cannot resurrect
 * a closed batch.
 */
export function mergeTodoBoardQueue(queue: Todo[] | null | undefined, current: Todo[]): Todo[] {
  const owed = Array.isArray(queue) ? queue : [];
  if (owed.length === 0) return current;
  const seen = new Set(owed.map(todoItemKey));
  const extra = current.filter((todo) => !seen.has(todoItemKey(todo)));
  return extra.length === 0 ? owed : [...owed, ...extra];
}

function todoItemKey(todo: Todo): string {
  const level = typeof todo.level === "number" ? todo.level : 0;
  return `${String(todo.content ?? "")}\u0000${level}`;
}

/**
 * Whether two meta views describe the same board: the batch id plus the queue
 * and archive. Kept here so the meta comparison stays one line at its call site.
 */
export function sameTodoBoard(
  a: { todoBatchId?: string; todoQueue?: Todo[] | null; todoArchive?: Todo[] | null },
  b: { todoBatchId?: string; todoQueue?: Todo[] | null; todoArchive?: Todo[] | null },
): boolean {
  return a.todoBatchId === b.todoBatchId
    && sameTodoList(a.todoQueue, b.todoQueue)
    && sameTodoList(a.todoArchive, b.todoArchive);
}

export function sameTodoList(a: Todo[] | null | undefined, b: Todo[] | null | undefined): boolean {
  if (a === b) return true;
  if (!Array.isArray(a) || !Array.isArray(b) || a.length !== b.length) return false;
  return a.every((todo, index) => {
    const other = b[index];
    return (
      todo.content === other.content &&
      todo.status === other.status &&
      todo.activeForm === other.activeForm &&
      todo.level === other.level
    );
  });
}

// TodoGroup mirrors the kernel's serialTodoSegments: a level-0 item owns the
// level-1 run immediately after it; any other item (including a stray level-1
// with no phase above it) is its own single-step group.
export interface TodoGroup {
  phase?: Todo;
  children: Todo[];
}

export function groupTodos(todos: Todo[]): TodoGroup[] {
  const groups: TodoGroup[] = [];
  let i = 0;
  while (i < todos.length) {
    const todo = todos[i];
    if ((todo.level ?? 0) === 0) {
      const children: Todo[] = [];
      let j = i + 1;
      while (j < todos.length && (todos[j].level ?? 0) === 1) {
        children.push(todos[j]);
        j++;
      }
      groups.push({ phase: todo, children });
      i = j;
    } else {
      groups.push({ children: [todo] });
      i++;
    }
  }
  return groups;
}

// phaseSummary reports the phase's completed sub-step count, or null when the
// group is a plain row / lone phase — no chip is rendered then.
export function phaseSummary(group: TodoGroup): { done: number; total: number } | null {
  if (!group.phase || group.children.length === 0) return null;
  const total = group.children.length;
  const done = group.children.filter((child) => todoStatus(child.status) === "completed").length;
  return { done, total };
}

export function todoDismissalKey(todos: Todo[]): string {
  if (todos.length === 0) return "";
  return JSON.stringify(todos.map((todo) => ({
    content: String(todo.content ?? ""),
    status: todoStatus(todo.status),
    activeForm: String(todo.activeForm ?? ""),
    level: typeof todo.level === "number" ? todo.level : 0,
  })));
}

export function todoPanelScope({ activeTab, activeTabId, eventChannel }: TodoPanelScopeInput): string {
  const tabId = String(activeTabId ?? "").trim();
  const tab = !tabId || activeTab?.id === tabId ? activeTab : null;
  const sessionPath = tab?.sessionPath?.trim();
  if (sessionPath) return `session:${sessionPath}`;
  if (tabId) return `tab:${tabId}`;
  const topicId = tab?.topicId?.trim();
  if (tab && topicId) return `topic:${tab.scope ?? ""}:${tab.workspaceRoot ?? ""}:${topicId}`;
  const channel = String(eventChannel ?? "").trim();
  return channel ? `event:${channel}` : "";
}

export function scopedTodoDismissalKey(scope: string | null | undefined, todoKey: string | null | undefined): string {
  const key = String(todoKey ?? "").trim();
  if (!key) return "";
  const prefix = String(scope ?? "").trim();
  return prefix ? `${prefix}\0${key}` : key;
}

export function dismissedTodoKeyForScope(
  scope: string | null | undefined,
  dismissedKeys: ReadonlySet<string> | null | undefined,
  todoKey: string | null | undefined,
): string | null {
  const scopedKey = scopedTodoDismissalKey(scope, todoKey);
  if (!scopedKey || !dismissedKeys?.has(scopedKey)) return null;
  return todoKey ?? null;
}

export function todoBatchKey(todos: Todo[]): string {
  if (todos.length === 0) return "";
  return JSON.stringify(todos.map((todo) => ({
    content: String(todo.content ?? ""),
    level: typeof todo.level === "number" ? todo.level : 0,
  })));
}

/**
 * The batch identity the shelf keys on: the host issues it, so editing a list
 * keeps its batch (and a closed batch stays closed). The content-derived key is
 * only a fallback for a backend that does not report one yet.
 */
export function todoBatchIdentity(hostBatchId: string | null | undefined, todos: Todo[]): string {
  const issued = String(hostBatchId ?? "").trim();
  return issued || todoBatchKey(todos);
}

export function scopedTodoBatchKey(scope: string | null | undefined, batchKey: string | null | undefined): string {
  const key = String(batchKey ?? "").trim();
  if (!key) return "";
  const prefix = String(scope ?? "").trim();
  return prefix ? `${prefix}\0${key}` : key;
}

export function shouldShowTodoPanel(
  todoKey: string | null | undefined,
  dismissedTodoKey: string | null,
  todos: Todo[],
  persisted?: { batchKey?: string | null; batches?: readonly string[] | null },
  readinessBlocking = true,
): boolean {
  if (!todoKey || todos.length === 0) return false;
  // An unfinished item the host enforces must stay visible: hiding it would hide
  // work that still blocks final readiness. A note the model left behind has no
  // such duty, so the user may dismiss it like any other batch.
  if (hasIncompleteTodos(todos) && readinessBlocking) return true;
  if (todoKey === dismissedTodoKey) return false;
  const batchKey = String(persisted?.batchKey ?? "").trim();
  if (batchKey && persisted?.batches?.includes(batchKey)) return false;
  return true;
}

export function sameStringList(a?: readonly string[] | null, b?: readonly string[] | null): boolean {
  if (a === b) return true;
  const left = Array.isArray(a) ? a : [];
  const right = Array.isArray(b) ? b : [];
  return left.length === right.length && left.every((value, index) => value === right[index]);
}

export function shouldOpenTodoPanelByDefault(): boolean {
  return false;
}

function todoStatus(status: unknown): string {
  const normalized = String(status ?? "").trim();
  return normalized || "pending";
}

function hasIncompleteTodos(todos: Todo[]): boolean {
  return todos.some((todo) => todoStatus(todo.status) !== "completed");
}
