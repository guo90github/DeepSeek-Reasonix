// Run: tsx src/__tests__/session-recap-page.test.tsx

import { JSDOM } from "jsdom";
import { registerHooks } from "node:module";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import type { SessionMeta, SessionRecap } from "../lib/types";

registerHooks({
  resolve(specifier, context, nextResolve) {
    if (specifier.endsWith(".css")) {
      return nextResolve("./asset-stub-for-tests.ts", { ...context, parentURL: import.meta.url });
    }
    return nextResolve(specifier, context);
  },
});

let failed = 0;
function ok(value: boolean, label: string) {
  if (value) process.stdout.write(`  PASS  ${label}\n`);
  else { process.stdout.write(`  FAIL  ${label}\n`); failed += 1; }
}

const dom = new JSDOM('<!doctype html><html><body><div id="root"></div></body></html>', {
  pretendToBeVisual: true,
  url: "http://localhost/",
});
const globals = globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean };
globals.IS_REACT_ACT_ENVIRONMENT = true;
globalThis.window = dom.window as unknown as Window & typeof globalThis;
globalThis.document = dom.window.document;
Object.defineProperty(globalThis, "navigator", { configurable: true, value: dom.window.navigator });
globalThis.Node = dom.window.Node;
globalThis.Element = dom.window.Element;
globalThis.HTMLElement = dom.window.HTMLElement;
globalThis.HTMLInputElement = dom.window.HTMLInputElement;
globalThis.Event = dom.window.Event;
globalThis.InputEvent = dom.window.InputEvent;
globalThis.KeyboardEvent = dom.window.KeyboardEvent;

function meta(path: string, title: string, turns: number): SessionMeta {
  return {
    path, preview: "", title, turns, turnsState: "complete",
    createdAt: 0, lastActivityAt: 0, modTime: 0, current: false, open: false,
  } as SessionMeta;
}

function recap(path: string, goal: string, generatedAt: string): SessionRecap {
  return {
    path, goal, actions: `${goal} 的步骤`, conclusion: `${goal} 的结论`, model: "deepseek/test", generatedAt,
  } as SessionRecap;
}

// Session titles sort by name in the opposite order to their dates, so each of
// the three sort modes has its own order. The fourth recap has no session
// metadata: it covers the file-name fallback and the missing jump target.
const unlistedPath = "C:\\sessions\\20260901-120000.000000000-deepseek-flash.jsonl";
const earlyPath = "C:\\sessions\\20260901-090000.000000000-deepseek-flash.jsonl";
const middlePath = "C:\\sessions\\20260902-090000.000000000-deepseek-flash.jsonl";
const latePath = "C:\\sessions\\20260903-090000.000000000-deepseek-flash.jsonl";
const newest = "最新近的目标";
const middle = "中间的目标";
const early = "最早的目标";
const unlisted = "没有会话记录的目标";
const recaps = [
  recap(latePath, newest, "2026-09-03T09:00:00Z"),
  recap(earlyPath, early, "2026-09-01T09:00:00Z"),
  recap(middlePath, middle, "2026-09-02T09:00:00Z"),
  recap(unlistedPath, unlisted, "2026-09-01T12:00:00Z"),
];
const sessions = [
  meta(earlyPath, "Beta 会话", 2),
  meta(middlePath, "Alpha 会话", 7),
  meta(latePath, "Gamma 会话", 12),
];

const resumed: SessionMeta[] = [];
let backCount = 0;
const [{ LocaleProvider }, { SessionRecapPage }] = await Promise.all([
  import("../lib/i18n"),
  import("../components/SessionRecapPage"),
]);

const rootEl = document.getElementById("root");
if (!rootEl) throw new Error("missing root");
const root = createRoot(rootEl);
await act(async () => {
  root.render(<LocaleProvider><SessionRecapPage active onBack={() => { backCount += 1; }}
    list={async () => recaps} listSessions={async () => sessions} resume={(session) => { resumed.push(session); }} /></LocaleProvider>);
  await new Promise((resolve) => setTimeout(resolve, 0));
});

