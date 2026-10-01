// Run: node --import ./scripts/css-stub-register.mjs --import ./scripts/svg-stub-register.mjs --import tsx src/__tests__/recap-heatmap-component.test.tsx
// 第十四 (docs/60 §2.1): the heatmap renders the page's own data as a bounded
// calendar — one row per project, one cell per day, intensity per count — and a
// cell selection only filters the page's list.

import { readFileSync } from "node:fs";
import { JSDOM } from "jsdom";
import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { LocaleProvider } from "../lib/i18n";
import { RECAP_HEATMAP_WINDOW_DAYS } from "../lib/recapHeatmap";

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

const { RecapHeatmap } = await import("../components/RecapHeatmap");

let root: Root | null = null;
let host: HTMLElement | null = null;

async function renderHeatmap(props: Parameters<typeof RecapHeatmap>[0]) {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  await act(async () => {
    root?.render(
      <LocaleProvider>
        <RecapHeatmap {...props} />
      </LocaleProvider>,
    );
  });
}

const now = "2026-10-01T12:00:00Z";
const insights = [
  { projects: ["reasonix"], occurrences: 3, seenAt: "2026-09-30T08:00:00Z" },
  { projects: ["chatting"], occurrences: 1, seenAt: "2026-10-01T09:00:00Z" },
];
const recaps = [{ generatedAt: "2026-10-01T01:00:00Z", projects: ["reasonix"] }];

await renderHeatmap({ insights, recaps, now });
const rows = host?.querySelectorAll(".recap-heatmap__row") ?? [];
ok(rows.length === 2, "one row per project");
const cells = host?.querySelectorAll(".recap-heatmap__cell") ?? [];
ok(cells.length === 2 * RECAP_HEATMAP_WINDOW_DAYS, "every row spans the whole window");
const filled = host?.querySelectorAll(".recap-heatmap__cell--filled") ?? [];
ok(filled.length === 3, "only days with data are filled");
const labels = Array.from(filled).map((cell) => cell.getAttribute("aria-label") ?? "");
ok(labels.some((label) => label.includes("2026-09-30") && label.includes("3")), "a cell names its day and count");
ok(labels.every((label) => label.includes("2026-")), "every filled cell is dated");
ok((host?.querySelector(".recap-heatmap__window")?.textContent ?? "").includes("2026-09-02"), "the window start is stated");
ok((host?.querySelector(".recap-heatmap__window")?.textContent ?? "").includes("2026-10-01"), "the window end is stated");

// Clicking a filled cell selects its day; clicking the selected cell clears it.
const selections: string[] = [];
await renderHeatmap({ insights, recaps, now, onSelectDay: (day) => selections.push(day) });
const firstFilled = host?.querySelector<HTMLElement>(".recap-heatmap__cell--filled");
await act(async () => {
  firstFilled?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
});
const clickedDay = (firstFilled?.getAttribute("aria-label") ?? "").split(" ")[0];
ok(selections.length === 1 && selections[0] === clickedDay, "a click reports the clicked cell's day");
ok(firstFilled?.getAttribute("aria-pressed") === "false", "an unselected cell is not pressed");

await renderHeatmap({ insights, recaps, now, selectedDay: "2026-10-01", onSelectDay: (day) => selections.push(day) });
const active = host?.querySelector<HTMLElement>(".recap-heatmap__cell--active");
ok(active?.getAttribute("aria-pressed") === "true", "the selected day's cell is pressed");
ok(active?.getAttribute("aria-label")?.includes("2026-10-01") === true, "the pressed cell is the selected day");
await act(async () => {
  active?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
});
ok(selections[1] === "", "clicking the selected cell clears the filter");

// No data at all renders nothing rather than an empty grid.
await renderHeatmap({ insights: [], recaps: [], now });
ok(host?.childElementCount === 0, "an empty record renders no heatmap");

// The stylesheet must not reach for retired tokens.
// Comments name the retired tokens on purpose, so strip them before checking.
const css = readFileSync("src/components/RecapHeatmap.css", "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
for (const retired of ["--fg-muted", "--bg-elev-1", "--hover", "--border-strong", "--shadow"]) {
  ok(!css.includes(retired), `the heatmap stylesheet avoids the retired token ${retired}`);
}

process.stdout.write(`\nrecap heatmap component: ${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exit(1);
