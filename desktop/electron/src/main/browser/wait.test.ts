// Run: node --import tsx --test src/main/browser/wait.test.ts
import assert from "node:assert/strict";
import { test } from "node:test";

import { FakeViewFactory, silentLog, type FakeGuestView } from "./fakeGuestViews.js";
import { BrowserSurfaceManager, WAIT_POLL_MS, type BrowserTab } from "./surfaceManager.js";

// The clock and the poll hook are injected, so waiting is asserted without
// real time: hooks[n] runs after the n-th poll.
function setup() {
  let now = 0;
  let polls = 0;
  const hooks = new Map<number, () => void>();
  const manager = new BrowserSurfaceManager({
    views: new FakeViewFactory(),
    contentSize: () => null,
    onTakeover() {},
    onCrash() {},
    log: silentLog,
    now: () => now,
    openWaitMs: 5,
    sleep: async (ms) => {
      polls += 1;
      now += ms;
      hooks.get(polls)?.();
    },
  });
  const viewOf = (tab: BrowserTab) => tab.view as FakeGuestView;
  return { manager, viewOf, pollCount: () => polls, onPoll: (n: number, fn: () => void) => hooks.set(n, fn) };
}

test("wait returns at once when the tab is already settled", async () => {
  const s = setup();
  const tab = await s.manager.open("https://a.test", { taskId: "t", temporary: false });
  assert.deepEqual(await s.manager.wait(tab.id, "", false, 5000), { found: true, elapsedMs: 0 });
  assert.equal(s.pollCount(), 0, "no polling was needed");
});

test("wait polls the loading flag instead of guessing a sleep", async () => {
  const s = setup();
  const tab = await s.manager.open("https://a.test", { taskId: "t", temporary: false });
  s.viewOf(tab).fire().onStartLoading();
  s.onPoll(1, () => s.viewOf(tab).fire().onStopLoading());
  assert.deepEqual(await s.manager.wait(tab.id, "", false, 5000), { found: true, elapsedMs: WAIT_POLL_MS });
});

test("wait gives up at the timeout and says how long it waited", async () => {
  const s = setup();
  const tab = await s.manager.open("https://a.test", { taskId: "t", temporary: false });
  s.viewOf(tab).fire().onStartLoading();
  const res = await s.manager.wait(tab.id, "", false, 300);
  assert.equal(res.found, false);
  assert.equal(res.elapsedMs, 300);
});

test("wait finds text through the page script and honours gone", async () => {
  const s = setup();
  const tab = await s.manager.open("https://a.test", { taskId: "t", temporary: false });
  let present = false;
  s.viewOf(tab).page.run = () => present;
  s.onPoll(1, () => { present = true; });
  assert.equal((await s.manager.wait(tab.id, "盈亏表", false, 5000)).found, true);
  present = true;
  assert.equal((await s.manager.wait(tab.id, "盈亏表", true, 5000)).found, false, "gone still waits while the text is there");
});

test("a page script failure counts as text absent instead of throwing", async () => {
  const s = setup();
  const tab = await s.manager.open("https://a.test", { taskId: "t", temporary: false });
  s.viewOf(tab).page.run = () => { throw new Error("no frame"); };
  assert.equal((await s.manager.wait(tab.id, "anything", false, 150)).found, false);
});
