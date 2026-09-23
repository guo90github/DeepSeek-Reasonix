// Run: tsx src/__tests__/split-tool-recovery.test.tsx
//
// The tool-recovery entry had one mount point, inside the transcript shell the
// single column renders; split dissolves that shell into the .chat-pane grid,
// so a pending recovery was reachable only after switching layouts. The notice
// also had no close control, and a plain refresh unmounted it — which re-opened
// whatever the reader had just collapsed or closed. Closing is now remembered
// per session path by the calls the notice names, so neither snapshot churn
// (revision, runtime epoch) nor a session switch — whose frontend session key
// carries a load generation — brings the closed notice back.
import assert from "node:assert/strict";
import { register } from "node:module";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { JSDOM } from "jsdom";
import type { ToolRecoverySnapshot } from "../lib/toolRecovery";

register(new URL("../../scripts/css-loader.mjs", import.meta.url));
register(new URL("../../scripts/svg-loader.mjs", import.meta.url));

class TestResizeObserver { observe() {} unobserve() {} disconnect() {} }
class TestIntersectionObserver { observe() {} unobserve() {} disconnect() {} takeRecords() { return []; } }
const noop = () => {};
const stubMatchMedia = () => ({
  matches: false, media: "", onchange: null,
  addEventListener: noop, removeEventListener: noop, addListener: noop, removeListener: noop,
  dispatchEvent: () => false,
});

const dom = new JSDOM("<div id='root'></div><div id='panel'></div><div id='remount'></div><div id='switch'></div><div id='action'></div>", { url: "http://localhost", pretendToBeVisual: true });
Object.assign(globalThis, {
  window: dom.window,
  document: dom.window.document,
  localStorage: dom.window.localStorage,
  IS_REACT_ACT_ENVIRONMENT: true,
  ResizeObserver: TestResizeObserver,
  IntersectionObserver: TestIntersectionObserver,
  requestAnimationFrame: dom.window.requestAnimationFrame.bind(dom.window),
  cancelAnimationFrame: dom.window.cancelAnimationFrame.bind(dom.window),
});
Object.assign(dom.window, { matchMedia: stubMatchMedia });

const { installDesktopHostStub } = await import("./desktopHostStub");
const { ChatPaneRegion } = await import("../app-shell/ChatPaneRegion");
const { ToolRecoveryPanel } = await import("../components/ToolRecoveryPanel");
const { initialState } = await import("../lib/useController");
const { LocaleProvider } = await import("../lib/i18n");
const { projectSessionAvailability } = await import("../lib/sessionAvailability");

const pendingCall = {
  identity: { attempt_id: "attempt-1", canonical_tool: "write_file", argument_digest: "sha256:abc", resource_scope: "src/x.ts" },
  state: "awaiting_resolution", read_only: false, inspection_id: "inspection-1",
};
const pending = { sessionPath: "/session", runtimeEpoch: "epoch-1", revision: "rev-1", calls: [pendingCall], retryEnabled: false };
const cleared = { sessionPath: "/session", runtimeEpoch: "epoch-1", revision: "rev-2", calls: [], retryEnabled: false };
const resolutions: { tabId: string; action: string }[] = [];
const host = installDesktopHostStub({
  GetToolRecoveryForTab: () => pending,
  ResolveToolRecoveryForTab: (tabId: string, request: { action: string }) => {
    resolutions.push({ tabId, action: request.action });
    return cleared;
  },
});

