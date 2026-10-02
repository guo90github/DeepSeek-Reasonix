// Run: tsx src/__tests__/agentbus-section.test.tsx
//
// The section owns two things only: it fetches, and it distinguishes "cannot read"
// from "nothing wrong". The card rendering itself is covered by agentbus-panel.test.

import { JSDOM } from "jsdom";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { WorkspaceAgentBusSection } from "../components/WorkspaceAgentBusSection";
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

const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>");
(globalThis as unknown as { document: Document }).document = dom.window.document;
(globalThis as unknown as { window: Window }).window = dom.window as unknown as Window;
(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// The section renders a board only for a session that joined one, so the enrolment
// is stated here rather than implied: what an unenrolled session sees is its own
// state, asserted further down.
const enrolledStatus = {
  enrolled: true,
  participant: "alice",
  board: "default",
  boardDir: "state/agentbus/default",
  defaultDir: "state/agentbus/default",
};

async function renderSection(node: HTMLElement) {
  const root = createRoot(node);
  await act(async () => {
    root.render(
      <LocaleProvider>
        <WorkspaceAgentBusSection loadStatus={async () => enrolledStatus} />
      </LocaleProvider>,
    );
  });
  return root;
}

const quiet = dom.window.document.createElement("div");
const first = await renderSection(quiet);
ok(
  quiet.textContent !== null && quiet.textContent.length > 0,
  "a readable briefing renders something visible",
);
ok(
  quiet.textContent?.includes("unavailable") !== true,
  "a readable briefing is not reported as unavailable",
);
await act(async () => {
  first.unmount();
});

const broken = dom.window.document.createElement("div");
const second = createRoot(broken);
await act(async () => {
  second.render(
    <LocaleProvider>
      <WorkspaceAgentBusSection
        loadStatus={async () => enrolledStatus}
        load={async () => {
          throw new Error("host unreachable");
        }}
      />
    </LocaleProvider>,
  );
});
ok(
  broken.textContent?.includes("unavailable") === true ||
    broken.textContent?.includes("取不到") === true,
  "an unreadable briefing says so instead of drawing an empty board",
);
await act(async () => {
  second.unmount();
});

// Opening a step reads that step's record and shows it here: the panel stays a pure
// component, and the section owns this second fetch like it owns the first.
const briefed = dom.window.document.createElement("div");
const withSignal = {
  participant: "alice",
  cards: [
    { subtree: "root", nodes: 2, atWork: 1, parked: 0, done: 1, worst: "disputed", signals: 1, orphans: 0, stalled: 0, disputed: 1 },
  ],
  signals: [{ kind: "disputed", subtree: "root", node: "publish", detail: "under deliberation" }],
  hidden: 0,
  hiddenCards: 0,
  healthySubtrees: 0,
};
const stepDetail = (node: string) => ({
  node,
  title: "",
  state: "contested",
  owner: "",
  ready: false,
  deps: ["signing-key"],
  noProgress: 0,
  refutations: [],
  authorizations: [{ actor: "operator", reason: "the key is installed", seq: 3 }],
  deliberating: false,
  verdict: "",
});
const third = createRoot(briefed);
await act(async () => {
  third.render(
    <LocaleProvider>
      <WorkspaceAgentBusSection loadStatus={async () => enrolledStatus} load={async () => withSignal} loadDetail={async (node) => stepDetail(node)} />
    </LocaleProvider>,
  );
});
const signalButton = Array.from(briefed.querySelectorAll("button")).find((button) => button.textContent === "publish");
ok(signalButton !== undefined, "a signal with a step behind it is clickable");
await act(async () => {
  signalButton?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
});
ok(
  briefed.textContent?.includes("operator") === true && briefed.textContent?.includes("the key is installed") === true,
  "opening a signal reads that step's record and shows who authorized it",
);
await act(async () => {
  third.unmount();
});

// A record that cannot be read says so, instead of looking like an empty step.
const failing = dom.window.document.createElement("div");
const fourth = createRoot(failing);
await act(async () => {
  fourth.render(
    <LocaleProvider>
      <WorkspaceAgentBusSection
        loadStatus={async () => enrolledStatus}
        load={async () => withSignal}
        loadDetail={async () => {
          throw new Error("host unreachable");
        }}
      />
    </LocaleProvider>,
  );
});
const failButton = Array.from(failing.querySelectorAll("button")).find((button) => button.textContent === "publish");
await act(async () => {
  failButton?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
});
ok(
  failing.textContent?.includes("Could not read") === true || failing.textContent?.includes("读不到") === true,
  "a step whose record cannot be read says so instead of drawing an empty one",
);
await act(async () => {
  fourth.unmount();
});

// Enrolment is the entry the user was missing: a session that joined nothing has to
// show that, name where joining would put it, and actually enrol when asked.
const offBoard = dom.window.document.createElement("div");
const fifth = createRoot(offBoard);
await act(async () => {
  fifth.render(
    <LocaleProvider>
      <WorkspaceAgentBusSection
        loadStatus={async () => ({ enrolled: false, participant: "", board: "", boardDir: "", defaultDir: "state/agentbus/default" })}
        join={async () => enrolledStatus}
        load={async () => withSignal}
      />
    </LocaleProvider>,
  );
});
ok(
  offBoard.textContent?.includes("not on a board") === true || offBoard.textContent?.includes("未入列") === true,
  "a session that joined nothing says so",
);
ok(
  offBoard.textContent?.includes("state/agentbus/default") === true,
  "the entry names the board joining would put the session on",
);
const joinButton = Array.from(offBoard.querySelectorAll("button")).find(
  (button) => button.textContent === "Join a board" || button.textContent === "加入看板",
);
ok(joinButton !== undefined, "the section offers a control that enrols the session");
await act(async () => {
  joinButton?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
});
ok(
  offBoard.textContent?.includes("publish") === true,
  "joining shows the board right away instead of leaving the entry as it was",
);
ok(
  offBoard.textContent?.includes("Leave the board") === true || offBoard.textContent?.includes("离开看板") === true,
  "an enrolled session can leave the board again",
);
await act(async () => {
  fifth.unmount();
});

// Enrolled but without an identity yet (a session with no path): saying so is the
// difference between "joining did nothing" and "it takes effect".
const pending = dom.window.document.createElement("div");
const sixth = createRoot(pending);
await act(async () => {
  sixth.render(
    <LocaleProvider>
      <WorkspaceAgentBusSection
        loadStatus={async () => ({ enrolled: true, participant: "", board: "", boardDir: "", defaultDir: "state/agentbus/default" })}
        load={async () => withSignal}
      />
    </LocaleProvider>,
  );
});
ok(
  pending.textContent?.includes("no identity") === true || pending.textContent?.includes("没有身份") === true,
  "a board without an identity yet is explained rather than drawn as an empty board",
);
await act(async () => {
  sixth.unmount();
});

process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exit(1);
