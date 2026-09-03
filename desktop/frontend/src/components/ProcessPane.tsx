// ProcessPane — the split layout's right column: one header per turn (badge +
// question) with its reasoning/tool/phase content beneath, newest turn expanded
// by default and history collapsed. Reuses InlineAssistantReasoning / ToolCard /
// PhaseCard / NoticeCard / CompactionCard so rendering matches the single
// column's process folds.

import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState, type Ref } from "react";
import { Virtuoso, type VirtuosoHandle } from "react-virtuoso";
import { paneTurnDefaultOpen, type ProcessPaneTurn } from "../lib/transcriptPanes";
import { usePaneTailFollow } from "../lib/usePaneTailFollow";
import { useTranscriptVirtuosoFirstItemIndex } from "../lib/transcriptVirtuosoIndex";
import { InlineAssistantReasoning } from "./InlineAssistantReasoning";
import { ToolCard } from "./ToolCard";
import { PhaseCard, NoticeCard, CompactionCard } from "./TranscriptCards";

const AuditInlineCard = lazy(() => import("./AuditInlineCard").then((module) => ({ default: module.AuditInlineCard })));

export function TurnBadge({ turn }: { turn: number }) {
  return <span className="turn-badge" aria-hidden="true">{turn + 1}</span>;
}

function ProcessTurnCard({
  turn,
  open,
  onToggle,
  mirrorActive,
  onPointerEnter,
  onPointerLeave,
  tabId,
}: {
  turn: ProcessPaneTurn;
  open: boolean;
  onToggle: () => void;
  mirrorActive: boolean;
  onPointerEnter?: () => void;
  onPointerLeave?: () => void;
  tabId?: string;
}) {
  return (
    <article className={["process-pane__turn", open ? "process-pane__turn--open" : "process-pane__turn--collapsed", mirrorActive ? "process-pane__turn--mirror" : ""].filter(Boolean).join(" ")} data-turn={turn.turn ?? ""} data-turn-key={turn.key} onPointerEnter={onPointerEnter} onPointerLeave={onPointerLeave}>
      <header className="process-pane__turn-head" role="button" tabIndex={0} aria-expanded={open} onClick={onToggle} onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onToggle();
        }
      }}>
        {turn.turn !== undefined && <TurnBadge turn={turn.turn} />}
        <span className="process-pane__turn-question" title={turn.question}>{turn.question}</span>
      </header>
      {open && (
        <div className="process-pane__turn-body">
          {turn.segments.map((segment) => (
            <div className="process-pane__segment" key={segment.key}>
              {segment.items.map((item) => {
                if (item.kind === "assistant") {
                  return (
                    <div className="turn-collapse__body" key={item.id}>
                      <InlineAssistantReasoning item={item} />
                      {item.reasoning && (
                        <Suspense fallback={null}>
                          <AuditInlineCard reasoning={item.reasoning} />
                        </Suspense>
                      )}
                    </div>
                  );
                }
                if (item.kind === "tool") {
                  return (
                    <div className="turn-collapse__body" key={item.id}>
                      <ToolCard item={item} subcalls={undefined} tabId={tabId} />
                    </div>
                  );
                }
                if (item.kind === "phase") {
                  return (
                    <div className="turn-collapse__body" key={item.id}>
                      <PhaseCard id={item.id} text={item.text} />
                    </div>
                  );
                }
                if (item.kind === "compaction") {
                  return (
                    <div className="turn-collapse__body" key={item.id}>
                      <CompactionCard item={item} />
                    </div>
                  );
                }
                if (item.kind === "notice") {
                  return (
                    <div className="turn-collapse__body" key={item.id}>
                      <NoticeCard item={item} />
                    </div>
                  );
                }
                return null;
              })}
              {segment.items.length === 0 && segment.isLast && (
                <div className="process-pane__empty-segment" />
              )}
            </div>
          ))}
          {turn.segments.length === 0 && <div className="process-pane__empty-segment" />}
        </div>
      )}
    </article>
  );
}

