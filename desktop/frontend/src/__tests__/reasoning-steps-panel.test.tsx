// Run: tsx src/__tests__/reasoning-steps-panel.test.tsx

import { JSDOM } from "jsdom";
import { registerHooks } from "node:module";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { LocaleProvider } from "../lib/i18n";
import { AssistantMessage } from "../components/Message";
import { hydrateReasoningDisplayMode, applyReasoningDisplayMode } from "../lib/reasoningDisplayPreference";

registerHooks({
  resolve(specifier, context, nextResolve) {
    if (specifier.endsWith(".css")) {
      return nextResolve("./asset-stub-for-tests.ts", { ...context, parentURL: import.meta.url });
    }
    return nextResolve(specifier, context);
  },
});

let failed = 0;
let passed = 0;

function ok(value: boolean, label: string) {
  if (value) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
  pretendToBeVisual: true,
  url: "http://localhost/",
});
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
globalThis.window = dom.window as unknown as Window & typeof globalThis;
globalThis.document = dom.window.document;
Object.defineProperty(globalThis, "navigator", { configurable: true, value: { ...dom.window.navigator, language: "en-US" } });
globalThis.Node = dom.window.Node;
globalThis.Element = dom.window.Element;
globalThis.HTMLElement = dom.window.HTMLElement;
globalThis.Event = dom.window.Event;
globalThis.CustomEvent = dom.window.CustomEvent;
globalThis.MouseEvent = dom.window.MouseEvent;
globalThis.localStorage = dom.window.localStorage;
globalThis.requestAnimationFrame = dom.window.requestAnimationFrame.bind(dom.window);
globalThis.cancelAnimationFrame = dom.window.cancelAnimationFrame.bind(dom.window);

const realNow = Date.now;
let fakeNow = 1000;
Date.now = () => fakeNow;

const rootEl = document.getElementById("root");
if (!rootEl) throw new Error("missing root");
const root = createRoot(rootEl);

type ReasoningItem = React.ComponentProps<typeof AssistantMessage>["item"];

async function render(item: ReasoningItem, props: { defaultExpanded?: boolean } = {}) {
  await act(async () => {
    root.render(
      <LocaleProvider>
        <AssistantMessage key={item.id} item={item} defaultExpanded={props.defaultExpanded} />
      </LocaleProvider>,
    );
  });
}

async function click(el: Element | null | undefined) {
  await act(async () => {
    el?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

console.log("\nreasoning steps panel");

// Streaming in auto mode: the active step tracks the stream, completed steps
// fold away by default, and the label counts detected steps live.
hydrateReasoningDisplayMode("auto", true);
await render({
  kind: "assistant",
  id: "s1",
  text: "",
  reasoning: "Step 1: 理解需求\n",
  streaming: true,
  reasoningComplete: false,
});
ok(!document.querySelector(".reasoning-steps"), "one marker keeps the flat streaming view");

fakeNow = 3000;
await render({
  kind: "assistant",
  id: "s1",
  text: "",
  reasoning: "Step 1: 理解需求\nStep 2: 分析代码结构\n正在检查循环…",
  streaming: true,
  reasoningComplete: false,
});
const steps = document.querySelectorAll(".reasoning-step");
ok(steps.length === 2, "the second marker activates the step list");
ok(document.querySelector(".reasoning-steps__summary")?.textContent?.includes("2 steps detected") ?? false, "live label counts detected steps");
ok(document.querySelector(".reasoning-step__title")?.textContent === "理解需求", "step 1 title renders");
ok(document.querySelector(".reasoning-step--complete")?.textContent?.includes("Step 1") ?? false, "step 1 is complete");
ok(document.querySelector(".reasoning-step--streaming")?.textContent?.includes("分析代码结构") ?? false, "step 2 is streaming");
ok(document.querySelector(".reasoning-step--complete .reasoning-step__meta")?.textContent === "[2s]" ?? false, "completed step shows its duration");
ok(document.querySelector(".reasoning-step--streaming .reasoning-step__body")?.textContent?.includes("正在检查循环") ?? false, "active step body streams open");
ok(!document.querySelector(".reasoning-step--complete .reasoning-step__body"), "completed step body folds by default");

// Completion settles every step and swaps the label to N/N. defaultExpanded
// keeps the panel open so the completed label stays visible (auto mode closes
// the untouched panel on completion — asserted behavior, not a bug).
fakeNow = 5000;
await render({
  kind: "assistant",
  id: "s1",
  text: "done",
  reasoning: "Step 1: 理解需求\nStep 2: 分析代码结构\n",
  streaming: false,
  reasoningComplete: true,
  reasoningDurationMs: 2500,
}, { defaultExpanded: true });
ok(document.querySelectorAll(".reasoning-step--streaming").length === 0, "completion settles every step");
ok(document.querySelector(".reasoning-steps__summary")?.textContent?.includes("2/2 steps complete") ?? false, "completed label reads N/N");

// History restore (no streaming observation): rows render without durations.
await render({
  kind: "assistant",
  id: "h1",
  text: "",
  reasoning: "Step 1: A\nbody one\nStep 2: B\nbody two\nStep 3: C\nbody three\n",
  streaming: false,
  reasoningComplete: true,
}, { defaultExpanded: true });
ok(document.querySelectorAll(".reasoning-step").length === 3, "history restore renders all steps");
ok(document.querySelectorAll(".reasoning-step__meta").length === 0, "history restore shows no per-step durations");

// Clicking a completed step opens its body; the fold button closes it again.
await click(document.querySelector('[data-step-index="1"] .reasoning-step__head'));
ok(document.querySelector('[data-step-index="1"] .reasoning-step__body')?.textContent?.includes("body one") ?? false, "clicking a completed step opens its body");
ok(Boolean(document.querySelector(".reasoning-steps__fold")), "an opened completed step reveals the fold button");
await click(document.querySelector(".reasoning-steps__fold"));
ok(!document.querySelector('[data-step-index="1"] .reasoning-step__body'), "fold completed steps closes every opened body");

// Expanded display mode keeps completed bodies open by default.
await act(async () => {
  applyReasoningDisplayMode("expanded");
});
await render({
  kind: "assistant",
  id: "e1",
  text: "",
  reasoning: "Step 1: A\nbody one\nStep 2: B\nbody two\n",
  streaming: false,
  reasoningComplete: true,
});
ok(document.querySelector('[data-step-index="1"] .reasoning-step__body')?.textContent?.includes("body one") ?? false, "expanded mode opens completed step bodies");

// Prose without markers never activates the step view.
await act(async () => {
  applyReasoningDisplayMode("auto");
});
await render({
  kind: "assistant",
  id: "p1",
  text: "",
  reasoning: "Just thinking out loud.\nNo step structure here.\n",
  streaming: true,
  reasoningComplete: false,
});
ok(!document.querySelector(".reasoning-steps"), "prose reasoning keeps the flat streaming view");

await act(async () => {
  root.unmount();
});
Date.now = realNow;
dom.window.close();

console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
