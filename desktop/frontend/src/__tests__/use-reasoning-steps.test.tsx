// Run: tsx src/__tests__/use-reasoning-steps.test.tsx

import { JSDOM } from "jsdom";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { useReasoningSteps, type ReasoningStepsResult } from "../lib/useReasoningSteps";

let failed = 0;
let passed = 0;

function ok(value: boolean, label: string) {
  if (value) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
  pretendToBeVisual: true,
  url: "http://localhost/",
});
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
globalThis.window = dom.window as unknown as Window & typeof globalThis;
globalThis.document = dom.window.document;
globalThis.requestAnimationFrame = dom.window.requestAnimationFrame.bind(dom.window);
globalThis.cancelAnimationFrame = dom.window.cancelAnimationFrame.bind(dom.window);

const realNow = Date.now;
let fakeNow = 1000;
Date.now = () => fakeNow;

let last: ReasoningStepsResult | null = null;

function Harness({ text, complete, total }: { text: string; complete: boolean; total?: number }) {
  last = useReasoningSteps(text, { reasoningComplete: complete, totalDurationMs: total });
  return <pre>{text}</pre>;
}

const rootEl = document.getElementById("root");
if (!rootEl) throw new Error("missing root");
const root = createRoot(rootEl);

async function render(text: string, complete: boolean, total?: number) {
  await act(async () => {
    root.render(<Harness text={text} complete={complete} total={total} />);
  });
}

console.log("\nuse reasoning steps");

// History restore: a completed turn mounts with no streaming observation, so
// per-step durations are unknowable and must stay absent.
await render("Step 1: A\nbody\nStep 2: B\nStep 3: C\n", true, 2800);
ok(last?.steps.length === 3, "history restore segments a completed turn");
ok(last?.steps.every((s) => s.status === "complete"), "history restore marks every step complete");
ok(last?.steps.every((s) => s.durationMs === undefined), "history restore shows no fabricated durations");

// Streaming timeline: markers stamp as they complete, durations are the gaps.
await render("", false);
ok(last?.detected === 0 && last?.steps.length === 0, "empty stream detects no steps");

fakeNow = 2000;
await render("Step 1: A\n", false);
ok(last?.detected === 1, "the first marker counts even below the two-step view threshold");
ok(last?.steps.length === 0, "one marker keeps the flat view");

fakeNow = 3000;
await render("Step 1: A\nStep 2: B\n", false);
ok(last?.steps.length === 2, "the second marker activates the step view");
ok(last?.steps[0]?.status === "complete" && last?.steps[1]?.status === "streaming", "all but the last step complete while streaming");
ok(last?.steps[0]?.durationMs === 1000, "step 1 duration is the gap to the next marker");
ok(last?.steps[1]?.durationMs === undefined, "the active step has no duration yet");

fakeNow = 4000;
await render("Step 1: A\nStep 2: B\nStep 3: C\n", false);
ok(last?.steps[1]?.status === "complete" && last?.steps[2]?.status === "streaming", "steps complete one by one as markers land");
ok(last?.steps[1]?.durationMs === 1000, "step 2 duration is the gap to step 3");

fakeNow = 5000;
await render("Step 1: A\nStep 2: B\nStep 3: C\n", true, 2800);
ok(last?.steps.every((s) => s.status === "complete"), "completion settles every step");
ok(last?.steps[2]?.durationMs === 800, "the final step duration = total − elapsed head (2800 − 2000)");

// A retry/discard shrinks the marker list and restarts the clock.
fakeNow = 6000;
await render("", false);
ok(last?.detected === 0, "a discard resets detection");
fakeNow = 7000;
await render("Step 1: A\nStep 2: B\n", false);
ok(last?.steps.length === 2 && last?.steps[0]?.durationMs === 0, "a restarted stream restamps both markers at once");

await act(async () => {
  root.unmount();
});
Date.now = realNow;
dom.window.close();

console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
