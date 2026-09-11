// Run: tsx src/__tests__/tabbar-running-dot.test.tsx
// The strip's status dot is the shell's only "this session owns a live turn"
// signal, so the whole contract is pinned here: an idle tab keeps a still blue
// dot, a running tab turns it red and breathes, and reduce-motion stops it.

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

// The stylesheet is walked by brace depth: a flat selector regex would also
// match rules nested inside @media/@keyframes blocks and read their bodies as
// the element's declarations.
function topLevelRules(source: string): Array<{ selectors: string[]; body: string }> {
  const rules: Array<{ selectors: string[]; body: string }> = [];
  let depth = 0;
  let openAt = -1;
  let start = 0;
  for (let index = 0; index < source.length; index += 1) {
    if (source[index] === "{") {
      if (depth === 0) openAt = index;
      depth += 1;
    } else if (source[index] === "}") {
      depth -= 1;
      if (depth === 0 && openAt >= 0) {
        rules.push({
          selectors: source.slice(start, openAt).split(",").map((part) => part.trim()),
          body: source.slice(openAt + 1, index),
        });
        openAt = -1;
        start = index + 1;
      }
    }
  }
  return rules;
}

const rules = topLevelRules(stylesSource);

function declaration(selector: string, property: string): string | undefined {
  let value: string | undefined;
  for (const rule of rules) {
    if (!rule.selectors.includes(selector)) continue;
    const found = new RegExp(`(?:^|;)\\s*${property}\\s*:\\s*([^;]+)`).exec(rule.body);
    if (found) value = found[1].trim();
  }
  return value;
}

// Nested blocks (keyframes, media queries) are read by brace depth instead.
function blockBody(source: string, header: string): string {
  const start = source.indexOf(header);
  assert.ok(start >= 0, `${header} exists`);
  const from = source.indexOf("{", start);
  let depth = 0;
  let index = from;
  for (; index < source.length; index += 1) {
    if (source[index] === "{") depth += 1;
    else if (source[index] === "}") {
      depth -= 1;
      if (depth === 0) break;
    }
  }
  return source.slice(from, index + 1);
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
assert.equal(declaration(".tabbar__status", "background"), "var(--tab-status-idle)", "an idle dot is blue");
assert.equal(declaration(".tabbar__status", "animation"), undefined, "an idle dot never animates");
assert.equal(declaration(".tabbar__tab--active .tabbar__status", "animation"), undefined, "the active tab's idle dot stays still");
assert.ok(
  declaration(".tabbar__tab--active .tabbar__status", "box-shadow")?.includes("var(--tab-status-idle)"),
  "the active tab's ring stays blue",
);

assert.equal(declaration(".tabbar__status--running", "background"), "var(--tab-status-running)", "a running dot is red");
assert.ok(
  declaration(".tabbar__status--running", "box-shadow")?.includes("var(--tab-status-running)"),
  "the running dot's ring is red too",
);
assert.ok(declaration(".tabbar__status--running", "animation")?.startsWith("tab-status-breathe-running"), "the running dot breathes");

const keyframes = blockBody(stylesSource, "@keyframes tab-status-breathe-running");
assert.ok(keyframes.includes("var(--tab-status-running)"), "the breathing keyframes take the running colour");
assert.ok(!keyframes.includes("--project-accent"), "the breathing keyframes ignore the project accent");

const runningAt = stylesSource.indexOf(".tabbar__status--running");
const reduceAt = stylesSource.indexOf("@media (prefers-reduced-motion: reduce)", runningAt);
assert.ok(reduceAt > runningAt, "the running dot carries an explicit reduce-motion override");
const reduceBlock = blockBody(stylesSource.slice(reduceAt), "@media (prefers-reduced-motion: reduce)");
assert.ok(reduceBlock.includes(".tabbar__status--running"), "the reduce override targets the running dot");
assert.ok(reduceBlock.includes("animation: none"), "reduce-motion stops the breathing");

assert.ok(stylesSource.includes("--tab-status-idle: #4d8df6"), "the idle blue is a token");
assert.ok(stylesSource.includes("--tab-status-running: var(--danger)"), "the running red reuses the danger token");

await act(async () => root.unmount());
console.log("tabbar running dot: ok");
