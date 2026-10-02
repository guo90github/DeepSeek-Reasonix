// Run: tsx src/__tests__/queued-mention-visibility.test.ts
//
// A notice that carries an inbox item is host-injected guidance tied to a
// durable queue entry — a room mention, a queued wake. It belongs to the
// conversation, not to the model's work process: the split view renders the
// process channel in the other column, so filing it as process material makes
// the mention vanish from the conversation side.

import { buildTurnModels, NO_LIVE } from "../lib/transcriptRows";
import type { Item } from "../lib/useController";
import { conversationPaneTurns } from "../lib/transcriptPanes";

let passed = 0;
let failed = 0;

function ok(cond: boolean, label: string) {
  if (cond) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

function user(id: string, text: string): Item {
  return { kind: "user", id, text, submissionId: `s-${id}`, createdAt: 1000 };
}

function notice(id: string, text: string, extra: Partial<Extract<Item, { kind: "notice" }>> = {}): Item {
  return { kind: "notice", id, level: "info", text, ...extra };
}

function arrivesInConversation(items: Item[], id: string): boolean {
  return conversationPaneTurns(buildTurnModels(items, NO_LIVE, false, false)).some((turn) =>
    turn.answers.some((item) => item.id === id),
  );
}

console.log("\nqueued mention visibility");

ok(
  arrivesInConversation([user("u1", "问"), notice("n1", "↪ Chat room #43: 点了你", { inboxItemId: "ibx-1" })], "n1"),
  "a steer notice carrying an inbox item stays in the conversation",
);
ok(
  arrivesInConversation([notice("n2", "↪ Chat room #43: 点了你", { inboxItemId: "ibx-2" })], "n2"),
  "a steer notice in a numberless turn stays in the conversation",
);
ok(
  arrivesInConversation([user("u3", "问"), notice("n3", "房间唤醒已排队：等它这一轮结束", { inboxItemId: "ibx-3" })], "n3"),
  "queued guidance with an inbox item reaches the conversation column",
);
ok(
  !arrivesInConversation([user("u4", "问"), notice("n4", "已排队：等它这一轮结束")], "n4"),
  "a plain info notice without an inbox item stays process material",
);

process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exit(1);
