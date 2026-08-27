// Split-layout ("split" desktop style) row derivation.
//
// The conversation/output area splits into two turn-synced panes instead of one
// linear transcript:
//   - conversation pane: the user message + the assistant's formal answer
//     (outsideItems), reasoning and tool activity excluded.
//   - process pane: per-turn reasoning + tool blocks (segments' displayItems),
//     one turn expanded at a time, scrollable across turns.
//
// Both panes derive from the same TurnModel[] (buildTurnModels), so a turn N in
// the conversation pane always corresponds to turn N in the process pane — the
// turn index is the shared anchor. The model already separates the two channels
// per turn (partitionTurnItems → processItems/outsideItems), so this file only
// reshapes it into the two flat lists the panes render.

import type { Item } from "./useController";
import type { AssistantItem, TurnModel, UserItem } from "./transcriptRows";

// ── Conversation pane turns ─────────────────────────────────────────────────

// A turn is shown in BOTH split panes iff it has a user question or an
// assistant answer. Both builders share this predicate so the conversation and
// process columns always hold the SAME turn set — a turn with no reasoning or
// tool activity still appears in the process column (as a header, empty body)
// so the per-turn correspondence and counts never drift apart.
function turnHasShownContent(turn: TurnModel): boolean {
  if (turn.user) return true;
  return turn.segments.some((segment) =>
    segment.outsideItems.some((item) => item.kind === "assistant"),
  );
}

// Each source turn renders as ONE card in the conversation pane: the turn badge
// + the user's question + every assistant answer, so a turn never fragments
// into several disconnected boxes. `turn` is the shared 1-based anchor.
export interface ConversationPaneTurn {
  key: string;
  turn: number | undefined;
  question: UserItem | undefined;
  answers: AssistantItem[];
}

export function buildConversationPaneTurns(turnModels: readonly TurnModel[]): ConversationPaneTurn[] {
  const turns: ConversationPaneTurn[] = [];
  for (const turn of turnModels) {
    const answers: AssistantItem[] = [];
    for (const segment of turn.segments) {
      for (const item of segment.outsideItems) {
        if (item.kind !== "assistant") continue;
        answers.push(item);
      }
    }
    if (!turnHasShownContent(turn)) continue;
    turns.push({
      key: turn.turn == null ? "pane-conv:prelude" : `pane-conv:${turn.turn}`,
      turn: turn.turn,
      question: turn.user,
      answers,
    });
  }
  return turns;
}

// ── Process pane turns ────────────────────────────────────────────────────────

export interface ProcessPaneSegment {
  key: string;
  items: Item[];
}

export interface ProcessPaneTurn {
  key: string;
  turn: number | undefined;
  /** The user's question text for this turn (the reliable label users match). */
  question: string | undefined;
  isActive: boolean;
  durationMs: number;
  segments: ProcessPaneSegment[];
}

export function buildProcessPaneTurns(turnModels: readonly TurnModel[]): ProcessPaneTurn[] {
  const turns: ProcessPaneTurn[] = [];
  for (const turn of turnModels) {
    // Same predicate as the conversation pane so both columns always hold the
    // same turn set. A turn with no reasoning/tool activity is still included
    // (with empty segments) so turn counts and the per-turn correspondence stay
    // aligned between the two panes.
    if (!turnHasShownContent(turn)) continue;
    const segments = turn.segments
      .filter((segment) => segment.displayItems.length > 0)
      .map((segment) => ({ key: segment.key, items: segment.displayItems }));
    const last = turn.segments[turn.segments.length - 1];
    turns.push({
      key: turn.turn == null ? "pane-process:prelude" : `pane-process:${turn.turn}`,
      turn: turn.turn,
      question: turn.user?.text ?? "",
      isActive: turn.isActive || Boolean(last?.turnActive),
      durationMs: last?.durationMs ?? 0,
      segments,
    });
  }
  return turns;
}
