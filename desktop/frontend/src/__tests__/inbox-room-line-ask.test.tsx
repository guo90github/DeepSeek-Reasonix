// Run: npx tsx src/__tests__/inbox-room-line-ask.test.tsx
//
// The panel asks the host what became of one room line, on demand, and prints the
// host's own answer: still queued, left the queue like this, never taken in, or —
// when the question cannot be asked — that it could not ask. Silence and a made-up
// ending are the two failures this pins.

import assert from "node:assert/strict";
import { act } from "react";
import { installDom, installBridgeApp, renderComposer } from "./composerInboxHarness";

const flush = () => new Promise<void>(resolve => setTimeout(resolve, 0));

async function ask(answer: () => Promise<unknown>): Promise<string> {
  installDom("zh-CN");
  installBridgeApp({
    InboxSnapshot: async () => ({
      revision: 1, paused: false, recovered: false, itemsCount: 1, bytes: 0, maxItems: 64, maxBytes: 1024,
      items: [{
        id: "ibx-1", preview: "房间 #43 点名了你", state: "queued", intent: "steer", source: "push",
        room: { seq: 43, from: "chatside", topic: 6 },
      }],
    }),
    ReadInboxItem: async () => ({ displayText: "房间 #43 点名了你", submitText: "", rawText: "" }),
    InboxRoomLine: async () => answer(),
  });
  await renderComposer({ inboxSessionPath: "session-a" });
  await act(async () => { await flush(); await flush(); });

  const button = [...document.querySelectorAll("button")].find((node) => node.textContent === "问一下宿主");
  assert.ok(button, `the ask button should be offered on a room line, got: ${document.body.textContent}`);
  await act(async () => {
    button!.dispatchEvent(new window.MouseEvent("click", { bubbles: true, cancelable: true }));
    await flush();
    await flush();
  });
  for (let attempt = 0; attempt < 20; attempt += 1) {
    const text = document.querySelector(".composer-guidance-item__roomline")?.textContent || "";
    if (text) return text;
    await act(async () => { await flush(); });
  }
  process.stdout.write(`  NO ANSWER. body=${document.body.textContent}
`);
  return "";
}

const queued = await ask(async () => ({ found: true, line: { state: "queued" } }));
assert.ok(queued.startsWith("宿主说：还在它的队列里"), `still queued reads as an answer, got: ${queued}`);
// An answer without the hour it was asked is an old answer pretending to be new.
assert.match(queued, / · \d{1,2}:\d{2}:\d{2} 问的$/, `the answer must carry when it was asked, got: ${queued}`);

const settled = await ask(async () => ({ found: false, line: { settled: "acknowledged", settledAt: "2026-09-26T00:00:00Z" } }));
assert.ok(settled.startsWith("宿主说：已经离开队列——acknowledged（跑完并确认）"),
  `the ending is printed with the host's own word, got: ${settled}`);

const cancelled = await ask(async () => ({ found: false, line: { settled: "discarded" } }));
assert.ok(cancelled.startsWith("宿主说：已经离开队列——discarded（被丢弃）"), `got: ${cancelled}`);

const never = await ask(async () => ({ found: false }));
assert.ok(never.startsWith("宿主说：没收过这一条"), `got: ${never}`);

// A question that could not be asked must say exactly that — not silence, and
// certainly not an ending nobody reported.
const unreachable = await ask(async () => { throw new Error("host unreachable"); });
assert.ok(unreachable.startsWith("问不到宿主，未知"), `got: ${unreachable}`);

process.stdout.write("\ninbox room-line ask: all assertions passed\n");
