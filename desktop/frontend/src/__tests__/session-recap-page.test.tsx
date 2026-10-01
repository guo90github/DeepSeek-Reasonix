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
      { id: `fact-${goal}`, kind: "fact", body: `${goal} 的事实`, evidence: "internal/parser.go", target: "memory",
        refs: [{ kind: "path", value: "internal/parser.go", detail: "L40" }], scope: "generic", scopeReason: "两个项目都踩过",
        observedIn: ["c--guosj-ai-chatting"] },
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
const closedResolutions: string[] = [];
const reopenedItems: string[] = [];
const generated: string[] = [];
let acceptGenerate = true;
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
  close: async (id: string, _body: string, _evidence: string, resolution: string) => { closedItems.push(id); closedResolutions.push(resolution); },
  reopen: async (id: string) => { reopenedItems.push(id); },
  // generate reports whether the lane accepted the work; a refused queue is a
  // failure the page must show rather than pretend it was queued.
  generate: async (path: string) => { generated.push(path); return acceptGenerate; },
  draftSkill: async (_kind: string, body: string) => ({
    name: `recap-mock-${body.length}`, path: `.reasonix/skills/recap-mock-${body.length}/SKILL.md`,
  }),
  previewMemory: async (source: { kind: string; body: string }) => ({
    kind: "memory", text: source.body, fallback: source.body, promptTag: "memory-v1", model: "mock/model",
  }),
  previewSkill: async () => ({
    kind: "skill", text: "# Playbook", fallback: "1. step", promptTag: "skill-v1", model: "mock/model",
  }),
  draftTopicSkill: async (sources: { kind: string; body: string }[]) => ({
    name: 'recap-topic-' + String(sources.length),
    path: '.reasonix/skills/recap-topic-' + String(sources.length) + '/SKILL.md',
  }),
  listInsights: async () => [{ kind: "refuted", body: "只取分支统计未提交数会漏掉 CJK 路径", projects: ["alpha", "beta"], occurrences: 1, seenAt: new Date().toISOString() }],
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
// One bounded region above one scroller: a growing note count may scroll inside
// the region, never squeeze the card list itself out of the window.
const panel = rootEl.querySelector<HTMLElement>(".recap-page__panel");
ok(panel !== null && panel.style.maxHeight === "min(40vh, 30%)" && panel.style.overflowY === "auto",
  "the region above the list is capped and scrolls on its own");
ok(panel !== null && panel.style.flexShrink === "0",
  "the capped region keeps its size; the list is what yields the room");
ok(rootEl.querySelector<HTMLElement>(".recap-page__head")?.style.maxHeight === "33%",
  "the fixed head is capped against the page, not the viewport");
ok(panel !== null && !panel.contains(scroller), "the list is outside the capped region, not inside it");
ok(scroller instanceof HTMLElement && scroller.style.minHeight === "160px" && scroller.style.flexGrow === "1",
  "the list keeps a height floor and takes the remaining room");

ok(rootEl.textContent?.includes("Gamma 会话") === true, "a recap shows its session title");
ok(rootEl.textContent?.includes("Alpha 会话") === true, "the other session title is shown too");
ok(rootEl.textContent?.includes("2026-09-01 12:00 · deepseek-flash") === true,
  "a recap whose session is not listed reads its file name as a stamp");

const order = () => cards().map((card) => [newest, middle, early, unlisted].find((goal) => card.textContent?.includes(goal)) ?? "?");
const okOrder = (expected: string[], label: string) => {
  const seen = order();
  if (JSON.stringify(seen) === JSON.stringify(expected)) { ok(true, label); return; }
  process.stdout.write(`  FAIL  ${label}\n    saw: ${seen.join(" | ")}\n`);
  failed += 1;
};
// Cards start folded except the newest one; the rest of this suite is about what a
// recap contains, so it expands them all once, here.
const expandedCards = () => cards().filter((card) => [...card.querySelectorAll("[data-recap-row]")].length > 0);
ok(expandedCards().length === 1, "only the newest recap starts expanded");
await act(async () => {
  rootEl.querySelector<HTMLButtonElement>(".recap-expand-all")?.click();
  await new Promise((resolve) => setTimeout(resolve, 0));
});
ok(expandedCards().length === cards().length, "the expand-all control opens every card");
okOrder([newest, middle, unlisted, early], "the newest recap comes first by default");

