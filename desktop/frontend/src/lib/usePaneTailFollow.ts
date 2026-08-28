// usePaneTailFollow — the split panes' tail writer. ConversationPane and
// ProcessPane mount bare Virtuoso lists whose followOutput stops converging on
// async row growth (worker markdown, images) and on reasoning folds. This
// reuses the single-column transcript's native-geometry settle loop
// (createTranscriptTailSettle) with a pane-local pinned/mode track instead of
// the full arbiter state machine.

import { useCallback, useEffect, useRef, type RefObject } from "react";
import type { VirtuosoHandle } from "react-virtuoso";
import { createTranscriptTailSettle, type TranscriptTailSettle } from "./transcriptTailSettle";
import type { TranscriptScrollMode } from "./transcriptScrollArbiter";
import { nativeTranscriptDistanceFromBottom, TRANSCRIPT_AT_BOTTOM_THRESHOLD_PX } from "./transcriptScrollGeometry";

export function usePaneTailFollow({
  virtuosoRef,
  scrollerRef,
  contentVersion,
  enabled = true,
}: {
  virtuosoRef: RefObject<VirtuosoHandle | null> | null;
  scrollerRef: RefObject<HTMLDivElement | null>;
  /** New identity per stream chunk / fold-state switch (the pane turns array). */
  contentVersion: unknown;
  /** False while hydrating: restoration owns the viewport, tail writes would fight it. */
  enabled?: boolean;
}) {
  const modeRef = useRef<TranscriptScrollMode>("tail-follow");
  const generationRef = useRef(0);
  const layoutTransientRef = useRef(false);
  const enabledRef = useRef(enabled);
  enabledRef.current = enabled;

  const settleRef = useRef<TranscriptTailSettle | null>(null);
  settleRef.current ??= createTranscriptTailSettle({
    virtuosoRef: virtuosoRef ?? { current: null },
    scrollRef: scrollerRef,
    modeRef,
    generationRef,
    layoutTransientRef,
  });
  const settle = settleRef.current;

  // A user gesture re-samples the native distance: at the bottom it keeps
  // tail ownership, anywhere above it releases the tail until the user
  // returns. Mirrors the arbiter's USER_SCROLL_INTENT without the reader
  // intent window.
  const onUserGesture = useCallback(() => {
    const element = scrollerRef.current;
    if (!element) return;
    modeRef.current = nativeTranscriptDistanceFromBottom(element) <= TRANSCRIPT_AT_BOTTOM_THRESHOLD_PX
      ? "tail-follow"
      : "manual";
  }, [scrollerRef]);

  const reaim = useCallback(() => {
    if (!enabledRef.current || modeRef.current !== "tail-follow") return;
    // The settle loop re-checks native distance every frame and skips
    // sub-threshold jitter and fold shrinks (its own tailPinned guard).
    settle.schedule(false);
  }, [settle]);

  // Data identity: stream chunks rebuild the turns array; each change is a
  // growth signal (a no-op when nothing grew — schedule re-checks distance).
  useEffect(() => {
    reaim();
  }, [contentVersion, reaim]);

  useEffect(() => () => {
    generationRef.current += 1;
    settle.cancel();
  }, [settle]);

  return { onUserGesture, reaim };
}
