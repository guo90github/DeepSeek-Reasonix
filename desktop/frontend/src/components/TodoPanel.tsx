import { useEffect, useMemo, useRef, useState } from "react";
import { useT } from "../lib/i18n";
import type { Todo } from "../lib/tools";
import { groupTodos, phaseSummary, shouldOpenTodoPanelByDefault, todoPresentationStatus, type TodoPresentationStatus } from "../lib/todoVisibility";
import { PromptBadge, PromptHeaderAction, PromptShelf } from "./PromptShelf";

const STORAGE_KEY = "todoPanel:openStates";
const MAX_STORED_OPEN_STATES = 80;

function loadOpenStates(): Record<string, boolean> {
  try {
    const saved = localStorage.getItem(STORAGE_KEY);
    if (!saved) return {};
    const parsed = JSON.parse(saved) as unknown;
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return {};
    const states: Record<string, boolean> = {};
    for (const [key, value] of Object.entries(parsed)) {
      if (typeof value === "boolean") states[key] = value;
    }
    return states;
  } catch {
    return {};
  }
}

function loadOpenState(stateKey: string, defaultOpen: boolean): boolean {
  const states = loadOpenStates();
  return Object.prototype.hasOwnProperty.call(states, stateKey) ? states[stateKey] : defaultOpen;
}

function saveOpenState(stateKey: string, open: boolean): void {
  try {
    const entries = Object.entries(loadOpenStates()).filter(([key]) => key !== stateKey);
    entries.push([stateKey, open]);
    const trimmed = entries.slice(-MAX_STORED_OPEN_STATES);
    localStorage.setItem(STORAGE_KEY, JSON.stringify(Object.fromEntries(trimmed)));
  } catch {
    /* ignore quota errors */
  }
}

