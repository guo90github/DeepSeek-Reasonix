// Run: npx tsx src/__tests__/inbox-wait-chip.test.tsx
//
// A queued wake that sits for a minute must say so on the chip: "已等 1 分 5 秒".
// It answers "how long" only — the gate and the host's own sentence still say why
// it is not running yet. With no wait reported, the chip says nothing extra.

import assert from "node:assert/strict";
import { act } from "react";
import { installDom, installBridgeApp, renderComposer } from "./composerInboxHarness";
import { inboxWaitParts } from "../lib/inboxWait";
import { guidanceFromInboxSnapshot } from "../lib/composerInboxQueue";

const flush = () => new Promise<void>(resolve => setTimeout(resolve, 0));

assert.equal(inboxWaitParts(undefined), null, "no wait means nothing to show");
assert.equal(inboxWaitParts(0), null, "0ms is not a wait");
assert.equal(inboxWaitParts(999), null, "under a second is not worth a label");
assert.deepEqual(inboxWaitParts(1000), { minutes: 0, seconds: 1 });
assert.deepEqual(inboxWaitParts(65000), { minutes: 1, seconds: 5 });
assert.deepEqual(inboxWaitParts(3600000), { minutes: 60, seconds: 0 });

const carried = guidanceFromInboxSnapshot({
  items: [{ id: "ibx-1", preview: "房间 #43 点名了你", state: "queued", waitMs: 65000 }],
} as never);
assert.equal(carried[0]?.waitMs, 65000, "the snapshot's wait reaches the chip model");
const absent = guidanceFromInboxSnapshot({
  items: [{ id: "ibx-2", preview: "房间 #44 点名了你", state: "queued" }],
} as never);
assert.equal(absent[0]?.waitMs, undefined, "no wait reported stays absent");

const dom = installDom("zh-CN");
void dom;
installBridgeApp({
  InboxSnapshot: async () => ({
    revision: 1, paused: false, recovered: false, itemsCount: 1, bytes: 0, maxItems: 64, maxBytes: 1024,
    items: [{
      id: "ibx-1", preview: "房间 #43 点名了你", state: "queued", intent: "steer", source: "push",
      waitMs: 65000, room: { seq: 43, from: "chatside", topic: 6 },
    }],
  }),
  ReadInboxItem: async () => ({ displayText: "房间 #43 点名了你", submitText: "", rawText: "" }),
});
await renderComposer({ inboxSessionPath: "session-a" });
await act(async () => { await flush(); await flush(); });

const text = document.body.textContent || "";
assert.ok(text.includes("已等 1 分 5 秒"), `chip should say how long it waited, got: ${text}`);

const waitChip = document.querySelector(".composer-guidance-item__wait");
assert.ok(waitChip, "the wait is its own element, not glued into the preview text");
assert.equal((document.querySelector(".composer-guidance-item__text")?.textContent || "").includes("已等"), false,
  "the preview text stays the mention itself");

process.stdout.write("\ninbox wait chip: all assertions passed\n");
