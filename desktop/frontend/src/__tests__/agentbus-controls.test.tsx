// Run: tsx src/__tests__/agentbus-controls.test.tsx
//
// The panel's human actions have to be the model's verbs, not a parallel vocabulary:
// what the form sends is asserted field by field, and a board refusal has to reach the
// user rather than looking like a successful write.

import { JSDOM } from "jsdom";

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

// Globals first, React after: React reads document/window at load time, and the root
// must live in the document for its event listeners to see dispatched input events.
const dom = new JSDOM('<!doctype html><html><body><div id="root"></div></body></html>', {
  url: "http://localhost/",
  pretendToBeVisual: true,
});
Object.assign(globalThis, {
  window: dom.window,
  document: dom.window.document,
  Element: dom.window.Element,
  HTMLElement: dom.window.HTMLElement,
  HTMLInputElement: dom.window.HTMLInputElement,
  Node: dom.window.Node,
  localStorage: dom.window.localStorage,
  IS_REACT_ACT_ENVIRONMENT: true,
});
Object.defineProperty(globalThis, "navigator", { configurable: true, value: dom.window.navigator });
const { default: React, act } = await import("react");
const { createRoot } = await import("react-dom/client");
const { AgentBusControls, agentBusArgsFor, parseChildren } = await import("../components/AgentBusControls");
const { LocaleProvider } = await import("../lib/i18n");

const fields = {
  node: "", reason: "", ref: "", depID: "", depTitle: "", children: "", steps: "", outcome: "done", reproducedBy: "",
};

ok(
  parseChildren("a:first, b")[1].id === "b" && parseChildren("a:first, b")[0].title === "first",
  "one line of steps parses into ids with optional titles",
);
ok(parseChildren(" , ").length === 0, "an empty step line creates nothing");

const assertArgs = agentBusArgsFor("assert", { ...fields, node: " build ", reason: "why", ref: "go test ./..." });
ok(
  assertArgs.action === "assert" && assertArgs.node === "build" && assertArgs.reason === "why" && assertArgs.ref === "go test ./...",
  "a recorded step carries its node, summary and evidence",
);
const requireArgs = agentBusArgsFor("require", { ...fields, node: "build", depID: "key", depTitle: "signing key" });
ok(
  requireArgs.dep !== null && requireArgs.dep !== undefined && requireArgs.dep.id === "key" && requireArgs.dep.title === "signing key",
  "a dependency is sent as the dependency the board expects",
);
const claimArgs = agentBusArgsFor("claim", { ...fields, node: "build", steps: "3" });
ok(claimArgs.steps === 3, "a claim declares how many steps the work may take");
const decideArgs = agentBusArgsFor("decide", { ...fields, node: "build", outcome: "blocked", ref: "ci/7", reproducedBy: "bob" });
ok(
  decideArgs.outcome === "blocked" && decideArgs.reproducedBy === "bob" && decideArgs.ref === "ci/7",
  "a delivery carries its outcome, evidence and re-runner",
);

const container = dom.window.document.createElement("div");
dom.window.document.body.appendChild(container);
const root = createRoot(container);
const sent: unknown[] = [];
await act(async () => {
  root.render(
    <LocaleProvider>
      <AgentBusControls
        apply={async (args) => {
          sent.push(args);
          return "assert on \"build\" recorded at seq 4";
        }}
      />
    </LocaleProvider>,
  );
});
const text = () => container.textContent ?? "";
ok(
  text().includes("Board actions") || text().includes("看板操作"),
  "the board actions are offered to the human, not only to the model",
);
ok(
  container.querySelector('select[aria-label="Action"], select[aria-label="动作"]') !== null,
  "the action is chosen from the board's own verbs",
);

const inputs = Array.from(container.querySelectorAll("input"));
// The locale in a bare jsdom is not fixed, so labels are matched in both languages
// rather than depending on which dictionary loaded.
const byLabel = (...labels: string[]) =>
  inputs.find((input) => labels.includes(input.getAttribute("aria-label") ?? "")) as HTMLInputElement | undefined;
await act(async () => {
  const set = (input: HTMLInputElement | undefined, value: string) => {
    if (!input) return;
    const setter = Object.getOwnPropertyDescriptor(dom.window.HTMLInputElement.prototype, "value")?.set;
    setter?.call(input, value);
    // React 19 reads a text change from the input event; the change event is kept for
    // the same reason older harnesses needed it, so the harness works either way.
    input.dispatchEvent(new dom.window.Event("input", { bubbles: true }));
    input.dispatchEvent(new dom.window.Event("change", { bubbles: true }));
  };
  set(byLabel("Step", "步骤"), "build");
  set(byLabel("Evidence (command, path, URL)", "证据（命令、路径、URL）"), "go test ./...");
});
await act(async () => {
  container.querySelector("form")?.dispatchEvent(new dom.window.Event("submit", { bubbles: true, cancelable: true }));
});
ok(sent.length === 1, "submitting sends exactly one board action");
const sentArgs = sent[0] as { action: string; node: string; ref: string };
ok(
  sentArgs.action === "assert" && sentArgs.node === "build" && sentArgs.ref === "go test ./...",
  `what the human typed reaches the board as the tool's arguments (sent ${JSON.stringify(sent[0])})`,
);
ok(
  text().includes("recorded at seq 4"),
  "the board's own answer is shown, so the human sees what the board decided",
);
await act(async () => {
  root.unmount();
});

// A refusal is the board's answer: it must be visible, and it must not look like a write.
const refused = dom.window.document.createElement("div");
const second = createRoot(refused);
await act(async () => {
  second.render(
    <LocaleProvider>
      <AgentBusControls
        apply={async () => {
          throw new Error("refused (missing_evidence): pass evidence with a ref another participant can check");
        }}
      />
    </LocaleProvider>,
  );
});
await act(async () => {
  refused.querySelector("form")?.dispatchEvent(new dom.window.Event("submit", { bubbles: true, cancelable: true }));
});
ok(
  (refused.textContent ?? "").includes("missing_evidence"),
  "a refusal reaches the human with the board's reason instead of looking like a write",
);
await act(async () => {
  second.unmount();
});

process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exit(1);
