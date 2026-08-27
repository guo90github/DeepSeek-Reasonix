// ProcessPane renders the per-turn reasoning + tool activity in the "split"
// desktop style's right column. One turn is expanded at a time (the focused
// turn, synced to the conversation pane by turn index); the rest collapse to a
// header so vertical scrolling navigates across turns without the inline
// reasoning/tool cards polluting the conversation column.
//
// The body reuses the same row cards the single-column transcript renders inside
// a fold (InlineAssistantReasoning, ToolCard, ToolGroup, …), fed from each turn
// segment's displayItems. The Virtuoso instance is exposed via ref so the pane
// sync hook can drive both panes to the same turn.

import { forwardRef, useCallback, useImperativeHandle, useRef } from "react";
import { Virtuoso, type VirtuosoHandle } from "react-virtuoso";

import { useT } from "../lib/i18n";
import type { Item } from "../lib/useController";
import type { ToolItem } from "../lib/transcriptRows";
import type { ProcessPaneTurn } from "../lib/transcriptPanes";

import { ToolCard } from "./ToolCard";
import { InlineAssistantReasoning } from "./InlineAssistantReasoning";
import { CompactionCard, NoticeCard, PhaseCard } from "./TranscriptCards";

export interface ProcessPaneHandle {
  scrollToTurn(turnKey: string): void;
  scrollToBottom(): void;
}

// Shared round "turn N" badge used by both panes so the conversation and
// process columns line up by number (1-based) without needing the arrow.
export function TurnBadge({ number }: { number: number }) {
  return <span className="process-pane__turn-round" aria-hidden="true">{number}</span>;
}

interface ProcessPaneProps {
  turns: ProcessPaneTurn[];
  focusedTurnKey: string;
  onFocusTurn: (turnKey: string) => void;
  subcallsByParent: ReadonlyMap<string, ToolItem[]>;
  tabId?: string;
  onRangeChange?: (startIndex: number) => void;
  running: boolean;
  /** Called on the user's first scroll/click so turn-sync can enable. */
  onUserInteract?: () => void;
}

export const ProcessPane = forwardRef<ProcessPaneHandle, ProcessPaneProps>(function ProcessPane(
  { turns, focusedTurnKey, onFocusTurn, subcallsByParent, tabId, onRangeChange, running, onUserInteract },
  ref,
) {
  const t = useT();
  const listRef = useRef<VirtuosoHandle>(null);
  const turnIndexByKey = useRef(new Map(turns.map((turn, index) => [turn.key, index])));
  turnIndexByKey.current = new Map(turns.map((turn, index) => [turn.key, index]));

  // Mark the first genuine user gesture (scroll or click) on the scroller so the
  // turn-sync hook stops ignoring range-changes (same rationale as the
  // conversation pane).
  const handleScroller = useCallback(
    (node: HTMLElement | Window | null) => {
      if (!node || !(node instanceof HTMLElement) || !onUserInteract) return;
      const mark = () => onUserInteract();
      node.addEventListener("wheel", mark, { passive: true });
      node.addEventListener("touchstart", mark, { passive: true });
      node.addEventListener("pointerdown", mark);
    },
    [onUserInteract],
  );

  useImperativeHandle(ref, () => ({
    scrollToTurn(turnKey: string) {
      const index = turnIndexByKey.current.get(turnKey);
      if (index == null) return;
      listRef.current?.scrollToIndex({ index, align: "start" });
    },
    scrollToBottom() {
      listRef.current?.scrollToIndex({ index: Math.max(0, turns.length - 1), align: "end" });
    },
  }), [turns.length]);

  const renderProcessItem = useCallback((item: Item) => {
    switch (item.kind) {
      case "assistant":
        return (
          <div className="process-pane__reasoning">
            <InlineAssistantReasoning item={item} onManualOpen={() => {}} />
          </div>
        );
      case "tool":
        return (
          <div className="process-pane__tool">
            <ToolCard item={item} subcalls={subcallsByParent.get(item.id)} tabId={tabId} />
          </div>
        );
      case "phase":
        return <PhaseCard id={item.id} text={item.text} />;
      case "notice":
        return <NoticeCard item={item} />;
      case "compaction":
        return <CompactionCard item={item} />;
      default:
        return null;
    }
  }, [subcallsByParent, tabId]);

  const renderTurn = useCallback((_index: number, turn: ProcessPaneTurn) => {
    const focused = turn.key === focusedTurnKey;
    const hasContent = turn.segments.length > 0;
    const label = turn.question?.trim() || (turn.turn == null ? t("transcriptPanes.prelude") : `${turn.turn}`);
    return (
      <div className="process-pane__turn" data-focused={focused || undefined}>
        <button
          type="button"
          className="process-pane__turn-header"
          aria-expanded={focused}
          onClick={() => onFocusTurn(turn.key)}
        >
          <span className="process-pane__turn-caret" aria-hidden="true">{hasContent ? (focused ? "▾" : "▸") : ""}</span>
          {/* Number from the turn index (not the array position) so it always
              matches the conversation pane's badge; a turn with no reasoning/
              tool activity still shows its header to keep the columns aligned. */}
          {turn.turn != null && <TurnBadge number={turn.turn + 1} />}
          <span className="process-pane__turn-label">{label}</span>
          {turn.durationMs > 0 && (
            <span className="process-pane__turn-duration">{formatDuration(turn.durationMs)}</span>
          )}
        </button>
        {focused && hasContent && (
          <div className="process-pane__turn-body">
            {turn.segments.map((segment) => (
              <div key={segment.key} className="process-pane__segment">
                {segment.items.map((item) => (
                  <div key={`${segment.key}:${item.id}`} className="process-pane__segment-item">
                    {renderProcessItem(item)}
                  </div>
                ))}
              </div>
            ))}
          </div>
        )}
      </div>
    );
  }, [focusedTurnKey, onFocusTurn, renderProcessItem, t]);

  return (
    <div className="process-pane">
      <div className="process-pane__title">
        <span>{t("transcriptPanes.processTitle")}</span>
        <span className="process-pane__title-count">{turns.length}</span>
      </div>
      <Virtuoso
        ref={listRef}
        className="process-pane__list"
        data={turns}
        itemContent={renderTurn}
        scrollerRef={handleScroller}
        overscan={400}
        rangeChanged={onRangeChange ? ({ startIndex }) => onRangeChange(startIndex) : undefined}
        // Follow the growing streaming reasoning in the active (last) turn, the
        // same way the conversation pane follows its live answer. Without this
        // the process pane stays put while the reasoning streams, so the view
        // can't reach the newest content.
        followOutput={running ? (isAtBottom) => (isAtBottom ? "smooth" : false) : false}
        components={{
          EmptyPlaceholder: () => (
            <div className="process-pane__empty">{t("transcriptPanes.processEmpty")}</div>
          ),
        }}
      />
    </div>
  );
});

function formatDuration(ms: number): string {
  const seconds = Math.max(0, Math.round(ms / 1000));
  if (seconds < 60) return `${seconds}s`;
  return `${Math.floor(seconds / 60)}m${seconds % 60}s`;
}
