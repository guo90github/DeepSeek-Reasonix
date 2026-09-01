// Run: pnpm test (auto-discovered by scripts/run-tests.mjs)
// TodoPanel rendering invariants: legacy flat rows stay byte-identical, phase
// groups render nested with a progress chip, chevron collapses only the
// sublist, and the shelf auto-collapses when every todo completes.

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
// TodoPanel's auto-scroll effect calls scrollIntoView on the current item.
dom.window.Element.prototype.scrollIntoView = () => {};

const { TodoPanel } = await import("../components/TodoPanel");

let activeRoot: Root | null = null;
let activeHost: HTMLElement | null = null;

async function flush() {
  await new Promise((resolve) => setTimeout(resolve, 25));
}

async function renderPanel(todos: Todo[]) {
  localStorage.clear();
  activeHost = document.createElement("div");
  document.body.appendChild(activeHost);
  activeRoot = createRoot(activeHost);
  await act(async () => {
    activeRoot?.render(
      <LocaleProvider>
        <TodoPanel stateKey="test-batch" todos={todos} onDismiss={() => {}} />
      </LocaleProvider>,
    );
    await flush();
  });
}

async function rerender(todos: Todo[]) {
  await act(async () => {
    activeRoot?.render(
      <LocaleProvider>
        <TodoPanel stateKey="test-batch" todos={todos} onDismiss={() => {}} />
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
  localStorage.clear();
}

function card(): HTMLElement {
  const el = document.querySelector(".prompt-shelf__card--collapsible");
  if (!el) throw new Error("missing collapsible shelf card");
  return el as HTMLElement;
}

function cardCollapsed(): boolean {
  return card().classList.contains("prompt-shelf__card--collapsed");
}

async function openPanel() {
  if (cardCollapsed()) {
    await act(async () => {
      card().click();
      await flush();
    });
  }
}

function q(selector: string): Element | null {
  return document.querySelector(selector);
}

function qa(selector: string): Element[] {
  return Array.from(document.querySelectorAll(selector));
}

async function clickElement(el: Element | null) {
  if (!el) throw new Error("missing element to click");
  await act(async () => {
    (el as HTMLElement).click();
    await flush();
  });
}

async function testFlatListRendersLegacyDom() {
  await renderPanel([
    { content: "Inspect env", status: "in_progress" },
    { content: "Write code", status: "pending" },
    { content: "Run tests", status: "completed" },
  ]);
  await openPanel();
  await check("flat list renders the todobar list", () => Promise.resolve(q(".todobar__list") !== null));
  await check("flat list has no nested sublist", () => Promise.resolve(q(".todobar__sublist") === null));
  await check("flat list has no phase rows", () => Promise.resolve(qa(".todobar__item--phase").length === 0));
  await check("flat list has no chips", () => Promise.resolve(qa(".todobar__chip").length === 0));
  await check("flat list has no sub rows", () => Promise.resolve(qa(".todobar__item--sub").length === 0));
  await check("flat list renders one li per todo", () => Promise.resolve(qa(".todobar__list > li").length === 3));
  await check("badge counts all rows", () => Promise.resolve(q(".prompt-shelf__badges")?.textContent?.trim() === "1/3"));
}

async function testPhaseRendersNestedWithChip() {
  await renderPanel([
    { content: "Phase A", status: "pending", level: 0 },
    { content: "A1", status: "completed", level: 1 },
    { content: "A2", status: "in_progress", level: 1 },
    { content: "A3", status: "pending", level: 1 },
    { content: "Plain", status: "pending" },
  ]);
  await openPanel();
  await check("phase renders a phase row", () => Promise.resolve(qa(".todobar__item--phase").length === 1));
  await check("phase children sit in a nested sublist", () => Promise.resolve(qa(".todobar__sublist > li").length === 3));
  await check("sub rows carry the sub class", () => Promise.resolve(qa(".todobar__sublist .todobar__item--sub").length === 3));
  await check("chip counts completed sub-steps", () => Promise.resolve(q(".todobar__chip")?.textContent?.trim() === "1/3"));
  await check("plain row stays outside the sublist", () => Promise.resolve(qa(".todobar__list > li").length === 2));
  await check("badge counts all five rows", () => Promise.resolve(q(".prompt-shelf__badges")?.textContent?.trim() === "1/5"));
}

async function testLonePhaseRendersAsPlainRow() {
  await renderPanel([{ content: "Solo", status: "in_progress", level: 0 }]);
  await openPanel();
  await check("lone phase has no phase treatment", () => Promise.resolve(qa(".todobar__item--phase").length === 0));
  await check("lone phase renders no chip", () => Promise.resolve(qa(".todobar__chip").length === 0));
  await check("lone phase renders no sublist", () => Promise.resolve(q(".todobar__sublist") === null));
  await check("lone phase renders one plain row", () => Promise.resolve(qa(".todobar__list > li").length === 1));
}

async function testStrayLevel1RendersStandaloneSubRow() {
  await renderPanel([
    { content: "Stray", status: "in_progress", level: 1 },
    { content: "P", status: "pending", level: 0 },
  ]);
  await openPanel();
  await check("stray level-1 stays outside a sublist", () => Promise.resolve(q(".todobar__sublist") === null));
  await check("stray level-1 keeps the sub row style", () => Promise.resolve(qa(".todobar__item--sub").length === 1));
  await check("no phase rows for stray input", () => Promise.resolve(qa(".todobar__item--phase").length === 0));
}

async function testChevronCollapsesSublistOnly() {
  await renderPanel([
    { content: "Phase A", status: "pending", level: 0 },
    { content: "A1", status: "in_progress", level: 1 },
    { content: "A2", status: "pending", level: 1 },
  ]);
  await openPanel();
  await check("chevron present on phase row", () => Promise.resolve(q(".todobar__chevron") !== null));
  await check("sublist visible before collapse", () => Promise.resolve(q(".todobar__sublist") !== null));
  await clickElement(q(".todobar__chevron"));
  await check("chevron click hides the sublist", () => Promise.resolve(q(".todobar__sublist") === null));
  await check("chevron click keeps the shelf open", () => Promise.resolve(!cardCollapsed() && q(".todobar__list") !== null));
  await clickElement(q(".todobar__chevron"));
  await check("second click restores the sublist", () => Promise.resolve(q(".todobar__sublist") !== null));
}

async function testAllDoneAutoCollapses() {
  await renderPanel([{ content: "X", status: "in_progress" }, { content: "Y", status: "pending" }]);
  await openPanel();
  await check("shelf open before completion", () => Promise.resolve(!cardCollapsed()));
  await rerender([{ content: "X", status: "completed" }, { content: "Y", status: "completed" }]);
  await check("shelf collapses when every todo completes", () => Promise.resolve(cardCollapsed()));
  await check("list unmounts when collapsed", () => Promise.resolve(q(".todobar__list") === null));
}

async function main() {
  await testFlatListRendersLegacyDom();
  await cleanup();
  await testPhaseRendersNestedWithChip();
  await cleanup();
  await testLonePhaseRendersAsPlainRow();
  await cleanup();
  await testStrayLevel1RendersStandaloneSubRow();
  await cleanup();
  await testChevronCollapsesSublistOnly();
  await cleanup();
  await testAllDoneAutoCollapses();
  await cleanup();

  process.stdout.write(`\nTodoPanel: ${passed} passed, ${failed} failed\n`);
  if (failed > 0) process.exit(1);
}

await main();
