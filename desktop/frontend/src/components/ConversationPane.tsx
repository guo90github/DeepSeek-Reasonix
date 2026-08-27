// ConversationPane renders the clean question→answer column in the "split"
// desktop style's left column. Each source turn is ONE card: the turn badge,
// the user's question, then every assistant answer. Only the LATEST turn is
// expanded by default; older turns auto-collapse to a header (badge + question
// + caret) and can be manually expanded/collapsed. Reasoning and tool activity
// live in ProcessPane, so the conversation keeps a clean Q/A rhythm. Both panes
// are anchored by the same turn index.
//
// Reuses the row cards the single-column transcript renders (UserMessage for
// the question, LiveAssistantMessage with reasoning hidden for the answer).

import { forwardRef, useCallback, useEffect, useImperativeHandle, useMemo, useRef, useState } from "react";
import { Virtuoso, type VirtuosoHandle } from "react-virtuoso";

import { useT } from "../lib/i18n";
import type { ConversationPaneTurn } from "../lib/transcriptPanes";

import { UserMessage } from "./Message";
import { TurnBadge } from "./ProcessPane";
import { LiveAssistantMessage } from "./TranscriptVirtuosoParts";

export interface ConversationPaneHandle {
  scrollToTurn(turn: number): void;
  scrollToBottom(): void;
}

interface ConversationPaneProps {
  turns: ConversationPaneTurn[];
  creationMode: boolean;
  running: boolean;
  onEditPrompt?: (turn: number, displayText: string, submitText?: string) => boolean | void | Promise<boolean | void>;
  rewindDisabled: boolean;
  onRangeChange?: (startIndex: number) => void;
  /** Turn whose card should be highlighted to match the focused process turn. */
  focusedTurn?: number;
  /** Called on the user's first scroll/click so turn-sync can enable. */
  onUserInteract?: () => void;
  hasOlderHistory?: boolean;
  loadingOlderHistory?: boolean;
  olderHistoryError?: string;
  onLoadOlderHistory?: (targetTurn?: number) => boolean | Promise<boolean>;
}

