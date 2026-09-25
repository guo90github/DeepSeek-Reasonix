// Run: tsx src/__tests__/host-recovery-hidden.test.ts
//
// Auto Guard guidance is model-facing policy, not a user-side steer: the
// transcript keeps it out of both panes on purpose. The drop used to be an
// unwritten intent; this pins it, so widening the prefix list or losing the
// guard fails here instead of quietly painting a policy line as the user.

import { partitionTurnItems, type Item } from "../lib/transcriptRows";
import { isHostRecoveryGuidance } from "../lib/hostRecoverySteer";

let passed = 0;
let failed = 0;

function ok(cond: boolean, label: string) {
  if (cond) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

function eq<T>(a: T, b: T, label: string) {
  if (a === b) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}: expected ${JSON.stringify(b)}, got ${JSON.stringify(a)}\n`);
    failed += 1;
  }
}

function user(id: string, text: string): Item {
  return { kind: "user", id, text, submissionId: `s-${id}`, createdAt: 1000 };
}

function steerNotice(id: string, text: string): Item {
  return { kind: "notice", id, level: "info", text: `↪ ${text}` };
}

console.log("\nhost recovery guidance stays out of the transcript");

ok(
  isHostRecoveryGuidance("↪ A tool failed. Use read-only diagnosis as needed, then retry."),
  "recovery prefix after the steer marker is recognized",
);
ok(
  isHostRecoveryGuidance("The tool timed out or hit a transient execution limit."),
  "recovery prefix bare is recognized",
);
ok(!isHostRecoveryGuidance("↪ 改用 Pillow 10 验证"), "an ordinary steer is not recovery guidance");

const mixed = partitionTurnItems([
  user("u1", "第一问"),
  steerNotice("n1", "A tool failed. Use read-only diagnosis as needed, then retry."),
  steerNotice("n2", "改用 Pillow 10 验证"),
]);
const processIds = mixed.flatMap((segment) => segment.processItems).map((item) => item.id);
const outsideIds = mixed.flatMap((segment) => segment.outsideItems).map((item) => item.id);

eq(processIds.includes("n1"), false, "recovery guidance is not process material");
eq(outsideIds.includes("n1"), false, "recovery guidance is not conversation material either");
eq(outsideIds.includes("n2"), true, "an ordinary steer still reaches the conversation channel");
eq(processIds.includes("n2"), false, "an ordinary steer does not fall into the process fold");

process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exit(1);