export function ProcessPane({
  turns,
  running,
  listRef,
  onUserInteract,
  scrollerRef,
  hoveredIndex,
  onHoverIndex,
  tabId,
}: {
  turns: readonly ProcessPaneTurn[];
  running: boolean;
  listRef?: Ref<VirtuosoHandle>;
  onUserInteract?: () => void;
  scrollerRef?: (node: HTMLElement | Window | null) => void;
  hoveredIndex?: number | null;
  onHoverIndex?: (index: number | null) => void;
  tabId?: string;
}) {
  const [overrides, setOverrides] = useState<ReadonlyMap<string, boolean>>(new Map());
  const toggle = useCallback((key: string) => {
    setOverrides((current) => {
      const next = new Map(current);
      next.set(key, !(next.get(key) ?? false));
      return next;
    });
  }, []);
  // Newest turn is identified by stable key, not list index: Virtuoso hands
  // itemContent firstItemIndex-offset absolute indices that never equal the
  // data length, so an index comparison collapsed every settled turn.
  const newestKey = useMemo(() => turns[turns.length - 1]?.key, [turns]);
  // Single-open policy: manual overrides survive only until the current run
  // settles, so a finished answer leaves exactly the newest turn expanded.
  const wasRunningRef = useRef(running);
  useEffect(() => {
    const wasRunning = wasRunningRef.current;
    wasRunningRef.current = running;
    if (wasRunning && !running) {
      setOverrides((current) => (current.size > 0 ? new Map() : current));
    }
  }, [running]);
  // Older history pages prepend turns at the top; keep their absolute index
  // anchored so Virtuoso does not shift already-mounted rows out of sync.
  const firstItemIndex = useTranscriptVirtuosoFirstItemIndex(turns, tabId ?? "");

  const itemContent = useCallback((index: number, turn: ProcessPaneTurn) => (
    <ProcessTurnCard
      turn={turn}
      open={overrides.get(turn.key) ?? paneTurnDefaultOpen(turn.isActive, turn.key, newestKey)}
      onToggle={() => toggle(turn.key)}
      mirrorActive={hoveredIndex === index}
      onPointerEnter={() => onHoverIndex?.(index)}
      onPointerLeave={() => onHoverIndex?.(null)}
      tabId={tabId}
    />
  ), [hoveredIndex, newestKey, onHoverIndex, overrides, tabId, toggle]);

  // Tail-follow replaces Virtuoso's followOutput: stream chunks and the
  // reasoning fold (an internal row state change, not a data change) re-arm a
  // native-geometry settle loop, so the pane converges to the true bottom.
  const scrollerElRef = useRef<HTMLDivElement | null>(null);
  const setScroller = useCallback((node: HTMLElement | Window | null) => {
    scrollerElRef.current = node as HTMLDivElement | null;
    scrollerRef?.(node);
  }, [scrollerRef]);
  const virtuosoObjectRef = listRef && typeof listRef === "object" ? listRef : null;
  const { onUserGesture, reaim } = usePaneTailFollow({
    virtuosoRef: virtuosoObjectRef,
    scrollerRef: scrollerElRef,
    contentVersion: turns,
  });
  const onUserGestureCapture = useCallback(() => {
    onUserGesture();
    onUserInteract?.();
  }, [onUserGesture, onUserInteract]);

  return (
    <Virtuoso<ProcessPaneTurn>
      ref={listRef}
      className="process-pane"
      data={turns}
      computeItemKey={(_index, turn) => turn.key}
      itemContent={itemContent}
      firstItemIndex={firstItemIndex}
      increaseViewportBy={{ top: 320, bottom: 320 }}
      totalListHeightChanged={reaim}
      onWheelCapture={onUserGestureCapture}
      onTouchStartCapture={onUserGestureCapture}
      onPointerDownCapture={onUserGestureCapture}
      scrollerRef={setScroller}
    />
  );
}
