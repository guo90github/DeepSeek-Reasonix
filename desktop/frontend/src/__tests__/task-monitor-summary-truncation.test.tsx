// Run: pnpm test (auto-discovered by scripts/run-tests.mjs)
// P0.2 (N3): the desktop panel shows error_summary truncated by default with a
// click-to-expand, aligning its sensitive-field exposure with the CLI's
// content-free boundary.

import { JSDOM } from "jsdom";
import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { LocaleProvider } from "../lib/i18n";
import { taskNodeKey } from "../lib/taskTree";
import type { TaskNode } from "../lib/taskCatalogTypes";
import type { TaskSnapshot } from "../lib/types";

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

async function check(label: string, run: () => Promise<boolean>) {
  try {
    ok(await run(), label);
  } catch (error) {
    ok(false, `${label} — ${String(error)}`);
  }
}

const dom = new JSDOM("<!doctype html><html><body></body></html>", {
  pretendToBeVisual: true,
  url: "http://localhost/",
});
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
globalThis.window = dom.window as unknown as Window & typeof globalThis;
globalThis.document = dom.window.document;
globalThis.Node = dom.window.Node;
globalThis.Element = dom.window.Element;
globalThis.HTMLElement = dom.window.HTMLElement;
globalThis.SVGElement = dom.window.SVGElement;
globalThis.Event = dom.window.Event;
globalThis.MouseEvent = dom.window.MouseEvent;
globalThis.localStorage = dom.window.localStorage;
globalThis.requestAnimationFrame = dom.window.requestAnimationFrame.bind(dom.window);
globalThis.cancelAnimationFrame = dom.window.cancelAnimationFrame.bind(dom.window);

const { TaskTreeNode } = await import("../components/TaskTreeNode");

let activeRoot: Root | null = null;
let activeHost: HTMLElement | null = null;

async function flush() {
  await new Promise((resolve) => setTimeout(resolve, 25));
}

const longSummary = "build failed: " + "x".repeat(300);

function failedTask(): TaskSnapshot {
  return {
    schema_version: 2,
    task_id: "task-1",
    session_id: "session-1",
    state: "failed",
    version: 3,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:01:00Z",
    error_code: "job_failed",
    error_summary: longSummary,
  };
}

async function renderRow() {
  activeHost = document.createElement("div");
  document.body.appendChild(activeHost);
  activeRoot = createRoot(activeHost);
  const node: TaskNode = {
    projectKey: "proj-a",
    projectLabel: "Project A",
    task: failedTask(),
    children: [],
  };
  const key = taskNodeKey(node.projectKey, node.task.task_id);
  const stopRefs = React.createRef<Map<string, HTMLButtonElement>>();
  stopRefs.current = new Map();
  await act(async () => {
    activeRoot?.render(
      <LocaleProvider>
        <TaskTreeNode
          node={node}
          depth={0}
          collapsed={new Set()}
          expanded={new Set([key])}
          taskEvents={new Map()}
          eventsLoading={new Set()}
          eventsError={new Map()}
          pendingStopKey={null}
          actionTaskKey={null}
          nowMs={Date.now()}
          scope="project"
          confirmStopRef={React.createRef<HTMLButtonElement | null>()}
          stopButtonRefs={stopRefs}
          onToggleRow={() => {}}
          onToggleSubtree={() => {}}
          onRequestStop={() => {}}
          onDismissStop={() => {}}
          onAction={() => {}}
        />
      </LocaleProvider>,
    );
    await flush();
  });
  return document.querySelector(".taskmonitor__err-summary");
}

async function cleanup() {
  if (activeRoot) {
    await act(async () => activeRoot?.unmount());
  }
  activeHost?.remove();
  activeRoot = null;
  activeHost = null;
}

async function testSummaryTruncatesAndExpands() {
  await renderRow();
  const btn = document.querySelector<HTMLElement>(".taskmonitor__err-summary");
  ok(!!btn, "error_summary renders as an element");
  if (!btn) return;
  ok(!btn.classList.contains("taskmonitor__err-summary--expanded"), "summary is clamped by default");
  ok(btn.getAttribute("aria-expanded") === "false", "aria-expanded starts false");
  ok(btn.textContent === longSummary, "full summary text is present in the DOM");
  await act(async () => {
    btn.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    await flush();
  });
  ok(btn.classList.contains("taskmonitor__err-summary--expanded"), "click expands the summary");
  ok(btn.getAttribute("aria-expanded") === "true", "aria-expanded flips to true");
  await cleanup();
  return true;
}

async function testNoSummaryRendersNoButton() {
  const node: TaskNode = {
    projectKey: "proj-a",
    projectLabel: "Project A",
    task: { ...failedTask(), error_summary: undefined },
    children: [],
  };
  activeHost = document.createElement("div");
  document.body.appendChild(activeHost);
  activeRoot = createRoot(activeHost);
  const key = taskNodeKey(node.projectKey, node.task.task_id);
  const stopRefs = React.createRef<Map<string, HTMLButtonElement>>();
  stopRefs.current = new Map();
  await act(async () => {
    activeRoot?.render(
      <LocaleProvider>
        <TaskTreeNode
          node={node}
          depth={0}
          collapsed={new Set()}
          expanded={new Set([key])}
          taskEvents={new Map()}
          eventsLoading={new Set()}
          eventsError={new Map()}
          pendingStopKey={null}
          actionTaskKey={null}
          nowMs={Date.now()}
          scope="project"
          confirmStopRef={React.createRef<HTMLButtonElement | null>()}
          stopButtonRefs={stopRefs}
          onToggleRow={() => {}}
          onToggleSubtree={() => {}}
          onRequestStop={() => {}}
          onDismissStop={() => {}}
          onAction={() => {}}
        />
      </LocaleProvider>,
    );
    await flush();
  });
  ok(!document.querySelector(".taskmonitor__err-summary"), "no summary button when error_summary is absent");
  await cleanup();
  return true;
}