const scroller = rootEl.querySelector(".history-list");
ok(scroller !== null, "the list lives in a scroll container");
const cards = () => [...(scroller?.querySelectorAll("li") ?? [])];
ok(cards().length === 4, "every recap is rendered");
ok(scroller !== null && cards().every((card) => scroller.contains(card)), "the cards are inside the scroll container");

ok(rootEl.textContent?.includes("Gamma 会话") === true, "a recap shows its session title");
ok(rootEl.textContent?.includes("Alpha 会话") === true, "the other session title is shown too");
ok(rootEl.textContent?.includes("20260901-120000.000000000-deepseek-flash.jsonl") === true,
  "a recap whose session is not listed falls back to the file name");

const order = () => cards().map((card) => [newest, middle, early, unlisted].find((goal) => card.textContent?.includes(goal)) ?? "?");
const okOrder = (expected: string[], label: string) => {
  const seen = order();
  if (JSON.stringify(seen) === JSON.stringify(expected)) { ok(true, label); return; }
  process.stdout.write(`  FAIL  ${label}\n    saw: ${seen.join(" | ")}\n`);
  failed += 1;
};
okOrder([newest, middle, unlisted, early], "the newest recap comes first by default");

// Pills are [newest, oldest, by session name] in that order; they are clicked by
// index so the assertions never depend on the UI language. The by-name order is
// asserted over the titled cards only: the unlisted recap has a file name.
const clickPill = async (index: number) => {
  const pill = rootEl.querySelectorAll<HTMLButtonElement>(".history-filter__pill")[index];
  await act(async () => {
    pill?.click();
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
};
await clickPill(1);
okOrder([early, unlisted, middle, newest], "sorting by oldest first reorders the list");
await clickPill(2);
const titled = order().filter((goal) => goal !== unlisted).join(" | ");
ok(titled === [middle, early, newest].join(" | "), `sorting by session name uses the session titles: ${titled}`);
await clickPill(0);
okOrder([newest, middle, unlisted, early], "switching back restores the newest-first order");

const search = rootEl.querySelector<HTMLInputElement>(".history-search input");
ok(search !== null, "the page offers a search box");
// A synthetic input event does not reach React's change plugin through this
// page's wrapper (the click path does, and other suites dispatch inputs fine),
// so the query goes through the element's own React props handler — the page's
// onChange — while the filtering it feeds stays under test.
const reactProps = (node: Element | null) => {
  const key = Object.keys(node ?? {}).find((name) => name.startsWith("__reactProps$"));
  return key ? (node as unknown as Record<string, { onChange?: (event: { target: { value: string } }) => void; value?: string }>)[key] : undefined;
};
const setQuery = async (value: string) => {
  await act(async () => {
    reactProps(search)?.onChange?.({ target: { value } });
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
};
await setQuery(early);
ok(reactProps(search)?.value === early, "the search box is bound to the typed query");
okOrder([early], `search narrows the list to the matching recap: ${order().join(" | ")}`);
await setQuery("Gamma 会话");
okOrder([newest], "search matches the session title as well");
await setQuery("没有会话记录");
okOrder([unlisted], "search also covers recaps whose session is not listed");
await setQuery("nothing matches this query");
ok(cards().length === 0, "a query with no hit renders no cards");
ok([...rootEl.querySelectorAll(".management-notice")].some((node) => !scroller?.contains(node) && node.querySelector("button") !== null),
  "a query with no hit offers a way back to the full list");
await setQuery("");

const cardWith = (text: string) => cards().find((card) => card.textContent?.includes(text));
ok(cards().filter((card) => card.querySelector("button") !== null).length === 3, "only recaps with a listed session can be opened");
ok(cardWith(unlisted)?.querySelector("button") === null, "a recap without a listed session offers no jump");
await act(async () => {
  cardWith("Alpha 会话")?.querySelector<HTMLButtonElement>("button")?.click();
  await new Promise((resolve) => setTimeout(resolve, 0));
});
ok(resumed.length === 1 && resumed[0]?.path === middlePath, "opening a recap resumes its session");
ok(backCount === 1, "opening a recap leaves the recap page");

await act(async () => { root.unmount(); });
process.stdout.write(`\n${failed === 0 ? "OK" : "FAILED"}: ${failed} failed\n`);
if (failed > 0) process.exit(1);
