// Run: tsx src/__tests__/ask-card-timeout.test.tsx
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { JSDOM } from "jsdom";
import React from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { AskCard } from "../components/AskCard";
import { LocaleProvider } from "../lib/i18n";
import type { QuestionAnswer, WireAsk } from "../lib/types";

const testDir = dirname(fileURLToPath(import.meta.url));
const styles = readFileSync(resolve(testDir, "../styles.css"), "utf8");
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
function eq(actual: unknown, expected: unknown, label: string) {
  if (actual === expected) ok(true, label);
  else ok(false, `${label}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`);
}
function flushTimers(delay = 0): Promise<void> {
  return new Promise((res) => setTimeout(res, delay));
}
function installDom() {
  const dom = new JSDOM('<!doctype html><html><head></head><body><div id="root"></div></body></html>', {
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
  Object.defineProperty(dom.window.HTMLElement.prototype, "scrollIntoView", {
    configurable: true,
    value() {},
  });
  const style = document.createElement("style");
  style.textContent = styles;
  document.head.appendChild(style);
  return dom;
}

function makeAsk(autoAnswerInMs?: number): WireAsk {
  return {
    id: "ask-timeout",
    questions: [{ id: "q1", header: "Deploy", prompt: "Deploy now?", options: [{ label: "yes" }, { label: "no" }] }],
    autoAnswerInMs,
  };
}

async function renderAsk(ask: WireAsk) {
  const dom = installDom();
  const rootEl = document.getElementById("root");
  if (!rootEl) throw new Error("missing root");
  const root = createRoot(rootEl);
  await act(async () => {
    root.render(
      React.createElement(LocaleProvider, null,
        React.createElement(AskCard, {
          ask,
          onAnswer: (_id: string, _next: QuestionAnswer[]) => undefined,
          onDismiss: () => undefined,
          onStop: () => undefined,
        }),
      ),
    );
    await flushTimers();
  });
  return { dom, root };
}

console.log("\nask card host-fallback countdown");
{
  const { dom, root } = await renderAsk(makeAsk(65000));
  const chip = document.querySelector(".ask-shelf__timeout") as HTMLElement | null;
  ok(chip != null, "a positive autoAnswerInMs renders a countdown chip");
  eq(chip?.getAttribute("role"), "status", "the countdown is announced as live status");
  ok(/\d+:\d\d/.test(chip?.textContent ?? ""), "the countdown renders as mm:ss");
  ok((chip?.textContent ?? "").includes("Host answers"), "the countdown names the host fallback");
  await act(async () => { root.unmount(); });
  dom.window.close();
}
{
  const { dom, root } = await renderAsk(makeAsk(0));
  eq(document.querySelector(".ask-shelf__timeout"), null, "autoAnswerInMs <= 0 shows no countdown");
  await act(async () => { root.unmount(); });
  dom.window.close();
}
{
  const { dom, root } = await renderAsk(makeAsk(300));
  await act(async () => { await flushTimers(1400); });
  const chip = document.querySelector(".ask-shelf__timeout") as HTMLElement | null;
  ok(chip != null, "the chip stays visible after the window elapses");
  eq(chip?.classList.contains("ask-shelf__timeout--expired"), true, "the chip switches to the expired style");
  ok((chip?.textContent ?? "").includes("host answered"), "at zero the chip states the host answered with the recommended option");
  await act(async () => { root.unmount(); });
  dom.window.close();
}
console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