await check("truncated error_summary expands on click", testSummaryTruncatesAndExpands);
await check("no error_summary renders no button", testNoSummaryRendersNoButton);

// --- P1.2/P1.4: cost/steps detail rows + long-running warning ---

async function renderTaskNode(node: TaskNode, nowMs: number) {
  activeHost = document.createElement("div");
  document.body.appendChild(activeHost);
  activeRoot = createRoot(activeHost);
  const key = taskNodeKey(node.projectKey, node.task.task_id);
  const stopRefs = React.createRef<Map<string, HTMLButtonElement>>();
  stopRefs.current = new Map();
  await act(async () => {
    activeRoot?.render(
      <LocaleProvider>
        <TaskTreeNode
          node={node}
          depth={0}
          collapsed={new Set()}
          expanded={new Set([key])}
          taskEvents={new Map()}
          eventsLoading={new Set()}
          eventsError={new Map()}
          pendingStopKey={null}
          actionTaskKey={null}
          nowMs={nowMs}
          scope="project"
          confirmStopRef={React.createRef<HTMLButtonElement | null>()}
          stopButtonRefs={stopRefs}
          onToggleRow={() => {}}
          onToggleSubtree={() => {}}
          onRequestStop={() => {}}
          onDismissStop={() => {}}
          onAction={() => {}}
        />
      </LocaleProvider>,
    );
    await flush();
  });
}

function runningTask(createdAt: string): TaskSnapshot {
  return {
    schema_version: 2,
    task_id: "task-run",
    session_id: "session-1",
    state: "running",
    runtime_state: "alive",
    version: 1,
    created_at: createdAt,
    updated_at: createdAt,
  };
}

async function testLongRunningWarningAppearsPastThreshold() {
  const nowMs = Date.now();
  const old = new Date(nowMs - 11 * 60 * 1000).toISOString();
  await renderTaskNode(
    { projectKey: "p", projectLabel: "P", task: runningTask(old), children: [] },
    nowMs,
  );
  ok(!!document.querySelector(".taskmonitor__long-running"), "live task past 10 minutes shows the warning badge");
  await cleanup();

  const fresh = new Date(nowMs - 60 * 1000).toISOString();
  await renderTaskNode(
    { projectKey: "p", projectLabel: "P", task: runningTask(fresh), children: [] },
    nowMs,
  );
  ok(!document.querySelector(".taskmonitor__long-running"), "live task under 10 minutes shows no warning");
  await cleanup();
  return true;
}

async function testCostAndStepsRowsRenderWhenAvailable() {
  const nowMs = Date.now();
  const task: TaskSnapshot = {
    ...failedTask(),
    steps_used: 7,
    cost_total: "$ 0.42",
    cost_status: "ok",
  };
  await renderTaskNode({ projectKey: "p", projectLabel: "P", task, children: [] }, nowMs);
  const detail = document.querySelector<HTMLElement>(".taskmonitor__detail");
  ok(!!detail && detail.textContent?.includes("7"), "detail shows steps used");
  ok(!!detail && detail.textContent?.includes("$ 0.42"), "detail shows cost when available");
  await cleanup();

  await renderTaskNode(
    { projectKey: "p", projectLabel: "P", task: { ...failedTask(), steps_used: 3, cost_status: "unavailable" }, children: [] },
    nowMs,
  );
  const detail2 = document.querySelector<HTMLElement>(".taskmonitor__detail");
  ok(!!detail2 && detail2.textContent?.includes("3"), "detail still shows steps without pricing");
  ok(!!detail2 && !detail2.textContent?.includes("$"), "cost row is hidden when pricing is unavailable");
  await cleanup();
  return true;
}

await check("long-running warning past the 10-minute threshold", testLongRunningWarningAppearsPastThreshold);
await check("cost and steps rows render when available", testCostAndStepsRowsRenderWhenAvailable);

async function testRemovedTombstoneRendersReadOnly() {
  const nowMs = Date.now();
  const task: TaskSnapshot = {
    ...failedTask(),
    state: "removed",
  };
  await renderTaskNode({ projectKey: "p", projectLabel: "P", task, children: [] }, nowMs);
  const badge = document.querySelector<HTMLElement>(".taskmonitor__badge");
  ok(!!badge && ["Removed", "已移除", "已移除"].includes(badge.textContent ?? ""), "removed state renders its badge");
  ok(!document.querySelector(".taskmonitor__requeue") && !document.querySelector(".taskmonitor__stop"), "removed task offers no requeue/stop actions");
  await cleanup();
  return true;
}

await check("removed tombstone renders read-only", testRemovedTombstoneRendersReadOnly);

if (failed > 0) {
  process.stdout.write(`task-monitor-summary-truncation: ${failed} failed, ${passed} passed\n`);
  process.exit(1);
}
process.stdout.write(`task-monitor-summary-truncation: ${passed} passed\n`);

