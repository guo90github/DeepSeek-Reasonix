// Run: npx tsx src/__tests__/inbox-wait-why-chip.test.tsx
//
// The chip says why a queued item is not running yet, and it says it only in words
// somebody actually provided: the host's own sentence verbatim when there is one,
// the gate's own name when there is only a gate. No wait fields ⇒ no extra text.

import assert from "node:assert/strict";
import { act } from "react";
import { installDom, installBridgeApp, renderComposer } from "./composerInboxHarness";
import { guidanceFromInboxSnapshot } from "../lib/composerInboxQueue";

const flush = () => new Promise<void>(resolve => setTimeout(resolve, 0));

const carried = guidanceFromInboxSnapshot({
  items: [{
    id: "ibx-1", preview: "房间 #43 点名了你", state: "queued",
    waitMs: 65000, waitGate: "dispatch", waitReason: "这个会话在桌面端已经没有标签页在托管它",
  }],
} as never);
assert.equal(carried[0]?.waitReason, "这个会话在桌面端已经没有标签页在托管它", "the host's sentence reaches the chip model");
assert.equal(carried[0]?.waitGate, "dispatch", "the gate reaches the chip model");
const blank = guidanceFromInboxSnapshot({
  items: [{ id: "ibx-2", preview: "房间 #44 点名了你", state: "queued", waitGate: "  ", waitReason: "" }],
} as never);
assert.equal(blank[0]?.waitGate, undefined, "a blank gate is not a gate");
assert.equal(blank[0]?.waitReason, undefined, "a blank reason is not a reason");

async function renderWithItem(item: Record<string, unknown>): Promise<string> {
  installDom("zh-CN");
  installBridgeApp({
    InboxSnapshot: async () => ({
      revision: 1, paused: false, recovered: false, itemsCount: 1, bytes: 0, maxItems: 64, maxBytes: 1024,
      items: [item],
    }),
    ReadInboxItem: async () => ({ displayText: String(item.preview || ""), submitText: "", rawText: "" }),
  });
  await renderComposer({ inboxSessionPath: "session-a" });
  await act(async () => { await flush(); await flush(); });
  return document.querySelector(".composer-guidance-item__wait")?.textContent || "";
}

const withReason = await renderWithItem({
  id: "ibx-1", preview: "房间 #43 点名了你", state: "queued", intent: "steer", source: "push",
  waitMs: 65000, waitGate: "dispatch", waitReason: "这个会话在桌面端已经没有标签页在托管它",
});
assert.ok(withReason.includes("已等 1 分 5 秒"), `want how long, got: ${withReason}`);
assert.ok(withReason.includes("这个会话在桌面端已经没有标签页在托管它"), `want the host's sentence verbatim, got: ${withReason}`);
assert.equal(withReason.includes("卡在"), false, "the host's sentence replaces the bare gate, it does not add to it");

const gateOnly = await renderWithItem({
  id: "ibx-2", preview: "房间 #44 点名了你", state: "queued", intent: "steer", source: "push",
  waitMs: 2000, waitGate: "dispatch",
});
assert.ok(gateOnly.includes("已等 2 秒"), `want how long, got: ${gateOnly}`);
assert.ok(gateOnly.includes("卡在 dispatch"), `want the gate's own name, got: ${gateOnly}`);

const neither = await renderWithItem({
  id: "ibx-3", preview: "房间 #45 点名了你", state: "queued", intent: "steer", source: "push",
});
assert.equal(neither, "", `no wait fields must add no text, got: ${neither}`);
assert.equal(document.querySelector(".composer-guidance-item__wait"), null, "no wait element at all");

process.stdout.write("\ninbox wait-why chip: all assertions passed\n");
