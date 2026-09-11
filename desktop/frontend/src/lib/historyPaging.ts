const HISTORY_PAGE_TURNS = 60;
const HISTORY_JUMP_MAX_TURNS = 500;
const HISTORY_JUMP_MAX_ENTRIES = 1000;

// A started older-page request that never settles would latch
// historyOlderLoading, which is the pane's backfill guard: the surface would sit
// on "loading" forever. Real page fetches answer in well under a second.
export const HISTORY_OLDER_STALL_MS = 15_000;

export function shouldReleaseStalledOlder(
  requestSeq: number,
  currentSeq: number | undefined,
  loading: boolean,
): boolean {
  return loading && currentSeq === requestSeq;
}

export function historyTurnsToLoad(startTurn: number, totalTurns: number, targetTurn?: number): number {
  const normalizedTarget = Number.isInteger(targetTurn) && (targetTurn ?? 0) > 0
    ? Math.min(Math.max(1, totalTurns), targetTurn as number)
    : undefined;
  const missingTurns = normalizedTarget === undefined
    ? HISTORY_PAGE_TURNS
    : Math.max(HISTORY_PAGE_TURNS, startTurn - normalizedTarget);
  return Math.min(HISTORY_JUMP_MAX_TURNS, missingTurns);
}

export function historyPageRequestBudget(
  startTurn: number,
  totalTurns: number,
  targetTurn?: number,
): { turns: number; entries?: number } {
  const turns = historyTurnsToLoad(startTurn, totalTurns, targetTurn);
  return targetTurn === undefined ? { turns } : { turns, entries: HISTORY_JUMP_MAX_ENTRIES };
}
