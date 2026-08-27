// Run: tsx src/lib/__tests__/transcriptPanes.test.ts

import { buildConversationPaneTurns, buildProcessPaneTurns } from "../lib/transcriptPanes";
import type { SegmentModel, TurnModel } from "../lib/transcriptRows";

const userItem = { kind: "user" as const, id: "u1", text: "hello" };
const answerItem = { kind: "assistant" as const, id: "a1", text: "the answer", reasoning: "", streaming: false };
const reasonItem = { kind: "assistant" as const, id: "a2", text: "", reasoning: "thinking", streaming: false };

function segment(processItems: TurnModel["segments"][number]["processItems"] = [reasonItem]): SegmentModel {
  return {
    key: processItems[0]?.id ?? "s1",
    processItems,
    outsideItems: [answerItem],
    displayItems: processItems,
    hasOutsideContent: true,
    hasRunningWork: false,
    durationMs: 0,
    labelStyle: "full",
    turnActive: false,
  };
}

function turn(turnNumber: number | undefined, overrides: Partial<TurnModel> = {}): TurnModel {
  return {
    user: userItem,
    turn: turnNumber,
    isActive: false,
    turnItems: [answerItem, reasonItem],
    segments: [segment()],
    actionText: answerItem.text,
    ...overrides,
  };
}

let failed = 0;
function ok(value: boolean, label: string) {
  process.stdout.write(`  ${value ? "PASS" : "FAIL"}  ${label}\n`);
  if (!value) failed += 1;
}
function eq<T>(actual: T, expected: T, label: string) {
  const value = JSON.stringify(actual) === JSON.stringify(expected);
  ok(value, label);
  if (!value) process.stdout.write(`    actual=${JSON.stringify(actual)} expected=${JSON.stringify(expected)}\n`);
}

// buildConversationPaneTurns groups each source turn into ONE card (question +
// answers), keeping the turn anchor so the two panes line up by number.
{
  const turns = buildConversationPaneTurns([turn(1)]);
  eq(turns.length, 1, "one conversation turn per source turn");
  eq(turns[0].turn, 1, "conversation turn keeps the turn anchor");
  eq(turns[0].question?.id, "u1", "conversation turn carries the question");
  eq(turns[0].answers.map((a) => a.id), ["a1"], "conversation turn collects the answers");
}
{
  const turns = buildConversationPaneTurns([turn(1), turn(2)]);
  eq(turns.map((item) => item.turn), [1, 2], "conversation turns preserve turn order");
}
{
  const noUser = turn(1, { user: undefined, segments: [] });
  eq(buildConversationPaneTurns([noUser]).length, 0, "turns with no question and no answers are skipped");
}

// buildProcessPaneTurns groups process material per turn. A turn with no
// reasoning/tool activity still appears (empty segments) so its count matches
// the conversation pane.
{
  const processTurns = buildProcessPaneTurns([turn(1)]);
  eq(processTurns.length, 1, "one process turn per source turn");
  eq(processTurns[0].turn, 1, "process turn keeps the turn anchor");
  eq(processTurns[0].segments[0].items, [reasonItem], "process segment carries reasoning displayItems");
}
{
  const emptySegment: SegmentModel = { ...segment(), displayItems: [], processItems: [] };
  const processTurns = buildProcessPaneTurns([turn(1, { segments: [emptySegment] })]);
  eq(processTurns.length, 1, "turn with no process material still appears (count matches conversation)");
  eq(processTurns[0].segments.length, 0, "…with empty segments");
}
{
  // A mixed session (some turns with process content, some without) must yield
  // the SAME turn count in both panes so the per-turn correspondence stays.
  const plain: SegmentModel = { ...segment(), displayItems: [], processItems: [] };
  const turns = [turn(1), turn(2, { segments: [plain] })];
  const conv = buildConversationPaneTurns(turns);
  const proc = buildProcessPaneTurns(turns);
  eq(conv.length, proc.length, "conversation and process turn counts match");
  eq(proc.map((item) => item.turn), [1, 2], "process turns keep contiguous anchors");
}
{
  const processTurns = buildProcessPaneTurns([turn(1), turn(2, { isActive: true, segments: [segment()] })]);
  eq(processTurns.map((item) => item.isActive), [false, true], "streaming turn flagged active");
}

if (failed > 0) process.exit(1);
