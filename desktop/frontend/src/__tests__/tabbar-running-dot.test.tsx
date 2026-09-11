// Run: tsx src/__tests__/tabbar-running-dot.test.tsx
// The strip's status dot is the shell's only "this session owns a live turn"
// signal, so both halves are pinned here: only a running tab marks its dot, and
// only the running mark animates.

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { JSDOM } from "jsdom";
import type { TabMeta } from "../lib/types";

const dom = new JSDOM("<div id='root'></div>", { pretendToBeVisual: true });
Object.assign(globalThis, {
  window: dom.window,
  document: dom.window.document,
  HTMLElement: dom.window.HTMLElement,
  KeyboardEvent: dom.window.KeyboardEvent,
  IS_REACT_ACT_ENVIRONMENT: true,
});
dom.window.HTMLElement.prototype.scrollIntoView = () => {};

const { default: React, act } = await import("react");
const { createRoot } = await import("react-dom/client");
const { TabBar } = await import("../components/TabBar");
const { LocaleProvider } = await import("../lib/i18n");

const stylesSource = readFileSync(resolve(dirname(fileURLToPath(import.meta.url)), "../styles.css"), "utf8")
  .replace(/\/\*[\s\S]*?\*\//g, "");

function declaration(selector: string, property: string): string | undefined {
  const rule = /([^{}]+)\{([^{}]*)\}/g;
  let match: RegExpExecArray | null;
  let value: string | undefined;
  while ((match = rule.exec(stylesSource)) !== null) {
    if (!match[1].split(",").map((part) => part.trim()).includes(selector)) continue;
    const found = new RegExp(`(?:^|;)\\s*${property}\\s*:\\s*([^;]+)`).exec(match[2]);
    if (found) value = found[1].trim();
  }
  return value;
}

const tab = (id: string, running: boolean): TabMeta => ({
  id,
  scope: "project",
  workspaceRoot: "/repo",
  workspaceName: "repo",
  topicId: `topic-${id}`,
  topicTitle: id,
  label: "model",
  ready: true,
  running,
  cancellable: running,
  mode: "normal",
  active: false,
  cwd: "/repo",
});

const root = createRoot(document.getElementById("root")!);
await act(async () => root.render(
  <LocaleProvider>
    <TabBar
      tabs={[tab("idle", false), tab("busy", true)]}
      activeTabId="idle"
      onTabChange={() => {}}
      onTabClose={() => {}}
      onTabsClose={() => {}}
      onTabsReorder={() => {}}
      onNewTab={() => {}}
    />
  </LocaleProvider>,
));

const nodes = [...document.querySelectorAll<HTMLElement>(".tabbar__tab")];
assert.equal(nodes.length, 2, "both sessions render as tabs");
assert.equal(document.querySelectorAll(".tabbar__status").length, 2, "every session tab carries a status dot");
assert.equal(document.querySelectorAll(".tabbar__status--running").length, 1, "only the running session's dot is marked running");
const busy = nodes.find((node) => node.getAttribute("aria-label")?.startsWith("Running"));
assert.ok(busy, "the running tab advertises its state");
assert.ok(busy!.querySelector(".tabbar__status--running"), "the running dot belongs to the running tab");
const idle = nodes.find((node) => node !== busy);
assert.equal(idle?.querySelector(".tabbar__status--running"), null, "an idle tab keeps the plain dot");

assert.equal(declaration(".tabbar__status", "width"), "7px", "the idle dot is a visible mark");
assert.equal(declaration(".tabbar__status", "height"), "7px", "the idle dot is a visible mark");
assert.equal(declaration(".tabbar__status", "border-radius"), "50%", "the mark reads as a dot");
assert.equal(declaration(".tabbar__status", "background"), "var(--project-accent, var(--accent))", "the dot carries the session's project accent");
assert.equal(declaration(".tabbar__status", "animation"), undefined, "an idle dot never animates");
assert.equal(declaration(".tabbar__tab--active .tabbar__status", "animation"), undefined, "the active tab's idle dot stays still");
assert.ok(declaration(".tabbar__status--running", "animation")?.startsWith("tab-status-breathe-running"), "the running dot breathes");
assert.ok(stylesSource.includes("@keyframes tab-status-breathe-running"), "the breathing keyframes exist");

await act(async () => root.unmount());
console.log("tabbar running dot: ok");
