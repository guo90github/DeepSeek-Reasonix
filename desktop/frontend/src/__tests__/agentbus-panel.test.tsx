// Run: tsx src/__tests__/agentbus-panel.test.tsx

import { JSDOM } from "jsdom";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { AgentBusPanel, type AgentBusBriefingView } from "../components/AgentBusPanel";
import { LocaleProvider } from "../lib/i18n";

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

function render(view: AgentBusBriefingView, onOpenNode?: (node: string) => void): string {
  return renderToStaticMarkup(
    <LocaleProvider>
      <AgentBusPanel view={view} onOpenNode={onOpenNode} />
    </LocaleProvider>,
  );
}

const attention: AgentBusBriefingView = {
  participant: "alice",
  cards: [
    {
      subtree: "root",
      nodes: 3,
      atWork: 1,
      parked: 1,
      done: 1,
      worst: "stalled",
      signals: 2,
      orphans: 0,
      stalled: 1,
      disputed: 1,
    },
    {
      subtree: "hub",
      nodes: 1,
      atWork: 0,
      parked: 0,
      done: 0,
      worst: "orphan",
      signals: 1,
      orphans: 1,
      stalled: 0,
      disputed: 0,
    },
  ],
  signals: [
    { kind: "stalled", subtree: "root", node: "mid", detail: "needs handoff: lease lapsed" },
    { kind: "disputed", subtree: "root", node: "design", detail: "under deliberation" },
    { kind: "orphan", subtree: "hub", node: "dep1", detail: "depends on gone, which does not exist" },
  ],
  hidden: 0,
  hiddenCards: 0,
  healthySubtrees: 4,
};

const html = render(attention);

ok(html.indexOf("alice") !== -1, "shows the identity this session speaks as");
ok(html.indexOf("hub") < html.indexOf("root"), "an orphan subtree outranks a stall on the first screen");
ok(html.indexOf("dep1") !== -1 && html.indexOf("depends on gone") !== -1, "every signal carries its address and its reason");
ok(html.indexOf("1 at work, 1 parked, 1/3 done") !== -1, "a card folds its counts");
ok(html.indexOf("Nothing needs attention") === -1, "a card list must not claim everything is fine");
ok(html.indexOf("did not fit") === -1, "hidden counts stay out of the way when nothing was hidden");

const quiet = render({ ...attention, cards: [], signals: [], healthySubtrees: 4 });
ok(quiet.indexOf("4 subtrees") !== -1, "a quiet board says how much is quiet");
ok(quiet.indexOf("data-worst") === -1, "a quiet board draws no card");

const truncated = render({ ...attention, hidden: 3, hiddenCards: 2 });
ok(truncated.indexOf("2 more subtrees") !== -1 && truncated.indexOf("3 more signals") !== -1, "what did not fit is counted, not hidden");

const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>");
(globalThis as unknown as { document: Document }).document = dom.window.document;
(globalThis as unknown as { window: Window }).window = dom.window as unknown as Window;
(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
const opened: string[] = [];
const container = dom.window.document.getElementById("root") as HTMLElement;
const root = createRoot(container);
await act(async () => {
  root.render(
    <LocaleProvider>
      <AgentBusPanel view={attention} onOpenNode={(node) => opened.push(node)} />
    </LocaleProvider>,
  );
});
const buttons = container.querySelectorAll("button");
ok(buttons.length === 3, "each signal is a drill-in control once a jump target exists");
const orphanButton = Array.from(buttons).find((button) => button.textContent === "dep1");
ok(orphanButton !== undefined, "the drill-in control is labelled with its node");
await act(async () => {
  orphanButton?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
});
ok(opened.length === 1 && opened[0] === "dep1", "clicking a signal opens the node it names");
await act(async () => {
  root.unmount();
});

process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exit(1);
