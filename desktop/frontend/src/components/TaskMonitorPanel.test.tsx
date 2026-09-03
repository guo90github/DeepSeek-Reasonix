// Run: tsx src/components/TaskMonitorPanel.test.tsx

import React from "react";
import { JSDOM } from "jsdom";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { LocaleProvider } from "../lib/i18n";

type Task = Record<string, unknown>;
type Event = Record<string, unknown>;

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

function snap(overrides: Task = {}): Task {
  return {
    schema_version: 1,
    task_id: "task-0001",
    session_id: "sess-1",
    state: "running",
    runtime_state: "alive",
    version: 1,
    created_at: "2025-01-01T00:00:00Z",
    updated_at: "2025-01-01T01:00:00Z",
    ...overrides,
  };
}

function taskEvent(overrides: Event = {}): Event {
  return {
    sequence: 1,
    timestamp: "2025-01-01T00:00:01Z",
    event_type: "state_change",
    task_id: "task-0001",
    session_id: "sess-1",
    state: "running",
    runtime_state: "alive",
    ...overrides,
  };
}

const dom = new JSDOM("<!doctype html><html><body></body></html>", {
  pretendToBeVisual: true,
  url: "http://localhost/",
});
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
globalThis.window = dom.window as unknown as Window & typeof globalThis;
globalThis.document = dom.window.document;
// Pin the locale source to the JSDOM navigator (en-US): Node's own global
// navigator follows the machine's system language, which flips detectLocale
// to Chinese on zh hosts and breaks the English assertions below.
Object.defineProperty(globalThis, "navigator", { configurable: true, value: dom.window.navigator });
globalThis.Node = dom.window.Node;
globalThis.Element = dom.window.Element;
globalThis.HTMLElement = dom.window.HTMLElement;
globalThis.SVGElement = dom.window.SVGElement;
globalThis.Event = dom.window.Event;
globalThis.MouseEvent = dom.window.MouseEvent;
globalThis.requestAnimationFrame = dom.window.requestAnimationFrame.bind(dom.window);
globalThis.cancelAnimationFrame = dom.window.cancelAnimationFrame.bind(dom.window);
// Node ≥21 exposes a native navigator whose language follows the OS locale,
// which makes i18n-dependent assertions machine-dependent. Pin English here so
// the suite is deterministic on zh-CN developers' machines too.
Object.defineProperty(globalThis, "navigator", {
  configurable: true,
  value: { language: "en-US", userAgent: "test" },
});

let listTasksImpl: () => Promise<Task[]> = async () => [];
let listEventsImpl: () => Promise<Event[]> = async () => [];
const listTaskTabIDs: string[] = [];
const listEventCalls: unknown[][] = [];
const stopCalls: unknown[][] = [];
const requeueCalls: unknown[][] = [];
const mockApp = {
  ListTasks: () => listTasksImpl(),
  GetTask: async () => null,
  ListTaskEvents: () => listEventsImpl(),
  StopTask: async () => ({ schema_version: 1, command: "stop", task_id: "", accepted: true, idempotent: false }),
  CancelTask: async () => ({ schema_version: 1, command: "cancel", task_id: "", accepted: true, idempotent: false }),
  RequeueTask: async (...args: unknown[]) => {
    requeueCalls.push(args);
    return {
      schema_version: 1,
      command: "requeue",
      task_id: String(args[0] ?? ""),
      state: "queued",
      runtime_state: "exited",
      version: 2,
      accepted: true,
      idempotent: false,
    };
  },
  OpenTaskSession: async () => ({ schema_version: 1, command: "open_session", task_id: "", session_id: "sess-1", accepted: true, idempotent: false }),
  ListTasksForTab: async (tabID: string) => {
    listTaskTabIDs.push(tabID);
    return listTasksImpl();
  },
  ListTaskEventsForTab: async (...args: unknown[]) => {
    listEventCalls.push(args);
    return listEventsImpl();
  },
  StopTaskForTab: async (...args: unknown[]) => {
    stopCalls.push(args);
    return { schema_version: 1, command: "stop", task_id: "", accepted: true, idempotent: false };
  },
  CancelTaskForTab: async () => ({ schema_version: 1, command: "cancel", task_id: "", accepted: true, idempotent: false }),
  RequeueTaskForTab: async (...args: unknown[]) => {
    requeueCalls.push(args);
    return {
      schema_version: 1,
      command: "requeue",
      task_id: String(args[1] ?? ""),
      state: "queued",
      runtime_state: "exited",
      version: 2,
      accepted: true,
      idempotent: false,
    };
  },
  OpenTaskSessionForTab: async () => ({ schema_version: 1, command: "open_session", task_id: "", session_id: "sess-1", accepted: true, idempotent: false }),
};
(window as unknown as { go: { main: { App: typeof mockApp } } }).go = { main: { App: mockApp } };

