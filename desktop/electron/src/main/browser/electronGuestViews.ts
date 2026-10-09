import { WebContentsView, session as electronSession, type BrowserWindow, type Session, type WebContents, type WebPreferences } from "electron";
import type { Logger } from "../log.js";
import { isBlockedNavigation, isPopupURL, type GuestView, type GuestViewEvents, type GuestViewFactory, type GuestWindow } from "./guestView.js";

const GUEST_PERMISSIONS = new Set(["clipboard-sanitized-write", "fullscreen"]);

export interface ElectronGuestViewDeps {
  window(): BrowserWindow | null;
  preloadPath: string;
  log: Logger;
  onSession?(partition: string, session: Session): void;
}

// Website views are untrusted: sandboxed, isolated, no Node, only the guest
// preload that reports user input. Every partition session gets the same
// permission policy the first time it is seen.
export class ElectronGuestViewFactory implements GuestViewFactory {
  private readonly sessions = new Set<string>();

  constructor(private readonly deps: ElectronGuestViewDeps) {}

  create(partition: string, inherited?: WebPreferences): GuestView {
    const win = this.deps.window();
    if (!win) throw new Error("browser tabs need the main window");
    this.prepareSession(partition);
    const view = new WebContentsView({
      webPreferences: {
        ...inherited,
        partition,
        preload: this.deps.preloadPath,
        sandbox: true,
        contextIsolation: true,
        nodeIntegration: false,
        nodeIntegrationInSubFrames: true,
        nodeIntegrationInWorker: false,
        webviewTag: false,
        spellcheck: true,
        backgroundThrottling: false,
      },
    });
    // A view paints nothing until its page does; without a colour of its own
    // Electron's default white covers the window while it loads or stays blank.
    // Kept equal to the colour BrowserWindowHost paints its window with.
    view.setBackgroundColor("#1a1a2e");
    win.contentView.addChildView(view);
    view.setVisible(false);
    return new ElectronGuestView(view, win, this, this.deps.log);
  }

  private prepareSession(partition: string): void {
    if (this.sessions.has(partition)) return;
    this.sessions.add(partition);
    const session = electronSession.fromPartition(partition);
    session.setPermissionRequestHandler((_contents, permission, callback) => callback(GUEST_PERMISSIONS.has(permission)));
    session.setPermissionCheckHandler((_contents, permission) => GUEST_PERMISSIONS.has(permission));
    this.deps.onSession?.(partition, session);
  }
}

class ElectronGuestView implements GuestView {
  private destroyed = false;

  constructor(
    private readonly view: WebContentsView,
    private win: GuestWindow,
    private readonly factory: GuestViewFactory,
    private readonly log: Logger,
  ) {}

  get page(): WebContents {
    return this.view.webContents;
  }

  bind(events: GuestViewEvents): void {
    const wc = this.view.webContents;
    wc.on("did-start-loading", () => events.onStartLoading());
    wc.on("did-stop-loading", () => events.onStopLoading());
    wc.on("did-navigate", (_event, url) => events.onNavigate(url, false));
    wc.on("did-navigate-in-page", (_event, url, isMainFrame) => {
      if (isMainFrame) events.onNavigate(url, true);
    });
    wc.on("page-title-updated", (_event, title) => events.onTitle(title));
    wc.on("did-fail-load", (_event, code, description, url, isMainFrame) => {
      if (isMainFrame && code !== -3) events.onFailLoad(code, description, url);
    });
    wc.on("render-process-gone", (_event, details) => events.onRenderProcessGone(details.reason));
    wc.on("destroyed", () => events.onDestroyed());
    wc.on("will-navigate", (details) => this.guardNavigation(details, details.url));
    wc.on("will-frame-navigate", (details) => this.guardNavigation(details, details.url));
    wc.on("will-redirect", (details) => this.guardNavigation(details, details.url));
    wc.on("will-attach-webview", (event) => event.preventDefault());
    wc.setWindowOpenHandler(({ url, disposition }) => {
      if (!isPopupURL(url)) return { action: "deny" };
      const adopt = events.onPopup(url, disposition);
      if (!adopt) return { action: "deny" };
      return {
        action: "allow",
        createWindow: (options) => {
          const child = this.factory.create(String(options.webPreferences?.partition ?? ""), options.webPreferences);
          adopt(child);
          return child.page as WebContents;
        },
      };
    });
  }

  private guardNavigation(details: { preventDefault(): void }, url: string): void {
    if (!isBlockedNavigation(url)) return;
    details.preventDefault();
    this.log.warn(`blocked browser navigation to ${url.slice(0, 120)}`);
  }

  setBounds(bounds: Electron.Rectangle): void {
    if (!this.destroyed) this.view.setBounds(bounds);
  }

  setVisible(visible: boolean): void {
    if (!this.destroyed) this.view.setVisible(visible);
  }

  // Detach from the old window before attaching to the new one, so neither ever
  // paints a view it no longer owns; the manager re-applies visibility once it
  // hands over the layout.
  moveTo(win: GuestWindow): void {
    if (this.destroyed || this.win === win) return;
    if (!this.win.isDestroyed()) {
      try {
        this.win.contentView.removeChildView(this.view);
      } catch (error) {
        this.log.warn(`removeChildView failed: ${String(error)}`);
      }
    }
    this.win = win;
    win.contentView.addChildView(this.view);
    this.view.setVisible(false);
  }

  // Detach first so the window never paints a closing view, then close the
  // WebContents; the prototype's quit order that never left orphans.
  destroy(): void {
    if (this.destroyed) return;
    this.destroyed = true;
    if (!this.win.isDestroyed()) {
      try {
        this.win.contentView.removeChildView(this.view);
      } catch (error) {
        this.log.warn(`removeChildView failed: ${String(error)}`);
      }
    }
    const wc = this.view.webContents;
    if (!wc.isDestroyed()) wc.close();
  }
}
