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
globalThis.HTMLTextAreaElement = dom.window.HTMLTextAreaElement;
globalThis.Event = dom.window.Event;
globalThis.InputEvent = dom.window.InputEvent;
globalThis.KeyboardEvent = dom.window.KeyboardEvent;

function meta(path: string, title: string, turns: number): SessionMeta {
  return {
    path, preview: "", title, turns, turnsState: "complete",
    createdAt: 0, lastActivityAt: 0, modTime: 0, current: false, open: false,
  } as SessionMeta;
}

// Each recap carries one reviewable note and one handoff note, both embedding the
// goal text so the ordering and search assertions still key on the recap they
// mean. The handoff note keeps the id the host returns for it once it is kept as
// an unfinished item, which is what the page matches on.
function recap(path: string, goal: string, generatedAt: string): SessionRecap {
  return {
    path, model: "deepseek/test", generatedAt,
    entries: [
      { id: `fact-${goal}`, kind: "fact", body: `${goal} 的事实`, evidence: "internal/parser.go", target: "memory" },
      { id: "handoff-mock", kind: "handoff", body: `${goal} 的交接`, target: "display" },
    ],
  } as SessionRecap;
}

// Session titles sort by name in the opposite order to their dates, so each of
// the three sort modes has its own order. The fourth recap has no session
// metadata: it covers the file-name fallback, the missing jump target, and the
// read-only rule for another project's sessions.
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
const accepted: { kind: string; body: string; edited: string }[] = [];
const rejected: { kind: string; body: string }[] = [];
const undone: { kind: string; body: string }[] = [];
const kept: { path: string; body: string; evidence: string }[] = [];
const closedItems: string[] = [];
const reopenedItems: string[] = [];
let backCount = 0;
let failNextAccept = false;
const [{ LocaleProvider }, { SessionRecapPage }] = await Promise.all([
  import("../lib/i18n"),
  import("../components/SessionRecapPage"),
]);

const reviewProps = {
  resume: (session: SessionMeta) => { resumed.push(session); },
  accept: async (kind: string, body: string, edited: string) => {
    if (failNextAccept) { failNextAccept = false; throw new Error("no writer"); }
    accepted.push({ kind, body, edited });
    return "recap-note.md";
  },
  reject: async (kind: string, body: string) => { rejected.push({ kind, body }); },
  undo: async (kind: string, body: string) => { undone.push({ kind, body }); },
  listOpenItems: async () => [],
  keep: async (path: string, body: string, evidence: string) => {
    kept.push({ path, body, evidence });
    return "handoff-mock";
  },
  close: async (id: string) => { closedItems.push(id); },
  reopen: async (id: string) => { reopenedItems.push(id); },
};

