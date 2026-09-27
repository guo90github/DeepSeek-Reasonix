// Run: npx tsx src/__tests__/inbox-room-line-exit.test.ts
//
// "Did my line finish?" must be read from the ending the host records, not guessed
// from a receipt: found=false with an ending means "it left the queue, like this",
// found=false with nothing means "never took it in". An unknown word is passed
// through as itself — the panel does not explain a value it does not know.

import assert from "node:assert/strict";
import { roomLineExit, settledLabel } from "../lib/inboxRoomLineExit";

assert.deepEqual(roomLineExit({ found: true, line: { state: "queued" } }), { kind: "queued", state: "queued" },
  "still in the queue stays a queue answer");

assert.deepEqual(roomLineExit({ found: false, line: { settled: "acknowledged", settledAt: "2026-09-26T00:00:00Z" } }),
  { kind: "settled", settled: "acknowledged", settledAt: "2026-09-26T00:00:00Z" },
  "ran and finished is read from the ending column");
assert.deepEqual(roomLineExit({ found: false, line: { settled: "discarded" } }), { kind: "settled", settled: "discarded" },
  "cancelled is read from the same column");
assert.deepEqual(roomLineExit({ found: false, line: { settled: "deleted" } }), { kind: "settled", settled: "deleted" },
  "removed is read from the same column");

assert.deepEqual(roomLineExit({ found: false }), { kind: "never" }, "nothing at all means never seen");
assert.deepEqual(roomLineExit(undefined), { kind: "never" }, "no answer is not an ending");
assert.deepEqual(roomLineExit({ found: false, line: null }), { kind: "never" }, "an absent line is not an ending");

// The ending is a column, so a missing one must not be invented from found=false.
assert.deepEqual(roomLineExit({ found: false, line: {} }), { kind: "never" },
  "found=false without the ending column is never seen, not a guessed ending");

assert.equal(settledLabel("acknowledged"), "acknowledged（跑完并确认）", "a known word carries its gloss");
assert.equal(settledLabel("discarded"), "discarded（被丢弃）");
assert.equal(settledLabel("deleted"), "deleted（被删掉）");
assert.equal(settledLabel("weird_new_word"), "weird_new_word", "an unknown word is passed through, never explained");
assert.equal(settledLabel("  "), "", "an empty word says nothing");

const unknown = roomLineExit({ found: false, line: { settled: "weird_new_word" } });
assert.deepEqual(unknown, { kind: "settled", settled: "weird_new_word" },
  "an unknown ending is still an ending, reported with its own word");

process.stdout.write("\ninbox room-line exit: all assertions passed\n");
