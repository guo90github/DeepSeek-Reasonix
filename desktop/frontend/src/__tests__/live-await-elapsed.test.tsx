// Run: tsx src/__tests__/live-await-elapsed.test.tsx

import { JSDOM } from "jsdom";
import { registerHooks } from "node:module";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { LocaleProvider } from "../lib/i18n";
import { LiveAwaitElapsed, formatAwaitSeconds } from "../components/LiveAwaitElapsed";

registerHooks({
  resolve(specifier, context, nextResolve) {
    if (specifier.endsWith(".css")) {
      return nextResolve("./asset-stub-for-tests.ts", { ...context, parentURL: import.meta.url });
    }
    return nextResolve(specifier, context);
  },
});

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

console.log("\nlive await elapsed");

const now = 10_000;
ok(formatAwaitSeconds(6_000, now) === 4, "elapsed floors to whole seconds");
ok(formatAwaitSeconds(now, now) === 0, "zero elapsed at the send instant");
ok(formatAwaitSeconds(now + 5_000, now) === 0, "never negative when the clock skews");
ok(formatAwaitSeconds(4_100, now) === 5, "5900ms window floors to 5s");

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

const rootEl = document.getElementById("root");
if (!rootEl) throw new Error("missing root");
const root = createRoot(rootEl);

// Start 5.9s in the past: floor gives a stable 5s across call skew.
await act(async () => {
  root.render(
    <LocaleProvider>
      <LiveAwaitElapsed since={Date.now() - 5_900} />
    </LocaleProvider>,
  );
});
const el = document.querySelector(".conversation-pane__await-elapsed");
ok(Boolean(el), "renders the elapsed span");
ok(el?.textContent?.includes("waited 5s") ?? false, "shows the rounded waiting seconds");

await act(async () => {
  root.unmount();
});
dom.window.close();

console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