// The notes are what the page now shows instead of four element paragraphs, and
// each reviewable note carries its own actions.
const notesOf = (card: Element) => [...card.querySelectorAll("[data-recap-row]")];
const rowWith = (card: Element | undefined, text: string) =>
  [...(card?.querySelectorAll("[data-recap-row]") ?? [])].find((row) => row.textContent?.includes(text));
const buttonsOf = (row: Element | undefined) => [...(row?.querySelectorAll("button") ?? [])];
ok(rootEl.textContent?.includes("Reached independently in 2+ projects") === true
  && rootEl.textContent?.includes("2 projects: alpha、beta") === true,
  "the cross-project report is named with the projects that reached it");
ok(cards().every((card) => notesOf(card).length === 2),
  "each recap renders its two notes as rows");
ok(cards().every((card) => card.textContent?.includes("internal/parser.go") === true),
  "a note shows the evidence it cites");
ok(cards().every((card) => card.textContent?.includes("path internal/parser.go L40") === true),
  "a note shows the pointers it cites, which is what an accept is judged on");
ok(cards().every((card) => card.textContent?.includes("Tier proposed: generic") === true),
  "a note shows the tier the model proposed and says it is not applied yet");
ok(cards().every((card) => card.textContent?.includes("1 other project(s): c--guosj-ai-chatting") === true),
  "a note another project also reached says so, which is the only checkable ground for a general tier");

