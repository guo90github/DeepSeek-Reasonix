// SplitWorkspace — the desktop "split" layout surface: a conversation pane
// (questions + final answers) beside a process pane (reasoning + tool calls),
// anchored turn-to-turn on the same TurnModel[]. The wrapper dissolves into the
// .chat-pane grid (display:contents) so the footer/composer keeps a dedicated
// bottom row under the conversation column only; the process column spans the
// full height. Deliberately reuses the transcript row components instead of
// re-implementing markdown/streaming.

import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore, type PointerEvent as ReactPointerEvent } from "react";
import { type VirtuosoHandle } from "react-virtuoso";
import { useT } from "../lib/i18n";
import { LiveStreamContext } from "./LiveStreamContext";
import { ConversationPane } from "./ConversationPane";
import { ProcessPane } from "./ProcessPane";
import { buildTurnModels, NO_LIVE, type TranscriptLiveFlags } from "../lib/transcriptRows";
import { conversationPaneTurns, processPaneTurns } from "../lib/transcriptPanes";
import { advanceSurfacePaintCommit, type SurfacePaintProgress } from "../lib/navigationSurfaceTransition";
import type { ControllerLiveStore, Item, LiveStream } from "../lib/useController";

export const SPLIT_PROCESS_WIDTH_STORAGE_KEY = "reasonix-split-process-width";
export const SPLIT_PROCESS_WIDTH_MIN = 0.4;
export const SPLIT_PROCESS_WIDTH_MAX = 0.6;
export const SPLIT_PROCESS_WIDTH_DEFAULT = 0.5;
const SPLIT_NARROW_QUERY = "(max-width: 1100px)";

function loadSplitProcessWidth(): number {
  try {
    const raw = Number.parseFloat(localStorage.getItem(SPLIT_PROCESS_WIDTH_STORAGE_KEY) ?? "");
    if (Number.isFinite(raw)) {
      return Math.min(SPLIT_PROCESS_WIDTH_MAX, Math.max(SPLIT_PROCESS_WIDTH_MIN, raw));
    }
  } catch {
    // Storage may be unavailable in hardened webviews.
  }
  return SPLIT_PROCESS_WIDTH_DEFAULT;
}

