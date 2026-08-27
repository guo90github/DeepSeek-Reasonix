// Run: tsx src/__tests__/transcriptPanes.test.ts

import { buildTurnModels, NO_LIVE, type Item } from "../lib/transcriptRows";
import { conversationPaneTurns, processPaneTurns, turnHasShownContent } from "../lib/transcriptPanes";

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

if (failed > 0) {
  process.stdout.write(`\ntranscriptPanes: ${failed} FAILED, ${passed} passed\n`);
  process.exitCode = 1;
} else {
  process.stdout.write(`\ntranscriptPanes: ${passed} passed\n`);
}
