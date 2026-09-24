// Run: tsx src/__tests__/split-history-backfill.test.tsx
//
// Locks the split layout's history backfill and its recovery affordance. The
// controller refuses an older-page request while the turn runs, so the pane
// must re-arm once the run settles; otherwise a split pane sits on a truncated
// history forever with no loading state, no error and no retry.
import assert from "node:assert/strict";
import { register } from "node:module";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { JSDOM } from "jsdom";

register(new URL("../../scripts/css-loader.mjs", import.meta.url));
register(new URL("../../scripts/svg-loader.mjs", import.meta.url));
const { ConversationPane } = await import("../components/ConversationPane");
const { ChatPaneRegion } = await import("../app-shell/ChatPaneRegion");
const { initialState } = await import("../lib/useController");
const { LocaleProvider } = await import("../lib/i18n");

class TestResizeObserver { observe() {} unobserve() {} disconnect() {} }
const dom = new JSDOM("<div id='root'></div><div id='split'></div>", { url: "http://localhost", pretendToBeVisual: true });
Object.assign(globalThis, {
  window: dom.window,
  document: dom.window.document,
  localStorage: dom.window.localStorage,
  IS_REACT_ACT_ENVIRONMENT: true,
  ResizeObserver: TestResizeObserver,
  requestAnimationFrame: dom.window.requestAnimationFrame.bind(dom.window),
  cancelAnimationFrame: dom.window.cancelAnimationFrame.bind(dom.window),
});

const paneTurns = [
  { key: "t0", turn: 0, user: { id: "u0", text: "first question" }, answers: [], hasShownContent: true, isActive: false },
  { key: "t1", turn: 1, user: { id: "u1", text: "second question" }, answers: [], hasShownContent: true, isActive: false },
];
const noop = () => {};

