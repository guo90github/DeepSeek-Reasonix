// SplitWorkspace — the desktop "split" layout surface: a conversation pane
// (questions + final answers) beside a process pane (reasoning + tool calls),
// anchored turn-to-turn on the same TurnModel[]. The wrapper dissolves into the
// .chat-pane grid (display:contents) so the footer/composer keeps a dedicated
// bottom row under the conversation column only; the process column spans the
// full height. Deliberately reuses the transcript row components instead of
// re-implementing markdown/streaming.

import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore, type KeyboardEvent as ReactKeyboardEvent, type PointerEvent as ReactPointerEvent } from "react";
import { type VirtuosoHandle } from "react-virtuoso";
import { useT } from "../lib/i18n";
import { LiveStreamContext } from "./LiveStreamContext";
import { ConversationPane } from "./ConversationPane";
import { ProcessPane } from "./ProcessPane";
import { buildTurnModels, NO_LIVE, type TranscriptLiveFlags } from "../lib/transcriptRows";
import { conversationPaneTurns, processPaneTurns } from "../lib/transcriptPanes";
import { resolveAutoSplitProcessWidth, snapSplitWidth, stepSplitWidth } from "../lib/splitDivider";
import { attachPaneParallax } from "../lib/paneParallax";
import { advanceSurfacePaintCommit, type SurfacePaintProgress } from "../lib/navigationSurfaceTransition";
import type { ControllerLiveStore, Item, LiveStream } from "../lib/useController";

export const SPLIT_PROCESS_WIDTH_STORAGE_KEY = "reasonix-split-process-width";
export const SPLIT_PROCESS_MODE_STORAGE_KEY = "reasonix-split-process-mode";
export const SPLIT_PROCESS_WIDTH_MIN = 0.4;
export const SPLIT_PROCESS_WIDTH_MAX = 0.6;
export const SPLIT_PROCESS_WIDTH_DEFAULT = 0.5;
export const SPLIT_PROCESS_WIDTH_AUTO_IDLE = 0.45;
const SPLIT_NARROW_QUERY = "(max-width: 1100px)";
const SPLIT_SNAP_POINTS = [SPLIT_PROCESS_WIDTH_MIN, SPLIT_PROCESS_WIDTH_DEFAULT, SPLIT_PROCESS_WIDTH_MAX];
const SPLIT_SNAP_THRESHOLD_PX = 12;
const SPLIT_KEYBOARD_STEP = 0.05;
const SPLIT_GLOW_MS = 2000;
const SPLIT_DRAWER_OPEN_FRACTION = 0.4;

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