const rootEl = document.getElementById("root");
if (!rootEl) throw new Error("missing root");
const root = createRoot(rootEl);
await act(async () => {
  root.render(<LocaleProvider><SessionRecapPage active onBack={() => { backCount += 1; }}
    list={async () => recaps} listSessions={async () => sessions} {...reviewProps} /></LocaleProvider>);
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

// The notes are what the page now shows instead of four element paragraphs, and
// each reviewable note carries its own actions.
const notesOf = (card: Element) => [...card.querySelectorAll("p")];
const rowWith = (card: Element | undefined, text: string) =>
  [...(card?.querySelectorAll("p") ?? [])].find((row) => row.textContent?.includes(text));
const buttonsOf = (row: Element | undefined) => [...(row?.querySelectorAll("button") ?? [])];
ok(cards().every((card) => notesOf(card).length === 2), "each recap renders its two notes as separate lines");
ok(cards().every((card) => card.textContent?.includes("internal/parser.go") === true),
  "a note shows the evidence it cites");

const alphaCard = () => cardWith("Alpha 会话");
const clickButton = async (button: HTMLButtonElement | undefined) => {
  await act(async () => {
    button?.click();
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
};

// Pills are [newest, oldest, by session name] in that order; they are clicked by
// index so the assertions never depend on the UI language. The by-name order is
// asserted over the titled cards only: the unlisted recap has a file name.
const clickPill = async (index: number) => {
  const pill = rootEl.querySelectorAll<HTMLButtonElement>(".history-filter__pill")[index];
  await clickButton(pill);
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
await setQuery("internal/parser.go");
ok(cards().length === 4, "search also covers the evidence a note cites");
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
await clickButton(cardWith("Alpha 会话")?.querySelector<HTMLButtonElement>("button"));
ok(resumed.length === 1 && resumed[0]?.path === middlePath, "opening a recap resumes its session");
ok(backCount === 1, "opening a recap leaves the recap page");

// ---- reviewing a note ----
const factOf = (card: Element | undefined) => rowWith(card, "的事实");
ok(buttonsOf(factOf(alphaCard())).length === 3, "a reviewable note offers accept, edit-and-accept, and drop");
ok(buttonsOf(rowWith(alphaCard(), "的交接")).length === 1, "a handoff note offers keeping it as an unfinished item");
ok(buttonsOf(factOf(cardWith(unlisted))).length === 0, "another project's session is read-only here");

await clickButton(buttonsOf(factOf(alphaCard()))[0]);
ok(accepted.length === 1 && accepted[0]?.kind === "fact" && accepted[0]?.body === `${middle} 的事实` && accepted[0]?.edited === "",
  `accepting a note saves it unedited: ${JSON.stringify(accepted[0])}`);
ok(buttonsOf(factOf(alphaCard())).length === 1, "an accepted note offers only the way back");

await clickButton(buttonsOf(factOf(alphaCard()))[0]);
ok(undone.length === 1 && undone[0]?.body === `${middle} 的事实`, "undo drops the recorded choice");
ok(buttonsOf(factOf(alphaCard())).length === 3, "undoing restores the full set of actions");

await clickButton(buttonsOf(factOf(alphaCard()))[2]);
ok(rejected.length === 1 && rejected[0]?.kind === "fact", "dropping a note reports it as ruled out");
ok(buttonsOf(factOf(alphaCard())).length === 1, "a dropped note offers only the way back");
await clickButton(buttonsOf(factOf(alphaCard()))[0]);
ok(undone.length === 2, "a dropped note can be taken back");

// Editing before accepting: the edited text is what reaches memory.
await clickButton(buttonsOf(factOf(alphaCard()))[1]);
const editor = alphaCard()?.querySelector<HTMLTextAreaElement>("textarea");
ok(editor !== null && editor !== undefined, "edit-and-accept opens an editor");
await act(async () => {
  reactProps(editor ?? null)?.onChange?.({ target: { value: `${middle} 的事实（改写）` } });
  await new Promise((resolve) => setTimeout(resolve, 0));
});
await clickButton(editor?.parentElement?.querySelectorAll("button")[0]);
ok(accepted.length === 2 && accepted[1]?.edited === `${middle} 的事实（改写）`,
  `an edited note saves the edited text: ${JSON.stringify(accepted[1])}`);
ok(alphaCard()?.querySelector("textarea") === null, "accepting closes the editor");

// ---- keeping an unfinished item ----
// A handoff note outlives its session: kept as an unfinished item, it belongs to
// the project and is offered to a later session that continues the subject.
const handoffRow = () => rowWith(alphaCard(), "的交接");
const openSection = () => [...rootEl.querySelectorAll(".management-notice")]
  .find((node) => !scroller?.contains(node) && node.querySelector("ul") !== null);
await clickButton(buttonsOf(handoffRow())[0]);
ok(kept.length === 1 && kept[0]?.path === middlePath && kept[0]?.body === `${middle} 的交接`,
  `keeping an unfinished item records it for the session's project: ${JSON.stringify(kept[0])}`);
ok(buttonsOf(handoffRow()).length === 0, "a kept note stops offering the action");
ok(openSection() !== undefined, "the project's unfinished items get their own list");
ok(openSection()?.querySelectorAll("li").length === 1, "the kept item is listed");

await clickButton(openSection()?.querySelector<HTMLButtonElement>("li button"));
ok(closedItems.length === 1 && closedItems[0] === "handoff-mock", "marking an item handled reports it");
ok(openSection()?.querySelectorAll("li").length === 1, "a handled item stays visible");
await clickButton(openSection()?.querySelector<HTMLButtonElement>("li button"));
ok(reopenedItems.length === 1, "a handled item can be put back on the list");

// An item nobody picked up for a month stops being offered, and the list has to
// say so: otherwise a retired item reads like a live offer. It stays actionable.
const staleHost = document.createElement("div");
document.body.appendChild(staleHost);
const staleRoot = createRoot(staleHost);
await act(async () => {
  staleRoot.render(<LocaleProvider><SessionRecapPage active onBack={() => {}}
    list={async () => recaps} listSessions={async () => sessions} {...reviewProps}
    listOpenItems={async () => [{
      id: "handoff-stale", body: "很久以前留下的交接", from: middlePath,
      openedAt: "2026-08-01T09:00:00Z", closed: false, stale: true, ageDays: 60,
    }]} /></LocaleProvider>);
  await new Promise((resolve) => setTimeout(resolve, 0));
});
const staleSection = [...staleHost.querySelectorAll(".management-notice")]
  .find((node) => node.closest(".history-list") === null && node.querySelector("ul") !== null);
// This suite renders en (LocaleProvider starts with an empty pref and jsdom
// reports en-US), so the page's own copy is asserted in that language; the item
// text itself comes from the fixture.
const says = (node: Element | null | undefined, text: string) =>
  node?.textContent?.toLowerCase().includes(text) === true;
const staleRow = staleSection?.querySelector("li");
ok(says(staleRow, "too old"), "an item past the window says it is no longer offered on its own");
ok(says(staleSection, "1 of them are too old"), "the list counts what it retired");
ok(staleRow?.querySelector("button") !== null, "a retired item is still something the person can handle");
await act(async () => { staleRoot.unmount(); });

// A failed write must say so instead of pretending the note was settled.
failNextAccept = true;
const untouched = () => factOf(cardWith(early));
const before = accepted.length;
await clickButton(buttonsOf(untouched())[0]);
ok(accepted.length === before && buttonsOf(untouched()).length === 3,
  "a failed write leaves the note unsettled");
ok(rootEl.querySelectorAll('[role="alert"]').length > 0, "a failed write is reported");

// A session that yielded nothing reusable must say so rather than render an
// empty card: one line, and no note line at all. It mounts on its own host so
// the assertion never depends on the reused root already having state.
const quietPath = "C:\\sessions\\20260904-090000.000000000-deepseek-flash.jsonl";
const quietHost = document.createElement("div");
document.body.appendChild(quietHost);
const quietRoot = createRoot(quietHost);
await act(async () => {
  quietRoot.render(<LocaleProvider><SessionRecapPage active onBack={() => {}}
    list={async () => [{ path: quietPath, model: "deepseek/test", generatedAt: "2026-09-04T09:00:00Z", entries: [] } as SessionRecap]}
    listSessions={async () => []} {...reviewProps} /></LocaleProvider>);
  await new Promise((resolve) => setTimeout(resolve, 0));
});
const quiet = [...quietHost.querySelectorAll("li")];
ok(quiet.length === 1 && notesOf(quiet[0]).length === 1 && quiet[0].textContent?.includes("的事实") !== true,
  "a recap with no notes says so instead of listing notes");
await act(async () => { quietRoot.unmount(); });

await act(async () => { root.unmount(); });
process.stdout.write(`\n${failed === 0 ? "OK" : "FAILED"}: ${failed} failed\n`);
if (failed > 0) process.exit(1);