try {
  // 1. The pane's backfill must survive a refusal that only `running` explains.
  const paneRoot = createRoot(document.getElementById("root")!);
  const calls: boolean[] = [];
  let running = true;
  const onLoadOlderHistory = () => {
    calls.push(running);
    return Promise.resolve(!running);
  };
  const paintPane = (flag: boolean) => act(async () => {
    paneRoot.render(<LocaleProvider><ConversationPane turns={paneTurns as never} tabId="tab-a" running={flag} footerHeight={0}
      hasOlderHistory loadingOlderHistory={false} onLoadOlderHistory={onLoadOlderHistory} hydrating={false} /></LocaleProvider>);
  });
  await paintPane(true);
  const callsWhileRunning = calls.length;
  assert.equal(callsWhileRunning, 0, "the split pane does not issue a backfill the controller will refuse while the turn runs");
  running = false;
  await paintPane(false);
  assert.ok(calls.includes(false), "the split pane re-arms the older-history backfill once the run settles");

  // 1b. Older pages are available with no request in flight and no error: the
  // header offers the manual load instead of padding the pane with an empty
  // strip, so a stall the automatic path cannot resolve stays one click away.
  const olderStrip = document.querySelector(".conversation-pane__older");
  assert.ok(olderStrip, "an idle truncated window keeps a reachable older-history control");
  const beforeManual = calls.length;
  await act(async () => olderStrip!.querySelector<HTMLButtonElement>("button")!.click());
  assert.ok(calls.length > beforeManual, "the manual older-history control reaches the pane's loader");
  await act(async () => paneRoot.unmount());

  // 1c. A request that moves none of the guard's inputs — refused, superseded or
  // an empty page — must be retried: the pane used to sit on the truncated
  // window until the user remounted the view.
  const retryRoot = createRoot(document.getElementById("root")!);
  let noopCalls = 0;
  const noopLoader = () => { noopCalls += 1; return Promise.resolve(false); };
  await act(async () => retryRoot.render(<LocaleProvider><ConversationPane turns={paneTurns as never} tabId="tab-a" running={false}
    footerHeight={0} hasOlderHistory loadingOlderHistory={false} onLoadOlderHistory={noopLoader} hydrating={false} /></LocaleProvider>));
  assert.equal(noopCalls, 1, "the first backfill request is immediate");
  await act(async () => { await new Promise((resolve) => setTimeout(resolve, 1400)); });
  assert.ok(noopCalls >= 2, `a no-op backfill is retried instead of parking the pane (calls=${noopCalls})`);
  await act(async () => retryRoot.unmount());

  // 2. Split mode must surface history recovery instead of an empty pane.
  const splitRoot = createRoot(document.getElementById("split")!);
  let retries = 0;
  const availability = { kind: "error" as const, source: "history" as const, detail: "history read failed" };
  await act(async () => splitRoot.render(
    <LocaleProvider>
      <ChatPaneRegion splitMode transitioning={false} t={((key: string) => key) as never} imDetail={null}
        transcript={{
          state: initialState, items: [], tabId: "local", geometrySessionKey: "local", footerHeight: 0,
          transcriptHydrating: false, navigationDataReady: true, readOnly: false, controllerReady: true,
          hydratePlaceholderActive: false, clearContextPending: false, creation: false, availability,
          rewind: { stateActive: false, committing: false, signal: undefined }, revealSignal: 0,
          invocationMetadata: undefined, surfaceCommitToken: undefined, liveStore: undefined,
        }}
        onRetryHistory={async () => { retries += 1; }}
        commands={{ onPrompt: noop, onDeliveryContinue: noop, onAcceptDelivery: noop, onOpenChanges: noop,
          onOpenVerification: noop, onEditPrompt: noop, onRewind: noop, onLoadOlderHistory: async () => false,
          onSurfacePaintReady: noop }} />
    </LocaleProvider>,
  ));
  const banner = document.querySelector(".session-recovery");
  assert.ok(banner, "split mode surfaces history recovery instead of leaving an empty pane");
  assert.ok(document.querySelector("main .session-recovery-placeholder"), "split mode keeps the placeholder inside the main surface");
  await act(async () => banner!.querySelector<HTMLButtonElement>(".btn--primary")!.click());
  assert.equal(retries, 1, "split recovery retry reaches the owning history command");
  await act(async () => splitRoot.unmount());

  // 1d. The controller refuses an older-page request while the turn runs, so a
  // control offered there can only do nothing: the header must state the pause.
  const pausedRoot = createRoot(document.getElementById("root")!);
  let pausedCalls = 0;
  await act(async () => pausedRoot.render(<LocaleProvider><ConversationPane turns={paneTurns as never} tabId="tab-a" running
    footerHeight={0} hasOlderHistory loadingOlderHistory={false} hydrating={false}
    onLoadOlderHistory={() => { pausedCalls += 1; return Promise.resolve(false); }} /></LocaleProvider>));
  assert.ok(document.querySelector(".conversation-pane__older-paused"), "a run in flight states the older-history pause");
  assert.equal(document.querySelector(".conversation-pane__older button"), null, "no older-history control is offered while the run refuses the request");
  assert.equal(pausedCalls, 0, "the paused header issues no request the controller would refuse");
  await act(async () => pausedRoot.unmount());

  // 1e. Split mode has no navigation mask, so an in-flight session load must be
  // visible in the pane itself — it used to look like a session with no content.
  const hydratingRoot = createRoot(document.getElementById("root")!);
  await act(async () => hydratingRoot.render(<LocaleProvider><ConversationPane turns={[] as never} tabId="tab-a" running={false}
    footerHeight={0} hasOlderHistory={false} loadingOlderHistory={false} hydrating /></LocaleProvider>));
  assert.ok(document.querySelector(".conversation-pane__hydrating"), "a session load in flight is visible in the split pane");
  await act(async () => hydratingRoot.unmount());

  // 3. The pane anchor belongs to one session: a switch reuses the split
  // instance, and the one-shot latch left the new session's panes unpinned.
  const { SplitWorkspace } = await import("../components/SplitWorkspace");
  const { setFrontendDiagnosticSink } = await import("../lib/frontendDiagnosticBridge");
  Object.assign(dom.window, {
    matchMedia: () => ({ matches: false, media: "", onchange: null, addEventListener() {}, removeEventListener() {}, addListener() {}, removeListener() {}, dispatchEvent: () => false }),
  });
  Object.assign(globalThis, { MutationObserver: dom.window.MutationObserver });
  const anchorProto = dom.window.HTMLElement.prototype as unknown as { scrollTo?: (...args: unknown[]) => void };
  anchorProto.scrollTo ??= () => {};
  const anchorRoot = createRoot(document.getElementById("split")!);
  const anchors: number[] = [];
  setFrontendDiagnosticSink((source, type, fields) => {
    if (source === "navigation" && type === "split.pane-anchor") anchors.push(Number(fields.totalRows));
  });
  const anchorItems = [
    { kind: "user", id: "a-u1", text: "first question", createdAt: 1000 },
    { kind: "assistant", id: "a-a1", text: "first answer", reasoning: "", streaming: false },
  ] as never;
  const paintAnchor = (tabId: string) => act(async () => {
    anchorRoot.render(<LocaleProvider><SplitWorkspace items={anchorItems} tabId={tabId} running={false} /></LocaleProvider>);
  });
  await paintAnchor("tab-a");
  assert.deepEqual(anchors, [1], "the split panes pin to the newest turn for the session they mount with");
  await paintAnchor("tab-a");
  assert.deepEqual(anchors, [1], "re-rendering the same session does not re-pin the panes");
  await paintAnchor("tab-b");
  assert.deepEqual(anchors, [1, 1], "a session switch re-arms the pane anchor instead of trusting a one-shot latch");
  await act(async () => anchorRoot.unmount());
  setFrontendDiagnosticSink(() => {});

  console.log("PASS split history: backfill re-arms after the run settles, recovery stays reachable, and a switch re-pins the panes");
} finally {
  dom.window.close();
}
