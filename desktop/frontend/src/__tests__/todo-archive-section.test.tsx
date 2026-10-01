// Run: node --import ./scripts/css-stub-register.mjs --import ./scripts/svg-stub-register.mjs --import tsx src/__tests__/todo-archive-section.test.tsx
// The board's archive (docs/40 S3): read-only history under the live list,
// folded by default, listed on demand, and absent when nothing finished.

import { JSDOM } from "jsdom";
import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { LocaleProvider } from "../lib/i18n";
import type { Todo } from "../lib/tools";

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
dom.window.Element.prototype.scrollIntoView = () => {};

const { TodoPanel } = await import("../components/TodoPanel");

let root: Root | null = null;
let host: HTMLElement | null = null;

async function flush() {
  await new Promise((resolve) => setTimeout(resolve, 25));
}

async function renderPanel(todos: Todo[], archive: Todo[]) {
  localStorage.clear();
  // shouldOpenTodoPanelByDefault() is false: open the shelf the way the app
  // persists it, so the list and the archive are both rendered.
  localStorage.setItem("todoPanel:openStates", JSON.stringify({ "archive-batch": true }));
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  await act(async () => {
    root?.render(
      <LocaleProvider>
        <TodoPanel stateKey="archive-batch" todos={todos} archive={archive} onDismiss={() => {}} />
      </LocaleProvider>,
    );
  });
  await act(flush);
}

const live: Todo[] = [
  { content: "第二步", status: "in_progress", level: 0 },
  { content: "第三步", status: "pending", level: 0 },
];
const archived: Todo[] = [
  { content: "第一步", status: "completed", level: 0 },
  { content: "旧的一步", status: "completed", level: 0 },
];

await renderPanel(live, archived);

const section = host?.querySelector(".todobar__archive");
ok(Boolean(section), "S3: a finished list is offered as an archive section");
const head = host?.querySelector<HTMLElement>(".todobar__archive-head");
ok(head?.getAttribute("aria-expanded") === "false", "the archive starts folded");
ok(head?.textContent?.includes("2") === true, "the folded header carries the count");
ok(!host?.querySelector(".todobar__archive .todobar__sublist"), "a folded archive lists nothing");

await act(async () => {
  head?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
});
await act(flush);
ok(
  host?.querySelector(".todobar__archive-head")?.getAttribute("aria-expanded") === "true",
  "expanding the archive flips aria-expanded",
);
const rows = Array.from(host?.querySelectorAll(".todobar__archive .todobar__item") ?? []);
ok(rows.length === 2, "the expanded archive lists its items");
ok(rows[0]?.textContent?.includes("第一步") === true, "in archive order");
ok(
  host?.querySelector(".todobar__archive button") === null,
  "archived items offer no action of their own",
);

await renderPanel(live, []);
ok(!host?.querySelector(".todobar__archive"), "S3: nothing finished means no archive section");
ok(
  host?.querySelectorAll(".todobar__list .todobar__item").length === 2,
  "the live list still renders on its own",
);

await renderPanel([], archived);
ok(!host?.querySelector(".prompt-shelf"), "a shelf with no live list still renders nothing");

process.stdout.write(`\ntodo archive section: ${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exit(1);
