// Run: tsx src/__tests__/split-divider.test.ts

import { resolveAutoSplitProcessWidth, snapSplitWidth, stepSplitWidth } from "../lib/splitDivider";

let passed = 0;
let failed = 0;

function eq<T>(actual: T, expected: T, label: string) {
  if (actual === expected) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}: expected ${String(expected)}, got ${String(actual)}\n`);
    failed += 1;
  }
}

console.log("\nsplit divider geometry");

// Snap: pulls to the nearest point inside the threshold, leaves values outside.
eq(
  snapSplitWidth({ next: 0.42, panelWidth: 1000, thresholdPx: 40, snapPoints: [0.4, 0.5, 0.6] }),
  0.4,
  "snap pulls a value inside the threshold to the nearest point",
);
eq(
  snapSplitWidth({ next: 0.48, panelWidth: 1000, thresholdPx: 40, snapPoints: [0.4, 0.5, 0.6] }),
  0.5,
  "snap picks the nearest point, not the first",
);
eq(
  snapSplitWidth({ next: 0.55, panelWidth: 1000, thresholdPx: 40, snapPoints: [0.4, 0.5, 0.6] }),
  0.55,
  "snap leaves a value outside every threshold untouched",
);
eq(
  snapSplitWidth({ next: 0.6, panelWidth: 1000, thresholdPx: 40, snapPoints: [0.4, 0.5, 0.6] }),
  0.6,
  "snap recognizes an exact hit",
);
eq(
  snapSplitWidth({ next: 0.5, panelWidth: 0, thresholdPx: 40, snapPoints: [0.4, 0.5, 0.6] }),
  0.5,
  "snap is a no-op on an unmeasurable panel (zero width)",
);

// Threshold scales with panel width: 40px on 1000px is 4%; on 500px it is 8%.
eq(
  snapSplitWidth({ next: 0.55, panelWidth: 500, thresholdPx: 40, snapPoints: [0.4, 0.5, 0.6] }),
  0.6,
  "snap threshold is relative to panel width",
);

// Keyboard stepping: clamps to the legal range, honors direction.
eq(
  stepSplitWidth({ current: 0.5, direction: 1, step: 0.05, min: 0.4, max: 0.6 }),
  0.55,
  "step advances by the given delta",
);
eq(
  stepSplitWidth({ current: 0.5, direction: -1, step: 0.05, min: 0.4, max: 0.6 }),
  0.45,
  "step backs off by the given delta",
);
eq(
  stepSplitWidth({ current: 0.58, direction: 1, step: 0.05, min: 0.4, max: 0.6 }),
  0.6,
  "step clamps at the max",
);
eq(
  stepSplitWidth({ current: 0.42, direction: -1, step: 0.05, min: 0.4, max: 0.6 }),
  0.4,
  "step clamps at the min",
);

// Auto mode: widens only while reasoning is streaming, falls back to idle
// when reasoning is absent or completed.
eq(
  resolveAutoSplitProcessWidth({ hasReasoning: true, reasoningComplete: false, activeWidth: 0.6, idleWidth: 0.45 }),
  0.6,
  "auto widens while reasoning is streaming",
);
eq(
  resolveAutoSplitProcessWidth({ hasReasoning: true, reasoningComplete: true, activeWidth: 0.6, idleWidth: 0.45 }),
  0.45,
  "auto falls back to idle once reasoning completes",
);
eq(
  resolveAutoSplitProcessWidth({ hasReasoning: false, reasoningComplete: false, activeWidth: 0.6, idleWidth: 0.45 }),
  0.45,
  "auto stays idle when there is no reasoning at all",
);

console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
