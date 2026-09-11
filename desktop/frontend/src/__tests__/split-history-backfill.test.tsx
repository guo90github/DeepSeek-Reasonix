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

  // 1b. Older pages are available but no request is in flight and no error is
  // recorded: the header must not render an empty strip that pads the pane and
  // reads as an empty control above the first turn.
  assert.equal(document.querySelector(".conversation-pane__older"), null,
    "an idle older-history header renders nothing and takes no layout space");
  await act(async () => paneRoot.unmount());

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

  console.log("PASS split history: backfill re-arms after the run settles and recovery stays reachable");
} finally {
  dom.window.close();
}
