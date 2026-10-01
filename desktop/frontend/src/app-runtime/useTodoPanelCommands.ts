import { useMemo, useState } from "react";
import { useCommittedCommand } from "../lib/useCommittedCommand";
import { loadDismissedTodoKeys, saveDismissedTodoKeys } from "../lib/todoDismissalStorage";
import { parseTodos, type Todo } from "../lib/tools";
import {
  dismissedTodoKeyForScope,
  resolveTodoPanelTodos,
  scopedTodoBatchKey,
  scopedTodoDismissalKey,
  mergeTodoBoardQueue,
  shouldShowTodoPanel,
  todoBatchIdentity,
  todoContinueTarget,
  todoDismissalKey,
  todoPanelScope,
} from "../lib/todoVisibility";
import type { Translator } from "../lib/i18n";
import type { Item } from "../lib/useController";
import type { TabMeta } from "../lib/types";
import type { useSessionOperations } from "./useSessionOperations";

export type TodoPanelCommandsInput = {
  items: readonly Item[];
  running: boolean;
  pendingPrompt: boolean;
  meta: {
    canonicalTodos?: Todo[] | null;
    sessionPath?: string;
    eventChannel?: string;
    dismissedTodoBatches?: string[];
    /** Host-issued batch identity; the content-derived key is only a fallback. */
    todoBatchId?: string;
    /** The board's unfinished queue, across every list this session carried. */
    todoQueue?: Todo[] | null;
    /** The board's archive: what finished, in order. */
    todoArchive?: Todo[] | null;
    todosSupervised?: boolean;
  } | undefined | null;
  activeTab: TabMeta | undefined;
  activeTabId: string | undefined;
  remote: boolean;
  remoteReady: boolean;
  controllerReady: boolean;
  sessionKey: string;
  operations: ReturnType<typeof useSessionOperations>;
  t: Translator;
  ports: {
    remoteSend(text: string): Promise<void>;
    sendToTab(tabId: string, text: string): Promise<void>;
    dismissTodoBatch(tabId: string, batchKey: string): Promise<void>;
  };
};

/**
 * Owns the pinned task list above the composer: the canonical todo_write
 * projection, session-scoped dismissal persistence and the dismiss/continue
 * commands. The live task list comes from the most recent successful
 * top-level todo_write result; failed or still-running attempts do not
 * advance the canonical panel state. An incomplete list the host enforces
 * stays pinned, so a stale dismissal cannot hide work that still blocks final
 * readiness; a note the model left behind may be dismissed. New lists start
 * collapsed while the header keeps showing live progress and the current
 * task. Live completion briefly shows 3/3 before retirement; restored
 * completed lists stay in transcript only. The dismissal key stays based on
 * stable todo content/state so history reloads do not resurrect a finished
 * list, and the status-agnostic batch key prevents false new batches;
 * dismissal is session-scoped and sidecar-persisted.
 */
export function useTodoPanelCommands(input: TodoPanelCommandsInput) {
  const { items, activeTab, activeTabId, remote, t, ports } = input;
  const todoEntry = useMemo(() => {
    for (let i = items.length - 1; i >= 0; i--) {
      const it = items[i];
      if (it.kind === "tool" && it.name === "todo_write" && !it.parentId && it.status === "done" && !it.error) {
        return { item: it, index: i };
      }
    }
    return null;
  }, [items]);
  const todoItem = todoEntry?.item ?? null;
  const metaTodos = remote ? undefined : input.meta?.canonicalTodos;
  const todosSupervised = input.meta?.todosSupervised === true;
  const canonicalTodos = useMemo(
    () => resolveTodoPanelTodos(metaTodos, todoItem ? parseTodos(todoItem.args) : undefined),
    [metaTodos, todoItem],
  );
  // The shelf shows what is still owed (the board's queue) plus the current
  // list's own items. Dismissal and the batch identity below deliberately keep
  // reading the canonical list, so history cannot resurrect a closed batch.
  const todos = useMemo(
    () => mergeTodoBoardQueue(remote ? undefined : input.meta?.todoQueue, canonicalTodos),
    [canonicalTodos, input.meta?.todoQueue, remote],
  );
  const [dismissedTodoKeys, setDismissedTodoKeys] = useState<Set<string>>(loadDismissedTodoKeys);
  const todoKey = useMemo(() => todoDismissalKey(canonicalTodos), [canonicalTodos]);
  // The board's archive is read-only history: the shelf may list it, never
  // resume or dismiss from it.
  const todoArchive = useMemo(
    () => (remote ? [] : (input.meta?.todoArchive ?? [])),
    [input.meta?.todoArchive, remote],
  );
  const todoBatch = useMemo(() => todoBatchIdentity(input.meta?.todoBatchId, todos), [input.meta?.todoBatchId, todos]);
  const todoScope = useMemo(
    () => todoPanelScope({ activeTab, activeTabId, eventChannel: remote ? undefined : input.meta?.eventChannel }),
    [activeTab, activeTabId, remote, input.meta?.eventChannel],
  );
  const dismissedTodo = useMemo(
    () => dismissedTodoKeyForScope(todoScope, dismissedTodoKeys, todoKey),
    [dismissedTodoKeys, todoKey, todoScope],
  );
  const scopedTodoKey = useMemo(() => scopedTodoDismissalKey(todoScope, todoKey), [todoKey, todoScope]);
  const scopedTodoBatch = useMemo(() => scopedTodoBatchKey(todoScope, todoBatch), [todoBatch, todoScope]);
  const showTodos = shouldShowTodoPanel(todoKey, dismissedTodo, todos, { batchKey: todoBatch, batches: !remote && input.meta?.sessionPath === activeTab?.sessionPath ? input.meta?.dismissedTodoBatches : undefined }, todosSupervised);
  const dismissTodos = useCommittedCommand(() => {
    if (!scopedTodoKey) return;
    setDismissedTodoKeys((current) => {
      if (current.has(scopedTodoKey)) return current;
      const next = new Set(current);
      next.add(scopedTodoKey);
      saveDismissedTodoKeys(next);
      return next;
    });
    if (!remote && activeTabId && todoBatch) {
      const target = { tabId: activeTabId, sessionKey: input.sessionKey };
      void input.operations(target, "todo-dismiss", {}, async (_input, authority) => (await import("./sessionRuntimeOwner")).executeTodoDismissal(
        target, todoBatch, (tabId, batchKey) => ports.dismissTodoBatch(tabId, batchKey), authority,
      )).catch(() => undefined);
    }
  });
  const handleTodoContinue = useCommittedCommand(() => {
    const targetTabId = todoContinueTarget(activeTabId, activeTabId, {
      ready: remote ? input.remoteReady : input.controllerReady,
      readOnly: Boolean(activeTab?.readOnly),
      running: input.running,
      pendingPrompt: input.pendingPrompt,
    });
    if (!targetTabId) return;
    const prompt = t("todo.continue");
    if (remote) {
      void ports.remoteSend(prompt);
      return;
    }
    void ports.sendToTab(targetTabId, prompt);
  });

  return { showTodos, scopedTodoBatch, todos, todoArchive, todosSupervised, dismissTodos, handleTodoContinue };
}