const readyLocal = { ...initialState, meta: { ready: true, eventChannel: "fixture" } };
const prompts: string[] = [];
const root = createRoot(document.getElementById("root")!);
try {
  // 1. Split mode mounts the entry and both of its commands reach the owner.
  await act(async () => root.render(
    <LocaleProvider>
      <ChatPaneRegion
        splitMode
        transitioning={false}
        t={((key: string) => key) as never}
        imDetail={null}
        transcript={{
          state: readyLocal,
          items: [], tabId: "split-tab", geometrySessionKey: "split-tab", footerHeight: 0,
          transcriptHydrating: false, navigationDataReady: true, readOnly: false, controllerReady: true,
          hydratePlaceholderActive: false, clearContextPending: false, creation: false,
          availability: projectSessionAvailability({ local: readyLocal }),
          rewind: { stateActive: false, committing: false, signal: undefined },
          revealSignal: 0, invocationMetadata: undefined, surfaceCommitToken: undefined, liveStore: undefined,
        }}
        onRetryHistory={async () => {}}
        commands={{
          onPrompt: (text: string) => { prompts.push(text); },
          onDeliveryContinue: noop, onAcceptDelivery: noop, onOpenChanges: noop, onOpenVerification: noop,
          onEditPrompt: noop, onRewind: noop, onLoadOlderHistory: async () => false, onSurfacePaintReady: noop,
        }}
      />
    </LocaleProvider>,
  ));
  await act(async () => {});

  const recoveryHost = document.querySelector(".split-recovery-host");
  assert.ok(recoveryHost, "split mode mounts a host for the tool-recovery entry");
  assert.ok(recoveryHost!.closest("main.main--split"), "the entry belongs to the split surface, not a stray row");
  const splitPanel = recoveryHost!.querySelector(".tool-recovery-panel");
  assert.ok(splitPanel, "a pending tool recovery surfaces in split mode instead of waiting for the single column");
  assert.ok(splitPanel!.textContent?.includes("write_file"), "the entry names the call awaiting resolution");

  await act(async () => splitPanel!.querySelector<HTMLButtonElement>(".notice-line__actions .btn")!.click());
  assert.deepEqual(resolutions, [{ tabId: "split-tab", action: "inspect" }], "the split entry reaches the owning resolution command");
  assert.equal(document.querySelector(".split-recovery-host .tool-recovery-panel .notice-line__actions"), null,
    "a resolved snapshot clears the pending actions");

  const resume = document.querySelector<HTMLButtonElement>(".split-recovery-host .tool-recovery-panel .notice-line__text > button");
  assert.ok(resume, "a cleared recovery keeps the resume action in split mode");
  await act(async () => resume!.click());
  assert.deepEqual(prompts, ["toolRecovery.resumePrompt"], "resume reaches the split surface's prompt command");
  await act(async () => root.unmount());

  // 2. Closing the notice must survive the items that keep arriving behind it:
  // the panel used to blank itself on every refresh, with no close control.
  let snapshot: ToolRecoverySnapshot = pending;
  let fetches = 0;
  const bindings = { GetToolRecoveryForTab: async () => { fetches += 1; return snapshot; } };
  const panelRoot = createRoot(document.getElementById("panel")!);
  const paintPanel = (refreshKey: number) => act(async () => panelRoot.render(
    <LocaleProvider>
      <ToolRecoveryPanel tabId="panel-tab" sessionKey="panel-tab" running={false} refreshKey={refreshKey}
        bindings={bindings} onResume={noop} />
    </LocaleProvider>,
  ));

  await paintPanel(0);
  assert.ok(document.querySelector("#panel .tool-recovery-panel"), "the entry renders while a call needs resolution");

  // 3. Collapsing keeps the title in view; a refresh used to remount the panel
  // open, so a collapse never stuck.
  const details = (): HTMLDetailsElement => document.querySelector("#panel .tool-recovery-panel > details")!;
  await act(async () => { details().open = false; details().dispatchEvent(new dom.window.Event("toggle", { bubbles: true })); });
  assert.equal(details().open, false, "the notice collapses");
  await paintPanel(1);
  assert.equal(details().open, false, "a refresh keeps the collapsed notice collapsed");

  // 4. Closing the notice must survive the items that keep arriving behind it:
  // the panel used to blank itself on every refresh, with no close control.
  await act(async () => document.querySelector<HTMLButtonElement>("#panel .tool-recovery-panel__dismiss")!.click());
  assert.equal(document.querySelector("#panel .tool-recovery-panel"), null, "关闭 closes the notice");
  await paintPanel(2);
  assert.ok(fetches >= 3, "an arriving item re-reads the recovery state");
  assert.equal(document.querySelector("#panel .tool-recovery-panel"), null, "an arriving item does not reopen the closed notice");

  // 5. Snapshot churn is not a later interruption: the server revision hashes
  // statistics and the runtime epoch, so a moved revision used to reopen the
  // closed notice while the same call was still the only one pending.
  snapshot = { ...pending, revision: "rev-2", runtimeEpoch: "epoch-2" };
  await paintPanel(3);
  assert.equal(document.querySelector("#panel .tool-recovery-panel"), null,
    "a moved revision or runtime epoch does not reopen the closed notice");
  await act(async () => panelRoot.unmount());

  // 6. A session switch is the case this identity exists for. The panel's key
  // is the frontend session key, which carries a load generation (and is a tab
  // id for remote surfaces), so A -> B -> A comes back under a different key
  // and the same session path. Closing must be remembered per session path.
  const switchSessions: Record<string, ToolRecoverySnapshot> = {
    A: { ...pending, sessionPath: "/session-a", revision: "rev-a" },
    B: { ...pending, sessionPath: "/session-b", revision: "rev-b", calls: [{ ...pendingCall, identity: { ...pendingCall.identity, attempt_id: "attempt-b" } }] },
  };
  let currentSession = "A";
  let loadGeneration = 0;
  const switchBindings = { GetToolRecoveryForTab: async () => switchSessions[currentSession] };
  const switchRoot = createRoot(document.getElementById("switch")!);
  const paintSession = (name: "A" | "B") => {
    currentSession = name;
    loadGeneration += 1;
    const sessionKey = `session\u0000/session-${name.toLowerCase()}\u0000${loadGeneration}`;
    return act(async () => switchRoot.render(
      <LocaleProvider>
        <ToolRecoveryPanel key={sessionKey} tabId={`tab-${name}`} sessionKey={sessionKey} running={false} refreshKey={0}
          bindings={switchBindings} onResume={noop} />
      </LocaleProvider>,
    ));
  };
  const switchNotice = () => document.querySelector("#switch .tool-recovery-panel");
  const closeSwitchNotice = async () => act(async () => document.querySelector<HTMLButtonElement>("#switch .tool-recovery-panel__dismiss")!.click());

  await paintSession("A");
  assert.ok(switchNotice(), "session A surfaces its notice");
  await closeSwitchNotice();
  assert.equal(switchNotice(), null, "closing in session A hides it");

  await paintSession("B");
  assert.ok(switchNotice(), "session B has its own notice, not A's dismissal");
  await closeSwitchNotice();
  assert.equal(switchNotice(), null, "closing in session B hides it");

  await paintSession("A");
  assert.equal(switchNotice(), null, "returning to session A keeps its notice closed");
  await paintSession("B");
  assert.equal(switchNotice(), null, "session B's dismissal survives the round trip too");

  // A newly interrupted call carries its own attempt id, so it is not covered.
  switchSessions.A = { ...switchSessions.A, calls: [pendingCall, { ...pendingCall, identity: { ...pendingCall.identity, attempt_id: "attempt-2" } }] };
  await paintSession("A");
  assert.ok(switchNotice(), "a newly interrupted call is not silenced by the old dismissal");
  await act(async () => switchRoot.unmount());

  // 7. A refresh landing mid-action must not leave the notice busy: the actions
  // would stay disabled with no way to reach the call again.
  let settle: ((value: ToolRecoverySnapshot) => void) | undefined;
  // Its own session path: dismissal is remembered per session path, so reusing
  // the one closed in sections 4 and 5 would hide this notice on purpose.
  const actionSnapshot = { ...pending, sessionPath: "/session-action", revision: "rev-9" };
  const actionBindings = {
    GetToolRecoveryForTab: async (): Promise<ToolRecoverySnapshot> => actionSnapshot,
    ResolveToolRecoveryForTab: () => new Promise<ToolRecoverySnapshot>((resolve) => { settle = resolve; }),
  };
  const actionRoot = createRoot(document.getElementById("action")!);
  const paintActionPanel = (refreshKey: number) => act(async () => actionRoot.render(
    <LocaleProvider>
      <ToolRecoveryPanel tabId="action-tab" sessionKey="action-tab" running={false} refreshKey={refreshKey}
        bindings={actionBindings} onResume={noop} />
    </LocaleProvider>,
  ));
  const inspectButton = () => document.querySelector<HTMLButtonElement>("#action .tool-recovery-panel .notice-line__actions .btn");

  await paintActionPanel(0);
  await act(async () => inspectButton()!.click());
  assert.equal(inspectButton()!.disabled, true, "a running action disables its buttons");
  await paintActionPanel(1);
  await act(async () => settle!({ ...actionSnapshot, revision: "rev-10" }));
  assert.equal(inspectButton()!.disabled, false, "a refresh mid-action does not leave the notice stuck busy");
  await act(async () => actionRoot.unmount());

  console.log("PASS split tool recovery: the entry mounts above both panes, closes, and survives churn and remounts");
} finally {
  host.uninstall();
  dom.window.close();
}
