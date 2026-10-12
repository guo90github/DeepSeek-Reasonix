// Run: node --import tsx --test src/main/browser/logs.test.ts
import assert from "node:assert/strict";
import { test } from "node:test";

import { silentLog } from "./fakeGuestViews.js";
import type { GuestDebugger, GuestPage } from "./guestView.js";
import { CONSOLE_LIMIT, TabLogs } from "./logs.js";

class FakeDebugger {
  attached = false;
  attachFails = false;
  commands: string[] = [];
  private listeners: Array<(event: unknown, method: string, params: unknown) => void> = [];

  isAttached(): boolean { return this.attached; }
  attach(): void { if (this.attachFails) throw new Error("no debugger"); this.attached = true; }
  detach(): void { this.attached = false; }
  async sendCommand(method: string): Promise<unknown> { this.commands.push(method); return {}; }
  on(_event: "message", listener: (event: unknown, method: string, params: unknown) => void): unknown {
    this.listeners.push(listener);
    return this;
  }
  removeListener(): unknown { return this; }
  emit(method: string, params: unknown): void { for (const listener of this.listeners) listener({}, method, params); }
}

function setup() {
  const debugger_ = new FakeDebugger();
  const page = { debugger: debugger_ as unknown as GuestDebugger } as unknown as GuestPage;
  return { page, debugger: debugger_, logs: new TabLogs({ log: silentLog, now: () => 7 }) };
}

test("attach enables the two domains once and records console output", () => {
  const s = setup();
  s.logs.attach(s.page);
  s.logs.attach(s.page);
  assert.deepEqual(s.debugger.commands, ["Runtime.enable", "Network.enable"], "a second attach changes nothing");
  s.debugger.emit("Runtime.consoleAPICalled", { type: "error", args: [{ value: "boom" }, { value: 42 }] });
  s.debugger.emit("Runtime.exceptionThrown", { exceptionDetails: { exception: { description: "TypeError: x is not a function\n  at a" } } });
  assert.deepEqual(s.logs.consoleRecords().map((entry) => `${entry.level}:${entry.text}`), [
    "error:boom 42",
    "error:TypeError: x is not a function",
  ]);
});

test("network pairs a response with its request and records failures", () => {
  const s = setup();
  s.logs.attach(s.page);
  s.debugger.emit("Network.requestWillBeSent", { requestId: "1", request: { method: "POST", url: "https://a.test/api" } });
  s.debugger.emit("Network.responseReceived", { requestId: "1", response: { status: 500, mimeType: "application/json" } });
  s.debugger.emit("Network.requestWillBeSent", { requestId: "2", request: { method: "GET", url: "https://a.test/x" } });
  s.debugger.emit("Network.loadingFailed", { requestId: "2", errorText: "net::ERR_FAILED" });
  assert.deepEqual(s.logs.networkRecords(), [
    { method: "POST", url: "https://a.test/api", status: 500, mime: "application/json", failed: "" },
    { method: "GET", url: "https://a.test/x", status: 0, mime: "", failed: "net::ERR_FAILED" },
  ]);
});

test("the console ring buffer keeps the newest entries only", () => {
  const s = setup();
  s.logs.attach(s.page);
  for (let i = 0; i < CONSOLE_LIMIT + 5; i++) s.debugger.emit("Runtime.consoleAPICalled", { type: "log", args: [{ value: `n${i}` }] });
  const records = s.logs.consoleRecords();
  assert.equal(records.length, CONSOLE_LIMIT);
  assert.equal(records[0].text, "n5", "the oldest entries were dropped, not the newest");
});

test("a debugger that cannot attach reports no records instead of throwing", () => {
  const s = setup();
  s.debugger.attachFails = true;
  s.logs.attach(s.page);
  assert.deepEqual(s.logs.consoleRecords(), []);
  assert.deepEqual(s.logs.networkRecords(), []);
});

test("clear empties both buffers", () => {
  const s = setup();
  s.logs.attach(s.page);
  s.debugger.emit("Runtime.consoleAPICalled", { type: "log", args: [{ value: "x" }] });
  s.debugger.emit("Network.requestWillBeSent", { requestId: "1", request: { method: "GET", url: "https://a.test/x" } });
  s.logs.clear();
  assert.deepEqual(s.logs.consoleRecords(), []);
  assert.deepEqual(s.logs.networkRecords(), []);
});
