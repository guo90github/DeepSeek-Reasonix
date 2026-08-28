// Run: tsx src/__tests__/message-settle-hardening.test.ts
//
// Covers the message-settle hardening: a Message event may only settle the
// active turn's own assistant item (the bubble turn_started opened, inside the
// newest turn's region). A Message with no such target is stale delivery —
// missed turn_started, replay/duplicate — and must be dropped without creating
// a new item (which buildTurnModels would attribute to a newer turn's
// question) or touching the live stream / currentAssistant of the newer turn.

import { initialState, reducer } from "../lib/useController";
import { buildTurnModels, NO_LIVE } from "../lib/transcriptRows";
import { conversationPaneTurns, processPaneTurns } from "../lib/transcriptPanes";
import type { WireEvent } from "../lib/types";

let passed = 0;
let failed = 0;

function eq(a: unknown, b: unknown, label: string) {
  if (a === b) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}: expected ${JSON.stringify(b)}, got ${JSON.stringify(a)}\n`);
    failed += 1;
  }
}

function ev(s: typeof initialState, e: WireEvent) {
  return reducer(s, { type: "event", e });
}

function user(s: typeof initialState, text: string, seq: number) {
  return reducer(s, { type: "user", text, seq, submissionId: `s-${seq}` });
}

function started(s: typeof initialState, turnId: string) {
  return ev(s, { kind: "turn_started", turnId });
}

function delta(s: typeof initialState, kind: "text" | "reasoning", text: string) {
  return reducer(s, { type: "stream_batch", segments: [{ kind, delta: text }] });
}

console.log("\nmessage settle hardening");

// 1. Normal settle: the anchor bubble opened by turn_started absorbs the
//    message; live and currentAssistant are cleared.
{
  let s = user(initialState, "第一问", 1);
  s = started(s, "t1");
  eq(s.items.some((it) => it.kind === "assistant" && it.id === "a:t1"), true, "turn_started opens the anchor bubble");
  s = delta(s, "reasoning", "思考中");
  s = delta(s, "text", "第一答");
  const done = ev(s, { kind: "message", text: "第一答", reasoning: "思考中" });
  const assistant = done.items.find((it): it is Extract<typeof it, { kind: "assistant" }> => it.kind === "assistant" && it.id === "a:t1");
  eq(assistant?.text, "第一答", "message settles the anchor bubble text");
  eq(assistant?.streaming, false, "message stops streaming");
  eq(done.live, undefined, "message clears the live stream");
  eq(done.currentAssistant, undefined, "message clears currentAssistant");
}

// 2. Stale message after a newer turn's question: no bubble exists in the
//    newest region, so the message is dropped without appending an item under
//    the newer question (the old code appended a new assistant at the end).
{
  let s = user(initialState, "第一问", 1);
  s = started(s, "t1");
  s = ev(s, { kind: "message", text: "第一答" });
  s = ev(s, { kind: "turn_done", turnId: "t1" });
  s = user(s, "第二问", 2);
  const itemsBefore = s.items.length;
  const seqBefore = s.seq;
  const dropped = ev(s, { kind: "message", text: "第一答迟到" });
  eq(dropped.items.length, itemsBefore, "stale message does not append an assistant item");
  eq(dropped.seq, seqBefore, "stale message does not advance seq");
  eq(dropped.items[dropped.items.length - 1]?.kind, "user", "newest item stays the second question");
  eq(dropped.live, s.live, "stale message does not touch live");
  eq(dropped.currentAssistant, s.currentAssistant, "stale message does not touch currentAssistant");
}

// 3. Empty message removes the empty pre-bubble opened by turn_started.
{
  let s = user(initialState, "空回合", 3);
  s = started(s, "t3");
  const cleared = ev(s, { kind: "message" });
  eq(cleared.items.some((it) => it.kind === "assistant" && it.id === "a:t3"), false, "empty message removes the empty bubble");
}

// 4. Empty message with no settle target is a full no-op (identity-preserving):
//    the pre-bubble window after a newer submit — the previous turn's empty
//    message must not remove or clear anything in the newer turn's state.
{
  let s = user(initialState, "第一问", 4);
  s = started(s, "t4");
  s = ev(s, { kind: "message", text: "第一答" });
  s = ev(s, { kind: "turn_done", turnId: "t4" });
  s = user(s, "第二问", 5); // newer submit, no turn_started yet → no bubble
  const before = s;
  const dropped = ev(s, { kind: "message" });
  eq(dropped === before, true, "empty message with no target returns state unchanged");
}

// 5. Message without text settles from the live stream (provider omits text
//    on the closing event; the streamed content must survive).
{
  let s = user(initialState, "流式结算", 6);
  s = started(s, "t6");
  s = delta(s, "reasoning", "思考");
  s = delta(s, "text", "流式答案");
  const done = ev(s, { kind: "message" });
  const assistant = done.items.find((it): it is Extract<typeof it, { kind: "assistant" }> => it.kind === "assistant" && it.id === "a:t6");
  eq(assistant?.text, "流式答案", "message without text keeps the streamed answer");
  eq(assistant?.reasoning, "思考", "message without reasoning keeps the streamed reasoning");
}

// 6. A stale currentAssistant mirror must not win over the active turn's
//    anchor: the message settles a:t2, not the old a:t1 the mirror still names.
{
  let s = user(initialState, "第一问", 7);
  s = started(s, "t1");
  s = ev(s, { kind: "message", text: "第一答" });
  s = ev(s, { kind: "turn_done", turnId: "t1" });
  s = user(s, "第二问", 8);
  s = started(s, "t2");
  s = delta(s, "text", "第二答");
  // Force the stale mirror: currentAssistant still names turn 1's bubble.
  s = { ...s, currentAssistant: "a:t1" };
  const done = ev(s, { kind: "message", text: "第二答终" });
  const t2 = done.items.find((it): it is Extract<typeof it, { kind: "assistant" }> => it.kind === "assistant" && it.id === "a:t2");
  const t1 = done.items.find((it): it is Extract<typeof it, { kind: "assistant" }> => it.kind === "assistant" && it.id === "a:t1");
  eq(t2?.text, "第二答终", "anchor wins over the stale mirror");
  eq(t1?.text, "第一答", "the previous turn's answer is not overwritten");
}

// 7. A content message with a live stream but no settle target (missed
//    turn_started or retracted pre-bubble) recreates the item: the answer must
//    land and the stream must close — never freeze the panes in "waiting".
{
  let s = user(initialState, "丢失气泡", 9);
  s = delta(s, "text", "思考过程"); // deltas open the live stream without turn_started
  eq(s.live !== undefined, true, "deltas open a live stream without turn_started");
  s = { ...s, items: s.items.filter((it) => it.kind !== "assistant") }; // bubble retracted
  const done = ev(s, { kind: "message", text: "最终答案", reasoning: "思考过程" });
  const assistant = done.items.find((it): it is Extract<typeof it, { kind: "assistant" }> => it.kind === "assistant");
  eq(assistant?.text, "最终答案", "content message recreates the item with the answer");
  eq(assistant?.streaming, false, "recreated item is settled");
  eq(done.live, undefined, "recreate path closes the live stream");
}

// 8. turn_done resets turnStartAt: after the session settles, UI that keys on
//    "a turn has started" (the split pane's awaiting label) must not keep
//    rendering as if a turn is still in flight.
{
  let s = user(initialState, "计时回合", 10);
  s = started(s, "t10");
  eq(s.turnStartAt > 0, true, "turn_started records the turn start");
  s = ev(s, { kind: "message", text: "答案" });
  const done = ev(s, { kind: "turn_done", turnId: "t10" });
  eq(done.turnStartAt, 0, "turn_done resets turnStartAt");
}

// 9. Multi-round turns keep every round: each model round (think → tool →
//    think → answer) settles into its own assistant item instead of
//    overwriting the previous round — the split panes then show every round's
//    reasoning and intermediate answer, matching the session transcript (the
//    collapse regression: only the last round's content survived).
{
  let s = user(initialState, "多轮回合", 11);
  s = started(s, "t11");
  s = delta(s, "reasoning", "第一轮思考");
  s = delta(s, "text", "第一轮回答");
  s = ev(s, { kind: "message", text: "第一轮回答", reasoning: "第一轮思考" });
  s = delta(s, "reasoning", "第二轮思考");
  s = delta(s, "text", "第二轮回答");
  s = ev(s, { kind: "message", text: "第二轮回答", reasoning: "第二轮思考" });
  s = delta(s, "reasoning", "第三轮思考");
  s = ev(s, { kind: "message", text: "第三轮回答", reasoning: "第三轮思考" });

  const assistants = s.items.filter((it): it is Extract<typeof it, { kind: "assistant" }> => it.kind === "assistant");
  eq(assistants.length, 3, "one assistant item per model round");
  eq(assistants[0]?.text, "第一轮回答", "round 1 answer survives");
  eq(assistants[0]?.reasoning, "第一轮思考", "round 1 reasoning survives");
  eq(assistants[1]?.id, "a:t11:2", "round 2 gets its own item");
  eq(assistants[1]?.text, "第二轮回答", "round 2 answer survives");
  eq(assistants[1]?.reasoning, "第二轮思考", "round 2 reasoning survives");
  eq(assistants[2]?.text, "第三轮回答", "round 3 answer survives");

  const models = buildTurnModels(s.items, NO_LIVE, false, false);
  const conv = conversationPaneTurns(models);
  const proc = processPaneTurns(models);
  eq(conv[0].answers.length, 3, "conversation pane lists every round's answer");
  const procReasoning = proc[0].segments.reduce(
    (n, seg) => n + seg.items.filter((it) => it.kind === "assistant").length,
    0,
  );
  eq(procReasoning, 3, "process pane lists every round's reasoning");
}

if (failed > 0) {
  process.stdout.write(`\nmessage-settle-hardening: ${failed} FAILED, ${passed} passed\n`);
  process.exitCode = 1;
} else {
  process.stdout.write(`\nmessage-settle-hardening: ${passed} passed\n`);
}
