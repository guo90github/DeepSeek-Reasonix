// Run: tsx src/__tests__/transcriptPanes.test.ts

import { buildTurnModels, NO_LIVE, type Item, type TranscriptLiveFlags } from "../lib/transcriptRows";
import { conversationPaneTurns, paneTurnDefaultOpen, processPaneTurns, turnHasShownContent } from "../lib/transcriptPanes";

let passed = 0;
let failed = 0;

function eq<T>(a: T, b: T, label: string) {
  if (a === b) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}: expected ${JSON.stringify(b)}, got ${JSON.stringify(a)}\n`);
    failed += 1;
  }
}

function user(id: string, text: string): Item {
  return { kind: "user", id, text, submissionId: `s-${id}`, createdAt: 1000 };
}

function answer(id: string, text: string, reasoning = ""): Item {
  return { kind: "assistant", id, text, reasoning, streaming: false, reasoningComplete: true };
}

function tool(id: string, name = "bash", opts: Partial<Extract<Item, { kind: "tool" }>> = {}): Item {
  return { kind: "tool", id, name, args: "", readOnly: false, status: "done", ...opts };
}

console.log("\ntranscript pane derivation");

const items: Item[] = [
  user("u1", "第一问"),
  answer("a1", "第一个回答，无推理"),
  user("u2", "第二问"),
  answer("a2", "第二个回答", "第二问的思考"),
  tool("t1", "read_file"),
  user("u3", "第三问"),
  tool("t2", "task", { parentId: "t1" }),
  tool("t3", "todo_write"),
  answer("a3", "第三个回答", "第三问的思考"),
];
const models = buildTurnModels(items, NO_LIVE, false, false);
const conversation = conversationPaneTurns(models);
const processTurns = processPaneTurns(models);

eq(conversation.length, 3, "conversation has one turn per question");
eq(processTurns.length, 3, "process has the same turn count as conversation");
eq(conversation[1].turn, processTurns[1].turn, "turn anchors align per index");
eq(conversation[0].answers.length, 1, "answer without reasoning lands in conversation");
eq(processTurns[0].segments[0].items.length, 0, "turn without process content stays in process as empty header");
eq(processTurns[1].segments[0].items.length, 1, "reasoning-only assistant lands in process");
eq(processTurns[2].segments[0].items.length, 1, "parented/todo tools are excluded from process");
eq(conversation[2].answers[0].kind, "assistant", "answer with reasoning still lands in conversation");
eq(turnHasShownContent(models[0]), true, "question turn has shown content");
eq(turnHasShownContent(models[1]), true, "answer turn has shown content");

// Prepending older history must not rename already-mounted pane turns.
const older: Item[] = [user("u0", "更早的问题"), answer("a0", "更早的回答")];
const prepended = buildTurnModels([...older, ...items], NO_LIVE, false, false);
const conversationPre = conversationPaneTurns(prepended);
const processPre = processPaneTurns(prepended);
eq(conversationPre.length, 4, "prepend adds a turn");
eq(conversationPre[0].key === conversation[0].key, false, "new turn gets a new key");
for (let i = 0; i < conversation.length; i += 1) {
  eq(conversationPre[i + 1].key, conversation[i].key, `existing conversation turn ${i} key survives prepend`);
  eq(processPre[i + 1].key, processTurns[i].key, `existing process turn ${i} key survives prepend`);
}

// A leading process-only prelude keeps both panes aligned with an undefined turn.
const preludeModels = buildTurnModels([tool("t0", "read_file"), answer("a0", "回答")], NO_LIVE, false, false);
const preludeConversation = conversationPaneTurns(preludeModels);
const preludeProcess = processPaneTurns(preludeModels);
eq(preludeConversation.length, 1, "prelude plus answer is one conversation turn");
eq(preludeProcess.length, 1, "prelude keeps process count equal");
eq(preludeConversation[0].turn, undefined, "prelude turn has no turn number");
eq(preludeProcess[0].turn, undefined, "process prelude turn has no turn number");
eq(turnHasShownContent(preludeModels[0]), true, "answer turn shows content");

// A bare recovery notice yields a turn with no shown content but stays present.
const recoveryModels = buildTurnModels([{ kind: "notice", id: "n0", level: "info", text: "恢复通知" }], NO_LIVE, false, false);
const recoveryConversation = conversationPaneTurns(recoveryModels);
eq(recoveryConversation.length, 1, "recovery-only prelude still appears");
eq(turnHasShownContent(recoveryModels[0]), false, "recovery-only turn has no shown content");

// Streaming reasoning must survive the pane filter: during a live turn the
// reasoning text lives in the LiveStream (item.reasoning is still empty), so
// the process pane must keep the item when live flags point at it. This is the
// split-layout regression: the right pane showed no thinking while streaming.
const streamingItems: Item[] = [
  user("u4", "第四问"),
  { kind: "assistant", id: "a4", text: "", reasoning: "", streaming: true, reasoningComplete: false },
];
const streamingLive: TranscriptLiveFlags = { id: "a4", hasAnswerText: false, hasReasoning: true, reasoningComplete: false };
const streamingModels = buildTurnModels(streamingItems, streamingLive, true, false);
const streamingProcess = processPaneTurns(streamingModels, streamingLive);
eq(streamingProcess.length, 1, "streaming turn keeps one process turn");
eq(streamingProcess[0].segments[0].items.length, 1, "streaming reasoning item survives the live-aware filter");
eq(streamingProcess[0].segments[0].items[0].kind, "assistant", "streaming reasoning renders as an assistant item");
const streamingConversation = conversationPaneTurns(streamingModels);
eq(streamingConversation[0].answers.length, 0, "reasoning-only streaming turn has no conversation answer yet");

// Without matching live flags the filter still drops empty-reasoning items
// (settled turns and stale live ids), so the old contract is preserved.
const staleLive: TranscriptLiveFlags = { id: "other", hasAnswerText: true, hasReasoning: true, reasoningComplete: false };
eq(processPaneTurns(streamingModels, staleLive)[0].segments[0].items.length, 0, "non-matching live id still filters the empty-reasoning item out");
eq(processPaneTurns(streamingModels, NO_LIVE)[0].segments[0].items.length, 0, "no live still filters the empty-reasoning item out");

// Answer streaming keeps the reasoning copy in the process pane while the
// answer itself lands in the conversation pane.
const answerLive: TranscriptLiveFlags = { id: "a4", hasAnswerText: true, hasReasoning: true, reasoningComplete: false };
const answerModels = buildTurnModels(streamingItems, answerLive, true, false);
const answerProcess = processPaneTurns(answerModels, answerLive);
eq(answerProcess[0].segments[0].items.length, 1, "reasoning copy survives while the answer streams");
eq(conversationPaneTurns(answerModels)[0].answers.length, 1, "streaming answer lands in the conversation pane");

// ── Turn-card default disclosure ─────────────────────────────────────────────
// Regression: the panes must decide "newest stays open" by stable key. The old
// code compared the Virtuoso itemContent index against turns.length - 1, but
// itemContent receives firstItemIndex-offset absolute indices, so the compare
// never matched and every settled turn (including the one that just finished
// answering) collapsed the moment the run ended.
console.log("pane turn default open");
{
  // `conversation` above is a settled 3-turn session (running=false): exactly
  // the newest turn stays open once run-end clears manual overrides.
  const newestKey = conversation[conversation.length - 1].key;
  eq(paneTurnDefaultOpen(conversation[2].isActive, conversation[2].key, newestKey), true, "settled newest turn stays open after its answer ends");
  eq(paneTurnDefaultOpen(conversation[1].isActive, conversation[1].key, newestKey), false, "older settled turn collapses");
  eq(paneTurnDefaultOpen(conversation[0].isActive, conversation[0].key, newestKey), false, "earliest settled turn collapses");
  const openTurns = conversation.filter((turn) => paneTurnDefaultOpen(turn.isActive, turn.key, newestKey));
  eq(openTurns.length, 1, "single-open policy: exactly one turn open on a settled session");
  eq(openTurns[0].key, newestKey, "the open turn is the newest one");
  eq(paneTurnDefaultOpen(false, "missing", undefined), false, "no newest turn stays closed");

  // A running session opens its active (last) turn even before it settles.
  const runningModels = buildTurnModels([user("u1", "q1"), answer("a1", "r1"), user("u2", "q2"), answer("a2", "r2", "思考")], NO_LIVE, true, false);
  const runningConv = conversationPaneTurns(runningModels);
  eq(paneTurnDefaultOpen(runningConv[1].isActive, runningConv[1].key, runningConv[1].key), true, "active running turn stays open");
  eq(paneTurnDefaultOpen(runningConv[0].isActive, runningConv[0].key, runningConv[1].key), false, "older turn stays collapsed while running");
}

if (failed > 0) {
  process.stdout.write(`\ntranscriptPanes: ${failed} FAILED, ${passed} passed\n`);
  process.exitCode = 1;
} else {
  process.stdout.write(`\ntranscriptPanes: ${passed} passed\n`);
}