// Auto is the default surface; a stored manual width (the user has dragged the
// divider before) keeps manual so their choice survives an upgrade.
function loadSplitProcessMode(): "auto" | "manual" {
  try {
    const raw = localStorage.getItem(SPLIT_PROCESS_MODE_STORAGE_KEY);
    if (raw === "manual") return "manual";
    if (raw === "auto") return "auto";
    return localStorage.getItem(SPLIT_PROCESS_WIDTH_STORAGE_KEY) !== null ? "manual" : "auto";
  } catch {
    return "auto";
  }
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
  const [processWidthMode, setProcessWidthMode] = useState<"auto" | "manual">(loadSplitProcessMode);
  const [processWidthResizing, setProcessWidthResizing] = useState(false);
  const [dividerGlow, setDividerGlow] = useState(false);
  const [snapFlash, setSnapFlash] = useState(false);
  const [badgeY, setBadgeY] = useState(0);
  const [drawerDrag, setDrawerDrag] = useState<{ x: number; startX: number; fromOpen: boolean } | null>(null);
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);
  const glowTimerRef = useRef<number | null>(null);
  const flashTimerRef = useRef<number | null>(null);
  const dividerRef = useRef<HTMLDivElement>(null);
  const rootRef = useRef<HTMLDivElement>(null);
  const chatPaneRef = useRef<HTMLElement | null>(null);
  const processHostRef = useRef<HTMLDivElement>(null);
  const drawerDragRef = useRef<{ x: number; startX: number; fromOpen: boolean } | null>(null);

  // In auto mode the effective width tracks the live stream (widen while
  // reasoning streams, idle otherwise); manual mode pins the user's width.
  const effectiveProcessWidth = useMemo(
    () =>
      processWidthMode === "auto"
        ? resolveAutoSplitProcessWidth({
            hasReasoning: liveFlags.hasReasoning,
            reasoningComplete: liveFlags.reasoningComplete === true,
            activeWidth: SPLIT_PROCESS_WIDTH_MAX,
            idleWidth: SPLIT_PROCESS_WIDTH_AUTO_IDLE,
          })
        : processWidth,
    [liveFlags.hasReasoning, liveFlags.reasoningComplete, processWidth, processWidthMode],
  );

  // Auto width changes glide through the same grid-track transition; a manual
  // drag must stay 1:1, so the resizing class (checked via :has in CSS) freezes
  // the transition while the pointer is down.
  useEffect(() => {
    const pane = rootRef.current?.closest(".chat-pane") as HTMLElement | null;
    pane?.style.setProperty("--split-process-width", `${Math.round(effectiveProcessWidth * 100)}%`);
  }, [effectiveProcessWidth]);

  // Glow lingers 2s after the last divider interaction so the affordance does
  // not vanish the instant a drag ends; snap flash is a short-lived pop.
  useEffect(() => {
    return () => {
      if (glowTimerRef.current !== null) window.clearTimeout(glowTimerRef.current);
      if (flashTimerRef.current !== null) window.clearTimeout(flashTimerRef.current);
      convParallaxDetachRef.current?.();
      procParallaxDetachRef.current?.();
    };
  }, []);

  const scheduleDividerGlow = useCallback(() => {
    setDividerGlow(true);
    if (glowTimerRef.current !== null) window.clearTimeout(glowTimerRef.current);
    glowTimerRef.current = window.setTimeout(() => setDividerGlow(false), SPLIT_GLOW_MS);
  }, []);

  const flashSnap = useCallback(() => {
    setSnapFlash(true);
    if (flashTimerRef.current !== null) window.clearTimeout(flashTimerRef.current);
    flashTimerRef.current = window.setTimeout(() => setSnapFlash(false), 260);
  }, []);

  const conversationRef = useRef<VirtuosoHandle>(null);
  const processRef = useRef<VirtuosoHandle>(null);
  const convScrollerRef = useRef<HTMLDivElement | null>(null);
  const procScrollerRef = useRef<HTMLDivElement | null>(null);
  const convParallaxDetachRef = useRef<(() => void) | undefined>(undefined);
  const procParallaxDetachRef = useRef<(() => void) | undefined>(undefined);
  const setConvScroller = useCallback((node: HTMLElement | Window | null) => {
    convScrollerRef.current = node as HTMLDivElement | null;
    convParallaxDetachRef.current?.();
    convParallaxDetachRef.current = attachPaneParallax(node as HTMLDivElement | null);
  }, []);
  const setProcScroller = useCallback((node: HTMLElement | Window | null) => {
    procScrollerRef.current = node as HTMLDivElement | null;
    procParallaxDetachRef.current?.();
    procParallaxDetachRef.current = attachPaneParallax(node as HTMLDivElement | null);
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
    setProcessWidthMode("manual");
    setProcessWidth(effectiveProcessWidth);
    setProcessWidthResizing(true);
    setDividerGlow(true);
    if (glowTimerRef.current !== null) window.clearTimeout(glowTimerRef.current);
    chatPaneRef.current = (event.currentTarget as HTMLElement).closest(".chat-pane") as HTMLElement | null;
    event.currentTarget.setPointerCapture(event.pointerId);
  }, [effectiveProcessWidth]);

  const onProcessResizeMove = useCallback((event: ReactPointerEvent) => {
    if (!processWidthResizing || !chatPaneRef.current) return;
    const rect = chatPaneRef.current.getBoundingClientRect();
    if (rect.width <= 0) return;
    const next = (rect.right - event.clientX) / rect.width;
    const clamped = Math.min(SPLIT_PROCESS_WIDTH_MAX, Math.max(SPLIT_PROCESS_WIDTH_MIN, next));
    const snapped = snapSplitWidth({
      next: clamped,
      panelWidth: rect.width,
      thresholdPx: SPLIT_SNAP_THRESHOLD_PX,
      snapPoints: SPLIT_SNAP_POINTS,
    });
    if (snapped !== clamped) flashSnap();
    setProcessWidth(snapped);
    const dividerTop = dividerRef.current?.getBoundingClientRect().top ?? 0;
    setBadgeY(event.clientY - dividerTop);
  }, [flashSnap, processWidthResizing]);

  const endProcessResize = useCallback(() => {
    if (!processWidthResizing) return;
    setProcessWidthResizing(false);
    scheduleDividerGlow();
    chatPaneRef.current = null;
    try {
      localStorage.setItem(SPLIT_PROCESS_WIDTH_STORAGE_KEY, String(processWidth));
      localStorage.setItem(SPLIT_PROCESS_MODE_STORAGE_KEY, "manual");
    } catch {
      // Storage may be unavailable; the in-memory value still applies.
    }
  }, [processWidth, processWidthResizing, scheduleDividerGlow]);

  const persistSplitProcessWidth = useCallback((width: number) => {
    try {
      localStorage.setItem(SPLIT_PROCESS_WIDTH_STORAGE_KEY, String(width));
      localStorage.setItem(SPLIT_PROCESS_MODE_STORAGE_KEY, "manual");
    } catch {
      // Storage may be unavailable; the in-memory value still applies.
    }
  }, []);

  const restoreAutoProcessWidth = useCallback(() => {
    setProcessWidthMode("auto");
    try {
      localStorage.setItem(SPLIT_PROCESS_MODE_STORAGE_KEY, "auto");
    } catch {
      // Storage may be unavailable; the in-memory value still applies.
    }
    scheduleDividerGlow();
  }, [scheduleDividerGlow]);

  const resetSplitProcessWidth = useCallback(() => {
    setProcessWidthMode("manual");
    setProcessWidth(SPLIT_PROCESS_WIDTH_DEFAULT);
    persistSplitProcessWidth(SPLIT_PROCESS_WIDTH_DEFAULT);
    scheduleDividerGlow();
  }, [persistSplitProcessWidth, scheduleDividerGlow]);

  const onDividerKeyDown = useCallback((event: ReactKeyboardEvent<HTMLDivElement>) => {
    const direction = event.key === "ArrowLeft" ? -1 : event.key === "ArrowRight" ? 1 : 0;
    if (direction === 0) return;
    event.preventDefault();
    const next = stepSplitWidth({
      current: effectiveProcessWidth,
      direction,
      step: SPLIT_KEYBOARD_STEP,
      min: SPLIT_PROCESS_WIDTH_MIN,
      max: SPLIT_PROCESS_WIDTH_MAX,
    });
    setProcessWidthMode("manual");
    setProcessWidth(next);
    persistSplitProcessWidth(next);
    scheduleDividerGlow();
  }, [effectiveProcessWidth, persistSplitProcessWidth, scheduleDividerGlow]);

  // Narrow drawer: a right-edge grab zone starts a drag that follows the
  // pointer; releasing past the open fraction commits the new state, otherwise
  // the drawer springs back. Drawer geometry is measured from the live host so
  // the threshold stays proportional to the actual drawer width.
  const beginDrawerDrag = useCallback((event: ReactPointerEvent, fromOpen: boolean) => {
    const width = processHostRef.current?.getBoundingClientRect().width ?? 0;
    const drag = { x: fromOpen ? 0 : width, startX: event.clientX, fromOpen };
    drawerDragRef.current = drag;
    setDrawerDrag(drag);
    event.currentTarget.setPointerCapture(event.pointerId);
  }, []);

  const onDrawerDragMove = useCallback((event: ReactPointerEvent) => {
    const drag = drawerDragRef.current;
    if (!drag || !processHostRef.current) return;
    const width = processHostRef.current.getBoundingClientRect().width;
    const delta = event.clientX - drag.startX;
    const next = drag.fromOpen ? Math.max(0, Math.min(width, delta)) : Math.max(0, Math.min(width, width + delta));
    const updated = { ...drag, x: next };
    drawerDragRef.current = updated;
    setDrawerDrag(updated);
  }, []);

  const endDrawerDrag = useCallback((event: ReactPointerEvent) => {
    const drag = drawerDragRef.current;
    drawerDragRef.current = null;
    setDrawerDrag(null);
    if (!drag || !processHostRef.current) return;
    const width = processHostRef.current.getBoundingClientRect().width;
    if (width <= 0) return;
    // Directional commit: from closed, releasing past 40% revealed opens;
    // from open, dragging past 40% hidden closes. The other direction springs
    // back to the committed state.
    const revealed = width - drag.x;
    const open = drag.fromOpen ? revealed > width * (1 - SPLIT_DRAWER_OPEN_FRACTION) : revealed > width * SPLIT_DRAWER_OPEN_FRACTION;
    const wasClosed = !processOpen;
    setProcessOpen(open);
    if (open && wasClosed) {
      // Opening the drawer focuses the newest turn in the process pane.
      const last = processTurns.length - 1;
      if (last >= 0) {
        processRef.current?.scrollToIndex({ index: last, align: "end" });
      }
    }
    event.currentTarget.releasePointerCapture(event.pointerId);
  }, [processOpen, processTurns.length]);

  return (
    <div
      ref={rootRef}
      className={[
        "split-workspace",
        processWidthResizing ? "split-workspace--resizing" : "",
        isNarrow ? "split-workspace--narrow" : "",
        isNarrow && processOpen ? "split-workspace--process-open" : "",
        isNarrow && drawerDrag ? "split-workspace--drawer-dragging" : "",
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
          hoveredIndex={hoveredIndex}
          onHoverIndex={setHoveredIndex}
        />
        <div
          ref={dividerRef}
          className={[
            "split-workspace__divider",
            processWidthResizing ? "split-workspace__divider--active" : "",
            dividerGlow ? "split-workspace__divider--glow" : "",
          ].filter(Boolean).join(" ")}
          role="separator"
          aria-orientation="vertical"
          aria-label={t("split.resizeProcessPane")}
          aria-valuemin={Math.round(SPLIT_PROCESS_WIDTH_MIN * 100)}
          aria-valuemax={Math.round(SPLIT_PROCESS_WIDTH_MAX * 100)}
          aria-valuenow={Math.round(effectiveProcessWidth * 100)}
          tabIndex={0}
          onPointerDown={beginProcessResize}
          onPointerMove={onProcessResizeMove}
          onPointerUp={endProcessResize}
          onPointerCancel={endProcessResize}
          onKeyDown={onDividerKeyDown}
          onDoubleClick={resetSplitProcessWidth}
        >
          <span
            className={[
              "split-workspace__divider-badge",
              processWidthResizing || dividerGlow ? "split-workspace__divider-badge--visible" : "",
              snapFlash ? "split-workspace__divider-badge--snapped" : "",
            ].filter(Boolean).join(" ")}
            style={processWidthResizing ? { top: badgeY } : undefined}
          >
            {Math.round(effectiveProcessWidth * 100)}%
          </span>
          <button
            type="button"
            className={[
              "split-workspace__divider-auto",
              processWidthMode === "auto" ? "split-workspace__divider-auto--active" : "",
            ].filter(Boolean).join(" ")}
            aria-label={t("split.autoProcessPane")}
            aria-pressed={processWidthMode === "auto"}
            onClick={restoreAutoProcessWidth}
          />
        </div>
        <div
          ref={processHostRef}
          className="process-pane-host"
          style={drawerDrag ? { transform: `translateX(${drawerDrag.x}px)` } : undefined}
        >
          {isNarrow && processOpen && (
            <div
              className="split-workspace__drawer-handle"
              aria-hidden="true"
              onPointerDown={(event) => beginDrawerDrag(event, true)}
              onPointerMove={onDrawerDragMove}
              onPointerUp={endDrawerDrag}
              onPointerCancel={endDrawerDrag}
            />
          )}
          <ProcessPane
            turns={processTurns}
            listRef={processRef}
            onUserInteract={markUserInteracted}
            scrollerRef={setProcScroller}
            hoveredIndex={hoveredIndex}
            onHoverIndex={setHoveredIndex}
            tabId={tabId}
          />
        </div>
        {isNarrow && !processOpen && (
          <div
            className="split-workspace__drawer-grab"
            aria-hidden="true"
            onPointerDown={(event) => {
              // The right edge also hosts the transcript's native scrollbar;
              // only touch pointers open the drawer there, mouse users use the
              // toggle so scrollbar dragging is never hijacked.
              if (event.pointerType === "touch") beginDrawerDrag(event, false);
            }}
            onPointerMove={onDrawerDragMove}
            onPointerUp={endDrawerDrag}
            onPointerCancel={endDrawerDrag}
          />
        )}
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
