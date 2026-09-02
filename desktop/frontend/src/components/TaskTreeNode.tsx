// TaskTreeNode renders one row of the task tree recursively (docs
// TASK_TREE_DESIGN.md §4.1): a subtree toggle + aggregate badge when the node
// has children, event-stream expansion on the row head, and the existing
// detail/events/actions block when expanded. Rendering is display-only — the
// engine (fleet driveFleet) owns scheduling.

import { useState, type RefObject } from "react";
import {
  AlertCircle,
  ChevronDown,
  ChevronRight,
  List,
  Loader2,
  XCircle,
} from "lucide-react";
import { useT } from "../lib/i18n";
import type { TaskEvent, TaskSnapshot } from "../lib/types";
import type { TaskNode } from "../lib/taskCatalogTypes";
import { aggregateState, taskNodeKey } from "../lib/taskTree";

// --- helpers (shared with TaskMonitorPanel) ---

type TaskTimerSnapshot = TaskSnapshot & { runtime_lease_until?: string };

const STATE_CONFIG: Record<
  string,
  { key: "queued" | "running" | "waiting" | "succeeded" | "failed" | "cancelled" | "stale" | "skipped" | "removed"; color: string; dot: string }
> = {
  queued: { key: "queued", color: "#6b7280", dot: "⚪" },
  running: { key: "running", color: "#3b82f6", dot: "🔵" },
  waiting: { key: "waiting", color: "#f59e0b", dot: "🟡" },
  succeeded: { key: "succeeded", color: "#22c55e", dot: "🟢" },
  failed: { key: "failed", color: "#ef4444", dot: "🔴" },
  cancelled: { key: "cancelled", color: "#9ca3af", dot: "⏹️" },
  stale: { key: "stale", color: "#d4d4d8", dot: "⬜" },
  skipped: { key: "skipped", color: "#a1a1aa", dot: "➖" },
  removed: { key: "removed", color: "#71717a", dot: "🗑️" },
};

export function stateConfig(state: string, t: ReturnType<typeof useT>) {
  const config = STATE_CONFIG[state];
  return config
    ? { ...config, label: t(`task.state.${config.key}` as never) }
    : { label: state, color: "#6b7280", dot: "❓" };
}

export function runtimeConfig(state: string | undefined, t: ReturnType<typeof useT>) {
  switch (state) {
    case "alive":
      return { label: t("task.runtime.live"), color: "#22c55e" };
    case "exited":
      return { label: t("task.runtime.exited"), color: "#9ca3af" };
    default:
      return { label: t("task.runtime.unknown"), color: "#6b7280" };
  }
}

export function safeStateClass(state: string): string {
  // Sanitize state for use in CSS class names — only allow word chars.
  return state.replace(/[^a-zA-Z0-9_-]/g, "_");
}

export function isTerminalState(state: string): boolean {
  return state === "succeeded" || state === "failed" || state === "cancelled" || state === "stale" || state === "skipped" || state === "removed";
}

export function isStoppableState(state: string): boolean {
  return state === "queued" || state === "running" || state === "waiting";
}

export function elapsed(task: TaskTimerSnapshot, nowMs: number): string {
  if (!task.created_at) return "—";
  const startMs = new Date(task.created_at).getTime();
  if (task.state === "queued") return "—";
  const live = task.runtime_state === "alive" && !isTerminalState(task.state);
  let endMs = live ? nowMs : new Date(task.updated_at).getTime();
  if (task.state === "stale" && task.runtime_lease_until) {
    const leaseEndMs = new Date(task.runtime_lease_until).getTime();
    // Stale is inferred when an alive runtime lease expires. The observer does
    // not rewrite updated_at, so the expired lease is the best bounded end time.
    if (!isNaN(leaseEndMs) && leaseEndMs >= startMs && leaseEndMs <= nowMs) {
      endMs = leaseEndMs;
    }
  }
  const ms = endMs - startMs;
  if (isNaN(ms) || ms < 0) return "—";
  const s = Math.floor(ms / 1000);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  return `${h}h`;
}

// taskLongRunning flags a live task past the warning threshold (P1.4/A4).
export function taskLongRunning(task: TaskTimerSnapshot, nowMs: number, thresholdMs = 10 * 60 * 1000): boolean {
  if (!task.created_at || task.runtime_state !== "alive" || isTerminalState(task.state)) return false;
  const startMs = new Date(task.created_at).getTime();
  return !isNaN(startMs) && nowMs - startMs >= thresholdMs;
}

export function shortID(id: string): string {
  return id.length > 8 ? id.slice(0, 8) : id;
}

export function eventSummary(ev: TaskEvent, t: ReturnType<typeof useT>): string {
  if (ev.error_code) return t("task.event.error", { code: ev.error_code });
  switch (ev.event_type) {
    case "state_change":
      return t("task.event.stateChange", { state: stateConfig(ev.state, t).label, runtime: runtimeConfig(ev.runtime_state, t).label });
    case "error":
      return ev.error_summary || t("task.error");
    default:
      return ev.detail || ev.event_type;
  }
}

