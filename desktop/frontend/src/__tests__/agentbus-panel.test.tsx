// Run: tsx src/__tests__/agentbus-panel.test.tsx

import { JSDOM } from "jsdom";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { AgentBusPanel, type AgentBusBriefingView, type AgentBusNodeDetailView } from "../components/AgentBusPanel";
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

function renderStep(detail: AgentBusNodeDetailView): string {
  return renderToStaticMarkup(
    <LocaleProvider>
      <AgentBusPanel view={emptyBoard} detail={detail} onCloseDetail={() => {}} />
    </LocaleProvider>,
  );
}

const emptyBoard: AgentBusBriefingView = {
  participant: "alice",
  cards: [],
  signals: [],
  hidden: 0,
  hiddenCards: 0,
  healthySubtrees: 0,
};

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
    { kind: "budget", subtree: "", node: "", detail: "a ceiling refused 2 claim(s): node 2" },
    { kind: "rate_limited", subtree: "", node: "", detail: "rode out 3 rate limit(s) this process: deepseek 3" },
  ],
  hidden: 0,
  hiddenCards: 0,
  healthySubtrees: 4,
};

const html = render(attention);

ok(html.indexOf("alice") !== -1, "shows the identity this session speaks as");
ok(html.indexOf("hub") < html.indexOf("root"), "an orphan subtree outranks a stall on the first screen");
ok(html.indexOf("dep1") !== -1 && html.indexOf("depends on gone") !== -1, "every signal carries its address and its reason");
ok(html.indexOf("parked for budget") !== -1, "the host's own refusal row carries the panel's own label");
ok(html.indexOf("rate limited") !== -1 && html.indexOf("deepseek 3") !== -1, "the host's rate-limit row names the lane");
ok(html.indexOf("1 at work, 1 parked, 1/3 done") !== -1, "a card folds its counts");
ok(html.indexOf("Nothing needs attention") === -1, "a card list must not claim everything is fine");
ok(html.indexOf("did not fit") === -1, "hidden counts stay out of the way when nothing was hidden");

const quiet = render({ ...attention, cards: [], signals: [], healthySubtrees: 4 });
ok(quiet.indexOf("4 subtrees") !== -1, "a quiet board says how much is quiet");
ok(quiet.indexOf("data-worst") === -1, "a quiet board draws no card");

const truncated = render({ ...attention, hidden: 3, hiddenCards: 2 });
ok(truncated.indexOf("2 more subtrees") !== -1 && truncated.indexOf("3 more signals") !== -1, "what did not fit is counted, not hidden");

// The host marshals a Go nil slice as null, so an empty board arrives as
// cards/signals: null. That shipped as a crash in v0.0.0-dev.101 ("cards is not
// iterable"), so a null list must now read as an empty board.
const nullLists = render({ ...attention, cards: null, signals: null });
ok(nullLists.indexOf("data-worst") === -1, "a null card list draws no card instead of crashing");
const nullStep = renderStep({
  node: "publish", title: "publish", state: "open", owner: "", ready: true,
  deps: null, noProgress: 0, refutations: null, authorizations: null,
  deliberating: false, verdict: "",
});
ok(
  nullStep.indexOf('data-node="publish"') !== -1,
  "a step whose lists arrive as null still renders instead of crashing",
);

// The footer is for the signals that belong to no subtree (a budget refusal, a rate
// limit). An empty or subtree-only list must not draw an empty footer.
const hostOnly = render({ ...attention, signals: attention.signals.filter((signal) => !signal.subtree) });
const subtreeOnly = render({ ...attention, signals: attention.signals.filter((signal) => signal.subtree) });
ok(
  hostOnly.indexOf("agentbus-panel__host-signals") !== -1 &&
    subtreeOnly.indexOf("agentbus-panel__host-signals") === -1,
  "the host footer carries the subtree-less signals and only those",
);
ok(quiet.indexOf("agentbus-panel__host-signals") === -1, "a quiet board draws no host footer");
ok(
  hostOnly.indexOf('data-kind="budget"') !== -1 && hostOnly.indexOf('data-kind="rate_limited"') !== -1,
  "a budget refusal and a rate limit are addressed as themselves, not as a dispute",
);
const unknownKind = render({ ...attention, signals: [{ kind: "mystery", subtree: "", node: "", detail: "no idea" }] });
ok(
  unknownKind.indexOf('data-kind="mystery"') !== -1 && unknownKind.indexOf("no idea") !== -1,
  "a kind the panel does not know still renders its row instead of dropping it",
);

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

// The step's own record: what it is, what it waits for, what was disputed and who
// authorized it. Authorizations come from the op log, so the panel shows them without
// the board's state having to carry them (§13.8).
const stepHtml = renderStep({
  node: "publish",
  title: "publish",
  state: "contested",
  owner: "",
  ready: false,
  deps: ["signing-key"],
  noProgress: 0,
  refutations: [{ actor: "skeptic", reason: "the test does not cover the migration" }],
  authorizations: [{ actor: "operator", reason: "the key is installed and scoped", seq: 12 }],
  deliberating: true,
  verdict: "escalate",
});
ok(stepHtml.indexOf('data-node="publish"') !== -1, "the opened step is named");
ok(stepHtml.indexOf('data-state="contested"') !== -1, "the step shows the state the record gives it");
ok(stepHtml.indexOf("signing-key") !== -1, "the step shows what it waits for");
ok(
  stepHtml.indexOf("skeptic") !== -1 && stepHtml.indexOf("does not cover the migration") !== -1,
  "a challenge and the reason it was made are both shown",
);
ok(
  stepHtml.indexOf("operator") !== -1 && stepHtml.indexOf("installed and scoped") !== -1,
  "who authorized the step is shown, with why",
);
ok(stepHtml.indexOf("escalate") !== -1, "a question that is still open is part of the step");
ok(
  stepHtml.includes("Close") || stepHtml.includes("关闭"),
  "an opened step can be closed again",
);

