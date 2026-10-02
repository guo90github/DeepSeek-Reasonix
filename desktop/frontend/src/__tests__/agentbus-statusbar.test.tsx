// Run: tsx src/__tests__/agentbus-statusbar.test.tsx
//
// The status bar's collaboration entry is the one place the board is visible without
// opening the workspace panel, so what it shows without a board and what it shows with
// one are both asserted here. The card rendering itself is covered by
// agentbus-panel.test / agentbus-section.test.

import { JSDOM } from "jsdom";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { AgentBusStatusItem } from "../components/AgentBusStatusItem";
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

// AnchoredPopover positions itself through requestAnimationFrame, so this DOM needs a
// visual environment rather than a bare document.
const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", { pretendToBeVisual: true });
(globalThis as unknown as { document: Document }).document = dom.window.document;
(globalThis as unknown as { window: Window }).window = dom.window as unknown as Window;
(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const offBoard = { enrolled: false, participant: "", board: "", boardDir: "", defaultDir: "state/agentbus/default" };
const onBoard = { enrolled: true, participant: "alice", board: "default", boardDir: "state/agentbus/default", defaultDir: "state/agentbus/default" };
const briefingWith = (signals: number) => ({
  participant: "alice",
  cards: [],
  signals: Array.from({ length: signals }, (_, i) => ({ kind: "stalled", subtree: "root", node: `n${i}`, detail: "needs handoff: lease lapsed" })),
  hidden: 0,
  hiddenCards: 0,
  healthySubtrees: 0,
});

const quiet = dom.window.document.createElement("div");
const first = createRoot(quiet);
await act(async () => {
  first.render(
    <LocaleProvider>
      <AgentBusStatusItem
        loadBriefing={async () => {
          throw new Error("this session is not on a board");
        }}
        loadStatus={async () => offBoard}
      />
    </LocaleProvider>,
  );
});
ok(
  quiet.textContent?.includes("Collaboration") === true || quiet.textContent?.includes("协作") === true,
  "the collaboration entry is on screen even when the session joined nothing",
);
ok(
  quiet.querySelector(".statusbar__collab-badge") === null,
  "no board means no badge: an unenrolled session is not counted as needing attention",
);
await act(async () => {
  first.unmount();
});

const attention = dom.window.document.createElement("div");
const second = createRoot(attention);
await act(async () => {
  second.render(
    <LocaleProvider>
      <AgentBusStatusItem loadBriefing={async () => briefingWith(3)} loadStatus={async () => onBoard} />
    </LocaleProvider>,
  );
});
const badge = attention.querySelector(".statusbar__collab-badge");
ok(badge !== null && badge.textContent === "3", "a board with signals carries their count");
await act(async () => {
  second.unmount();
});

// Clicking the entry has to reach the same surface the workspace panel shows —
// enrolment included — or the entry would be a dead end for a new session.
const opened = dom.window.document.createElement("div");
const third = createRoot(opened);
await act(async () => {
  third.render(
    <LocaleProvider>
      <AgentBusStatusItem loadBriefing={async () => briefingWith(0)} loadStatus={async () => offBoard} join={async () => onBoard} />
    </LocaleProvider>,
  );
});
const chip = opened.querySelector(".statusbar__collab") as HTMLElement | null;
ok(chip !== null, "the entry is a control, not a label");
await act(async () => {
  chip?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
});
// The popover is a portal into document.body, so the surface is asserted there.
const popover = dom.window.document.body.querySelector('[data-anchored-popover="active"]');
ok(popover !== null, "clicking the entry opens a panel anchored to it");
const popoverText = popover?.textContent ?? "";
ok(
  popoverText.includes("Join a board") || popoverText.includes("加入看板"),
  "that panel is the collaboration surface, enrolment and all",
);
await act(async () => {
  third.unmount();
});

process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exit(1);
