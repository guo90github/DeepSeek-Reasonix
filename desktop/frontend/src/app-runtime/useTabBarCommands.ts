import { useState, type Dispatch, type SetStateAction } from "react";
import { app } from "../lib/bridge";
import { useCommittedCommand } from "../lib/useCommittedCommand";
import { guardBackendNavigationResult } from "../lib/navigationSurfaceTransition";
import { useOverlayStore } from "../store/overlays";
import type { ActiveWorkView, TabMeta } from "../lib/types";
import type { ComposerProfile } from "../lib/composerProfile";
import type { Translator } from "../lib/i18n";

export type TabClosePolicy = "keep_running" | "stop_and_close";

export type TabBarCommandsInput = {
  activeTabId: string | undefined;
  tabMetas: readonly TabMeta[];
  deliveryWorktreeRoot: string | undefined;
  t: Translator;
  showToast(message: string, level: "error", options?: { durationMs?: number }): void;
  setTabMetas: Dispatch<SetStateAction<TabMeta[]>>;
  // setTabOrderIds, ports.reorderTabs and ports.switchRemoteTab lost their only
  // reader with the app tab strip; they stay declared until the caller drops them.
  setTabOrderIds: Dispatch<SetStateAction<string[]>>;
  setComposerProfilesByTab: Dispatch<SetStateAction<Record<string, ComposerProfile>>>;
  setTabRevealSignal: Dispatch<SetStateAction<number>>;
  clearWorkspaceConflict(): void;
  ports: {
    closeTab(id: string, policy: TabClosePolicy): Promise<boolean>;
    closeTabs(ids: string[], keepTabId: string | undefined, policy: TabClosePolicy): Promise<boolean>;
    reorderTabs(ids: string[]): Promise<void>;
    switchTab(id: string, tab?: TabMeta, seq?: number): Promise<unknown>;
    switchRemoteTab(tab: TabMeta, seq?: number): Promise<unknown>;
    refreshTabMetas(apply?: () => boolean, options?: { afterMutation?: boolean }): Promise<TabMeta[]>;
    refreshBackgroundRuntimes(): Promise<void>;
    cancelActive(): void;
    noteNavigationIntent(): number;
    beginNavigationSurface(seq: number): void;
    settleNavigationSurface(seq: number): void;
    isNavigationIntentCurrent(seq: number): boolean;
    reassertVisibleTabAfterStaleNavigation(kind: string, staleTabId: string): Promise<void>;
    enterChatView(): void;
    createIsolatedWorktree(root: string, seq: number): Promise<unknown>;
  };
};

export type PendingClose = {
  tabIds: string[];
  work: ActiveWorkView;
  stopping: boolean;
  keepTabId?: string;
};

/**
 * Owns the tab close command and prompt, the background-runtime reveals and the
 * delivery-worktree continuation. Tab close prompts and reveal navigation share
 * one navigation-intent/surface lifecycle; only the visible tab list, reveal
 * signal and close prompt stay on the caller's stores.
 */
