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
const { unattendedPresentation, watchdogHold } = await import("../custom/features/heartbeat/heartbeat.presentation");

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


// 开关是"人的意图"，driving 是"本次启动的真实状态"：崩溃降级时开关仍为开（故意的），
// 面板必须能把这两件事分开说，否则标签就是一句没人兑现的承诺。
const heldSwitch = unattendedPresentation({ on: true, budgeted: true, driving: false, hold: "the previous launches crashed" });
ok(heldSwitch.stateKey === "heartbeat.unattendedHeld", "a switch that is on while nothing drives says so");
ok(heldSwitch.hintParams?.reason === "the previous launches crashed", "and the hint carries the reason the host gave");
ok(unattendedPresentation({ on: true, budgeted: true }).stateKey === "heartbeat.unattendedOn", "a healthy launch still reads as on");


// 开关承诺"会持续推进"，可"崩了有人把宿主拉回来"这半依赖 OS 条目：条目没生效时
// 面板必须说出来，否则就是第二处"对用户说反话"。
ok(watchdogHold(null) === "", "no reading means no claim either way");
ok(watchdogHold({ registered: true }) === "", "a registered entry is not a hold");
ok(watchdogHold({ registered: true, lastRunAt: "" }) === "", "never having run yet is normal, not a hold");
ok(watchdogHold({ registered: false }) !== "", "an unregistered entry is a hold");
ok(watchdogHold({ registered: false, lastError: "schtasks create: access denied" }) === "schtasks create: access denied", "the machine's own reason wins when there is one");
ok(watchdogHold({ supported: false, registered: false }) === "", "a platform without the entry makes no promise to break");
const heldWatchdog = unattendedPresentation({ on: true, budgeted: true, watchdog: { registered: false, note: "not registered" } });
ok(heldWatchdog.stateKey === "heartbeat.unattendedWatchdogHeld", "the switch says crash recovery is off");
ok(heldWatchdog.hintParams?.reason === "not registered", "and why");
const bothHeld = unattendedPresentation({ on: true, budgeted: true, driving: false, hold: "the previous launches crashed", watchdog: { registered: false } });
ok(bothHeld.stateKey === "heartbeat.unattendedHeld", "not driving outranks a missing watchdog entry");

dom.window.close();
console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