const alphaCard = () => cardWith("Alpha 会话");
const clickButton = async (button: HTMLButtonElement | undefined) => {
  await act(async () => {
    button?.click();
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
};

// confirmPreview presses the preview panel's confirm button: the only place a
// preview becomes a stored memory or a file on disk.
const confirmPreview = async () => {
  await clickButton(rootEl.querySelector<HTMLButtonElement>(".recap-preview-confirm") ?? undefined);
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

// Accepting now goes through the preview: the button opens it, confirming writes.
await clickButton(buttonsOf(factOf(alphaCard()))[0]);
await confirmPreview();
// What gets stored is exactly what the preview showed, which is why a confirmed
// draft arrives as the edited body rather than an empty one.
ok(accepted.length === 1 && accepted[0]?.kind === "fact" && accepted[0]?.body === `${middle} 的事实`
  && accepted[0]?.edited === `${middle} 的事实`,
  `accepting a note stores the reviewed text: ${JSON.stringify(accepted[0])}`);
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

// Handling an item is where an outcome can be written down: the first click opens
// the field, the second records whatever the person typed as sediment.
await clickButton(openSection()?.querySelector<HTMLButtonElement>("li button"));
ok(closedItems.length === 0, "the handled button opens the outcome field before closing anything");
const outcomeInput = openSection()?.querySelector<HTMLInputElement>("li input");
ok(outcomeInput?.placeholder === "Outcome (optional): how did this end?",
  `the handled button opens an outcome field first: ${outcomeInput?.outerHTML}`);
await clickButton([...openSection()!.querySelectorAll<HTMLButtonElement>("li button")]
  .find((button) => button.textContent?.includes("Record and mark handled")));
// The outcome is optional: an item handled without one still closes, and passes an
// empty resolution rather than inventing text.
ok(closedItems.length === 1 && closedItems[0] === "handoff-mock" && closedResolutions[0] === "",
  `marking an item handled reports it with the outcome: ${JSON.stringify(closedResolutions)}`);
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
await confirmPreview();
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
ok(quiet.length === 1 && notesOf(quiet[0]).length === 0 && quiet[0].textContent?.includes("的事实") !== true,
  "a recap with no notes says so instead of listing notes");
await act(async () => { quietRoot.unmount(); });

// The rail states its counts and keeps the bodies one click away; a folded
// <details> still holds its content in the DOM, which the assertions above rely on.
const railFolds = [...rootEl.querySelectorAll<HTMLDetailsElement>("details.recap-fold")];
ok(railFolds.length > 0 && railFolds.every((fold) => fold.open === false),
  "the rail reports its counts by default");
ok(railFolds.some((fold) => (fold.textContent ?? "").includes("只取分支统计未提交数会漏掉 CJK 路径")),
  "a folded report still holds its text in the DOM");

// A failed *refresh* of a session that already has a recap is not a session waiting
// to be generated: the row states the attempt, and the count above it must not
// claim a recap is missing.
const refreshPath = "C:\\sessions\\20260908-090000.000000000-deepseek-flash.jsonl";
const refreshHost = document.createElement("div");
document.body.appendChild(refreshHost);
const refreshRoot = createRoot(refreshHost);
await act(async () => {
  refreshRoot.render(<LocaleProvider><SessionRecapPage active onBack={() => {}}
    list={async () => [{
      path: refreshPath, state: "pending", model: "deepseek/test", generatedAt: "2026-09-08T09:00:00Z",
      entries: [{ id: "refresh-a", kind: "fact", body: "刷新失败的记录仍有一条条目", target: "memory" }],
      pending: { attempts: 1, reason: "bounded reviewer request exceeds 114688 bytes", updatedAt: "2026-09-08T10:00:00Z" },
    } as SessionRecap]}
    listSessions={async () => []} {...reviewProps} /></LocaleProvider>);
  await new Promise((resolve) => setTimeout(resolve, 0));
});
const refreshCards = [...refreshHost.querySelectorAll("li")];
ok(refreshCards.length === 1 && says(refreshCards[0], "attempt 1") && says(refreshCards[0], "exceeds 114688"),
  "a failed refresh on an existing recap still reports the attempt");
ok(says(refreshHost, "have not been generated") === false,
  "a session that already has a recap is not counted as waiting to be generated");
await act(async () => { refreshRoot.unmount(); });

// A failed attempt leaves no record at all, so the host lists it from its own
// marker. The page has to say the attempt failed — "nothing reusable" would be
// the wrong one of the two silences — and count what is still waiting.
const failedPath = "C:\\sessions\\20260905-090000.000000000-deepseek-flash.jsonl";
const failedHost = document.createElement("div");
document.body.appendChild(failedHost);
const failedRoot = createRoot(failedHost);
await act(async () => {
  failedRoot.render(<LocaleProvider><SessionRecapPage active onBack={() => {}}
    list={async () => [{
      path: failedPath, state: "pending", model: "", generatedAt: "", entries: [],
      pending: { attempts: 2, reason: "unparseable answer", updatedAt: "2026-09-05T09:00:00Z" },
    } as SessionRecap]}
    listSessions={async () => []} {...reviewProps} /></LocaleProvider>);
  await new Promise((resolve) => setTimeout(resolve, 0));
});
const failedCards = [...failedHost.querySelectorAll("li")];
ok(failedCards.length === 1, "a failed attempt is listed even though it has no recap");
ok(says(failedCards[0], "attempt 2") && says(failedCards[0], "unparseable answer"),
  "a failed attempt reports the attempt count and the reason");
ok(says(failedCards[0], "no reusable notes") === false,
  "a failed attempt is not reported as a session with nothing to distil");
ok(says(failedHost, "have not been generated"), "the page counts what is still waiting");
// The point of listing a failure is being able to try it again.
const retry = [...failedHost.querySelectorAll("button")].find((button) => button.textContent?.includes("Retry"));
ok(retry !== undefined, "a failed attempt offers to try again");
await clickButton(retry);
ok(generated.length === 1 && generated[0] === failedPath,
  `retrying asks the host for that session: ${JSON.stringify(generated)}`);
await act(async () => { failedRoot.unmount(); });

// A session that produced nothing is in neither list, so a recap that never
// arrived would be unreachable; the newest few are offered here instead.
const barePath = "C:\\sessions\\20260906-090000.000000000-deepseek-flash.jsonl";
const bareHost = document.createElement("div");
document.body.appendChild(bareHost);
const bareRoot = createRoot(bareHost);
await act(async () => {
  bareRoot.render(<LocaleProvider><SessionRecapPage active onBack={() => {}}
    list={async () => []} listSessions={async () => [meta(barePath, "Delta 会话", 4)]} {...reviewProps} /></LocaleProvider>);
  await new Promise((resolve) => setTimeout(resolve, 0));
});
const bareSection = [...bareHost.querySelectorAll(".management-notice")].find((node) => node.querySelector("ul") !== null);
ok(says(bareSection, "no recap yet"), "a session with no recap is offered a generation");
ok(says(bareSection?.querySelector("li"), "delta 会话"), "the offered session is named by its title");
await clickButton([...(bareSection?.querySelectorAll("button") ?? [])][0]);
ok(generated.length === 2 && generated[1] === barePath,
  `generating asks the host for that session: ${JSON.stringify(generated)}`);
ok(says(bareSection, "queued"), "a queued generation says so instead of claiming a result");
await act(async () => { bareRoot.unmount(); });

// 第十四 (docs/60 §2.1): a heatmap cell filters the list it sits above. The
// fixtures are dated relative to now so the window holds them whenever this runs.
const dayHost = document.createElement("div");
document.body.appendChild(dayHost);
const dayRoot = createRoot(dayHost);
// Day keys are local calendar days — the day the page prints for a recap — so the
// fixtures step by local days and the cell is found by that same key.
const today = new Date();
const dayStamp = (offsetDays: number) => {
  const stamp = new Date(today);
  stamp.setDate(today.getDate() - offsetDays);
  stamp.setHours(12, 0, 0, 0);
  return stamp.toISOString();
};
const dayStampKey = (stamp: string) => {
  const at = new Date(stamp);
  return `${at.getFullYear()}-${String(at.getMonth() + 1).padStart(2, "0")}-${String(at.getDate()).padStart(2, "0")}`;
};
const dayRecaps = [
  recap("C:\sessions\today.jsonl", "Today 会话", dayStamp(0)),
  recap("C:\sessions\earlier.jsonl", "Earlier 会话", dayStamp(9)),
];
await act(async () => {
  // No insights here: the shared stub carries one dated now, which would add a
  // filled cell per project and make the count depend on the clock.
  dayRoot.render(<LocaleProvider><SessionRecapPage active onBack={() => {}}
    list={async () => dayRecaps} listSessions={async () => []} {...reviewProps}
    listInsights={async () => []} /></LocaleProvider>);
  await new Promise((resolve) => setTimeout(resolve, 0));
});
const dayCards = () => [...dayHost.querySelectorAll(".history-list li")];
const filledCells = () => [...dayHost.querySelectorAll(".recap-heatmap__cell--filled")];
ok(filledCells().length === 2, "the heatmap marks the days the page holds data for");
ok(dayCards().length === 2, "both recaps are listed before any filter");
const todayCell = filledCells().find((cell) => (cell.getAttribute("aria-label") ?? "").includes(dayStampKey(dayStamp(0))));
await act(async () => {
  (todayCell as HTMLButtonElement | undefined)?.click();
  await new Promise((resolve) => setTimeout(resolve, 0));
});
ok(dayCards().length === 1, "selecting a day narrows the list to that day");
ok(dayHost.textContent?.includes("Today 会话") === true && dayHost.textContent?.includes("Earlier 会话") === false,
  "the surviving row is the selected day's session");
// A day filter has to be visible and removable on its own: otherwise a day with no
// recaps leaves an empty list with no way back to the full one.
const dayChip = () => [...dayHost.querySelectorAll<HTMLButtonElement>(".history-filter__pill")]
  .find((button) => button.textContent?.includes(dayStampKey(dayStamp(0))) === true);
ok(dayChip() !== undefined, "the filtered day stands in the toolbar as a chip");
await act(async () => {
  dayChip()?.click();
  await new Promise((resolve) => setTimeout(resolve, 0));
});
ok(dayCards().length === 2, "the day chip clears the filter on its own");
await act(async () => {
  (todayCell as HTMLButtonElement | undefined)?.click();
  await new Promise((resolve) => setTimeout(resolve, 0));
});
const pressedCell = [...dayHost.querySelectorAll(".recap-heatmap__cell--active")][0];
await act(async () => {
  (pressedCell as HTMLButtonElement | undefined)?.click();
  await new Promise((resolve) => setTimeout(resolve, 0));
});
ok(dayCards().length === 2, "clearing the selection restores the list");
await act(async () => { dayRoot.unmount(); });

// 第十六 (docs/60 §2.2): the session detail carries its recall/skill fingerprints,
// read by transcript path (R-45: this is the page-level interaction assertion).
const stripHost = document.createElement("div");
document.body.appendChild(stripHost);
const stripRoot = createRoot(stripHost);
const stripRecaps = [recap("C:\sessions\strip.jsonl", "Strip 会话", new Date().toISOString())];
await act(async () => {
  stripRoot.render(<LocaleProvider><SessionRecapPage active onBack={() => {}}
    list={async () => stripRecaps} listSessions={async () => []} {...reviewProps}
    listInsights={async () => []}
    recallRecord={async (sessionPath: string) => ({
      available: true,
      sessionPath,
      turns: [{ turnSeq: 2, hits: [{ id: "mem-page", revision: 1, score: 0.8, injected: true }] }],
      skills: [],
    })} /></LocaleProvider>);
  await new Promise((resolve) => setTimeout(resolve, 0));
});
const pageStrip = stripHost.querySelector(".recap-recall");
ok(pageStrip !== null, "the session detail offers its recall record");
ok((pageStrip?.textContent ?? "").includes("mem-page") === false, "the folded strip lists no fingerprint yet");
const pageStripHead = pageStrip?.querySelector<HTMLButtonElement>(".recap-recall__head");
await act(async () => {
  pageStripHead?.click();
  await new Promise((resolve) => setTimeout(resolve, 0));
});
ok((stripHost.textContent ?? "").includes("mem-page"), "expanding the page strip lists the recalled id");
await act(async () => { stripRoot.unmount(); });

// A same-topic group is one row acting on the notes the reader checked, and a note
// that is already settled keeps its own way back. Both were wrong before: the row's
// drop button ignored the checkboxes, and a settled note in a group had no undo.
const groupPath = "C:\\sessions\\20260907-090000.000000000-deepseek-flash.jsonl";
const groupHost = document.createElement("div");
document.body.appendChild(groupHost);
const groupRoot = createRoot(groupHost);
const groupedRecap = {
  path: groupPath, model: "deepseek/test", generatedAt: "2026-09-07T09:00:00Z",
  entries: [
    { id: "group-a", kind: "fact", body: "session_recap heatmapDay label fix", target: "memory" },
    { id: "group-b", kind: "fact", body: "session_recap heatmapDay label regression", target: "memory" },
  ],
} as SessionRecap;
await act(async () => {
  groupRoot.render(<LocaleProvider><SessionRecapPage active onBack={() => {}}
    list={async () => [groupedRecap]} listSessions={async () => [meta(groupPath, "Group 会话", 3)]} {...reviewProps} /></LocaleProvider>);
  await new Promise((resolve) => setTimeout(resolve, 0));
});
const groupRows = () => [...groupHost.querySelectorAll("[data-recap-row]")];
const groupButton = (label: string) => [...groupHost.querySelectorAll<HTMLButtonElement>("button")]
  .find((button) => button.textContent?.includes(label));
ok(groupRows().length === 2, "two notes of one topic are listed");
ok(groupHost.querySelectorAll('input[type="checkbox"]').length === 2, "each grouped note carries a selection box");
ok(groupButton("Save to memory") !== undefined && groupButton("Don't save") !== undefined,
  "the topic row offers its own accept and drop");
const rejectedBefore = rejected.length;
await act(async () => {
  groupHost.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')[0]?.click();
  await new Promise((resolve) => setTimeout(resolve, 0));
});
await clickButton(groupButton("Don't save"));
ok(rejected.length === rejectedBefore + 1 && rejected[rejected.length - 1]?.body.includes("regression") === true,
  `the row drops only the checked notes: ${JSON.stringify(rejected.slice(rejectedBefore))}`);
// The unchecked note is still there to act on: checking it back puts it into the
// row's accept, and settling it must leave it its own way back.
ok(groupButton("Save to memory") === undefined, "an all-unchecked row offers no accept to act on");
await act(async () => {
  groupHost.querySelectorAll<HTMLInputElement>('input[type="checkbox"]')[0]?.click();
  await new Promise((resolve) => setTimeout(resolve, 0));
});
await clickButton(groupButton("Save to memory"));
await clickButton(groupHost.querySelector<HTMLButtonElement>(".recap-preview-confirm") ?? undefined);
const rowHasUndo = (row: Element) => [...row.querySelectorAll("button")].some((button) => button.textContent?.includes("Undo"));
ok(groupRows().length === 2 && groupRows().every(rowHasUndo),
  "a settled note inside a group keeps its own undo");
await act(async () => { groupRoot.unmount(); });

await act(async () => { root.unmount(); });
process.stdout.write(`\n${failed === 0 ? "OK" : "FAILED"}: ${failed} failed\n`);
if (failed > 0) process.exit(1);