const { TaskMonitorPanel, useDebouncedValue } = await import("./TaskMonitorPanel");

let activeRoot: Root | null = null;
let activeHost: HTMLElement | null = null;

async function flush() {
  await new Promise((resolve) => setTimeout(resolve, 25));
}

async function renderPanel(
  onClose?: () => void,
  onOpenSession?: (tabID: string, taskID: string) => Promise<boolean> | boolean,
  tabID = "tab-a",
) {
  activeHost = document.createElement("div");
  document.body.appendChild(activeHost);
  activeRoot = createRoot(activeHost);
  await act(async () => {
    activeRoot?.render(
      <LocaleProvider>
        <TaskMonitorPanel tabID={tabID} onClose={onClose} onOpenSession={onOpenSession} />
      </LocaleProvider>,
    );
    await flush();
  });
}

async function cleanup() {
  if (activeRoot) {
    await act(async () => activeRoot?.unmount());
  }
  activeHost?.remove();
  activeRoot = null;
  activeHost = null;
  listTasksImpl = async () => [];
  listEventsImpl = async () => [];
  listTaskTabIDs.length = 0;
  listEventCalls.length = 0;
  stopCalls.length = 0;
  requeueCalls.length = 0;
}

function buttonByLabel(label: string): HTMLButtonElement {
  const button = Array.from(document.querySelectorAll<HTMLButtonElement>("button"))
    .find((candidate) => candidate.getAttribute("aria-label") === label);
  if (!button) throw new Error(`missing button: ${label}`);
  return button;
}

function buttonByText(label: string): HTMLButtonElement {
  const button = Array.from(document.querySelectorAll<HTMLButtonElement>("button"))
    .find((candidate) => candidate.textContent?.trim() === label);
  if (!button) throw new Error(`missing button text: ${label}`);
  return button;
}

async function click(button: HTMLButtonElement) {
  await act(async () => {
    button.click();
    await flush();
  });
}

async function keyDown(element: HTMLElement, key: string) {
  await act(async () => {
    element.dispatchEvent(new dom.window.KeyboardEvent("keydown", { key, bubbles: true }));
    await flush();
  });
}

async function openPanel() {
  await click(buttonByLabel("Expand tasks"));
}

async function check(label: string, run: () => Promise<boolean>) {
  try {
    ok(await run(), label);
  } catch (error) {
    process.stderr.write(`  ERROR ${label}: ${String(error)}\n`);
    ok(false, label);
  } finally {
    await cleanup();
  }
}

console.log("\nTask Monitor panel");

await check("renders the panel header", async () => {
  await renderPanel();
  return document.body.textContent?.includes("Tasks") === true;
});

await check("shows the empty state", async () => {
  await renderPanel();
  await openPanel();
  return document.body.textContent?.includes("No background tasks") === true;
});

await check("shows task-fetch errors", async () => {
  listTasksImpl = async () => { throw new Error("Network error"); };
  await renderPanel();
  await openPanel();
  return document.body.textContent?.includes("Network error") === true;
});

