// usePaneTailFollow — the split panes' tail writer. ConversationPane and
// ProcessPane mount bare Virtuoso lists whose followOutput stops converging on
// async row growth (worker markdown, images) and on reasoning folds. This
// reuses the single-column transcript's native-geometry settle loop
// (createTranscriptTailSettle) with a pane-local pinned/mode track instead of
// the full arbiter state machine.

import { useCallback, useEffect, useRef, useState, type RefObject } from "react";
import type { VirtuosoHandle } from "react-virtuoso";
import { createTranscriptTailSettle, type TranscriptTailSettle } from "./transcriptTailSettle";
import { createTranscriptScrollWriter } from "./transcriptScrollWriter";
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
  const ownershipEpochRef = useRef(0);
  const geometryRevisionRef = useRef(0);
  const layoutTransientRef = useRef(false);
  const enabledRef = useRef(enabled);
  enabledRef.current = enabled;
  const wasEnabledRef = useRef(enabled);

  const settleRef = useRef<TranscriptTailSettle | null>(null);
  settleRef.current ??= createTranscriptTailSettle({
    writer: createTranscriptScrollWriter({
      virtuosoRef: virtuosoRef ?? { current: null },
      scrollRef: scrollerRef,
      modeRef,
      generationRef,
      ownershipEpochRef,
      geometryRevisionRef,
    }),
    scrollRef: scrollerRef,
    modeRef,
    generationRef,
    ownershipEpochRef,
    geometryRevisionRef,
    layoutTransientRef,
  });
  const settle = settleRef.current;

  // Virtuoso can mount its scroller one commit after the pane (its own
  // internal pass), so the first passive effects here may run with
  // scrollerRef.current still null — and their deps never re-run. Mirror the
  // ref into state via a bounded frame poll so the effects that must attach
  // to the DOM (scroll re-arm, growth detector) arm on the LATE arrival, not
  // the first pass. The settle loop reads the ref lazily at write time and
  // is immune either way.
  const [scrollerNode, setScrollerNode] = useState<HTMLDivElement | null>(null);
  useEffect(() => {
    const current = scrollerRef.current;
    if (current) {
      setScrollerNode(current);
      return;
    }
    let cancelled = false;
    let frame = 0;
    const poll = () => {
      if (cancelled) return;
      const node = scrollerRef.current;
      if (node) setScrollerNode(node);
      else frame = requestAnimationFrame(poll);
    };
    frame = requestAnimationFrame(poll);
    return () => {
      cancelled = true;
      cancelAnimationFrame(frame);
    };
  }, [scrollerRef]);

  // A user gesture re-samples the native distance: at the bottom it keeps
  // tail ownership, anywhere above it releases the tail until the user
  // returns. Mirrors the arbiter's USER_SCROLL_INTENT without the reader
  // intent window.
  const sampleUserGesture = useCallback(() => {
    const element = scrollerRef.current;
    if (!element) return;
    modeRef.current = nativeTranscriptDistanceFromBottom(element) <= TRANSCRIPT_AT_BOTTOM_THRESHOLD_PX
      ? "tail-follow"
      : "manual";
  }, [scrollerRef]);
  const onUserGesture = sampleUserGesture;

  // Native scrolls re-arm the tail when the user returns to the bottom, but
  // never release it: Virtuoso's own layout scrolls (late row measurement,
  // range shifts) emit scroll events while the tail is short of the true
  // bottom — treating those as a manual release froze auto-follow mid-growth.
  // Only real gestures (wheel/touch/pointer captures above) pause the tail.
  const rearmFromScroll = useCallback(() => {
    const element = scrollerRef.current;
    if (!element) return;
    if (modeRef.current !== "tail-follow"
      && nativeTranscriptDistanceFromBottom(element) <= TRANSCRIPT_AT_BOTTOM_THRESHOLD_PX) {
      modeRef.current = "tail-follow";
    }
  }, [scrollerRef]);
  useEffect(() => {
    if (!scrollerNode) return;
    scrollerNode.addEventListener("scroll", rearmFromScroll, { passive: true });
    return () => scrollerNode.removeEventListener("scroll", rearmFromScroll);
  }, [rearmFromScroll, scrollerNode]);

  const reaim = useCallback(() => {
    if (!enabledRef.current || modeRef.current !== "tail-follow") return;
    // Single-column advances the geometry revision per layout event via
    // note(); the pane has no arbiter, so without a bump here every settle
    // loop after the session's first would burn its budget re-submitting
    // writes under frame keys the dedup already consumed. Treat each accepted
    // growth signal as one new revision — the settle still re-checks native
    // distance, so a no-op signal costs one revision and no write.
    geometryRevisionRef.current += 1;
    settle.schedule(false);
  }, [settle]);

  // Growth detector independent of Virtuoso's callbacks: card folds, async
  // renders, and late row commits grow the list container even when no
  // totalListHeightChanged surfaces. A ResizeObserver cannot see it — the
  // pane scroller's first child is Virtuoso's fixed-height viewport layer,
  // whose growing content overflows inside — so watch the DOM subtree
  // instead. Folds/unfolds mount/unmount the card body (childList); streamed
  // text edits it (characterData). rAF-coalesced, and reaim's own guards
  // (manual mode, hydrating, settle's distance re-check) keep shrink-only
  // mutations at one idle frame with no write.
  useEffect(() => {
    if (!scrollerNode || typeof MutationObserver === "undefined") return;
    const content = scrollerNode.firstElementChild;
    if (!content) return;
    let scheduled = false;
    const observer = new MutationObserver(() => {
      if (scheduled) return;
      scheduled = true;
      requestAnimationFrame(() => {
        scheduled = false;
        reaim();
      });
    });
    observer.observe(content, { childList: true, subtree: true, characterData: true });
    return () => observer.disconnect();
  }, [reaim, scrollerNode]);

  // Data identity: stream chunks rebuild the turns array; each change is a
  // growth signal (a no-op when nothing grew — schedule re-checks distance).
  useEffect(() => {
    reaim();
  }, [contentVersion, reaim]);

  // Re-arm when the writer turns on (e.g. hydration/transition ends on the
  // left pane): the pane is otherwise left parked wherever restoration put it,
  // with no data change left to re-aim it. Enabled→false is already handled —
  // reaim's own guard keeps the loop inert while disabled.
  useEffect(() => {
    const turnedOn = !wasEnabledRef.current && enabled;
    wasEnabledRef.current = enabled;
    if (turnedOn) reaim();
  }, [enabled, reaim]);

  useEffect(() => () => {
    generationRef.current += 1;
    settle.cancel();
  }, [settle]);

  return { onUserGesture, reaim };
}
