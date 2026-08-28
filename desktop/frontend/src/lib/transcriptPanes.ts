// transcriptPanes — the two-pane (conversation | process) turn model behind
// the desktop "split" layout. Both panes derive from the same TurnModel[]
// with the same inclusion predicate, so pane turn counts stay equal and the
// turn index is a valid anchor between them; a turn with no process content
// renders as a bare header in the process pane instead of being skipped.
// Everything here is pure so the alignment invariant is testable without a DOM.

import { stableStringHash } from "./stableStringHash";
import type { ExtensionItem, Item } from "./useController";
import { NO_LIVE, turnStableIdentity, type AssistantItem, type NoticeItem, type TranscriptLiveFlags, type TurnModel } from "./transcriptRows";

export type PaneAnswerItem = AssistantItem | NoticeItem | ExtensionItem;

export interface ConversationPaneTurn {
  key: string;
  turn: number | undefined;
  user: TurnModel["user"];
  answers: PaneAnswerItem[];
  hasShownContent: boolean;
  isActive: boolean;
}

export interface ProcessPaneSegment {
  key: string;
  items: Item[];
  hasRunningWork: boolean;
  isLast: boolean;
  durationMs: number;
}

export interface ProcessPaneTurn {
  key: string;
  turn: number | undefined;
  question: string;
  segments: ProcessPaneSegment[];
  hasShownContent: boolean;
  isActive: boolean;
  durationMs: number;
}

// A turn is visible to the user when it carries a question or an answer;
// process-only turns (recovery noise, empty turns) still stay in both panes
// so the turn indices never drift apart.
export function turnHasShownContent(model: TurnModel): boolean {
  if (model.user) return true;
  return model.segments.some((segment) => segment.outsideItems.length > 0);
}

// Mirror of transcriptRows.foldDisplayItems for the pane model: assistant
// items reach the pane stripped to their reasoning (answer text renders in the
// conversation pane), parented/plan-bookkeeping tools never surface. The live
// check mirrors foldDisplayItems too: during streaming the reasoning text
// lives in the LiveStream, not the item, so an empty item.reasoning must not
// drop the active turn's thinking from the process pane.
function processSegmentItems(segment: TurnModel["segments"][number], live: TranscriptLiveFlags): Item[] {
  return segment.processItems.filter((item) => {
    if (item.kind === "assistant") return Boolean(item.reasoning || (live.id === item.id && live.hasReasoning));
    if (item.kind === "phase" || item.kind === "notice" || item.kind === "compaction") return true;
    if (item.kind !== "tool") return false;
    if (item.parentId || item.name === "todo_write" || item.name === "exit_plan_mode") return false;
    return true;
  });
}

export function conversationPaneTurns(models: readonly TurnModel[]): ConversationPaneTurn[] {
  const turns: ConversationPaneTurn[] = [];
  for (const model of models) {
    const answers: PaneAnswerItem[] = [];
    for (const segment of model.segments) {
      for (const item of segment.outsideItems) answers.push(item);
    }
    turns.push({
      key: stableStringHash(turnStableIdentity(model)),
      turn: model.turn,
      user: model.user,
      answers,
      hasShownContent: turnHasShownContent(model),
      isActive: model.isActive,
    });
  }
  return turns;
}

export function processPaneTurns(models: readonly TurnModel[], live: TranscriptLiveFlags = NO_LIVE): ProcessPaneTurn[] {
  const turns: ProcessPaneTurn[] = [];
  for (const model of models) {
    const segments: ProcessPaneSegment[] = model.segments.map((segment, index) => ({
      key: segment.key,
      items: processSegmentItems(segment, live),
      hasRunningWork: segment.hasRunningWork,
      isLast: index === model.segments.length - 1,
      durationMs: segment.durationMs,
    }));
    turns.push({
      key: stableStringHash(turnStableIdentity(model)),
      turn: model.turn,
      question: model.user?.text ?? "",
      segments,
      hasShownContent: turnHasShownContent(model),
      isActive: model.isActive,
      durationMs: model.segments.reduce((ms, segment) => ms + segment.durationMs, 0),
    });
  }
  return turns;
}
