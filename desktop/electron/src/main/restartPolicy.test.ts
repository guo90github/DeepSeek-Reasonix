import assert from "node:assert/strict";
import { test } from "node:test";
import { unattendedRestartDelayMs } from "./restartPolicy.js";

test("unattended restarts back off exponentially", () => {
  assert.equal(unattendedRestartDelayMs(0), 1_000);
  assert.equal(unattendedRestartDelayMs(1), 2_000);
  assert.equal(unattendedRestartDelayMs(2), 4_000);
  assert.equal(unattendedRestartDelayMs(5), 32_000);
});

test("the delay never exceeds the cap, however long the crash loop runs", () => {
  assert.equal(unattendedRestartDelayMs(6), 60_000);
  assert.equal(unattendedRestartDelayMs(50), 60_000);
  assert.equal(unattendedRestartDelayMs(Number.MAX_SAFE_INTEGER), 60_000);
});

test("a nonsensical attempt count still waits, never spins", () => {
  assert.equal(unattendedRestartDelayMs(-3), 1_000);
  assert.equal(unattendedRestartDelayMs(Number.NaN), 1_000);
  assert.equal(unattendedRestartDelayMs(1.7), 2_000);
});
