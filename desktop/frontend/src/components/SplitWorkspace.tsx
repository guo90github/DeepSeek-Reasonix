// SplitWorkspace composes the two turn-synced panes that replace the single
// transcript in the "split" desktop style: a clean conversation column (user +
// answer) and a per-turn process column (reasoning + tools). It reuses the
// live-stream wiring the Transcript uses, so streaming answers and process
// still render in real time. The Composer is NOT here — it stays in the App
// footer, and the split CSS grid places that footer under the CONVERSATION
// column (left), so growing the composer only compresses the conversation.

import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";

import { useReasoningDisplayMode } from "../lib/reasoningDisplayPreference";
import { buildConversationPaneTurns, buildProcessPaneTurns } from "../lib/transcriptPanes";
import { buildTurnModels, NO_LIVE, type ToolItem } from "../lib/transcriptRows";
import type { ControllerLiveStore, Item, LiveStream } from "../lib/useController";
import { useTurnPaneSync } from "../lib/useTurnPaneSync";

import { ConversationPane } from "./ConversationPane";
import { LiveStreamContext } from "./LiveStreamContext";
import { ProcessPane } from "./ProcessPane";

interface SplitWorkspaceProps {
  items: readonly Item[];
  live?: LiveStream;
  liveStore?: ControllerLiveStore;
  tabId?: string;
  running: boolean;
  creationMode: boolean;
  onEditPrompt?: (turn: number, displayText: string, submitText?: string) => boolean | void | Promise<boolean | void>;
  rewindDisabled: boolean;
  hasOlderHistory?: boolean;
  loadingOlderHistory?: boolean;
  olderHistoryError?: string;
  onLoadOlderHistory?: (targetTurn?: number) => boolean | Promise<boolean>;
}