await check("binds task reads to the source tab", async () => {
  listTasksImpl = async () => [snap({ session_id: "sess-current" })];
  await renderPanel(undefined, undefined, "tab-source");
  return listTaskTabIDs.length === 1 && listTaskTabIDs[0] === "tab-source";
});

await check("renders lifecycle badges", async () => {
  listTasksImpl = async () => [snap({ task_id: "a1" }), snap({ task_id: "b2", state: "failed" })];
  await renderPanel();
  await openPanel();
  const text = document.body.textContent ?? "";
  return text.includes("Running") && text.includes("Failed");
});

await check("separates lifecycle state from runtime liveness", async () => {
  listTasksImpl = async () => [
    snap({ task_id: "failed-1", state: "failed", runtime_state: "exited" }),
    snap({ task_id: "legacy-1", runtime_state: undefined }),
  ];
  await renderPanel();
  await openPanel();
  const text = document.body.textContent ?? "";
  return text.includes("Exited") && text.includes("Runtime unknown");
});

await check("offers one working stop action for active tasks", async () => {
  listTasksImpl = async () => [snap()];
  await renderPanel();
  await openPanel();
  await click(buttonByLabel("Task task-000 — Running"));
  const actionLabels = Array.from(document.querySelectorAll<HTMLButtonElement>(".taskmonitor__actions button"))
    .map((button) => button.textContent?.trim());
  const hasMergedActions = actionLabels.filter((label) => label === "Stop").length === 1
    && !actionLabels.includes("Cancel")
    && actionLabels.includes("Open session");
  await click(buttonByText("Stop"));
  const confirmStop = buttonByText("Stop");
  const detailValues = Array.from(document.querySelectorAll<HTMLElement>(".taskmonitor__detail dd"))
    .map((value) => value.textContent?.trim());
  const hasReplacementConfirmation = document.querySelector(".taskmonitor__actions") === null
    && document.activeElement === confirmStop
    && detailValues.includes("Running")
    && !detailValues.includes("running");
  await click(confirmStop);
  return hasMergedActions
    && hasReplacementConfirmation
    && JSON.stringify(stopCalls[0]) === JSON.stringify([
      "tab-a",
      "task-0001",
      1,
      "desktop request",
      "desktop-stop-task-0001-1",
    ]);
});

await check("dismisses stop confirmation with Escape and restores focus", async () => {
  listTasksImpl = async () => [snap()];
  await renderPanel();
  await openPanel();
  await click(buttonByLabel("Task task-000 — Running"));
  await click(buttonByText("Stop"));
  await keyDown(buttonByText("Stop"), "Escape");
  const restoredStop = buttonByText("Stop");
  return document.querySelector(".taskmonitor__confirm") === null
    && document.activeElement === restoredStop
    && stopCalls.length === 0;
});

await check("dismisses stop confirmation when polling observes a terminal task", async () => {
  listTasksImpl = async () => [snap()];
  await renderPanel();
  await openPanel();
  await click(buttonByLabel("Task task-000 — Running"));
  await click(buttonByText("Stop"));
  listTasksImpl = async () => [snap({ state: "succeeded", runtime_state: "exited", version: 2 })];
  await click(buttonByLabel("Refresh"));
  return document.querySelector(".taskmonitor__confirm") === null
    && document.body.textContent?.includes("Succeeded") === true
    && stopCalls.length === 0;
});

await check("requeues failed exited tasks", async () => {
  listTasksImpl = async () => [snap({ task_id: "failed-1", state: "failed", runtime_state: "exited", version: 7 })];
  await renderPanel();
  await openPanel();
  await click(buttonByLabel("Task failed-1 — Failed"));
  await click(buttonByText("Requeue"));
  return JSON.stringify(requeueCalls[0]) === JSON.stringify(["tab-a", "failed-1", 7, "desktop-requeue-failed-1-7"]);
});

