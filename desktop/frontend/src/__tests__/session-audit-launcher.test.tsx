// Run: tsx src/__tests__/session-audit-launcher.test.tsx
//
// Regression coverage for the whole-session audit trigger and modal:
// - The trigger sits immediately after the new-session button in the tab strip,
//   is disabled when the loaded turns carry no reasoning, and never starts a run
//   in that state.
// - Clicking it opens the modal, and the streamed steps plus the final verdict
//   render from the host event stream (score, cross-turn issues, per-turn rows).

import { JSDOM } from "jsdom";
import React from "react";
import { act } from "react";
import type { Root } from "react-dom/client";
import { TabBar } from "../components/TabBar";
import { SessionAuditLauncher } from "../components/SessionAuditLauncher";
import { LocaleProvider } from "../lib/i18n";
import { __emitMockSessionAuditDone, __emitMockSessionAuditEvent } from "../lib/sessionAuditStream";
import type { SessionAuditTotals } from "../generated/desktopContract.generated";
import type { Item } from "../lib/useController";
import type { TabMeta } from "../lib/types";

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

function eq(actual: unknown, expected: unknown, label: string) {
  if (actual === expected) ok(true, label);
  else ok(false, `${label}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`);
}

function flush(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

function installDom() {
  const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
    pretendToBeVisual: true,
    url: "http://localhost/",
  });
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  globalThis.window = dom.window as unknown as Window & typeof globalThis;
  globalThis.document = dom.window.document;
  Object.defineProperty(globalThis, "navigator", { configurable: true, value: dom.window.navigator });
  globalThis.Node = dom.window.Node;
  globalThis.HTMLElement = dom.window.HTMLElement;
  globalThis.KeyboardEvent = dom.window.KeyboardEvent;
  globalThis.MouseEvent = dom.window.MouseEvent;
  dom.window.HTMLElement.prototype.scrollIntoView = () => {};
  return dom;
}

const dom = installDom();
const { createRoot } = await import("react-dom/client");

const tab = (id: string): TabMeta => ({
  id,
  scope: "project",
  workspaceRoot: "/repo",
  workspaceName: "repo",
  topicId: `topic-${id}`,
  topicTitle: id,
  label: "model",
  ready: true,
  running: false,
  cancellable: false,
  mode: "normal",
  active: true,
  cwd: "/repo",
});

const withReasoning = [
  { kind: "user", id: "u1", text: "问一", historyTurn: 1 },
  { kind: "assistant", id: "a1", text: "答一", reasoning: "思考一", streaming: false },
  { kind: "user", id: "u2", text: "问二", historyTurn: 2 },
  { kind: "assistant", id: "a2", text: "答二", reasoning: "思考二", streaming: false },
] as unknown as Item[];

const withoutReasoning = [
  { kind: "user", id: "u1", text: "问一" },
  { kind: "assistant", id: "a1", text: "答一", reasoning: "", streaming: false },
] as unknown as Item[];

function renderTabBar(items: readonly Item[]): Root {
  const root = createRoot(document.getElementById("root")!);
  act(() => {
    root.render(
      <LocaleProvider>
        <TabBar
          tabs={[tab("t1")]}
          activeTabId="t1"
          onTabChange={() => {}}
          onTabClose={() => {}}
          onTabsClose={() => {}}
          onTabsReorder={() => {}}
          onNewTab={() => {}}
          sessionAudit={<SessionAuditLauncher items={items} turnBase={0} />}
        />
      </LocaleProvider>,
    );
  });
  return root;
}

// ── Placement and gating ──────────────────────────────────────────────────────

let root = renderTabBar(withReasoning);
const rowButtons = [...document.querySelectorAll<HTMLButtonElement>(".tabbar button")];
const newIndex = rowButtons.findIndex((node) => node.classList.contains("tabbar__new"));
eq(newIndex >= 0, true, "the session row carries the new-session button");
eq(rowButtons[newIndex + 1]?.classList.contains("tabbar__audit"), true, "the audit trigger sits immediately after it");
const auditButton = document.querySelector<HTMLButtonElement>(".tabbar__audit");
eq(auditButton?.disabled, false, "a session with reasoning offers the audit");

await act(async () => { root.unmount(); });
root = renderTabBar(withoutReasoning);
eq(document.querySelector<HTMLButtonElement>(".tabbar__audit")?.disabled, true, "a reasoning-free session disables the audit");
await act(async () => { root.unmount(); });

// ── Trigger opens the modal; the streamed verdict renders ─────────────────────

root = renderTabBar(withReasoning);
eq(document.querySelector(".reasonix-audit-dialog"), null, "the modal is closed until it is asked for");
await act(async () => {
  document.querySelector<HTMLButtonElement>(".tabbar__audit")?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true }));
  await flush();
});
ok(Boolean(document.querySelector(".reasonix-audit-dialog")), "clicking the trigger opens the session audit modal");

const totals: SessionAuditTotals = {
  audited: true,
  elapsedMs: 1200,
  score: 0.72,
  trend: "degrading",
  explanation: "轮 2 与前轮结论冲突",
  issues: [{ type: "cross_turn_contradiction", turns: [1, 2], note: "轮 2 推翻了轮 1", quote: "7×8=54" }],
  turns: [
    { turn: 1, score: 0.8, contradiction: 0, factualError: 0, invalidInference: 1, redundancy: 0, instructionDrift: 0, omission: 0, issues: 1, conclusion: "先做 A", priorConflict: "", explanation: "一次无效推理", findings: [{ type: "invalid_inference", quote: "7×8=54" }], truncated: false },
    { turn: 2, score: 0.6, contradiction: 1, factualError: 0, invalidInference: 0, redundancy: 0, instructionDrift: 0, omission: 0, issues: 1, conclusion: "改用 B", priorConflict: "与轮 1 相反", explanation: "矛盾", findings: [], truncated: true },
  ],
  turnCount: 2,
  segmentCount: 1,
  evalTokens: 900,
  evalCost: 0.012,
};

await act(async () => {
  __emitMockSessionAuditEvent("t1", { stage: "segment", index: 1, total: 2, kind: "request", systemPrompt: "prompt", input: "[轮 1]\n思考一", turnFrom: 1, turnTo: 2 });
  __emitMockSessionAuditEvent("t1", { stage: "segment", index: 1, total: 2, kind: "text", text: "{\"turns\":[]}" });
  __emitMockSessionAuditEvent("t1", { stage: "segment", index: 1, total: 2, kind: "step_done" });
  __emitMockSessionAuditDone("t1", totals);
  await flush();
});

eq(document.querySelector<HTMLElement>(".audit-result__score")?.textContent, "0.72", "the session score renders");
eq(document.querySelectorAll(".session-audit__issue").length, 1, "the cross-turn issue renders");
eq(document.querySelectorAll(".session-audit__turn").length, 2, "every audited turn renders a score row");
ok(
  (document.querySelector(".session-audit__turn .audit-finding__quote")?.textContent ?? "").includes("7×8=54"),
  "a flagged turn keeps its quoted excerpt",
);
ok(
  (document.querySelector(".session-audit__issue-turns")?.textContent ?? "").includes("#1"),
  "the cross-turn issue names the turns it spans",
);

await act(async () => { root.unmount(); });

if (failed > 0) {
  process.stdout.write(`\nsession audit launcher: ${failed} failed\n`);
  process.exit(1);
}
process.stdout.write(`session audit launcher: ${passed} passed\n`);