export function SplitWorkspace({
  items,
  live: liveProp,
  liveStore,
  tabId,
  running,
  creationMode,
  onEditPrompt,
  rewindDisabled,
  hasOlderHistory,
  loadingOlderHistory,
  olderHistoryError,
  onLoadOlderHistory,
}: SplitWorkspaceProps) {
  // Split styles are loaded on demand (Vite emits them as a lazy CSS chunk) so
  // they never weigh on the app-shell stylesheet budget.
  useEffect(() => {
    void import("./splitWorkspace.css");
  }, []);

  const subscribeLive = useCallback(
    (listener: () => void) => liveStore?.subscribe(tabId, listener) ?? (() => {}),
    [liveStore, tabId],
  );
  const getLiveSnapshot = useCallback(
    () => liveStore?.getSnapshot(tabId) ?? liveProp,
    [liveProp, liveStore, tabId],
  );
  const live = useSyncExternalStore(subscribeLive, getLiveSnapshot, getLiveSnapshot);

  const reasoningDisplayMode = useReasoningDisplayMode();
  const hideReasoning = reasoningDisplayMode === "hidden" || reasoningDisplayMode === "pending";

  const liveFlags = useMemo(
    () => (live?.id
      ? {
          id: live.id,
          hasAnswerText: Boolean(live.text.trim()),
          hasReasoning: Boolean(live.reasoning),
          reasoningComplete: live.reasoningComplete,
        }
      : NO_LIVE),
    [live?.id, live?.text, live?.reasoning, live?.reasoningComplete],
  );

  const turnModels = useMemo(
    () => buildTurnModels(items, liveFlags, running, hideReasoning),
    [items, liveFlags, running, hideReasoning],
  );

  const conversationTurns = useMemo(() => buildConversationPaneTurns(turnModels), [turnModels]);
  const processTurns = useMemo(() => buildProcessPaneTurns(turnModels), [turnModels]);

  const subcallsByParent = useMemo(() => {
    const m = new Map<string, ToolItem[]>();
    for (const it of items) {
      if (it.kind === "tool" && it.parentId) {
        const arr = m.get(it.parentId) ?? [];
        arr.push(it);
        m.set(it.parentId, arr);
      }
    }
    return m;
  }, [items]);

  const activeTurnKey = useMemo(() => {
    const active = processTurns.find((turn) => turn.isActive);
    return active?.key ?? processTurns[processTurns.length - 1]?.key;
  }, [processTurns]);

  const sync = useTurnPaneSync({
    conversationTurns,
    processTurns,
    // Always the most recent turn (the active one when streaming, else the
    // last): the panes open focused on it. Gating on `running` let the initial
    // focus default to "" and let the panes' first range-change hijack it to
    // turn 1.
    activeTurnKey,
    running,
  });

  const focusedTurn = useMemo(
    () => processTurns.find((turn) => turn.key === sync.focusedTurnKey)?.turn,
    [processTurns, sync.focusedTurnKey],
  );

  // Flowchart-style arrow: connects the focused conversation rows (right edge)
  // to the focused process turn (left edge). Re-measured on focus change, on
  // either pane's scroll, and on layout shifts (window resize, Composer growth,
  // turn expand/collapse) via a ResizeObserver.
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [connector, setConnector] = useState<{ fx: number; fy: number; tx: number; ty: number } | null>(null);
  useEffect(() => {
    // .split-workspace is display:contents (its panes are chat-pane grid
    // cells), so it has no box — measure against the positioned chat-pane the
    // connector SVG covers instead.
    const el = containerRef.current?.closest<HTMLElement>(".chat-pane") ?? containerRef.current;
    if (!el || focusedTurn == null) {
      setConnector(null);
      return;
    }
    let raf = 0;
    // measureArrow is dynamically imported so it never lands in the initial bundle.
    const update = () => void import("../lib/correspondenceArrow").then((m) => setConnector(m.measureArrow(el)));
    const schedule = () => { cancelAnimationFrame(raf); raf = requestAnimationFrame(update); };
    const scrollers = el.querySelectorAll("[data-virtuoso-scroller]");
    scrollers.forEach((s) => s.addEventListener("scroll", schedule));
    // ResizeObserver re-arms the measurement when the container resizes, the
    // footer (Composer) grows, or the focused row/turn header resizes. Turn
    // expand/collapse also re-runs this effect via the focusedTurn dependency.
    const ro = new ResizeObserver(schedule);
    ro.observe(el);
    const footer = el.querySelector(".footer");
    if (footer) ro.observe(footer);
    const convRow = el.querySelector(".conversation-pane__turn[data-focused]");
    if (convRow) ro.observe(convRow);
    const procHeader = el.querySelector(".process-pane__turn[data-focused] .process-pane__turn-header");
    if (procHeader) ro.observe(procHeader);
    update();
    return () => {
      cancelAnimationFrame(raf);
      scrollers.forEach((s) => s.removeEventListener("scroll", schedule));
      ro.disconnect();
    };
  }, [focusedTurn]);

  return (
    <LiveStreamContext.Provider value={live}>
      <div className="split-workspace" ref={containerRef}>
        <ConversationPane
          ref={sync.conversationRef}
          turns={conversationTurns}
          creationMode={creationMode}
          running={running}
          onEditPrompt={onEditPrompt}
          rewindDisabled={rewindDisabled}
          onRangeChange={sync.onConversationRangeChange}
          focusedTurn={focusedTurn}
          onUserInteract={sync.markUserInteracted}
          hasOlderHistory={hasOlderHistory}
          loadingOlderHistory={loadingOlderHistory}
          olderHistoryError={olderHistoryError}
          onLoadOlderHistory={onLoadOlderHistory}
        />
        <ProcessPane
          ref={sync.processRef}
          turns={processTurns}
          focusedTurnKey={sync.focusedTurnKey}
          onFocusTurn={sync.onProcessTurnFocus}
          subcallsByParent={subcallsByParent}
          tabId={tabId}
          onRangeChange={sync.onProcessRangeChange}
          running={running}
          onUserInteract={sync.markUserInteracted}
        />
        {connector && (
          <svg className="split-workspace__connector" aria-hidden="true">
            <line className="connector-line" x1={connector.fx} y1={connector.fy} x2={connector.tx} y2={connector.ty} />
            <path className="connector-head" d={`M ${connector.tx} ${connector.ty} l -8 -4 v 8 Z`} />
            <path className="connector-head" d={`M ${connector.fx} ${connector.fy} l 8 -4 v 8 Z`} />
          </svg>
        )}
      </div>
    </LiveStreamContext.Provider>
  );
}