export function SplitWorkspace({
  items,
  live,
  liveStore,
  tabId,
  running,
  footerHeight = 0,
  hasOlderHistory,
  loadingOlderHistory,
  olderHistoryError,
  onLoadOlderHistory,
  hydrating = false,
  surfaceCommitToken,
  onSurfacePaintReady,
}: {
  items: readonly Item[];
  live?: LiveStream;
  liveStore?: ControllerLiveStore;
  tabId?: string;
  running: boolean;
  footerHeight?: number;
  hasOlderHistory?: boolean;
  loadingOlderHistory?: boolean;
  olderHistoryError?: string;
  onLoadOlderHistory?: () => void;
  hydrating?: boolean;
  surfaceCommitToken?: string;
  onSurfacePaintReady?: (token: string, outcome: "ready" | "degraded") => void;
}) {
  const t = useT();
  const liveStream = useSyncExternalStore(
    useCallback((listener: () => void) => liveStore?.subscribe(tabId, listener) ?? (() => {}), [liveStore, tabId]),
    () => liveStore?.getSnapshot(tabId) ?? live,
    () => liveStore?.getSnapshot(tabId) ?? live,
  );

  // Split styles load as a lazy chunk so the app-shell budget stays flat.
  useEffect(() => {
    void import("./splitWorkspace.css");
  }, []);

  const liveFlags: TranscriptLiveFlags = useMemo(() => {
    if (!liveStream) return NO_LIVE;
    return {
      id: liveStream.id,
      hasAnswerText: liveStream.text.length > 0,
      hasReasoning: liveStream.reasoning.length > 0,
      reasoningComplete: liveStream.reasoningComplete,
    };
  }, [liveStream]);

  const models = useMemo(() => buildTurnModels(items, liveFlags, running, false), [items, liveFlags, running]);
  const conversationTurns = useMemo(() => conversationPaneTurns(models), [models]);
  const processTurns = useMemo(() => processPaneTurns(models, liveFlags), [models, liveFlags]);

  const [processWidth, setProcessWidth] = useState<number>(loadSplitProcessWidth);
  const [processWidthResizing, setProcessWidthResizing] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const chatPaneRef = useRef<HTMLElement | null>(null);

  // The grid that consumes --split-process-width is .chat-pane, an ancestor of
  // this display:contents wrapper — custom properties only inherit downward,
  // so write the variable onto the grid container itself.
  useEffect(() => {
    const pane = rootRef.current?.closest(".chat-pane") as HTMLElement | null;
    pane?.style.setProperty("--split-process-width", `${Math.round(processWidth * 100)}%`);
  }, [processWidth]);

  const conversationRef = useRef<VirtuosoHandle>(null);
  const processRef = useRef<VirtuosoHandle>(null);
  const convScrollerRef = useRef<HTMLDivElement | null>(null);
  const procScrollerRef = useRef<HTMLDivElement | null>(null);
  const setConvScroller = useCallback((node: HTMLElement | Window | null) => {
    convScrollerRef.current = node as HTMLDivElement | null;
  }, []);
  const setProcScroller = useCallback((node: HTMLElement | Window | null) => {
    procScrollerRef.current = node as HTMLDivElement | null;
  }, []);
  const userInteractedRef = useRef(false);
  const markUserInteracted = useCallback(() => {
    userInteractedRef.current = true;
  }, []);

  // Navigation paint gate: sample both panes on rAF until the layout settles
  // (2 stable frames at the bottom) or 180 attempts degrade — otherwise the
  // runtimeTransitioning navigation waits out the full degraded timeout.
  useEffect(() => {
    if (!surfaceCommitToken || hydrating || !onSurfacePaintReady) return;
    let frame: number | null = null;
    let cancelled = false;
    let progress: SurfacePaintProgress = { attempts: 0, stableFrames: 0 };
    const tick = () => {
      frame = null;
      if (cancelled) return;
      const conv = convScrollerRef.current;
      const proc = procScrollerRef.current;
      const rendered = conversationTurns.length === 0 || Boolean(conv && proc);
      const placementReady = Boolean(conv && proc);
      const bottomReady = Boolean(conv && proc && conv.scrollHeight - conv.scrollTop - conv.clientHeight <= 40);
      const decision = advanceSurfacePaintCommit(progress, {
        rendered,
        placementReady,
        geometryReady: bottomReady,
        geometryKey: conv && proc ? `${conv.scrollHeight}x${conv.clientHeight}|${proc.scrollHeight}` : undefined,
      });
      progress = decision.progress;
      if (decision.outcome) {
        onSurfacePaintReady(surfaceCommitToken, decision.outcome);
        return;
      }
      frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => {
      cancelled = true;
      if (frame !== null) cancelAnimationFrame(frame);
    };
  }, [conversationTurns.length, hydrating, onSurfacePaintReady, surfaceCommitToken]);

  // Narrow viewports collapse the process pane into an overlay drawer so the
  // conversation keeps a usable width (40% of a 900px chat area is too thin).
  const [isNarrow, setIsNarrow] = useState<boolean>(() => window.matchMedia(SPLIT_NARROW_QUERY).matches);
  const [processOpen, setProcessOpen] = useState(false);
  useEffect(() => {
    const mq = window.matchMedia(SPLIT_NARROW_QUERY);
    const onChange = (event: MediaQueryListEvent) => {
      setIsNarrow(event.matches);
      if (!event.matches) setProcessOpen(false);
    };
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);

  // Pin both panes to the newest turn once the lists are measurable. A single
  // scrollToIndex no-ops pre-measurement on heavy sessions, so retry briefly;
  // any user scroll or click ends the loop.
  const establishScheduledRef = useRef(false);
  useEffect(() => {
    if (establishScheduledRef.current) return;
    if (conversationTurns.length === 0) return;
    establishScheduledRef.current = true;
    const lastIndex = conversationTurns.length - 1;
    let attempts = 0;
    const timer = window.setInterval(() => {
      attempts += 1;
      if (userInteractedRef.current || attempts > 20) {
        window.clearInterval(timer);
        return;
      }
      conversationRef.current?.scrollToIndex({ index: lastIndex, align: "end" });
      processRef.current?.scrollToIndex({ index: lastIndex, align: "end" });
      if (attempts >= 20) window.clearInterval(timer);
    }, 120);
    return () => window.clearInterval(timer);
  }, [conversationTurns.length]);

  const beginProcessResize = useCallback((event: ReactPointerEvent) => {
    setProcessWidthResizing(true);
    chatPaneRef.current = (event.currentTarget as HTMLElement).closest(".chat-pane") as HTMLElement | null;
    event.currentTarget.setPointerCapture(event.pointerId);
  }, []);

  const onProcessResizeMove = useCallback((event: ReactPointerEvent) => {
    if (!processWidthResizing || !chatPaneRef.current) return;
    const rect = chatPaneRef.current.getBoundingClientRect();
    if (rect.width <= 0) return;
    const next = (rect.right - event.clientX) / rect.width;
    setProcessWidth(Math.min(SPLIT_PROCESS_WIDTH_MAX, Math.max(SPLIT_PROCESS_WIDTH_MIN, next)));
  }, [processWidthResizing]);

  const endProcessResize = useCallback(() => {
    if (!processWidthResizing) return;
    setProcessWidthResizing(false);
    chatPaneRef.current = null;
    try {
      localStorage.setItem(SPLIT_PROCESS_WIDTH_STORAGE_KEY, String(processWidth));
    } catch {
      // Storage may be unavailable; the in-memory value still applies.
    }
  }, [processWidth, processWidthResizing]);

  return (
    <div
      ref={rootRef}
      className={[
        "split-workspace",
        processWidthResizing ? "split-workspace--resizing" : "",
        isNarrow ? "split-workspace--narrow" : "",
        isNarrow && processOpen ? "split-workspace--process-open" : "",
      ].filter(Boolean).join(" ")}
    >
      <LiveStreamContext.Provider value={liveStream}>
        <ConversationPane
          turns={conversationTurns}
          running={running}
          footerHeight={footerHeight}
          hasOlderHistory={hasOlderHistory}
          loadingOlderHistory={loadingOlderHistory}
          olderHistoryError={olderHistoryError}
          onLoadOlderHistory={onLoadOlderHistory}
          hydrating={hydrating}
          listRef={conversationRef}
          onUserInteract={markUserInteracted}
          scrollerRef={setConvScroller}
        />
        <div
          className={["split-workspace__divider", processWidthResizing ? "split-workspace__divider--active" : ""].filter(Boolean).join(" ")}
          role="separator"
          aria-orientation="vertical"
          aria-label={t("split.resizeProcessPane")}
          aria-valuemin={Math.round(SPLIT_PROCESS_WIDTH_MIN * 100)}
          aria-valuemax={Math.round(SPLIT_PROCESS_WIDTH_MAX * 100)}
          aria-valuenow={Math.round(processWidth * 100)}
          onPointerDown={beginProcessResize}
          onPointerMove={onProcessResizeMove}
          onPointerUp={endProcessResize}
          onPointerCancel={endProcessResize}
        />
        <div className="process-pane-host">
          <ProcessPane
            turns={processTurns}
            listRef={processRef}
            onUserInteract={markUserInteracted}
            scrollerRef={setProcScroller}
          />
        </div>
        {isNarrow && processOpen && (
          <div className="split-workspace__process-backdrop" onClick={() => setProcessOpen(false)} />
        )}
        {isNarrow && (
          <button
            type="button"
            className={["split-workspace__process-toggle", processOpen ? "split-workspace__process-toggle--active" : ""].filter(Boolean).join(" ")}
            aria-label={t("split.toggleProcessPane")}
            aria-expanded={processOpen}
            onClick={() => setProcessOpen((open) => !open)}
          >
            {t("split.toggleProcessPane")}
          </button>
        )}
      </LiveStreamContext.Provider>
    </div>
  );
}
