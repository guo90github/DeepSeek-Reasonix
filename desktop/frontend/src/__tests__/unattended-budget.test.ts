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
// 渲染断言里用 act()：不声明这个环境标记，React 会每帧告警（同目录其他测试也都设了它）。
(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// What the operator's [agentbus] section says, as the host would report it.
let hostBudget = false;

// 看门狗条目自己的开关（tab 条里那枚「崩溃恢复」）：status 里的 policy 就是"要不要它"。
let watchdogPolicy = true;
let watchdogRegistered = true;

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
  async WatchdogStatus() {
    return { supported: true, policy: watchdogPolicy, registered: watchdogRegistered, platform: "windows" };
  },
  async SetWatchdogEnabled(on: boolean) {
    watchdogPolicy = on;
    watchdogRegistered = on;
    return { supported: true, policy: on, registered: on, platform: "windows" };
  },
});

const { heartbeatUnattended, heartbeatSetUnattended, heartbeatWatchdogStatus, heartbeatWatchdogSet } = await import("../custom/features/heartbeat/heartbeat.bridge");
const { unattendedPresentation, watchdogHold, watchdogTogglePresentation } = await import("../custom/features/heartbeat/heartbeat.presentation");

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
// 用户实际会问的那个组合：无人值守开着、而崩溃恢复被显式关掉。总开关必须自己说出来 ——
// 否则"无人值守"就是一句没人兑现的承诺（宿主崩了没人把它拉回来）。
ok(watchdogHold({ policy: false, registered: false }) !== "", "an opted-out entry is a hold too");
const recoveryOff = unattendedPresentation({ on: true, budgeted: true, watchdog: { policy: false, registered: false } });
ok(recoveryOff.stateKey === "heartbeat.unattendedWatchdogHeld", "unattended on + crash recovery off must say so on the switch itself");


// tab 条里紧挨总开关的那枚「崩溃恢复」：它是**条目自己的**开关，与总开关互相独立。
// 关掉它的代价（宿主崩了没人拉回来、手机够不到）必须写在提示里；而且它必须**记住**。
console.log("\ncrash recovery switch: its own switch, and turning it off has to stick");
const entryLoaded = await heartbeatWatchdogStatus();
ok(entryLoaded?.policy === true, "the switch reads the entry's own policy from the host");
ok(watchdogTogglePresentation(entryLoaded).stateKey === "heartbeat.watchdogOn", "a wanted entry reads as on");
ok(watchdogTogglePresentation({ policy: true, registered: true }).hintKey === "heartbeat.watchdogOnHint", "with the hint saying what it buys");
const wantButMissing = watchdogTogglePresentation({ policy: true, registered: false, note: "not registered" });
ok(wantButMissing.on === true && wantButMissing.hintKey === "heartbeat.watchdogHeldHint", "an entry that is wanted but not there says so");
ok(wantButMissing.hintParams?.reason === "not registered", "and carries the machine's own reason");
const optedOutEntry = watchdogTogglePresentation({ policy: false, registered: false });
ok(optedOutEntry.on === false && optedOutEntry.stateKey === "heartbeat.watchdogOff", "an opted-out entry reads as off");
ok(optedOutEntry.hintKey === "heartbeat.watchdogOffHint", "and its hint names the price");
const flippedEntry = await heartbeatWatchdogSet(false);
ok(flippedEntry?.policy === false, "turning it off reaches the host and comes back read");
ok((await heartbeatWatchdogStatus())?.policy === false, "and it is still off when the status is read again");


// 渲染断言：这枚开关必须真的画出来、并真的接线到宿主 —— 组件的 JSX/接线只有跑一次才算验过。
const React = await import("react");
const { createRoot } = await import("react-dom/client");
const { LocaleProvider } = await import("../lib/i18n");
const { WatchdogToggle } = await import("../custom/features/heartbeat/WatchdogToggle");
watchdogPolicy = true;
watchdogRegistered = true;
const container = document.createElement("div");
document.body.appendChild(container);
const root = createRoot(container);
await React.act(async () => {
  root.render(React.createElement(LocaleProvider, null, React.createElement(WatchdogToggle)));
});
const renderedSwitch = container.querySelector('[role="switch"]') as HTMLElement | null;
ok(renderedSwitch !== null, "the crash recovery switch renders next to the unattended one");
ok(renderedSwitch?.getAttribute("aria-checked") === "true", "and a wanted entry reads as on");
await React.act(async () => {
  renderedSwitch?.click();
});
ok(
  (container.querySelector('[role="switch"]') as HTMLElement | null)?.getAttribute("aria-checked") === "false",
  "clicking it flips the switch",
);
ok((await heartbeatWatchdogStatus())?.policy === false, "and the click really reached the host");
await React.act(async () => {
  root.unmount();
});
container.remove();

dom.window.close();
console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
