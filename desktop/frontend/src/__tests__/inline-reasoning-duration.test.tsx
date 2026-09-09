// Run: tsx src/__tests__/inline-reasoning-duration.test.tsx

import { JSDOM } from "jsdom";
import { registerHooks } from "node:module";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { LocaleProvider } from "../lib/i18n";
import { hydrateSessionExperience } from "../lib/sessionExperience";
import { InlineAssistantReasoning } from "../components/InlineAssistantReasoning";
import { LiveStreamContext } from "../components/LiveStreamContext";
import type { LiveStream } from "../lib/useController";

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

console.log("\ninline reasoning duration meta");

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

hydrateSessionExperience("standard");

type ReasoningItem = React.ComponentProps<typeof InlineAssistantReasoning>["item"];

async function render(item: ReasoningItem, live?: LiveStream) {
  await act(async () => {
    root.render(
      <LocaleProvider>
        <LiveStreamContext.Provider value={live}>
          <InlineAssistantReasoning item={item} />
        </LiveStreamContext.Provider>
      </LocaleProvider>,
    );
  });
}

const settled: ReasoningItem = {
  kind: "assistant",
  id: "s1",
  text: "",
  reasoning: "initial plan",
  streaming: false,
  reasoningComplete: true,
  reasoningDurationMs: 2_600,
};

const settledNoDuration: ReasoningItem = {
  kind: "assistant",
  id: "s2",
  text: "",
  reasoning: "initial plan",
  streaming: false,
  reasoningComplete: true,
};

const running: ReasoningItem = {
  kind: "assistant",
  id: "r1",
  text: "",
  reasoning: "live thought",
  streaming: true,
  reasoningComplete: false,
};

// Settled reasoning with a recorded duration shows the elapsed time next to
// the header label even while the fold stays collapsed.
await render(settled);
const head = document.querySelector<HTMLButtonElement>(".turn-collapse__reasoning-head");
ok(Boolean(head), "settled reasoning renders a toggle header");
ok(head?.textContent?.includes("thinking") ?? false, "header keeps the reasoning label");
ok(head?.textContent?.includes("lasted 3s") ?? false, "header shows the rounded reasoning duration");
ok(!document.querySelector(".turn-collapse__inline-reasoning"), "settled reasoning stays collapsed by default");
ok(head?.textContent?.includes("lasted 3s") ?? false, "duration meta is visible in the collapsed header");

// A completed reasoning without a recorded duration falls back to the plain
// done label instead of a blank or NaN meta.
await render(settledNoDuration);
const headNoDuration = document.querySelector<HTMLButtonElement>(".turn-collapse__reasoning-head");
ok(headNoDuration?.textContent?.includes("done") ?? false, "missing duration falls back to the done label");
ok(!(headNoDuration?.textContent?.includes("lasted") ?? true), "no duration label without recorded ms");

// Running reasoning keeps the thinking label and never shows a duration meta.
await render(running);
const headRunning = document.querySelector<HTMLButtonElement>(".turn-collapse__reasoning-head");
ok(headRunning?.textContent?.includes("thinking…") ?? false, "running header keeps the thinking label");
ok(headRunning?.hasAttribute("data-running") ?? false, "running header keeps the running state");
ok(!document.querySelector(".reasoning__meta"), "running reasoning shows no duration meta");

// While the live stream still matches the item (reasoning done, answer still
// streaming) the elapsed derives from the live timestamps, mirroring the
// single-column TranscriptVirtuosoParts merge.
const live = {
  id: "s1",
  text: "answer",
  reasoning: "initial plan",
  reasoningComplete: true,
  reasoningStartedAt: 1_000,
  reasoningCompletedAt: 64_000,
} satisfies LiveStream;
await render(settled, live);
const headLive = document.querySelector<HTMLButtonElement>(".turn-collapse__reasoning-head");
ok(headLive?.textContent?.includes("lasted 63s") ?? false, "live timestamps drive the duration while the turn still streams");

// A still-open live reasoning must not surface the meta.
const liveOpen = {
  id: "r1",
  text: "",
  reasoning: "live thought",
  reasoningComplete: false,
  reasoningStartedAt: 1_000,
} satisfies LiveStream;
await render(running, liveOpen);
ok(!document.querySelector(".reasoning__meta"), "open live reasoning shows no premature duration meta");

await act(async () => {
  root.unmount();
});
dom.window.close();

console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
