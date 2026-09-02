import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  AlertCircle,
  ChevronDown,
  ChevronRight,
  Clock,
  Loader2,
  RotateCw,
  X,
} from "lucide-react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { buildTaskTree, taskNodeKey } from "../lib/taskTree";
import type { TaskNode } from "../lib/taskCatalogTypes";
import type { TaskEvent, TaskSnapshot } from "../lib/types";
import { isTerminalState, isStoppableState } from "./TaskTreeNode";
import { TaskTreeView } from "./TaskTreeView";

type CatalogTask = TaskSnapshot & { __projectKey: string; __projectLabel: string; __catalogKey: string };

function hasTaskCatalogBinding(): boolean {
  const bound = (window as unknown as { go?: { main?: { App?: { ListTaskPage?: unknown } } } }).go?.main?.App?.ListTaskPage;
  return typeof bound === "function";
}

// Rebuilds the decorated row shape the panel's state uses from a tree node.
// The catalog key is taskNodeKey(projectKey, task_id) in both catalog and
// legacy mode (legacy tasks carry an empty projectKey), so event expansion
// and action state share one key namespace with the tree.
function catalogTaskOf(node: TaskNode): CatalogTask {
  return { ...node.task, __projectKey: node.projectKey, __projectLabel: node.projectLabel, __catalogKey: taskNodeKey(node.projectKey, node.task.task_id) };
}

// --- component ---

const POLL_INTERVAL_MS = 5000;

// useDebouncedValue settles fast-changing values (N10): the returned value only
// updates after the input has been quiet for delayMs.
export function useDebouncedValue(value: string, delayMs: number): string {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(timer);
  }, [value, delayMs]);
  return debounced;
}