// TodoPanel is the live task list pinned just above the composer — the kernel's
// latest todo_write call drives it, and it updates in place as the agent flips
// items to in_progress / completed. Each new todo batch starts collapsed so the
// header can show live progress and the current task without occupying extra
// space; manual expand/collapse persists per batch. Completion never hides the
// panel — the all-done Close action is the only way it leaves the composer.
export function TodoPanel({
  stateKey,
  todos,
  running,
  pendingPrompt,
  onContinue,
  onDismiss,
}: {
  stateKey: string;
  todos: Todo[];
  running: boolean;
  pendingPrompt: boolean;
  onContinue?: () => void;
  onDismiss: () => void;
}) {
  const t = useT();
  const currentRef = useRef<HTMLLIElement | null>(null);

  const done = todos.filter((t) => t.status === "completed").length;
  const current = todos.find((t) => t.status === "in_progress");
  const allDone = todos.length > 0 && done === todos.length;
  const summary = current?.activeForm || current?.content || todos[todos.length - 1]?.content || "";
  const [open, setOpen] = useState(() => loadOpenState(stateKey, shouldOpenTodoPanelByDefault()));
  // Phase rows expand by default; collapse state is per-batch (keyed by group
  // index) and resets on remount, unlike the shelf's persisted open state.
  const [collapsed, setCollapsed] = useState<Record<number, boolean>>({});
  const groups = useMemo(() => groupTodos(todos), [todos]);

  useEffect(() => {
    if (!open) return;
    currentRef.current?.scrollIntoView({ block: "nearest" });
  }, [open, current?.content, current?.activeForm]);

  if (todos.length === 0) return null;

  return (
    <PromptShelf
      titleId="todo-shelf-title"
      title={t("todo.title")}
      badges={<PromptBadge>{done}/{todos.length}</PromptBadge>}
      meta={summary}
      role="region"
      cardClassName="prompt-shelf--todo"
      cardCollapsible
      collapsed={!open}
      onToggleCollapse={() => setOpen((value) => {
        const next = !value;
        saveOpenState(stateKey, next);
        return next;
      })}
      headerActions={allDone ? (
        <PromptHeaderAction onClick={onDismiss}>
          {t("common.close")}
        </PromptHeaderAction>
      ) : current && !running && !pendingPrompt && onContinue ? (
        <PromptHeaderAction onClick={onContinue}>
          {t("todo.continue")}
        </PromptHeaderAction>
      ) : undefined}
    >
      {open && (
        <ul className="todobar__list">
          {groups.map((group, gi) => {
            if (group.phase && group.children.length > 0) {
              const status = normalizeTodoStatus(group.phase.status);
              const displayStatus = todoPresentationStatus(status, { running, pendingPrompt });
              const summary = phaseSummary(group);
              const subOpen = !collapsed[gi];
              return (
                <li
                  key={gi}
                  ref={status === "in_progress" ? currentRef : undefined}
                  className={`todobar__item todobar__item--phase todobar__item--${displayStatus}`}
                >
                  <span className="todobar__phase-head">
                    <span
                      role="button"
                      tabIndex={0}
                      aria-expanded={subOpen}
                      className={`todobar__chevron${subOpen ? "" : " todobar__chevron--closed"}`}
                      onClick={(e) => {
                        e.stopPropagation();
                        setCollapsed((value) => ({ ...value, [gi]: !value[gi] }));
                      }}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") {
                          e.preventDefault();
                          e.stopPropagation();
                          setCollapsed((value) => ({ ...value, [gi]: !value[gi] }));
                        }
                      }}
                    >
                      ▸
                    </span>
                    <span className={`todobar__status todobar__status--${displayStatus}`}>
                      {t(todoStatusLabelKey(displayStatus))}
                    </span>
                    {summary && (
                      <span className="todobar__chip">
                        {summary.done}/{summary.total}
                      </span>
                    )}
                    <span className="todobar__text">
                      {status === "in_progress" && group.phase.activeForm ? group.phase.activeForm : group.phase.content}
                    </span>
                  </span>
                  {subOpen && (
                    <ul className="todobar__sublist">
                      {group.children.map((child, ci) => {
                        const childStatus = normalizeTodoStatus(child.status);
                        const displayStatus = todoPresentationStatus(childStatus, { running, pendingPrompt });
                        return (
                          <li
                            key={ci}
                            ref={childStatus === "in_progress" ? currentRef : undefined}
                            className={`todobar__item todobar__item--sub todobar__item--${displayStatus}`}
                          >
                            <span className={`todobar__status todobar__status--${displayStatus}`}>
                              {t(todoStatusLabelKey(displayStatus))}
                            </span>
                            <span className="todobar__text">
                              {childStatus === "in_progress" && child.activeForm ? child.activeForm : child.content}
                            </span>
                          </li>
                        );
                      })}
                    </ul>
                  )}
                </li>
              );
            }
            const todo = group.phase ?? group.children[0];
            const status = normalizeTodoStatus(todo.status);
            const displayStatus = todoPresentationStatus(status, { running, pendingPrompt });
            return (
              <li
                key={gi}
                ref={status === "in_progress" ? currentRef : undefined}
                className={`todobar__item todobar__item--${displayStatus}${todo.level ? " todobar__item--sub" : ""}`}
              >
                <span className={`todobar__status todobar__status--${displayStatus}`}>
                  {t(todoStatusLabelKey(displayStatus))}
                </span>
                <span className="todobar__text">
                  {status === "in_progress" && todo.activeForm ? todo.activeForm : todo.content}
                </span>
              </li>
            );
          })}
        </ul>
      )}
    </PromptShelf>
  );
}

function normalizeTodoStatus(status: Todo["status"]): "pending" | "in_progress" | "completed" {
  switch (String(status ?? "").trim()) {
    case "completed":
      return "completed";
    case "in_progress":
      return "in_progress";
    default:
      return "pending";
  }
}

function todoStatusLabelKey(status: TodoPresentationStatus): "todo.pending" | "todo.inProgress" | "status.runtimePendingPrompt" | "todo.paused" | "todo.completed" {
  switch (status) {
    case "completed":
      return "todo.completed";
    case "in_progress":
      return "todo.inProgress";
    case "waiting":
      return "status.runtimePendingPrompt";
    case "paused":
      return "todo.paused";
    default:
      return "todo.pending";
  }
}
