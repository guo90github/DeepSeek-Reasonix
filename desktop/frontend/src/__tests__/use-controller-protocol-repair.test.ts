// Run: tsx src/__tests__/use-controller-protocol-repair.test.ts
//
// Regression for the split-layout Protocol-repair bug (2026-08-27): the
// backend injects a second model round under the SAME turn id when the model
// skips the required finish call (finalization.go requestProtocolRepair →
// conversation.Add, no user/turn_started event). Before the fix, ensureAssistant
// reused the settled first-round assistant (a:<turnId>), so turn_done clobbered
// its delivered answer with the second round's empty live text and pinned the
// second round's reasoning above the first round's tools. The second round must
// start its own assistant bubble so both rounds keep their content and render
// in chronological order.

import { initialState, reducer, type State } from "../lib/useController";
import type { WireEvent } from "../lib/types";
import { buildTurnModels } from "../lib/transcriptRows";
import { conversationPaneTurns, processPaneTurns } from "../lib/transcriptPanes";

let passed = 0;
let failed = 0;
function ok(v: boolean, label: string) {
  process.stdout.write(`  ${v ? "PASS" : "FAIL"}  ${label}\n`);
  if (v) passed += 1; else failed += 1;
}
function ev(s: typeof initialState, e: WireEvent) {
  return reducer(s, { type: "event", e });
}

console.log("\nProtocol-repair second round under the same turn id");

{
  let s: State = { ...initialState, activeTurnId: "turn_abc" };
  s = ev(s, { kind: "turn_started", turnId: "turn_abc" } as WireEvent);
  s = ev(s, { kind: "reasoning", text: "The user said hello" } as WireEvent);
  s = ev(s, { kind: "tool_dispatch", tool: { id: "t1", name: "read_file", args: "{}", readOnly: true } } as WireEvent);
  s = ev(s, { kind: "tool_result", tool: { id: "t1", name: "read_file", output: "file" } } as WireEvent);
  s = ev(s, { kind: "message", text: "你好！👋 answer" } as WireEvent);
  // Second model round, same turn id — Protocol repair.
  s = ev(s, { kind: "reasoning", text: "The user is asking me to finish the turn" } as WireEvent);
  s = ev(s, { kind: "tool_dispatch", tool: { id: "t2", name: "finish", args: "{}", readOnly: true } } as WireEvent);
  s = ev(s, { kind: "tool_result", tool: { id: "t2", name: "finish", output: "ok" } } as WireEvent);
  s = ev(s, { kind: "turn_done", turnId: "turn_abc" } as WireEvent);

  const assistants = s.items.filter((it) => it.kind === "assistant") as any[];
  // One bubble per provider sampling round (assistantItems.ensureAssistant): the
  // first round's thinking, the answer that round delivered in its own segment
  // (the tool round closed the previous one), and the second round.
  ok(assistants.length === 3, `each sampling round keeps its own bubble (got ${assistants.length})`);
  const answerBubbles = assistants.filter((it) => it.text.includes("你好！👋 answer"));
  ok(answerBubbles.length === 1, `the first round's answer survives turn_done once (got ${answerBubbles.length})`);
  const thinkingBubbles = assistants.filter((it) => (it.reasoning ?? "").includes("The user said hello"));
  ok(thinkingBubbles.length === 1, "the first round's reasoning is not restated by a later segment (that renders the same thinking twice)");
  ok(answerBubbles[0] !== thinkingBubbles[0], "the answer does not land on the thinking-only bubble");
  const roundTwo = assistants[assistants.length - 1];
  ok(roundTwo.reasoning.includes("finish the turn"), "second-round reasoning lands on its own bubble");
  ok(roundTwo.text === "", "second-round empty answer adds no text");

  // Pane derivation: conversation keeps the answer; process interleaves the two
  // rounds chronologically (turn1 thinking, read_file, turn2 thinking, finish).
  const turns = buildTurnModels(s.items);
  const conv = conversationPaneTurns(turns);
  const proc = processPaneTurns(turns);
  ok(conv.length === 1 && conv[0].answers.length === 1, "conversation pane keeps the answer");
  const flat: string[] = [];
  for (const turn of proc) {
    for (const seg of turn.segments) {
      for (const item of seg.items) {
        flat.push(item.kind === "assistant" ? "thinking" : `tool:${(item as any).name}`);
      }
    }
  }
  ok(
    JSON.stringify(flat) === JSON.stringify(["thinking", "tool:read_file", "thinking", "tool:finish"]),
    `process pane renders the two rounds in order (got ${JSON.stringify(flat)})`,
  );
}

if (failed > 0) {
  console.error(`\n${failed} protocol-repair test(s) failed; ${passed} passed.`);
  process.exit(1);
}
console.log(`\n${passed} protocol-repair tests passed.`);
