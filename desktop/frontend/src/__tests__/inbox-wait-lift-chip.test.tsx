// Run: npx tsx src/__tests__/inbox-wait-lift-chip.test.tsx
//
// A line the host refused reads the same whether or not another wake could still
// lift it — unless the chip says. The wire carries the flag only when it is true,
// so absence is read as "not liftable" *only* next to a host sentence: a bare gate
// is the queue's name, not a refusal, and nothing is claimed about it.

import assert from "node:assert/strict";
import { act } from "react";
import { installDom, installBridgeApp, renderComposer } from "./composerInboxHarness";
import { guidanceFromInboxSnapshot } from "../lib/composerInboxQueue";

const flush = () => new Promise<void>(resolve => setTimeout(resolve, 0));

const liftable = guidanceFromInboxSnapshot({
  items: [{ id: "a", preview: "p", state: "queued", waitReason: "忙", waitResumable: true }],
} as never);
assert.equal(liftable[0]?.waitResumable, true, "a true flag reaches the chip model");
const silent = guidanceFromInboxSnapshot({
  items: [{ id: "b", preview: "p", state: "queued", waitReason: "忙" }],
} as never);
assert.equal(silent[0]?.waitResumable, undefined, "an absent flag stays absent, never invented");

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

const resumable = await renderWithItem({
  id: "ibx-1", preview: "房间 #43 点名了你", state: "queued", intent: "steer",
  waitReason: "这个会话在桌面端已经没有标签页在托管它", waitResumable: true,
});
assert.ok(resumable.includes("这个会话在桌面端已经没有标签页在托管它"), `want the sentence, got: ${resumable}`);
assert.ok(resumable.includes("再叫一次能提起来"), `want the liftable reading, got: ${resumable}`);

const notResumable = await renderWithItem({
  id: "ibx-2", preview: "房间 #44 点名了你", state: "queued", intent: "steer",
  waitReason: "这个会话在桌面端已经没有标签页在托管它",
});
assert.ok(notResumable.includes("再叫一次也提不起来"), `want the unliftable reading, got: ${notResumable}`);

const gateOnly = await renderWithItem({
  id: "ibx-3", preview: "房间 #45 点名了你", state: "queued", intent: "steer",
  waitGate: "dispatch",
});
assert.ok(gateOnly.includes("卡在 dispatch"), `want the gate name, got: ${gateOnly}`);
assert.equal(gateOnly.includes("提不起来"), false, "a bare gate refuses nothing, so nothing is claimed");
assert.equal(gateOnly.includes("提起来"), false, "a bare gate refuses nothing, so nothing is claimed");

process.stdout.write("\ninbox wait-lift chip: all assertions passed\n");
