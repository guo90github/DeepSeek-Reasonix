// Run: tsx src/__tests__/pending-decision-banner.test.tsx
import { JSDOM } from "jsdom";
import React, { act } from "react";
import { createRoot } from "react-dom/client";

import { PendingDecisionBanner } from "../app-shell/PendingDecisionBanner";
import { LocaleProvider, useT } from "../lib/i18n";
import { selectPendingDecisions } from "../lib/pendingDecisions";
import { runtimeStateStore, type RuntimeProjection, type RuntimeSession } from "../lib/runtimeStateStore";

let passed = 0;
let failed = 0;

function ok(value: boolean, label: string) {
  if (value) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
    return;
  }
  process.stdout.write(`  FAIL  ${label}\n`);
  failed += 1;
}

function session(tabId: string, topicId: string, sessionPath: string, pendingPrompt: boolean): RuntimeSession {
  return {
    tabId,
    scope: "project",
    workspaceRoot: "/work",
    topicId,
    sessionPath,
    sessionGeneration: 1,
    open: true,
    remote: false,
    freshness: "synced",
    state: {
      schemaVersion: 1,
      runtimeEpoch: "epoch-1",
      revision: 1,
      phase: pendingPrompt ? "executing" : "idle",
      running: pendingPrompt,
      turnId: "turn-1",
      turnStatus: "running",
      turnEventSeq: 1,
      pendingPrompt,
      cancelRequested: false,
      cancellable: true,
      backgroundJobs: 0,
      activity: "waiting",
    },
  };
}

function projection(sessions: RuntimeSession[]): RuntimeProjection {
  return {
    epoch: "epoch-1",
    revision: 1,
    sessions,
    topics: [
      {
        scope: "project",
        workspaceRoot: "/work",
        node: {
          key: "project:/work",
          kind: "project",
          label: "work",
          root: "/work",
          children: [
            {
              key: "topic-b",
              kind: "topic",
              label: "主题 B",
              topicId: "topic-b",
              children: [
                { key: "session-b", kind: "session", label: "会话 B", topicId: "topic-b", sessionPath: "/work/b.jsonl" },
                { key: "session-c", kind: "session", label: "会话 C", topicId: "topic-b", sessionPath: "/work/c.jsonl" },
                { key: "session-d", kind: "session", label: "会话 D", topicId: "topic-b", sessionPath: "/work/d.jsonl" },
                { key: "session-e", kind: "session", label: "会话 E", topicId: "topic-b", sessionPath: "/work/e.jsonl" },
              ],
            },
          ],
        },
      },
    ],
  };
}

function installDom() {
  const dom = new JSDOM("<!doctype html><html><head></head><body><div id=\"root\"></div></body></html>", {
    pretendToBeVisual: true,
    url: "http://localhost/",
  });
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  globalThis.window = dom.window as unknown as Window & typeof globalThis;
  globalThis.document = dom.window.document;
  Object.defineProperty(globalThis, "navigator", { configurable: true, value: dom.window.navigator });
  globalThis.Node = dom.window.Node;
  globalThis.Element = dom.window.Element;
  globalThis.HTMLElement = dom.window.HTMLElement;
  globalThis.Event = dom.window.Event;
  globalThis.KeyboardEvent = dom.window.KeyboardEvent;
  globalThis.MouseEvent = dom.window.MouseEvent;
  globalThis.localStorage = dom.window.localStorage;
  globalThis.sessionStorage = dom.window.sessionStorage;
  globalThis.requestAnimationFrame = dom.window.requestAnimationFrame.bind(dom.window);
  globalThis.cancelAnimationFrame = dom.window.cancelAnimationFrame.bind(dom.window);
  return dom;
}

function Harness({ activeTabId, onOpen }: { activeTabId: string; onOpen: (tabId: string) => void }) {
  const t = useT();
  return React.createElement(PendingDecisionBanner, { t, activeTabId, onOpen });
}

console.log("\npending decisions");
{
  const picked = selectPendingDecisions(projection([
    session("tab-a", "topic-a", "/work/a.jsonl", true),
    session("tab-b", "topic-b", "/work/b.jsonl", true),
    session("tab-c", "topic-b", "/work/c.jsonl", false),
  ]), "tab-a");
  ok(picked.length === 1, "selector keeps only the pending tab that is not active");
  ok(picked[0]?.tabId === "tab-b", "selector keeps the waiting tab id");
  ok(picked[0]?.label === "会话 B", "selector labels the waiting tab with its session name");
  ok(selectPendingDecisions(projection([session("tab-a", "topic-a", "/work/a.jsonl", true)]), "tab-a").length === 0, "selector never counts the active tab");
  ok(selectPendingDecisions(projection([session("tab-b", "topic-b", "/work/untracked.jsonl", true)]), "tab-a")[0]?.label === "主题 B", "selector falls back to the topic label");
  ok(selectPendingDecisions(projection([session("tab-b", "topic-z", "/work/untracked.jsonl", true)]), "tab-a")[0]?.label === "tab-b", "selector falls back to the tab id");
  ok(selectPendingDecisions(undefined, "tab-a").length === 0, "selector without a snapshot yields nothing");
}

console.log("\npending decision banner");
{
  const dom = installDom();
  const rootEl = document.getElementById("root");
  if (!rootEl) throw new Error("missing root");
  const root = createRoot(rootEl);
  const opened: string[] = [];
  const render = (activeTabId: string) => root.render(React.createElement(
    LocaleProvider,
    null,
    React.createElement(Harness, { activeTabId, onOpen: (tabId: string) => opened.push(tabId) }),
  ));

  await act(async () => {
    runtimeStateStore.commit(projection([
      session("tab-a", "topic-a", "/work/a.jsonl", true),
      session("tab-b", "topic-b", "/work/b.jsonl", true),
      session("tab-c", "topic-b", "/work/c.jsonl", true),
      session("tab-d", "topic-b", "/work/d.jsonl", true),
      session("tab-e", "topic-b", "/work/e.jsonl", true),
    ]));
    render("tab-a");
  });
  ok(document.querySelector(".banner") !== null, "banner renders while another tab waits");
  ok(document.querySelectorAll(".pending-decisions__open").length === 3, "at most three waiting tabs are named");
  ok(document.querySelectorAll(".banner__hint").length === 2, "the count and the overflow are both reported");
  ok((document.querySelector(".banner")?.textContent ?? "").includes("4"), "the count reports four waiting sessions");

  const first = document.querySelector(".pending-decisions__open");
  if (!(first instanceof dom.window.HTMLButtonElement)) throw new Error("missing waiting tab button");
  await act(async () => {
    first.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
  });
  ok(opened.join(",") === "tab-b", "clicking a named tab opens that tab");

  await act(async () => {
    runtimeStateStore.commit(projection([session("tab-b", "topic-b", "/work/b.jsonl", true)]));
    render("tab-a");
  });
  const only = document.querySelector(".pending-decisions__open");
  ok(only?.textContent === "会话 B", "a single waiting tab is named by its session label");

  await act(async () => {
    runtimeStateStore.commit(projection([session("tab-b", "topic-b", "/work/b.jsonl", true)]));
    render("tab-b");
  });
  ok(document.querySelector(".banner") === null, "no banner once the only waiting tab owns the view");

  await act(async () => {
    runtimeStateStore.commit(projection([]));
    render("tab-a");
  });
  ok(document.querySelector(".banner") === null, "no banner without a waiting tab");
  await act(async () => {
    root.unmount();
  });
}

console.log(`\n${passed} passed, ${failed} failed`);
process.exit(failed === 0 ? 0 : 1);
