// Run: tsx src/__tests__/session-audit-turns.test.ts
// The whole-session audit payload is built from the loaded transcript, so the
// turn numbering, the per-turn request, and the "nothing to audit" gate are
// pinned here: a windowed list must not renumber, and a turn without reasoning
// must not reach the audit model.

import assert from "node:assert/strict";
import { collectSessionAuditTurns, countAuditableTurns } from "../lib/sessionAuditTurns";
import type { Item } from "../lib/useController";

const items = [
  { kind: "user", id: "u1", text: "第一问", historyTurn: 3 },
  { kind: "assistant", id: "a1", text: "答一", reasoning: "思考 A", streaming: false },
  { kind: "assistant", id: "a2", text: "", reasoning: "思考 A2", streaming: false },
  { kind: "user", id: "u2", text: "第二问", historyTurn: 4 },
  { kind: "assistant", id: "a3", text: "答二", reasoning: "   ", streaming: false },
  { kind: "user", id: "u3", text: "第三问", historyTurn: 5 },
  { kind: "assistant", id: "a4", text: "答三", reasoning: "思考 C", streaming: false },
] as unknown as Item[];

const turns = collectSessionAuditTurns(items);
assert.equal(turns.length, 2, "only turns with a reasoning chain are audited");
assert.deepEqual(turns.map((turn) => turn.turn), [3, 5], "turn numbers stay session-absolute");
assert.equal(turns[0].prompt, "第一问", "each turn carries the request it answered");
assert.equal(turns[0].reasoning, "思考 A\n\n思考 A2", "a turn's reasoning segments are joined in order");
assert.equal(turns[1].reasoning, "思考 C", "a whitespace-only chain is dropped, not sent");

// A windowed transcript without history metadata falls back to the window base,
// so the badge and the audit agree on which turn is being scored.
const live = [
  { kind: "user", id: "u1", text: "问" },
  { kind: "assistant", id: "a1", text: "答", reasoning: "思考", streaming: false },
] as unknown as Item[];
const based = collectSessionAuditTurns(live, 9);
assert.deepEqual(based.map((turn) => turn.turn), [10], "the window base seeds an unnumbered turn");

assert.equal(countAuditableTurns(items), 2, "the trigger's gate counts auditable turns");
assert.equal(countAuditableTurns([{ kind: "user", id: "u1", text: "问" }] as unknown as Item[]), 0, "a reasoning-free session offers nothing");

console.log("session audit turns: ok");
