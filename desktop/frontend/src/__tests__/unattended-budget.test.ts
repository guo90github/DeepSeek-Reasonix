// Run: tsx src/__tests__/unattended-budget.test.ts

import { JSDOM } from "jsdom";

let passed = 0;
let failed = 0;

function ok(value: unknown, label: string) {
  if (value) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

const dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost/" });
globalThis.window = dom.window as unknown as Window & typeof globalThis;
globalThis.document = dom.window.document;

// What the operator's [agentbus] section says, as the host would report it.
let hostBudget = false;

const { installDesktopHostStub } = await import("./desktopHostStub");
installDesktopHostStub({
  async HeartbeatReloadConfig() {
    return { revision: 1, etag: "etag-1", unattended: true, agentBusBudget: hostBudget, tasks: [] };
  },
  async HeartbeatSaveConfig(update: Record<string, unknown>) {
    return {
      revision: 2,
      etag: "etag-2",
      unattended: Boolean(update.unattended),
      agentBusBudget: hostBudget,
      tasks: [],
    };
  },
});

const { heartbeatUnattended, heartbeatSetUnattended } = await import("../custom/features/heartbeat/heartbeat.bridge");
const { unattendedPresentation } = await import("../custom/features/heartbeat/heartbeat.presentation");

console.log("\nunattended switch: a run with no brake has to say so");

const loaded = await heartbeatUnattended();
ok(loaded.on === true, "the switch reads the master switch from the host");
ok(loaded.budgeted === false, "and carries the host's answer that nothing bounds it");

const unbounded = unattendedPresentation({ on: true, budgeted: false });
ok(unbounded.stateKey === "heartbeat.unattendedOnNoBudget", "an unattended host with no budget labels itself as such");
ok(unbounded.hintKey === "heartbeat.unattendedNoBudgetHint", "and its hint names what is missing");
ok(
  unattendedPresentation({ on: true, budgeted: true }).stateKey === "heartbeat.unattendedOn",
  "a host that did set a ceiling keeps the plain label",
);
ok(
  unattendedPresentation({ on: false, budgeted: false }).stateKey === "heartbeat.unattendedOff",
  "the off state is unchanged: no brake is only news once the switch is on",
);

hostBudget = true;
const saved = await heartbeatSetUnattended(true);
ok(saved.on === true, "turning the switch on answers with the switch");
ok(saved.budgeted === true, "and with the brake the same save reported");

const turnedOff = await heartbeatSetUnattended(false);
ok(turnedOff.on === false && turnedOff.budgeted === true, "turning it off keeps reporting the host's ceiling");

dom.window.close();
console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
