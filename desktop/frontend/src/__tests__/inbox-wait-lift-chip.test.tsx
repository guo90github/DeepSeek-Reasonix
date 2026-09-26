// Run: npx tsx src/__tests__/inbox-wait-lift-chip.test.tsx
//
// Whether the host answered for a line is its own column now, so the chip reads the
// lift from that column instead of inferring it from a sentence. An old host that
// sends no column claims nothing: the frontend must not turn a missing field into
// an assertion about a line nobody spoke about.

import assert from "node:assert/strict";
import { act } from "react";
import { installDom, installBridgeApp, renderComposer } from "./composerInboxHarness";
import { guidanceFromInboxSnapshot } from "../lib/composerInboxQueue";

const flush = () => new Promise<void>(resolve => setTimeout(resolve, 0));

const carried = guidanceFromInboxSnapshot({
  items: [{ id: "a", preview: "p", state: "queued", waitReason: "忙", waitRefused: true, waitResumable: true }],
} as never);
assert.equal(carried[0]?.waitRefused, true, "the refusal column reaches the chip model");
assert.equal(carried[0]?.waitResumable, true, "the lift column reaches the chip model");
const absent = guidanceFromInboxSnapshot({
  items: [{ id: "b", preview: "p", state: "queued", waitReason: "忙" }],
} as never);
assert.equal(absent[0]?.waitRefused, undefined, "an absent column stays absent, never invented");

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
  waitReason: "这个会话在桌面端已经没有标签页在托管它", waitRefused: true, waitResumable: true,
});
assert.ok(resumable.includes("这个会话在桌面端已经没有标签页在托管它"), `want the sentence, got: ${resumable}`);
assert.ok(resumable.includes("再叫一次能提起来"), `want the liftable reading, got: ${resumable}`);

const notResumable = await renderWithItem({
  id: "ibx-2", preview: "房间 #44 点名了你", state: "queued", intent: "steer",
  waitReason: "这个会话在桌面端已经没有标签页在托管它", waitRefused: true,
});
assert.ok(notResumable.includes("再叫一次也提不起来"), `want the unliftable reading, got: ${notResumable}`);

// The column, not the sentence, is what the chip reads: an old host's answer that
// carries a sentence but no column says nothing about lifting.
const noColumn = await renderWithItem({
  id: "ibx-3", preview: "房间 #45 点名了你", state: "queued", intent: "steer",
  waitReason: "这个会话在桌面端已经没有标签页在托管它",
});
assert.ok(noColumn.includes("这个会话在桌面端已经没有标签页在托管它"), `want the sentence still shown, got: ${noColumn}`);
assert.equal(noColumn.includes("提起来"), false, `no column means no claim about lifting, got: ${noColumn}`);

const gateOnly = await renderWithItem({
  id: "ibx-4", preview: "房间 #46 点名了你", state: "queued", intent: "steer",
  waitGate: "dispatch",
});
assert.ok(gateOnly.includes("卡在 dispatch"), `want the gate name, got: ${gateOnly}`);
assert.equal(gateOnly.includes("提起来"), false, "a bare gate refuses nothing, so nothing is claimed");

process.stdout.write("\ninbox wait-lift chip: all assertions passed\n");