// --- component ---

export interface TaskTreeNodeProps {
  node: TaskNode;
  depth: number;
  collapsed: Set<string>; // subtree collapse keys (taskNodeKey)
  expanded: Set<string>; // event-stream expand keys (taskNodeKey)
  taskEvents: Map<string, TaskEvent[]>;
  eventsLoading: Set<string>;
  eventsError: Map<string, string>;
  pendingStopKey: string | null;
  actionTaskKey: string | null;
  nowMs: number;
  scope: string;
  confirmStopRef: RefObject<HTMLButtonElement | null>;
  stopButtonRefs: RefObject<Map<string, HTMLButtonElement>>;
  onToggleRow: (node: TaskNode) => void;
  onToggleSubtree: (key: string) => void;
  onRequestStop: (node: TaskNode) => void;
  onDismissStop: () => void;
  onAction: (node: TaskNode, action: "stop" | "requeue" | "open") => void;
}

export type TaskTreeRowProps = Omit<TaskTreeNodeProps, "node" | "depth">;

export function TaskTreeNode(props: TaskTreeNodeProps) {
  const { node, depth } = props;
  const t = useT();
  const task = node.task;
  const key = taskNodeKey(node.projectKey, task.task_id);
  const cfg = stateConfig(task.state, t);
  const runtime = runtimeConfig(task.runtime_state, t);
  const rowOpen = props.expanded.has(key);
  const treeCollapsed = props.collapsed.has(key);
  const [summaryExpanded, setSummaryExpanded] = useState(false);
  const hasChildren = node.children.length > 0;
  const agg = hasChildren ? aggregateState(node) : null;
  const terminal = isTerminalState(task.state);
  const live = task.runtime_state === "alive" && !terminal;
  const longRunning = taskLongRunning(task, props.nowMs);
  const evs = props.taskEvents.get(key) ?? [];
  const evLoading = props.eventsLoading.has(key);
  const evError = props.eventsError.get(key);

  return (
    <div className={`taskmonitor__task taskmonitor__task--${safeStateClass(task.state)}`}>
      <div className="taskmonitor__task-head">
        {hasChildren ? (
          <button
            type="button"
            className="taskmonitor__subtree-toggle"
            onClick={() => props.onToggleSubtree(key)}
            aria-expanded={!treeCollapsed}
            aria-label={t(treeCollapsed ? "summary.expandSubtree" : "summary.collapseSubtree")}
          >
            {treeCollapsed ? <ChevronRight size={12} /> : <ChevronDown size={12} />}
          </button>
        ) : (
          <span className="taskmonitor__subtree-toggle taskmonitor__subtree-toggle--empty" aria-hidden="true" />
        )}
        <button
          type="button"
          className="taskmonitor__expand"
          onClick={() => props.onToggleRow(node)}
          aria-expanded={rowOpen}
          aria-label={t("summary.taskLabel", { id: shortID(task.task_id), state: cfg.label })}
        >
          <span className="taskmonitor__dot" style={{ color: cfg.color }}>
            {cfg.dot}
          </span>
          <span className="taskmonitor__id">
            {shortID(task.task_id)}
          </span>
          {props.scope === "all" && <span className="taskmonitor__project">{node.projectLabel}</span>}
          <span className="taskmonitor__badge" style={{ backgroundColor: cfg.color + "18", color: cfg.color }}>
            {cfg.label}
          </span>
          <span className="taskmonitor__runtime" style={{ color: runtime.color }} title="Runtime process state">
            <span aria-hidden="true">{task.runtime_state === "alive" ? "●" : "○"}</span>
            {runtime.label}
          </span>
          {terminal && <XCircle size={12} className="taskmonitor__terminal" />}
          <span className="taskmonitor__time">{elapsed(task, props.nowMs)}</span>
          {longRunning && (
            <span className="taskmonitor__long-running" title={t("summary.longRunning", { time: elapsed(task, props.nowMs) })} aria-label={t("summary.longRunning", { time: elapsed(task, props.nowMs) })}>
              ⚠
            </span>
          )}
          {rowOpen ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
        </button>
        {agg && (
          <span
            className={`taskmonitor__agg${agg.failed ? " taskmonitor__agg--failed" : ""}`}
            title={`${agg.done}/${agg.total}${agg.skipped > 0 ? ` (${agg.skipped} skipped)` : ""}`}
          >
            {agg.done}/{agg.total}
          </span>
        )}
      </div>

      {rowOpen && (
        <div className="taskmonitor__detail">
          <dl>
            <dt>{t("summary.taskId")}</dt>
            <dd>{task.task_id}</dd>
            <dt>{t("summary.sessionId")}</dt>
            <dd>{task.session_id || "—"}</dd>
            <dt>{t("summary.state")}</dt>
            <dd>{cfg.label}</dd>
            <dt>{t("summary.runtime")}</dt>
            <dd>{runtime.label}</dd>
            <dt>{t("summary.updated")}</dt>
            <dd>{new Date(task.updated_at).toLocaleString()}</dd>
            {task.error_code && (
              <>
                <dt>{t("summary.errorCode")}</dt>
                <dd className="taskmonitor__err">{task.error_code}</dd>
              </>
            )}
            {task.error_summary && (
              <>
                <dt>{t("summary.detail")}</dt>
                <dd>
                  <button
                    type="button"
                    className={`taskmonitor__err-summary${summaryExpanded ? " taskmonitor__err-summary--expanded" : ""}`}
                    onClick={() => setSummaryExpanded((v) => !v)}
                    aria-expanded={summaryExpanded}
                  >
                    {task.error_summary}
                  </button>
                </dd>
              </>
            )}
            {(task.steps_used ?? 0) > 0 && (
              <>
                <dt>{t("summary.stepsUsed")}</dt>
                <dd>
                  {task.steps_used}
                  {task.steps_estimated ? ` / ~${task.steps_estimated}` : ""}
                </dd>
              </>
            )}
            {task.cost_total && task.cost_status !== "unavailable" && (
              <>
                <dt>{t("summary.cost")}</dt>
                <dd>{task.cost_total}</dd>
              </>
            )}
          </dl>

          {/* Events section */}
          <div className="taskmonitor__events">
            <div className="taskmonitor__events-head">
              <List size={12} />
              <span>{t("summary.recentEvents")}</span>
              {evs.length > 0 && (
                <span className="taskmonitor__events-count">{evs.length}</span>
              )}
            </div>

            {evLoading && evs.length === 0 && (
              <div className="taskmonitor__state">
                <Loader2 size={12} className="taskmonitor__spinner" />
                <span>{t("summary.loadingEvents")}</span>
              </div>
            )}

            {evError && (
              <div className="taskmonitor__state taskmonitor__state--error">
                <AlertCircle size={12} />
                <span>{evError}</span>
              </div>
            )}

            {!evLoading && !evError && evs.length === 0 && (
              <div className="taskmonitor__state taskmonitor__state--empty">
                <span>{t("summary.noEvents")}</span>
              </div>
            )}

            {evs.length > 0 && (
              <ul className="taskmonitor__event-list">
                {evs.map((ev) => (
                  <li key={ev.sequence} className="taskmonitor__event">
                    <span className="taskmonitor__event-seq">#{ev.sequence}</span>
                    <span className="taskmonitor__event-type">{eventSummary(ev, t)}</span>
                    <span className="taskmonitor__event-time">{new Date(ev.timestamp).toLocaleTimeString()}</span>
                  </li>
                ))}
              </ul>
            )}
          </div>
          {props.pendingStopKey === key ? (
            <div
              className="taskmonitor__confirm"
              role="group"
              aria-label={t("summary.confirmStop")}
              onKeyDown={(event) => {
                if (event.key === "Escape") {
                  event.preventDefault();
                  props.onDismissStop();
                }
              }}
            >
              <span className="taskmonitor__confirm-copy">
                {t("summary.confirmStop")}
                {live && <span className="taskmonitor__confirm-duration"> — {elapsed(task, props.nowMs)}</span>}
              </span>
              <div className="taskmonitor__confirm-actions">
                <button
                  ref={props.confirmStopRef}
                  type="button"
                  className="taskmonitor__confirm-stop"
                  disabled={props.actionTaskKey === key}
                  onClick={() => props.onAction(node, "stop")}
                >
                  {t("summary.stop")}
                </button>
                <button type="button" onClick={props.onDismissStop}>{t("summary.keep")}</button>
              </div>
            </div>
          ) : (
            <div className="taskmonitor__actions">
              {isStoppableState(task.state) && (
                <button
                  ref={(btn) => {
                    if (btn) props.stopButtonRefs.current.set(key, btn);
                    else props.stopButtonRefs.current.delete(key);
                  }}
                  type="button"
                  className="taskmonitor__stop"
                  disabled={props.actionTaskKey === key}
                  onClick={() => props.onRequestStop(node)}
                >
                  {t("summary.stop")}
                </button>
              )}
              {(task.state === "failed" || task.state === "stale") && (
                <button type="button" disabled={props.actionTaskKey === key || task.runtime_state === "alive"} onClick={() => props.onAction(node, "requeue")}>
                  {t("summary.requeue")}
                </button>
              )}
              <button type="button" disabled={props.actionTaskKey === key} onClick={() => props.onAction(node, "open")}>
                {t("summary.openSession")}
              </button>
            </div>
          )}
        </div>
      )}

      {hasChildren && !treeCollapsed && (
        <div className="taskmonitor__children" role="group">
          {node.children.map((child) => (
            <TaskTreeNode key={taskNodeKey(child.projectKey, child.task.task_id)} {...props} node={child} depth={depth + 1} />
          ))}
        </div>
      )}
    </div>
  );
}