export function useTabBarCommands(input: TabBarCommandsInput) {
  const { activeTabId, t, showToast, ports } = input;
  const [pendingClose, setPendingClose] = useState<PendingClose | null>(null);
  const setTransientOverlayDismissSignal = useOverlayStore((state) => state.setTransientOverlayDismissSignal);

  const closeTransientOverlays = useCommittedCommand(() => {
    setTransientOverlayDismissSignal((signal) => signal + 1);
  });

  const enterChatViewForTabNavigation = useCommittedCommand(() => {
    ports.enterChatView();
  });

  const revealBackgroundRuntime = useCommittedCommand(async (tabId: string): Promise<void> => {
    enterChatViewForTabNavigation();
    const navigationIntentSeq = ports.noteNavigationIntent();
    ports.beginNavigationSurface(navigationIntentSeq);
    try {
      const meta = await app.RevealBackgroundRuntime(tabId);
      if (!await guardBackendNavigationResult({
        intent: navigationIntentSeq,
        targetTabId: meta.id,
        kind: "tab.reveal-background",
        isIntentCurrent: ports.isNavigationIntentCurrent,
        reassert: ports.reassertVisibleTabAfterStaleNavigation,
      })) return;
      await ports.switchTab(meta.id, meta, navigationIntentSeq);
      if (!ports.isNavigationIntentCurrent(navigationIntentSeq)) return;
      await ports.refreshTabMetas(
        () => ports.isNavigationIntentCurrent(navigationIntentSeq),
        { afterMutation: true },
      );
    } catch (err) {
      if (ports.isNavigationIntentCurrent(navigationIntentSeq)) showToast(err instanceof Error ? err.message : String(err), "error");
    } finally {
      ports.settleNavigationSurface(navigationIntentSeq);
    }
  });

  // One close command for one or many tabs: a batch rewrites one navigation
  // intent and keeps one tab alive, so "close others" cannot race itself the
  // way N per-tab closes did.
  const finishTabsClose = useCommittedCommand(async (
    tabIds: string[],
    keepTabId: string | undefined,
    policy: TabClosePolicy,
  ): Promise<boolean> => {
    const closing = tabIds.filter((id) => id && id !== keepTabId);
    if (closing.length === 0) return true;
    closeTransientOverlays();
    const closed = await ports.closeTabs(closing, keepTabId, policy);
    if (!closed) {
      showToast(t("runtime.closeFailed"), "error");
      return false;
    }
    input.setComposerProfilesByTab((current) => {
      const next = { ...current };
      let changed = false;
      for (const id of closing) {
        if (!(id in next)) continue;
        delete next[id];
        changed = true;
      }
      return changed ? next : current;
    });
    input.setTabMetas((current) => {
      const remaining = current.filter((tab) => !closing.includes(tab.id));
      if (remaining.length === current.length || remaining.length === 0) return current;
      const nextActiveId = keepTabId && remaining.some((tab) => tab.id === keepTabId)
        ? keepTabId
        : remaining.find((tab) => tab.active)?.id ?? remaining[remaining.length - 1]?.id;
      return remaining.map((tab) => ({ ...tab, active: tab.id === nextActiveId }));
    });
    await ports.refreshTabMetas(undefined, { afterMutation: true });
    await ports.refreshBackgroundRuntimes();
    input.setTabRevealSignal((signal) => signal + 1);
    return true;
  });

  const finishTabClose = useCommittedCommand(async (
    id: string,
    policy: TabClosePolicy,
  ): Promise<boolean> => finishTabsClose([id], undefined, policy));

  const handleTabClose = useCommittedCommand(async (id: string) => {
    try {
      const work = await app.ActiveWorkForTab(id);
      if (work.running || work.pendingPrompt || work.jobs.length > 0) {
        setPendingClose({ tabIds: [id], work, stopping: false });
        return;
      }
    } catch {
      // CloseTabWithPolicy re-checks the controller state atomically.
    }
    await finishTabsClose([id], undefined, "stop_and_close");
  });

  const handleTabsClose = useCommittedCommand(async (tabIds: string[], keepTabId?: string): Promise<void> => {
    const closing = tabIds.filter((id) => id && id !== keepTabId);
    if (closing.length === 0) return;
    try {
      const works = await Promise.all(closing.map((id) => app.ActiveWorkForTab(id)));
      if (works.some((work) => work.running || work.pendingPrompt || work.jobs.length > 0)) {
        setPendingClose({
          tabIds: closing,
          keepTabId,
          stopping: false,
          work: {
            running: works.some((work) => work.running),
            pendingPrompt: works.some((work) => work.pendingPrompt),
            cancellable: works.some((work) => work.cancellable),
            jobs: works.flatMap((work) => work.jobs),
          },
        });
        return;
      }
    } catch {
      // CloseTabWithPolicy re-checks the controller state atomically per tab.
    }
    await finishTabsClose(closing, keepTabId, "stop_and_close");
  });

  const resolvePendingClose = useCommittedCommand(async (policy: TabClosePolicy) => {
    const request = pendingClose;
    if (!request || request.stopping) return;
    if (policy === "stop_and_close") setPendingClose({ ...request, stopping: true });
    const closed = await finishTabsClose(request.tabIds, request.keepTabId, policy);
    if (closed) setPendingClose(null);
    else setPendingClose((current) => current && current.tabIds.join("\u0000") === request.tabIds.join("\u0000") ? { ...current, stopping: false } : current);
  });

  const revealWorkspaceWriter = useCommittedCommand(async () => {
    if (!activeTabId) return;
    enterChatViewForTabNavigation();
    const navigationIntentSeq = ports.noteNavigationIntent();
    ports.beginNavigationSurface(navigationIntentSeq);
    try {
      const meta = await app.RevealWorkspaceWriterForTab(activeTabId);
      if (!await guardBackendNavigationResult({
        intent: navigationIntentSeq,
        targetTabId: meta.id,
        kind: "tab.reveal-workspace-writer",
        isIntentCurrent: ports.isNavigationIntentCurrent,
        reassert: ports.reassertVisibleTabAfterStaleNavigation,
      })) return;
      input.clearWorkspaceConflict();
      await ports.switchTab(meta.id, meta, navigationIntentSeq);
      if (!ports.isNavigationIntentCurrent(navigationIntentSeq)) return;
      await ports.refreshTabMetas(
        () => ports.isNavigationIntentCurrent(navigationIntentSeq),
        { afterMutation: true },
      );
    } catch (err) {
      if (ports.isNavigationIntentCurrent(navigationIntentSeq)) showToast(err instanceof Error ? err.message : String(err), "error");
    } finally {
      ports.settleNavigationSurface(navigationIntentSeq);
    }
  });

  const continueInDeliveryWorktree = useCommittedCommand(async () => {
    const root = input.deliveryWorktreeRoot;
    if (!root) return;
    ports.cancelActive();
    input.clearWorkspaceConflict();
    const navigationIntentSeq = ports.noteNavigationIntent();
    ports.beginNavigationSurface(navigationIntentSeq);
    try {
      await ports.createIsolatedWorktree(root, navigationIntentSeq);
      await ports.refreshTabMetas(undefined, { afterMutation: true });
    } catch (err) {
      if (ports.isNavigationIntentCurrent(navigationIntentSeq)) showToast(err instanceof Error ? err.message : String(err), "error");
    } finally {
      ports.settleNavigationSurface(navigationIntentSeq);
    }
  });

  return {
    pendingClose,
    setPendingClose,
    revealBackgroundRuntime,
    finishTabClose,
    finishTabsClose,
    handleTabClose,
    handleTabsClose,
    resolvePendingClose,
    revealWorkspaceWriter,
    continueInDeliveryWorktree,
  };
}
