// Run: node --import ./scripts/css-stub-register.mjs --import ./scripts/svg-stub-register.mjs --import tsx src/__tests__/browser-dock-mode.test.tsx
import assert from "node:assert/strict";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { JSDOM } from "jsdom";

import { WorkspaceDockRegion, type WorkspaceDockRegionProps } from "../app-shell/WorkspaceDockRegion";
import type { DesktopBrowserHost } from "../lib/browserHost";
import type { ReasonixDesktopHost } from "../lib/desktopHost";
import { LocaleProvider, type Translator } from "../lib/i18n";
import { useLayoutStore, type RightDockMode } from "../store/layout";

const dom = new JSDOM("<div id='root'></div>", { url: "http://localhost", pretendToBeVisual: true });
Object.assign(globalThis, { window: dom.window, document: dom.window.document, localStorage: dom.window.localStorage,
  IS_REACT_ACT_ENVIRONMENT: true });
class TestResizeObserver { observe() {} unobserve() {} disconnect() {} }
(dom.window as unknown as { ResizeObserver: unknown }).ResizeObserver = TestResizeObserver;

const noop = () => {};
let openedWindows = 0;
const browser: DesktopBrowserHost = {
  list: async () => [], open: async () => { throw new Error("unused"); }, close: async () => {}, activate: async () => {},
  navigate: async () => {}, setZoom: async () => {}, toggleDevTools: async () => {}, resume: async () => {},
  setLayout: noop, setOverlay: noop, onTabs: () => noop, onDownload: () => noop,
  openWindow: () => { openedWindows += 1; },
};
const electron: ReasonixDesktopHost = {
  kind: "electron",
  contract: { protocolVersion: 1, digest: "test", commands: [] },
  platform: { os: "linux", arch: "x64", versions: {} },
  invoke: async () => undefined,
  on: () => noop,
  native: {
    openExternal: async () => {},
    clipboard: { writeText: async () => true, readText: async () => "" },
    window: { setTheme: noop, setBackgroundColour: noop, getBounds: async () => ({ x: 0, y: 0, width: 0, height: 0, maximised: false }),
      isMaximised: async () => false, minimise: noop, toggleMaximise: noop, close: noop },
    getPathForFile: () => "",
    onServiceState: () => noop,
  },
  browser,
};

const modes: RightDockMode[] = [];
const props = (mode: RightDockMode, creation = false): WorkspaceDockRegionProps => ({
  visible: true, overlay: false, mode, creation, remoteAvailable: false, showContext: true,
  t: ((key: string) => key) as Translator,
  onMode: (next) => { modes.push(next); }, onRemote: noop,
  remote: {} as WorkspaceDockRegionProps["remote"], context: {} as WorkspaceDockRegionProps["context"],
  workspace: { tabId: "A" } as WorkspaceDockRegionProps["workspace"], workspaceKey: "k",
});
const root = createRoot(document.getElementById("root")!);
const paint = (mode: RightDockMode, creation = false) =>
  act(async () => root.render(<LocaleProvider><WorkspaceDockRegion {...props(mode, creation)} /></LocaleProvider>));
const tabLabels = () => [...document.querySelectorAll(".workbench-dock__tab-label")].map((el) => el.textContent ?? "");
const tabs = () => [...document.querySelectorAll<HTMLButtonElement>("[role='tab']")];
const tabSelected = (label: string) =>
  tabs().find((el) => el.textContent === label)?.getAttribute("aria-selected");
// Presence assertions stay boolean: a failure message that has to inspect a
// jsdom node graph is what makes these suites look like they hang.
const present = (selector: string) => document.querySelector(selector) !== null;
// The dock tab label bypasses the shared dictionaries, so accept every locale
// the panel can render.
const BROWSER_LABELS = ["Browser", "浏览器", "瀏覽器"];
const browserLabel = () => {
  const siblings = tabLabels().filter((label) => label !== "rightDock.overview" && label !== "workspace.filesTab");
  assert.equal(siblings.length, 1, "exactly one tab beside the overview tab");
  assert.ok(BROWSER_LABELS.includes(siblings[0]), `the extra tab is the browser tab, got ${siblings[0]}`);
  return siblings[0];
};
const settle = () => act(async () => { await new Promise((resolve) => setTimeout(resolve, 20)); });

try {
  console.log("\nbrowser dock mode");
  // No shell host: the 概览 row stands alone whether or not the browser mode is
  // selected — the host gate lives on the tab, not on the panel shell.
  await paint("files");
  assert.deepEqual(tabLabels(), ["rightDock.overview"], "no shell host: no browser tab");
  await paint("browser");
  await settle();
  assert.deepEqual(tabLabels(), ["rightDock.overview"], "no shell host: browser mode offers no browser tab");
  assert.equal(present(".browser-entry"), true, "browser mode still renders the window entry");
  assert.equal(document.querySelector<HTMLButtonElement>(".browser-entry__open")?.disabled, true, "without a host the open action is inert");

  window.reasonixDesktop = electron;
  await paint("files");
  await settle();
  const label = browserLabel();
  assert.equal(tabSelected("rightDock.overview"), "true", "the 概览 tab owns the merged body while files mode is selected");
  assert.equal(tabSelected(label), "false");
  assert.equal(present(".browser-panel"), false, "files mode does not mount the browser panel");

  await act(async () => tabs().find((el) => el.textContent === label)!.click());
  assert.deepEqual(modes, ["browser"], "the tab requests the browser dock mode through the shared mode command");

  await paint("browser");
  await settle();
  assert.equal(tabSelected("rightDock.overview"), "false", "the 概览 tab yields to the browser tab");
  assert.equal(tabSelected(label), "true");
  assert.equal(present(".browser-entry"), true, "browser mode mounts the window entry in the dock body");
  assert.equal(present(".browser-panel"), false, "the panel itself is no longer embedded in the dock");
  await act(async () => document.querySelector<HTMLButtonElement>(".browser-entry__open")!.click());
  assert.equal(openedWindows, 1, "the entry opens the independent window");
  assert.equal(present(".workbench-dock--browser"), true, "the dock carries the mode modifier");
  assert.equal(present(".workbench-dock__body--merged"), false, "the merged overview body is not rendered for the browser tab");

  // Creation keeps its minimal files-only dock: the browser tab is not offered.
  await paint("files", true);
  await settle();
  assert.deepEqual(tabLabels(), ["workspace.filesTab"], "creation keeps the files-only dock");

  useLayoutStore.getState().setRightDockMode("browser");
  assert.equal(useLayoutStore.getState().rightDockMode, "browser", "the layout store accepts the browser dock mode");
  useLayoutStore.getState().setRightDockMode("context");

  delete window.reasonixDesktop;
  await act(async () => root.unmount());
  console.log("browser dock mode: gating, mode switch, window entry and creation shape passed");
} catch (error) {
  console.error(error);
  process.exitCode = 1;
} finally {
  // The pretendToBeVisual frame loop outlives the assertions, so the root must
  // be torn down even when one of them throws.
  await act(async () => root.unmount());
  dom.window.close();
}
