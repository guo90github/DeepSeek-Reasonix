// ConversationPane — the split layout's left column: one card per turn with
// the question bubble and every final answer, newest turn expanded by default
// and history collapsed to a header row. Reuses the transcript row components
// (UserMessage / LiveAssistantMessage / NoticeCard / SteerCard / ExtensionCard)
// so markdown and live streaming behave identically to the single column.

import { useCallback, useEffect, useMemo, useRef, useState, type Ref } from "react";
import { Virtuoso, type VirtuosoHandle } from "react-virtuoso";
import { useT } from "../lib/i18n";
import { usePaneTailFollow } from "../lib/usePaneTailFollow";
import { isSteerNoticeText } from "../lib/useController";
import type { ConversationPaneTurn } from "../lib/transcriptPanes";
import { UserMessage } from "./Message";
import { LiveAssistantMessage } from "./TranscriptVirtuosoParts";
import { NoticeCard, SteerCard } from "./TranscriptCards";
import { ExtensionCard } from "./ExtensionCard";
import { TurnBadge } from "./ProcessPane";

function ConversationTurnCard({
  turn,
  running,
  open,
  onToggle,
}: {
  turn: ConversationPaneTurn;
  running: boolean;
  open: boolean;
  onToggle: () => void;
}) {
  const t = useT();
  const question = turn.user?.text ?? "";
  return (
    <article className={["conversation-pane__turn", open ? "conversation-pane__turn--open" : "conversation-pane__turn--collapsed"].filter(Boolean).join(" ")} data-turn={turn.turn ?? ""} data-turn-key={turn.key}>
      <header className="conversation-pane__turn-head" role="button" tabIndex={0} aria-expanded={open} onClick={onToggle} onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onToggle();
        }
      }}>
        {turn.turn !== undefined && <TurnBadge turn={turn.turn} />}
        <span className="conversation-pane__turn-question" title={question}>{question || (turn.hasShownContent ? "" : t("split.emptyTurn"))}</span>
      </header>
      {open && (
        <div className="conversation-pane__turn-body">
          {turn.user && (
            <UserMessage
              id={turn.user.id}
              text={turn.user.text}
              submitText={turn.user.submitText}
              failed={turn.user.failed}
              createdAt={turn.user.createdAt}
              turn={turn.turn}
            />
          )}
          {turn.answers.map((item) => {
            if (item.kind === "assistant") {
              return (
                <LiveAssistantMessage
                  key={item.id}
                  item={item}
                  defaultExpanded={false}
                  expandWhileStreaming={false}
                  creationMode={false}
                  reasoningDisplay="hide"
                />
              );
            }
            if (item.kind === "notice") {
              if (isSteerNoticeText(item.text)) return <SteerCard key={item.id} id={item.id} text={item.text} />;
              return <NoticeCard key={item.id} item={item} actionDisabled={running} />;
            }
            return <ExtensionCard key={item.id} item={item} tabId={undefined} />;
          })}
          {turn.answers.length === 0 && turn.user && turn.isActive && (
            <div className="conversation-pane__empty-answer">{t("split.awaitingAnswer")}</div>
          )}
        </div>
      )}
    </article>
  );
}

export function ConversationPane({
  turns,
  running,
  footerHeight,
  hasOlderHistory,
  loadingOlderHistory,
  olderHistoryError,
  onLoadOlderHistory,
  hydrating,
  listRef,
  onUserInteract,
  scrollerRef,
}: {
  turns: readonly ConversationPaneTurn[];
  running: boolean;
  footerHeight: number;
  hasOlderHistory?: boolean;
  loadingOlderHistory?: boolean;
  olderHistoryError?: string;
  onLoadOlderHistory?: () => void;
  hydrating?: boolean;
  listRef?: Ref<VirtuosoHandle>;
  onUserInteract?: () => void;
  scrollerRef?: (node: HTMLElement | Window | null) => void;
}) {
  const t = useT();
  // User overrides win; unoverridden turns follow the role default (newest
  // expanded, history collapsed).
  const [overrides, setOverrides] = useState<ReadonlyMap<string, boolean>>(new Map());
  const toggle = useCallback((key: string) => {
    setOverrides((current) => {
      const next = new Map(current);
      next.set(key, !(next.get(key) ?? false));
      return next;
    });
  }, []);

  // Backfill the full session on mount so both panes start at the real first
  // turn instead of mid-conversation (the backend only pages the tail).
  useEffect(() => {
    if (hydrating || !hasOlderHistory || loadingOlderHistory || olderHistoryError) return;
    onLoadOlderHistory?.();
  }, [hasOlderHistory, hydrating, loadingOlderHistory, olderHistoryError, onLoadOlderHistory]);

  const olderHeader = hasOlderHistory || loadingOlderHistory || olderHistoryError ? (
    <div className="conversation-pane__older">
      {loadingOlderHistory ? (
        <span>{t("common.loading")}</span>
      ) : olderHistoryError ? (
        <>
          <span>{olderHistoryError}</span>
          <button type="button" className="btn btn--small" onClick={() => onLoadOlderHistory?.()}>{t("common.retry")}</button>
        </>
      ) : null}
    </div>
  ) : null;

  const itemContent = useCallback((index: number, turn: ConversationPaneTurn) => (
    <ConversationTurnCard
      turn={turn}
      running={running}
      open={overrides.get(turn.key) ?? (turn.isActive || index === turns.length - 1)}
      onToggle={() => toggle(turn.key)}
    />
  ), [overrides, running, toggle, turns.length]);

  const listComponents = useMemo(() => ({
    Header: () => olderHeader,
    Footer: () => <div className="conversation-pane__spacer" style={footerHeight > 0 ? { height: footerHeight } : undefined} />,
  }), [footerHeight, olderHeader]);

  // Tail-follow replaces Virtuoso's followOutput: stream chunks and async row
  // growth (worker markdown, images) re-arm a native-geometry settle loop, so
  // the pane converges to the true bottom instead of parking above it.
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
    enabled: !hydrating,
  });
  const onUserGestureCapture = useCallback(() => {
    onUserGesture();
    onUserInteract?.();
  }, [onUserGesture, onUserInteract]);

  return (
    <Virtuoso<ConversationPaneTurn>
      ref={listRef}
      className="conversation-pane"
      data={turns}
      computeItemKey={(_index, turn) => turn.key}
      itemContent={itemContent}
      components={listComponents}
      increaseViewportBy={{ top: 320, bottom: 320 }}
      totalListHeightChanged={reaim}
      onWheelCapture={onUserGestureCapture}
      onTouchStartCapture={onUserGestureCapture}
      onPointerDownCapture={onUserGestureCapture}
      scrollerRef={setScroller}
    />
  );
}