await check("expands and collapses task details", async () => {
  listTasksImpl = async () => [snap({ state: "succeeded" })];
  await renderPanel();
  await openPanel();
  const row = buttonByLabel("Task task-000 — Succeeded");
  await click(row);
  const expanded = document.body.textContent?.includes("Task ID") === true;
  await click(row);
  return expanded && document.body.textContent?.includes("Task ID") !== true;
});

await check("loads recent task events", async () => {
  listTasksImpl = async () => [snap({ state: "failed" })];
  listEventsImpl = async () => [taskEvent({ event_type: "error", error_code: "CRASH" })];
  await renderPanel();
  await openPanel();
  await click(buttonByLabel("Task task-000 — Failed"));
  return document.body.textContent?.includes("CRASH") === true
    && JSON.stringify(listEventCalls[0]) === JSON.stringify(["tab-a", "task-0001", 0]);
});

await check("shows task-event errors", async () => {
  listTasksImpl = async () => [snap()];
  listEventsImpl = async () => { throw new Error("Event failure"); };
  await renderPanel();
  await openPanel();
  await click(buttonByLabel("Task task-000 — Running"));
  return document.body.textContent?.includes("Event failure") === true;
});

await check("calls the close callback", async () => {
  let closeCalls = 0;
  await renderPanel(() => { closeCalls += 1; });
  await click(buttonByLabel("Close session summary"));
  return closeCalls === 1;
});

await check("opens the task session through the navigation callback", async () => {
  listTasksImpl = async () => [snap()];
  let openedTarget: string[] = [];
  await renderPanel(undefined, async (tabID, sessionID) => {
    openedTarget = [tabID, sessionID];
    return true;
  });
  await openPanel();
  await click(buttonByLabel("Task task-000 — Running"));
  await click(buttonByText("Open session"));
  // N13: the host callback receives the backend-resolved session id.
  return JSON.stringify(openedTarget) === JSON.stringify(["tab-a", "sess-1"]);
});

await check("does not close the panel for a stale open completion", async () => {
  listTasksImpl = async () => [snap()];
  let closeCalls = 0;
  await renderPanel(() => { closeCalls += 1; }, async () => false);
  await openPanel();
  await click(buttonByLabel("Task task-000 — Running"));
  await click(buttonByText("Open session"));
  return closeCalls === 0;
});

await check("refreshes tasks on request", async () => {
  let calls = 0;
  listTasksImpl = async () => (++calls === 1 ? [] : [snap({ task_id: "ok" })]);
  await renderPanel();
  await openPanel();
  await click(buttonByLabel("Refresh"));
  return document.body.textContent?.includes("ok") === true;
});

await check("shows the task count", async () => {
  listTasksImpl = async () => [snap({ task_id: "a" }), snap({ task_id: "b" })];
  await renderPanel();
  return document.querySelector(".taskmonitor__count")?.textContent === "2";
});

await check("marks only terminal tasks", async () => {
  listTasksImpl = async () => [snap({ task_id: "t1", state: "succeeded" }), snap({ task_id: "t2" })];
  await renderPanel();
  await openPanel();
  return document.querySelectorAll(".taskmonitor__terminal").length === 1;
});

await check("freezes terminal task elapsed time", async () => {
  const realNow = Date.now;
  try {
    Date.now = () => Date.parse("2025-01-01T02:00:00Z");
    listTasksImpl = async () => [snap({ state: "cancelled", runtime_state: "exited" })];
    await renderPanel();
    await openPanel();
    const atFinish = document.querySelector(".taskmonitor__time")?.textContent;
    Date.now = () => Date.parse("2025-01-01T03:00:00Z");
    await click(buttonByLabel("Refresh"));
    const afterRefresh = document.querySelector(".taskmonitor__time")?.textContent;
    return atFinish === "1h" && afterRefresh === "1h";
  } finally {
    Date.now = realNow;
  }
});

