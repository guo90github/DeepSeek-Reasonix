import { useEffect } from "react";

import { app } from "../lib/bridge";
import { desktopHost } from "../lib/desktopHost";
import { setMainWindowMaximised, useWindowChromeStore } from "../store/windowChrome";

// Module-owned sync state for the single AppRuntime host: the enabled gate
// mirrors the active lifecycle, and the generation ticket discards
// out-of-order IsMainWindowMaximised resolutions.
let syncEnabled = false;
let syncGeneration = 0;

/**
 * Re-reads the native maximised flag into the windowChrome store. Event
 * handlers call this after a toggle/zoom; a no-op while the lifecycle is
 * disabled so a non-frameless platform never issues the bridge call.
 */
export function syncMainWindowMaximised(): void {
  if (!syncEnabled) return;
  const generation = ++syncGeneration;
  void app.IsMainWindowMaximised()
    .then((value) => { if (generation === syncGeneration) setMainWindowMaximised(value); })
    .catch(() => { if (generation === syncGeneration) setMainWindowMaximised(false); });
}

/**
 * Owns the maximised-sync lifecycle: initial sync, resize/focus listeners and
 * the disabled/unmount reset. The flag itself lives in the windowChrome store;
 * consumers select `mainWindowMaximised` from there.
 */
export function useWindowsMaximisedSync(enabled: boolean): void {
  useEffect(() => {
    if (!enabled) {
      syncEnabled = false;
      syncGeneration += 1;
      setMainWindowMaximised(false);
      return;
    }
    syncEnabled = true;
    syncMainWindowMaximised();
    window.addEventListener("resize", syncMainWindowMaximised);
    window.addEventListener("focus", syncMainWindowMaximised);
    return () => {
      syncEnabled = false;
      syncGeneration += 1;
      window.removeEventListener("resize", syncMainWindowMaximised);
      window.removeEventListener("focus", syncMainWindowMaximised);
    };
  }, [enabled]);
}

const BAR_DRAG_SURFACE = ".topicbar";
// Mirrors the native drag region: a control inside the bar keeps its click.
const BAR_DRAG_CONTROL = "button, input, textarea, select, a, [role='button'], [role='tab'], .windows-window-controls";

/**
 * Moves the window from the bar when the OS cannot. A maximised frameless window
 * gets no native caption drag on Windows and the press never reaches the page
 * either, so while maximised the bar carries no drag region (the
 * `.app--maximised` rule) and this hook drives the move through the shell.
 */
export function useFramelessBarDrag(enabled: boolean): void {
  useEffect(() => {
    if (!enabled) return;
    let moving = false;

    const finish = () => {
      if (!moving) return;
      moving = false;
      delete document.documentElement.dataset.windowDrag;
      void desktopHost().native.endWindowMove?.()?.catch(() => undefined);
      window.removeEventListener("mouseup", finish, true);
      window.removeEventListener("blur", finish, true);
    };

    const start = (event: MouseEvent) => {
      if (event.button !== 0 || moving) return;
      if (!useWindowChromeStore.getState().mainWindowMaximised) return;
      const target = event.target as HTMLElement | null;
      if (typeof target?.closest !== "function" || !target.closest(BAR_DRAG_SURFACE)) return;
      if (target.closest(BAR_DRAG_CONTROL)) return;
      moving = true;
      document.documentElement.dataset.windowDrag = "js";
      void desktopHost().native.beginWindowMove?.(event.clientX, event.clientY)?.catch(() => undefined);
      window.addEventListener("mouseup", finish, true);
      window.addEventListener("blur", finish, true);
    };

    window.addEventListener("mousedown", start, true);
    return () => {
      window.removeEventListener("mousedown", start, true);
      finish();
    };
  }, [enabled]);
}

export const nativeWindowCommands = {
  minimize: () => app.MinimiseMainWindow(),
  toggleMaximize: () => app.ToggleMaximiseMainWindow(),
  close: () => app.CloseMainWindow(),
};
