import type { WebContents, WebFrameMain } from "electron";

// Which renderers may call the shell's IPC. The main window owns the app; the
// independent browser window is a second trusted renderer, and only its main
// frame is — an iframe or a website view must never reach these channels.
export interface TrustedMainWindow {
  readonly webContents: WebContents;
}

export class RendererTrust {
  private readonly extra = new Set<WebContents>();

  constructor(private readonly mainWindow: () => TrustedMainWindow | null) {}

  trust(contents: WebContents | null | undefined): void {
    if (!contents || contents.isDestroyed()) return;
    this.extra.add(contents);
  }

  forget(contents: WebContents | null | undefined): void {
    if (contents) this.extra.delete(contents);
  }

  isTrusted(sender: WebContents, frame: WebFrameMain | null | undefined): boolean {
    if (!frame) return false;
    const main = this.mainWindow();
    if (main && sender === main.webContents && frame === main.webContents.mainFrame) return true;
    return this.extra.has(sender) && frame === sender.mainFrame;
  }
}