export function TaskMonitorPanel({
	tabID,
  onClose,
  onOpenSession,
  initialOpen = false,
  initialScope = "session",
  popover = false,
  summaryMode = false,
}: {
  tabID: string;
  onClose?: () => void;
  onOpenSession?: (tabID: string, sessionID: string) => Promise<boolean> | boolean;
  initialOpen?: boolean;
  initialScope?: "session" | "project" | "all";
  popover?: boolean;
  summaryMode?: boolean;
}) {
  const t = useT();
  const [tasks, setTasks] = useState<CatalogTask[]>([]);
	const [scope, setScope] = useState<"session" | "project" | "all">(initialScope);
	const [query, setQuery] = useState("");
	// Debounced query (N10): keystrokes settle 150ms before a Wails call.
	const debouncedQuery = useDebouncedValue(query, 150);
	const [nextCursor, setNextCursor] = useState("");
	const [indexProgress, setIndexProgress] = useState<{ indexed: number; total: number; partial: boolean }>({ indexed: 0, total: 0, partial: true });
	const [warnings, setWarnings] = useState<string[]>([]);
	const [staleNotice, setStaleNotice] = useState<string | null>(null);
	const requestSeq = useRef(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [open, setOpen] = useState(initialOpen);
  const [actionTask, setActionTask] = useState<string | null>(null);
  const [actionError, setActionError] = useState<string | null>(null);
  const [actionMessage, setActionMessage] = useState<string | null>(null);
  const [nowMs, setNowMs] = useState(() => Date.now());
  const [pendingStop, setPendingStop] = useState<CatalogTask | null>(null);
  const stopButtonRefs = useRef<Map<string, HTMLButtonElement>>(new Map());
  const confirmStopRef = useRef<HTMLButtonElement | null>(null);

  // Per-task event state
  const [taskEvents, setTaskEvents] = useState<Map<string, TaskEvent[]>>(
    () => new Map(),
  );
  const [eventsLoading, setEventsLoading] = useState<Set<string>>(new Set());
  const [eventsError, setEventsError] = useState<Map<string, string>>(
    () => new Map(),
  );
  const eventCursors = useRef<Map<string, number>>(new Map());

  // Tree view: build the forest from the flat page, then seed subtree collapse
  // defaults once (L1 popover collapses everything; L2 expands the first two
  // levels) so polling refreshes never reset the user's toggles.
  const tree = useMemo(
    () => buildTaskTree(tasks.map((task) => ({ projectKey: task.__projectKey, projectLabel: task.__projectLabel, task }))),
    [tasks],
  );
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
  const seededCollapse = useRef(false);
  useEffect(() => {
    if (seededCollapse.current || tree.length === 0) return;
    seededCollapse.current = true;
    const keys: string[] = [];
    const walk = (nodes: TaskNode[], depth: number) => {
      for (const n of nodes) {
        if (n.children.length > 0 && (popover || depth >= 2)) {
          keys.push(taskNodeKey(n.projectKey, n.task.task_id));
        }
        walk(n.children, depth + 1);
      }
    };
    walk(tree, 0);
    setCollapsed(new Set(keys));
  }, [tree, popover]);

  const fetchTasks = useCallback(async (cursor = "") => {
	const seq = ++requestSeq.current;
    try {
      setError(null);
			if (!hasTaskCatalogBinding()) {
				const legacy = await app.ListTasksForTab(tabID);
				if (seq !== requestSeq.current) return;
				const filtered = legacy.filter((task) => !debouncedQuery.trim() || [task.task_id, task.session_id, task.error_code].some((value) => (value || "").toLowerCase().includes(debouncedQuery.trim().toLowerCase())));
				setTasks(filtered.map((task) => ({ ...task, __projectKey: "", __projectLabel: "", __catalogKey: taskNodeKey("", task.task_id) })));
				setNextCursor("");
				setIndexProgress({ indexed: filtered.length, total: filtered.length, partial: false });
				setWarnings([]);
				setStaleNotice(null);
				return;
			}
			const page = await app.ListTaskPage({ scope, tabId: tabID, projectKey: "", states: [], query: debouncedQuery, cursor, limit: 50 });
			if (seq !== requestSeq.current) return;
			const decorated = (page.items ?? []).map((item) => ({ ...item.task, __projectKey: item.projectKey, __projectLabel: item.projectLabel, __catalogKey: `${item.projectKey}:${item.task.task_id}` }));
			// Incremental merge (N8): polls with an empty cursor keep the
			// accumulated "load more" pages instead of replacing the list; fresh
			// frames win per __catalogKey.
			setTasks((current) => {
				const merged = new Map(current.map((item) => [item.__catalogKey, item] as const));
				for (const item of decorated) merged.set(item.__catalogKey, item);
				return [...merged.values()];
			});
			// StaleCursor (N9): the revision moved under a cursor page. Drop the
			// cursor, surface a notice, and refetch the first page once.
			if (page.staleCursor) {
				setNextCursor("");
				setStaleNotice(t("summary.staleReload"));
				void fetchTasks("");
				return;
			}
			setNextCursor(page.nextCursor || "");
			if (staleNotice) setStaleNotice(null);
			setIndexProgress({ indexed: page.status.indexed, total: page.status.total, partial: page.partial });
			setWarnings(page.status.warnings ?? []);
    } catch (e) {
			if (seq !== requestSeq.current) return;
      setError(String(e));
    } finally {
      setLoading(false);
    }
  }, [debouncedQuery, scope, tabID]);

  // Fetch events for a single task, using afterSequence for incremental load.
	const fetchEvents = useCallback(async (node: TaskNode) => {
		const taskID = taskNodeKey(node.projectKey, node.task.task_id);
    setEventsLoading((prev) => new Set(prev).add(taskID));
    setEventsError((prev) => {
      const next = new Map(prev);
      next.delete(taskID);
      return next;
    });
    try {
      const cursor = eventCursors.current.get(taskID) ?? 0;
			const events = hasTaskCatalogBinding()
				? (await app.ListTaskEventPage({ projectKey: node.projectKey, taskId: node.task.task_id, after: cursor, limit: 50 })).items ?? []
				: await app.ListTaskEventsForTab(tabID, node.task.task_id, cursor);
      if (events.length > 0) {
        setTaskEvents((prev) => {
          const next = new Map(prev);
          const existing = next.get(taskID) ?? [];
          // Merge, deduplicate by sequence
          const seen = new Set(existing.map((e) => e.sequence));
          const merged = [...existing, ...events.filter((e) => !seen.has(e.sequence))];
          merged.sort((a, b) => a.sequence - b.sequence);
          next.set(taskID, merged);
          return next;
        });
        // Update cursor to the max sequence
        const maxSeq = events.reduce(
          (max, e) => Math.max(max, e.sequence),
          cursor,
        );
        eventCursors.current.set(taskID, maxSeq);
      }
    } catch (e) {
      setEventsError((prev) => {
        const next = new Map(prev);
        next.set(taskID, String(e));
        return next;
      });
    } finally {
      setEventsLoading((prev) => {
        const next = new Set(prev);
        next.delete(taskID);
        return next;
      });
    }
	}, [tabID]);

  // Initial fetch + periodic polling
  useEffect(() => {
		void fetchTasks("");
    const interval = setInterval(() => {
			void fetchTasks("");
    }, POLL_INTERVAL_MS);
    return () => clearInterval(interval);
	}, [fetchTasks]);

  // Live tasks need a ticking clock; terminal and queued tasks stay frozen at
  // their persisted end/update time.
  useEffect(() => {
    if (!tasks.some((task) => task.runtime_state === "alive" && !isTerminalState(task.state))) return;
    const interval = setInterval(() => setNowMs(Date.now()), 1000);
    return () => clearInterval(interval);
  }, [tasks]);

  useEffect(() => {
    if (pendingStop) confirmStopRef.current?.focus();
  }, [pendingStop]);

  useEffect(() => {
    if (!pendingStop) return;
    const current = tasks.find((task) => task.__catalogKey === pendingStop.__catalogKey);
    if (!current || !isStoppableState(current.state)) setPendingStop(null);
  }, [pendingStop, tasks]);

  const dismissStopConfirmation = () => {
    const taskKey = pendingStop?.__catalogKey;
    setPendingStop(null);
    if (taskKey) {
      requestAnimationFrame(() => stopButtonRefs.current.get(taskKey)?.focus());
    }
  };

  const toggleTask = (node: TaskNode) => {
    const id = taskNodeKey(node.projectKey, node.task.task_id);
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
        // Load events on first expand
        if (!taskEvents.has(id)) {
			void fetchEvents(node);
        }
      }
      return next;
    });
  };

  const controlTask = async (node: TaskNode, action: "stop" | "requeue" | "open") => {
    const task = catalogTaskOf(node);
    setPendingStop(null);
    setActionTask(task.__catalogKey);
    setActionError(null);
    setActionMessage(null);
    try {
			const request = { projectKey: task.__projectKey, taskId: task.task_id, expectedVersion: task.version, reason: "desktop request", idempotencyKey: `desktop-${action}-${task.task_id}-${task.version}` };
			if (action === "open") {
				// Single backend contract (N13): resolve the session through the
				// keyed binding for every scope, then let the host navigate.
				const result = hasTaskCatalogBinding()
					? await app.OpenTaskSessionByKey({ projectKey: task.__projectKey, taskId: task.task_id })
					: await app.OpenTaskSessionForTab(tabID, task.task_id);
				if (result.error) {
					setActionError(`${result.error.code}: ${result.error.message}`);
					return;
				}
				const sessionID = result.session_id?.trim();
				if (!sessionID) throw new Error("Task session is unavailable");
				if (onOpenSession) {
					const opened = await onOpenSession(tabID, sessionID);
					if (opened) onClose?.();
				} else {
					setActionMessage(`Session: ${sessionID}`);
				}
				return;
			}
			const result = hasTaskCatalogBinding()
				? action === "stop"
					? await app.StopTaskByKey(request)
					: await app.RequeueTaskByKey(request)
				: action === "stop"
					? await app.StopTaskForTab(tabID, task.task_id, task.version, request.reason, request.idempotencyKey)
					: await app.RequeueTaskForTab(tabID, task.task_id, task.version, request.idempotencyKey);
      if (result.error) {
        if (result.error.code === "task_version_conflict") {
          // B5: another process (CLI/Desktop) wrote this task. Surface a
          // readable notice and pull the latest snapshot instead of failing
          // silently with a stale expectedVersion.
          setActionMessage(t("summary.versionConflict"));
          await fetchTasks("");
        } else {
          setActionError(`${result.error.code}: ${result.error.message}`);
        }
      } else {
        setActionMessage(result.idempotent ? "Already applied" : "Task updated");
				await fetchTasks("");
      }
    } catch (e) {
      setActionError(String(e));
    } finally {
      setActionTask(null);
    }
  };

  return (
    <div className={`taskmonitor${popover ? " taskmonitor--popover" : ""}`}>
      <div className="taskmonitor__head">
        <button
          className="taskmonitor__toggle"
          onClick={() => setOpen((v) => !v)}
          aria-expanded={open}
          aria-label={open ? "Collapse tasks" : "Expand tasks"}
        >
          {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </button>
        <span className="taskmonitor__title">{summaryMode ? t("summary.session") : t("summary.tasks")}</span>
        <span className="taskmonitor__count" title={indexProgress.total > 0 ? `${indexProgress.indexed}/${indexProgress.total} indexed` : undefined}>
          {indexProgress.total > tasks.length ? `${tasks.length}/${indexProgress.total}` : tasks.length}
        </span>
        <button
          className="taskmonitor__refresh"
			onClick={() => {
            setLoading(true);
				void fetchTasks("");
          }}
          title={t("summary.refresh")}
          aria-label={t("summary.refresh")}
        >
          <RotateCw size={12} />
        </button>
        {onClose && (
          <button
            className="taskmonitor__close"
            onClick={onClose}
            title={t("common.close")}
            aria-label={t("summary.close")}
          >
            <X size={14} />
          </button>
        )}
      </div>

      {open && (
        <div className="taskmonitor__body">
			{!summaryMode && (
				<div className="taskmonitor__filters">
					<select value={scope} onChange={(event) => setScope(event.target.value as "session" | "project" | "all")} aria-label="Task scope">
						<option value="session">Current session</option>
						<option value="project">Current project</option>
						<option value="all">All projects</option>
					</select>
					<input value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Filter tasks" aria-label="Filter tasks" />
				</div>
			)}
			{indexProgress.partial && <div className="taskmonitor__indexing">Indexing tasks ({indexProgress.indexed}/{indexProgress.total})</div>}
			{staleNotice && <div className="taskmonitor__state taskmonitor__state--error">{staleNotice}</div>}
			{warnings.length > 0 && (
				<div className="taskmonitor__state taskmonitor__state--warning">
					{warnings.map((warning, i) => (
						<span key={i}>{warning}</span>
					))}
				</div>
			)}
          {summaryMode && <div className="taskmonitor__category-title">{t("summary.tasks")}</div>}
          {actionError && <div className="taskmonitor__state taskmonitor__state--error">{actionError}</div>}
          {actionMessage && <div className="taskmonitor__state">{actionMessage}</div>}
          {loading && (
            <div className="taskmonitor__state">
              <Loader2 size={16} className="taskmonitor__spinner" />
              <span>{t("common.loading")}</span>
            </div>
          )}

          {error && (
            <div className="taskmonitor__state taskmonitor__state--error">
              <AlertCircle size={16} />
              <span>{error}</span>
            </div>
          )}

          {!loading && !error && tasks.length === 0 && (
            <div className="taskmonitor__state taskmonitor__state--empty">
              <Clock size={16} />
              <span>{t("summary.noTasks")}</span>
            </div>
          )}

          {!loading && !error && tasks.length > 0 && (
            <TaskTreeView
              roots={tree}
              collapsed={collapsed}
              expanded={expanded}
              taskEvents={taskEvents}
              eventsLoading={eventsLoading}
              eventsError={eventsError}
              pendingStopKey={pendingStop?.__catalogKey ?? null}
              actionTaskKey={actionTask}
              nowMs={nowMs}
              scope={scope}
              confirmStopRef={confirmStopRef}
              stopButtonRefs={stopButtonRefs}
              onToggleRow={toggleTask}
              onToggleSubtree={(key) =>
                setCollapsed((prev) => {
                  const next = new Set(prev);
                  if (next.has(key)) next.delete(key);
                  else next.add(key);
                  return next;
                })
              }
              onRequestStop={(node) => setPendingStop(catalogTaskOf(node))}
              onDismissStop={dismissStopConfirmation}
              onAction={(node, action) => void controlTask(node, action)}
            />
          )}
			{nextCursor && !loading && !error && (
				<button className="taskmonitor__load-more" onClick={() => void fetchTasks(nextCursor)}>
					Load more
				</button>
			)}
        </div>
      )}
    </div>
  );
}
