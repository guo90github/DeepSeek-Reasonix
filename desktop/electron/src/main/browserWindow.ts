import type { BrowserWindowConstructorOptions } from "electron";
import { errorText, type Logger } from "./log.js";

// Options this host passes to Electron. Declared locally so the module stays
// unit-testable: tests inject their own createWindow and never load electron.
export interface BrowserWindowOptions {
  width: number;
  height: number;
  minWidth: number;
  minHeight: number;
  show: boolean;
  title: string;
  backgroundColor: string;
  autoHideMenuBar: boolean;
  icon?: string;
  webPreferences: {
    preload: string;
    sandbox: boolean;
    contextIsolation: boolean;
    nodeIntegration: boolean;
    spellcheck: boolean;
  };
}

/** The slice of Electron's BrowserWindow this host drives. */
export interface BrowserWindowView {
  isDestroyed(): boolean;
  isMinimized(): boolean;
  isVisible(): boolean;
  show(): void;
  focus(): void;
  restore(): void;
  close(): void;
  destroy(): void;
  loadURL(url: string): Promise<void>;
  setMenuBarVisibility(visible: boolean): void;
  on(event: "closed", listener: () => void): unknown;
  once(event: "ready-to-show", listener: () => void): unknown;
}

export interface BrowserWindowHostDeps {
  platform: NodeJS.Platform;
  icon?: string;
  /** App entry URL; the browser surface is selected with ?surface=browser. */
  appURL: string;
  preloadPath: string;
  createWindow(options: BrowserWindowConstructorOptions): BrowserWindowView;
  log: Logger;
  /** Reports the window it just created, so the shell can re-parent the views. */
  onOpened?(win: BrowserWindowView): void;
  onClosed?(): void;
}

const SURFACE_PARAM = "surface=browser";

/** Appends the browser-surface marker without dropping an existing query. */
export function browserSurfaceURL(appURL: string): string {
  const separator = appURL.includes("?") ? "&" : "?";
  return appURL.includes(SURFACE_PARAM) ? appURL : `${appURL}${separator}${SURFACE_PARAM}`;
}

/**
 * The independent browser window. One window for now, so a second open focuses
 * the first instead of stacking duplicates; the guest views themselves stay
 * owned by BrowserSurfaceManager and are re-parented in a later step.
 */
export class BrowserWindowHost {
  private win: BrowserWindowView | null = null;

  constructor(private readonly deps: BrowserWindowHostDeps) {}

  open(): void {
    const existing = this.view();
    if (existing) {
      if (existing.isMinimized()) existing.restore();
      existing.show();
      existing.focus();
      return;
    }
    const win = this.deps.createWindow({
      width: 1180,
      height: 820,
      minWidth: 760,
      minHeight: 480,
      show: false,
      title: "Reasonix 浏览器",
      backgroundColor: "#1a1a2e",
      autoHideMenuBar: true,
      ...(this.deps.icon ? { icon: this.deps.icon } : {}),
      webPreferences: {
        preload: this.deps.preloadPath,
        sandbox: true,
        contextIsolation: true,
        nodeIntegration: false,
        spellcheck: false,
      },
    });
    this.win = win;
    this.deps.onOpened?.(win);
    if (this.deps.platform !== "darwin") win.setMenuBarVisibility(false);
    win.once("ready-to-show", () => {
      if (this.win === win) win.show();
    });
    win.on("closed", () => {
      if (this.win !== win) return;
      this.win = null;
      this.deps.onClosed?.();
    });
    void win.loadURL(browserSurfaceURL(this.deps.appURL)).catch((error: unknown) => {
      this.deps.log.warn(`browser window load failed: ${errorText(error)}`);
    });
  }

  close(): void {
    const win = this.view();
    if (!win) return;
    win.close();
  }

  isOpen(): boolean {
    return this.view() !== null;
  }

  /** Quit path: destroy rather than close, so a page cannot block the exit. */
  closeAll(): void {
    const win = this.win;
    this.win = null;
    if (!win || win.isDestroyed()) return;
    win.destroy();
  }

  private view(): BrowserWindowView | null {
    if (!this.win) return null;
    if (this.win.isDestroyed()) {
      this.win = null;
      return null;
    }
    return this.win;
  }
}
