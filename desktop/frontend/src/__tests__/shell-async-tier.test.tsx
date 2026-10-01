// Run: node --import ./scripts/css-stub-register.mjs --import tsx src/__tests__/shell-async-tier.test.tsx

// The speed tier control: three choices, the current one marked, and a click
// hands the exact config value to the caller (who writes it through the host).

import { JSDOM } from "jsdom";
import React from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";

import { ShellAsyncTierField, normalizeShellAsyncTier, type ShellAsyncTier } from "../components/ShellAsyncTierField";
import { LocaleProvider } from "../lib/i18n";

let passed = 0;
let failed = 0;

function eq<T>(actual: T, expected: T, label: string) {
  if (Object.is(actual, expected)) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}: expected ${String(expected)}, got ${String(actual)}\n`);
    failed += 1;
  }
}

const dom = new JSDOM('<!doctype html><html><body><div id="root"></div></body></html>', { url: "http://localhost/" });
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
globalThis.window = dom.window as unknown as typeof globalThis.window;
globalThis.document = dom.window.document;
Object.defineProperty(globalThis, "navigator", { configurable: true, value: dom.window.navigator });
globalThis.HTMLElement = dom.window.HTMLElement;
globalThis.Node = dom.window.Node;
globalThis.Event = dom.window.Event;
globalThis.MouseEvent = dom.window.MouseEvent;
globalThis.localStorage = dom.window.localStorage;

const domWindow = dom.window as unknown as Window & typeof globalThis;

async function mount(value: string | undefined, busy = false) {
  const chosen: ShellAsyncTier[] = [];
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root = createRoot(container);
  await act(async () => {
    root.render(
      <LocaleProvider>
        <ShellAsyncTierField value={value} busy={busy} onSelect={(tier) => chosen.push(tier)} />
      </LocaleProvider>,
    );
  });
  return { root, chosen, container };
}

function buttons(container: HTMLElement): HTMLButtonElement[] {
  return Array.from(container.querySelectorAll("button"));
}

// --- the shape of the control ---------------------------------------------

const first = await mount("balanced");
let choices = buttons(first.container);
eq(choices.length, 3, "the control offers exactly three tiers");
eq(choices.map((b) => b.getAttribute("aria-pressed")).join(","), "false,true,false", "the stored tier is the one marked as pressed");
eq(choices.some((b) => b.className.includes("set-seg__btn--on")), true, "the marked tier uses the shared segmented styling");
eq(choices.every((b) => b.type === "button"), true, "choices never submit a settings form");
const labels = choices.map((b) => (b.textContent ?? "").trim());
eq(labels.every((label) => label.length > 0 && !label.startsWith("settings.")), true, `labels come from the dictionary (${labels.join(" / ")})`);

// --- what a click saves ---------------------------------------------------

await act(async () => {
  choices[2].dispatchEvent(new domWindow.MouseEvent("click", { bubbles: true, cancelable: true }));
});
eq(first.chosen.join(","), "fast", "clicking the fastest tier saves exactly the config value fast");
await act(async () => {
  choices[0].dispatchEvent(new domWindow.MouseEvent("click", { bubbles: true, cancelable: true }));
});
eq(first.chosen.join(","), "fast,off", "clicking off saves off as well");
await act(async () => first.root.unmount());

// --- busy and unknown values ----------------------------------------------

const busy = await mount("fast", true);
eq(buttons(busy.container).every((b) => b.disabled), true, "while a save is in flight every choice is disabled");
await act(async () => busy.root.unmount());

const unknown = await mount("turbo");
const pressed = buttons(unknown.container).map((b) => b.getAttribute("aria-pressed"));
eq(pressed.join(","), "true,false,false", "an unknown stored value falls back to the off choice");
eq(normalizeShellAsyncTier(undefined), "off", "a missing value reads as off");
eq(normalizeShellAsyncTier("BALANCED"), "off", "an unrecognized casing is not silently upgraded");
await act(async () => unknown.root.unmount());

dom.window.close();

console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