const unreadable = renderToStaticMarkup(
  <LocaleProvider>
    <AgentBusPanel view={emptyBoard} detailNotice="Could not read gone" />
  </LocaleProvider>,
);
ok(
  unreadable.indexOf("Could not read gone") !== -1,
  "a step that could not be read says so instead of drawing an empty one",
);

// Leftover work is history, not a call to act: it folds away by default, stays countable by hand,
// and a leftover card offers to retire it — which the panel hands back to the host (2026-10-05).
const leftoverBoard: AgentBusBriefingView = {
  participant: "alice",
  cards: [
    { subtree: "ab-old-decision", nodes: 2, atWork: 0, parked: 0, done: 1, worst: "stalled", signals: 1, orphans: 0, stalled: 1, disputed: 0, leftover: true },
    { subtree: "live-work", nodes: 1, atWork: 0, parked: 0, done: 0, worst: "disputed", signals: 1, orphans: 0, stalled: 0, disputed: 1 },
  ],
  signals: [],
  hidden: 0,
  hiddenCards: 0,
  healthySubtrees: 0,
};
const leftoverHtml = renderToStaticMarkup(
  <LocaleProvider>
    <AgentBusPanel view={leftoverBoard} onRetire={() => {}} />
  </LocaleProvider>,
);
const detailsAt = leftoverHtml.indexOf("<details");
ok(detailsAt !== -1, "the leftover work is folded into a details block");
ok(
  detailsAt !== -1 && detailsAt < leftoverHtml.indexOf("ab-old-decision"),
  "the leftover card sits inside that block rather than on the first screen",
);
ok(
  leftoverHtml.indexOf("live-work") !== -1 && (detailsAt === -1 || leftoverHtml.indexOf("live-work") < detailsAt),
  "a card whose participants are here stays on the first screen",
);
ok(
  leftoverHtml.indexOf("agentbus-panel__card-action") !== -1,
  "a leftover card offers to retire it in one click",
);
ok(
  leftoverHtml.indexOf("agentbus-panel__chip") !== -1,
  "the card's state reads as a chip rather than as loose text",
);

// Acting on a step happens where it is read: each node row offers the verbs a person most wants, and
// they hand the verb plus the node to the form instead of making anyone copy an id (2026-10-05).
const withNode = {
  participant: "alice",
  cards: [{ subtree: "build", nodes: 2, atWork: 0, parked: 0, done: 1, worst: "stalled", signals: 1, orphans: 0, stalled: 1, disputed: 0 }],
  signals: [{ kind: "stalled", subtree: "build", node: "build-step", detail: "no progress recorded 2 times" }],
  hidden: 0,
  hiddenCards: 0,
  healthySubtrees: 0,
};
const verbsHtml = renderToStaticMarkup(
  <LocaleProvider>
    <AgentBusPanel view={withNode} onVerb={() => {}} />
  </LocaleProvider>,
);
ok(verbsHtml.indexOf("agentbus-panel__quick") !== -1, "a node row offers the verbs a person needs");
ok(
  ["Claim", "认领", "認領"].some((label) => verbsHtml.includes(label)) &&
    ["Deliver", "交付"].some((label) => verbsHtml.includes(label)) &&
    ["Challenge", "质疑", "質疑"].some((label) => verbsHtml.includes(label)),
  "the three verbs are named on the row",
);
const noVerbsHtml = renderToStaticMarkup(
  <LocaleProvider>
    <AgentBusPanel view={withNode} />
  </LocaleProvider>,
);
ok(
  noVerbsHtml.indexOf("agentbus-panel__quick") === -1,
  "and a host that offers no verbs draws no buttons",
);

// Who is here with me, as chips: the roster answers the question the departed names cannot, and the
// reading session is marked as such (2026-10-05).
const rosterHtml = renderToStaticMarkup(
  <LocaleProvider>
    <AgentBusPanel
      view={{
        ...emptyBoard,
        members: [
          { participant: "p-self", label: "本会话", self: true },
          { participant: "p-peer", label: "另一个会话", self: false },
        ],
      }}
    />
  </LocaleProvider>,
);
ok(rosterHtml.indexOf("agentbus-panel__members") !== -1, "the panel says who is here with me");
ok(
  rosterHtml.includes("本会话") && rosterHtml.includes("另一个会话"),
  "the roster names each session the way it shows itself",
);
ok(
  rosterHtml.includes("（你）") || rosterHtml.includes("(you)"),
  "and marks the session doing the reading",
);
ok(
  renderToStaticMarkup(
    <LocaleProvider>
      <AgentBusPanel view={emptyBoard} />
    </LocaleProvider>,
  ).indexOf("agentbus-panel__members") === -1,
  "a host that sends no roster draws no roster line",
);

process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exit(1);
