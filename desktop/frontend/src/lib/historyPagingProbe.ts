/**
 * Read-only history-paging probe.
 *
 * Records only flags and counters — never text, paths, or ids — so the
 * frontend diagnostics export can carry the pane's history state on the stable
 * channel, where the opt-in recorder's own diagnostics stay out of reach.
 */

export type HistoryPaneView = {
  splitMode: boolean;
  paneHasOlder: boolean;
  paneLoading: boolean;
  paneError: boolean;
  hasOlder: boolean;
  olderLoading: boolean;
  running: boolean;
  hydrating: boolean;
  startTurn: number;
  totalTurns: number;
  revision: number;
};

export type HistoryOlderRefusal = {
  trigger: string;
  known: boolean;
  hasOlder: boolean;
  loading: boolean;
  running: boolean;
};

export type HistoryPagingProbeSnapshot = {
  view: HistoryPaneView | null;
  refusals: HistoryOlderRefusal[];
};

export const MAX_HISTORY_REFUSALS = 8;

let view: HistoryPaneView | null = null;
const refusals: HistoryOlderRefusal[] = [];

export function recordHistoryPaneView(next: HistoryPaneView): void {
  view = { ...next };
}

export function recordHistoryOlderRefusal(next: HistoryOlderRefusal): void {
  refusals.push({ ...next });
  if (refusals.length > MAX_HISTORY_REFUSALS) refusals.splice(0, refusals.length - MAX_HISTORY_REFUSALS);
}

export function historyPagingProbeSnapshot(): HistoryPagingProbeSnapshot {
  return { view: view ? { ...view } : null, refusals: refusals.map((entry) => ({ ...entry })) };
}

export function resetHistoryPagingProbe(): void {
  view = null;
  refusals.length = 0;
}