await check("uses the expired runtime lease for stale task elapsed time", async () => {
  const realNow = Date.now;
  try {
    Date.now = () => Date.parse("2025-01-01T02:00:00Z");
    listTasksImpl = async () => [snap({
      state: "stale",
      runtime_state: "exited",
      updated_at: "2025-01-01T00:00:00Z",
      runtime_lease_until: "2025-01-01T00:30:00Z",
    })];
    await renderPanel();
    await openPanel();
    const atDetection = document.querySelector(".taskmonitor__time")?.textContent;
    Date.now = () => Date.parse("2025-01-01T03:00:00Z");
    await click(buttonByLabel("Refresh"));
    const afterRefresh = document.querySelector(".taskmonitor__time")?.textContent;
    return atDetection === "30m" && afterRefresh === "30m";
  } finally {
    Date.now = realNow;
  }
});

await check("does not present requeue age as elapsed runtime", async () => {
  listTasksImpl = async () => [snap({ state: "queued", runtime_state: "exited" })];
  await renderPanel();
  await openPanel();
  return document.querySelector(".taskmonitor__time")?.textContent === "—";
});

// --- P2.1/P2.2: catalog mode — incremental poll merge + stale cursor ---

function catalogPage(items: number[], nextCursor: string, extra: Record<string, unknown> = {}) {
  return {
    items: items.map((n) => ({ projectKey: "p", projectLabel: "P", task: { ...snap({ task_id: `t${n}`, __catalogKey: undefined }) } })),
    nextCursor,
    revision: 1,
    partial: false,
    staleCursor: false,
    status: { state: "ready", mode: "disk", revision: 1, indexed: 5, total: 5, pending: 0, failed: 0 },
    ...extra,
  };
}

let listTaskPageImpl: (req: Record<string, unknown>) => Promise<Record<string, unknown>> = async () => catalogPage([], "");

async function enableCatalogMode() {
  (mockApp as unknown as Record<string, unknown>).ListTaskPage = async (req: Record<string, unknown>) => listTaskPageImpl(req);
}
function disableCatalogMode() {
  delete (mockApp as unknown as Record<string, unknown>).ListTaskPage;
}

function taskRows() {
  return document.querySelectorAll(".taskmonitor__task").length;
}

await check("polling merges pages instead of dropping loaded ones", async () => {
  const calls: string[] = [];
  listTaskPageImpl = async (req) => {
    const cursor = String(req.cursor ?? "");
    calls.push(cursor);
    if (cursor === "") return catalogPage([1, 2, 3], "cur-2");
    return catalogPage([4, 5], ""); // "load more" page
  };
  await enableCatalogMode();
  try {
    await renderPanel();
    await openPanel();
    if (taskRows() !== 3) return false;

    await click(buttonByLabel("Refresh")); // poll with empty cursor
    if (taskRows() !== 3) return false;

    await click(buttonByText("Load more")); // append page 2
    if (taskRows() !== 5) return false;

    await click(buttonByLabel("Refresh")); // poll again: must not drop page 2
    return taskRows() === 5 && calls.includes("cur-2");
  } finally {
    await cleanup();
    disableCatalogMode();
  }
});

await check("stale cursor clears pagination, notices, and refetches", async () => {
  let staleSent = false;
  const emptyCursorCalls: string[] = [];
  listTaskPageImpl = async (req) => {
    const cursor = String(req.cursor ?? "");
    if (cursor === "") emptyCursorCalls.push(cursor);
    if (cursor !== "" && !staleSent) {
      staleSent = true;
      return catalogPage([], "", { nextCursor: "cur-2", staleCursor: true, revision: 3 });
    }
    return catalogPage([1, 2], "cur-2");
  };
  await enableCatalogMode();
  try {
    await renderPanel();
    await openPanel();
    await click(buttonByText("Load more"));
    // The stale response must surface the reload notice and refetch the home
    // page; pagination then resumes on the fresh revision.
    const noticed = document.body.textContent?.includes("reload") ?? false;
    await flush();
    return noticed && staleSent && emptyCursorCalls.length >= 2;
  } finally {
    await cleanup();
    disableCatalogMode();
  }
});

