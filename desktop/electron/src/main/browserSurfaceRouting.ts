import type { BrowserWindow } from "electron";

// Which window currently hosts the website views. The independent browser
// window owns them while it is open, and closing it hands them back to the main
// window. A single owner is what keeps two renderers from fighting over the same
// native view: the layout IPC is gated on it, so a hidden panel's stale rect can
// never move a page the visible window is showing.
export class BrowserSurfaceRouting {
  private browserWindowId: number | null = null;

  constructor(private readonly mainWindowId: () => number | null) {}

  /** null hands ownership back to the main window. */
  setOwner(browserWindow: BrowserWindow | null): void {
    this.browserWindowId = browserWindow && !browserWindow.isDestroyed() ? browserWindow.webContents.id : null;
  }

  ownerId(): number | null {
    return this.browserWindowId ?? this.mainWindowId();
  }

  accepts(senderId: number): boolean {
    const owner = this.ownerId();
    return owner !== null && owner === senderId;
  }
}