export const ConversationPane = forwardRef<ConversationPaneHandle, ConversationPaneProps>(function ConversationPane(
  { turns, creationMode, running, onEditPrompt, rewindDisabled, onRangeChange, focusedTurn, onUserInteract, hasOlderHistory, loadingOlderHistory, olderHistoryError, onLoadOlderHistory },
  ref,
) {
  const t = useT();
  const listRef = useRef<VirtuosoHandle>(null);
  const turnIndexByTurn = useMemo(() => {
    const map = new Map<number, number>();
    turns.forEach((turn, index) => {
      if (turn.turn != null) map.set(turn.turn, index);
    });
    return map;
  }, [turns]);

  // Track the scroller node and whether the user is pinned at the bottom, so
  // streaming re-pinning never yanks them off a scrolled-up view.
  const scrollerRef = useRef<HTMLElement | null>(null);
  const atBottomRef = useRef(true);
  const handleScroller = useCallback(
    (node: HTMLElement | Window | null) => {
      if (!node || !(node instanceof HTMLElement)) return;
      scrollerRef.current = node;
      const onScroll = () => {
        atBottomRef.current = node.scrollHeight - node.scrollTop - node.clientHeight <= 8;
      };
      node.addEventListener("scroll", onScroll, { passive: true });
      if (!onUserInteract) return;
      const mark = () => onUserInteract();
      node.addEventListener("wheel", mark, { passive: true });
      node.addEventListener("touchstart", mark, { passive: true });
      node.addEventListener("pointerdown", mark);
    },
    [onUserInteract],
  );

  // react-virtuoso's followOutput can miss in-place streaming growth (new turn +
  // growing answer), and its internal scroll range stays stale (under-measured),
  // so scrollToIndex / autoscrollToBottom clamp short of the newest text. While
  // running, pin the DOM scroller straight to its true bottom whenever content
  // grew — but only if the user was already at the bottom.
  useEffect(() => {
    if (!running || !atBottomRef.current || !scrollerRef.current) return;
    scrollerRef.current.scrollTop = scrollerRef.current.scrollHeight;
  }, [running, turns]);

  // Collapse state: the latest turn is expanded by default; historical turns
  // collapse to a header. A user can flip any turn either way. `overrides`
  // stores explicit user overrides (true=expanded, false=collapsed); a turn
  // without an override follows the default for its role (latest expanded,
  // history collapsed), so streaming a new turn auto-expands it and auto-
  // collapses the previous latest without tracking every turn.
  const latestKey = useMemo(() => turns[turns.length - 1]?.key, [turns]);
  const [overrides, setOverrides] = useState<ReadonlyMap<string, boolean>>(() => new Map());
  const isExpanded = useCallback(
    (key: string) => overrides.get(key) ?? key === latestKey,
    [latestKey, overrides],
  );
  const toggleTurn = useCallback((key: string) => {
    setOverrides((prev) => {
      const next = new Map(prev);
      next.set(key, !(prev.get(key) ?? key === latestKey));
      return next;
    });
  }, [latestKey]);

  // The single-column transcript backfills older history on scroll; the split
  // layout loads it all up front so the whole session (and its per-turn
  // correspondence) is present from the real first turn instead of starting
  // mid-session. Each call prepends a page; the effect re-fires on the next
  // hasOlderHistory tick until the session is fully loaded.
  useEffect(() => {
    if (!hasOlderHistory || loadingOlderHistory || olderHistoryError || !onLoadOlderHistory) return;
    void onLoadOlderHistory();
  }, [hasOlderHistory, loadingOlderHistory, olderHistoryError, onLoadOlderHistory]);

  const OlderHistoryHeader = useCallback(() => {
    if (olderHistoryError) {
      return (
        <div className="conversation-pane__older">
          <span>{t("transcript.loadEarlierFailed")}</span>
          <button type="button" className="btn btn--small" onClick={() => void onLoadOlderHistory?.()}>
            {t("common.retry")}
          </button>
        </div>
      );
    }
    return null;
  }, [olderHistoryError, onLoadOlderHistory, t]);

  useImperativeHandle(ref, () => ({
    scrollToTurn(turn: number) {
      const index = turnIndexByTurn.get(turn);
      if (index == null) return;
      listRef.current?.scrollToIndex({ index, align: "start" });
    },
    scrollToBottom() {
      listRef.current?.scrollToIndex({ index: Math.max(0, turns.length - 1), align: "end" });
    },
  }), [turnIndexByTurn, turns.length]);

  const renderTurn = useCallback((_index: number, turn: ConversationPaneTurn) => {
    const focused = turn.turn != null && turn.turn === focusedTurn;
    const expanded = isExpanded(turn.key);
    return (
      <div className="conversation-pane__turn" data-focused={focused || undefined}>
        <button
          type="button"
          className="conversation-pane__turn-head"
          aria-expanded={expanded}
          onClick={() => toggleTurn(turn.key)}
        >
          {turn.turn != null && <TurnBadge number={turn.turn + 1} />}
          {!expanded && turn.question && (
            <span className="conversation-pane__turn-label">{turn.question.text}</span>
          )}
          <span className="conversation-pane__turn-caret" aria-hidden="true">{expanded ? "▾" : "▸"}</span>
        </button>
        {expanded && (
          <div className="conversation-pane__turn-body">
            {turn.question && (
              <UserMessage
                id={turn.question.id}
                text={turn.question.text}
                submitText={turn.question.submitText}
                failed={turn.question.failed}
                createdAt={turn.question.createdAt}
                turn={turn.turn}
                onEdit={onEditPrompt}
                editDisabled={rewindDisabled}
              />
            )}
            {turn.answers.map((item) => (
              <LiveAssistantMessage
                key={item.id}
                item={{ ...item, reasoning: "" }}
                defaultExpanded={false}
                expandWhileStreaming={false}
                creationMode={creationMode}
                reasoningDisplay="hide"
              />
            ))}
          </div>
        )}
      </div>
    );
  }, [creationMode, focusedTurn, isExpanded, onEditPrompt, rewindDisabled, toggleTurn]);

  return (
    <div className="conversation-pane">
      <Virtuoso
        ref={listRef}
        className="conversation-pane__list"
        data={turns}
        itemContent={renderTurn}
        scrollerRef={handleScroller}
        overscan={400}
        rangeChanged={onRangeChange ? ({ startIndex }) => onRangeChange(startIndex) : undefined}
        followOutput={running ? (isAtBottom) => (isAtBottom ? "smooth" : false) : false}
        components={{
          Header: olderHistoryError ? OlderHistoryHeader : undefined,
          // Bottom spacer guarantees the last turn card can always scroll clear
          // of the composer/footer below the pane, so its bottom edge is never
          // pressed against or covered by it.
          Footer: () => <div className="conversation-pane__spacer" aria-hidden="true" />,
          EmptyPlaceholder: () => (
            <div className="conversation-pane__empty">{t("transcriptPanes.conversationEmpty")}</div>
          ),
        }}
      />
    </div>
  );
});