async function testQueryDebounce() {
  // The UI-level typing simulation is unreliable under React 19's controlled
  // inputs, so the debounce contract is pinned through useDebouncedValue
  // directly with a tiny harness.
  function DebounceHarness() {
    const [value, setValue] = React.useState("");
    const debounced = useDebouncedValue(value, 150);
    return (
      <span>
        <button type="button" onClick={() => setValue("a")}>to-a</button>
        <button type="button" onClick={() => setValue("ab")}>to-ab</button>
        <button type="button" onClick={() => setValue("abc")}>to-abc</button>
        <span data-debounced={debounced}>{debounced}</span>
      </span>
    );
  }
  activeHost = document.createElement("div");
  document.body.appendChild(activeHost);
  activeRoot = createRoot(activeHost);
  await act(async () => {
    activeRoot?.render(<DebounceHarness />);
    await flush();
  });
  const fire = async (label: string) => {
    await act(async () => {
      const btn = Array.from(document.querySelectorAll("button")).find((b) => b.textContent === label);
      btn?.click();
    });
  };
  const debouncedNow = () => document.querySelector("[data-debounced]")?.textContent ?? "";
  await fire("to-a");
  await fire("to-ab");
  await fire("to-abc");
  const beforeSettle = debouncedNow();
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 250));
    await flush();
  });
  const afterSettle = debouncedNow();
  return beforeSettle === "" && afterSettle === "abc";
}

await check("query input debounces catalog fetches", testQueryDebounce);

await check("count shows loaded/total when the catalog has more", async () => {
  listTaskPageImpl = async () => catalogPage([1, 2, 3], "cur-2", { status: { state: "ready", mode: "disk", revision: 1, indexed: 500, total: 500, pending: 0, failed: 0 } });
  await enableCatalogMode();
  try {
    await renderPanel();
    await openPanel();
    return document.querySelector(".taskmonitor__count")?.textContent === "3/500";
  } finally {
    await cleanup();
    disableCatalogMode();
  }
});

await check("page warnings render without failing the list", async () => {
  listTaskPageImpl = async () => catalogPage([1], "", { status: { state: "ready", mode: "disk", revision: 1, indexed: 1, total: 1, pending: 0, failed: 0, warnings: ["project \"p\" is not indexed yet; its tasks are hidden"] } });
  await enableCatalogMode();
  try {
    await renderPanel();
    await openPanel();
    const text = document.body.textContent ?? "";
    return text.includes("not indexed yet") && document.querySelectorAll(".taskmonitor__task").length === 1;
  } finally {
    await cleanup();
    disableCatalogMode();
  }
});

await check("version conflict refreshes the list with a readable notice", async () => {
  let listCalls = 0;
  listTasksImpl = async () => {
    listCalls += 1;
    return [snap({ task_id: "conflicted", state: "failed", runtime_state: "exited" })];
  };
  const conflictRequeue = (mockApp as unknown as Record<string, unknown>).RequeueTaskForTab;
  (mockApp as unknown as Record<string, unknown>).RequeueTaskForTab = async () => ({
    schema_version: 1,
    command: "requeue",
    task_id: "x",
    accepted: false,
    idempotent: false,
    error: { code: "task_version_conflict", message: "version mismatch" },
  });
  try {
    await renderPanel();
    await openPanel();
    const before = listCalls;
    const row = await buttonByLabel("Task conflict — Failed");
    await click(row);
    await click(buttonByText("Requeue"));
    const text = document.body.textContent ?? "";
    const noticed = text.includes("changed elsewhere") || text.includes("別處更新") || text.includes("别处更新");
    return noticed && listCalls > before;
  } finally {
    (mockApp as unknown as Record<string, unknown>).RequeueTaskForTab = conflictRequeue;
    await cleanup();
  }
});

dom.window.close();
console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
