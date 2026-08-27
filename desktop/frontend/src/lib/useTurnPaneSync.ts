// useTurnPaneSync anchors the conversation and process panes to the same turn
// so "conversation turn N" always corresponds to "process turn N".
//
// The turn index is the shared anchor (never a pixel offset — the two panes'
// row heights differ). Rules:
//   - The streaming (active) turn becomes the focused turn and both panes
//     follow it.
//   - User scrolling the conversation pane focuses the process turn at the same
//     index and scrolls the process pane to it.
//   - User scrolling (or clicking) the process pane focuses that turn and
//     scrolls the conversation pane to it.
//
// react-virtuoso's rangeChanged does not report whether a scroll is
// programmatic, so to avoid the two panes fighting each other we suppress
// range-changes that arrive right after a scrollToIndex we issued ourselves
// (a short timestamp window around every programmatic scroll).

import { useCallback, useEffect, useMemo, useRef, useState, type RefObject } from "react";

import type { ConversationPaneHandle } from "../components/ConversationPane";
import type { ProcessPaneHandle } from "../components/ProcessPane";
import type { ConversationPaneTurn, ProcessPaneTurn } from "../lib/transcriptPanes";

const SUPPRESS_MS = 200;

export interface UseTurnPaneSyncArgs {
  conversationTurns: ConversationPaneTurn[];
  processTurns: ProcessPaneTurn[];
  activeTurnKey: string | undefined;
  running: boolean;
}

export interface UseTurnPaneSyncResult {
  conversationRef: RefObject<ConversationPaneHandle | null>;
  processRef: RefObject<ProcessPaneHandle | null>;
  focusedTurnKey: string;
  onProcessTurnFocus: (turnKey: string) => void;
  onConversationRangeChange: (startIndex: number) => void;
  onProcessRangeChange: (startIndex: number) => void;
  /** Notify that the user has scrolled/clicked a pane (enables range-change sync). */
  markUserInteracted: () => void;
}

export function useTurnPaneSync({
  conversationTurns,
  processTurns,
  activeTurnKey,
  running,
}: UseTurnPaneSyncArgs): UseTurnPaneSyncResult {
  const conversationRef = useRef<ConversationPaneHandle | null>(null);
  const processRef = useRef<ProcessPaneHandle | null>(null);
  const [focusedTurnKey, setFocusedTurnKey] = useState(activeTurnKey ?? "");
  const suppressUntilRef = useRef(0);
  // Focus follows the user's own scrolling/clicking. Until the first genuine
  // interaction, neither the panes' initial range-changes (list top = turn 1)
  // nor the older-history backfill (which prepends turns and shifts the visible
  // range) may move focus off the most recent turn.
  const userInteractedRef = useRef(false);
  // Whether the initial focus has been parked on the most recent turn.
  const establishedRef = useRef(false);

  // conversation turn index → turn number (each item is one turn).
  const turnAtRow = useMemo(() => {
    const map = new Map<number, number>();
    conversationTurns.forEach((turn, index) => {
      if (turn.turn != null) map.set(index, turn.turn);
    });
    return map;
  }, [conversationTurns]);

  // turn number → process pane key.
  const processKeyByTurn = useMemo(() => {
    const map = new Map<number, string>();
    for (const turn of processTurns) {
      if (turn.turn != null) map.set(turn.turn, turn.key);
    }
    return map;
  }, [processTurns]);

  const scrollProcessTo = useCallback((key: string) => {
    suppressUntilRef.current = Date.now() + SUPPRESS_MS;
    processRef.current?.scrollToTurn(key);
  }, []);
  const scrollConversationTo = useCallback((turn: number) => {
    suppressUntilRef.current = Date.now() + SUPPRESS_MS;
    conversationRef.current?.scrollToTurn(turn);
  }, []);
  const markUserInteracted = useCallback(() => {
    userInteractedRef.current = true;
  }, []);

  // Park the initial focus on the most recent turn and put both panes at it, so
  // the arrow opens on the latest conversation ↔ process pair instead of turn 1.
  // Runs once, when turns first exist; a prior user interaction wins. The
  // conversation is scrolled to its BOTTOM so followOutput can keep it pinned to
  // the newest answer once streaming starts.
  useEffect(() => {
    if (establishedRef.current || userInteractedRef.current || !activeTurnKey) return;
    establishedRef.current = true;
    setFocusedTurnKey(activeTurnKey);
    suppressUntilRef.current = Date.now() + SUPPRESS_MS;
    conversationRef.current?.scrollToBottom();
    scrollProcessTo(activeTurnKey);
  }, [activeTurnKey, scrollProcessTo]);

  // A new streaming turn takes focus; park both panes on it so the live answer
  // and its reasoning stay visible while they stream. The conversation pane is
  // NOT scrolled here: followOutput (running) keeps it pinned to the newest
  // answer, and a programmatic scrollToIndex here would leave isAtBottom false
  // and silently disable followOutput for the rest of the stream.
  useEffect(() => {
    if (!running || !activeTurnKey) return;
    setFocusedTurnKey((current) => (current === activeTurnKey ? current : activeTurnKey));
    suppressUntilRef.current = Date.now() + SUPPRESS_MS;
    scrollProcessTo(activeTurnKey);
  }, [activeTurnKey, running, scrollProcessTo]);

  const onConversationRangeChange = useCallback((startIndex: number) => {
    if (!userInteractedRef.current) return;
    if (Date.now() < suppressUntilRef.current) return;
    const turn = turnAtRow.get(startIndex);
    if (turn == null) return;
    const processKey = processKeyByTurn.get(turn);
    if (processKey == null) return;
    setFocusedTurnKey(processKey);
    scrollProcessTo(processKey);
  }, [processKeyByTurn, scrollProcessTo, turnAtRow]);

  const onProcessTurnFocus = useCallback((turnKey: string) => {
    userInteractedRef.current = true;
    // Toggle: clicking the already-focused (expanded) turn collapses it.
    if (turnKey === focusedTurnKey) {
      setFocusedTurnKey("");
      return;
    }
    setFocusedTurnKey(turnKey);
    const turn = processTurns.find((item) => item.key === turnKey)?.turn;
    if (turn == null) return;
    scrollConversationTo(turn);
  }, [focusedTurnKey, processTurns, scrollConversationTo]);

  const onProcessRangeChange = useCallback((startIndex: number) => {
    if (!userInteractedRef.current) return;
    if (Date.now() < suppressUntilRef.current) return;
    const turn = processTurns[startIndex]?.turn;
    if (turn == null) return;
    setFocusedTurnKey(processTurns[startIndex].key);
    scrollConversationTo(turn);
  }, [processTurns, scrollConversationTo]);

  return {
    conversationRef,
    processRef,
    focusedTurnKey,
    onProcessTurnFocus,
    onConversationRangeChange,
    onProcessRangeChange,
    markUserInteracted,
  };
}
