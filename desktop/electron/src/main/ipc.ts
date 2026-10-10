import type { IpcMain, IpcMainEvent, IpcMainInvokeEvent } from "electron";
import {
  IPC,
  type BrowserLayoutRect,
  type BrowserNavigateTarget,
  type BrowserOpenOptions,
  type BrowserTabView,
  type IpcResult,
  type ServiceState,
  type WindowBounds,
  type WindowTheme,
} from "../shared/ipc.js";
import { isAllowedCommand, type LoadedContract } from "./contract.js";
import { errorText, type Logger } from "./log.js";
import { bool, finite, record, str } from "./params.js";
import { RpcError } from "./rpc.js";
import type { GraphicsSettingsStore } from "./graphics.js";

export interface RendererWindowApi {
  isTrustedSender(sender: IpcMainEvent["sender"], frame: IpcMainEvent["senderFrame"]): boolean;
  minimise(): void;
  toggleMaximise(): void;
  isMaximised(): boolean;
  close(): void;
  bounds(): WindowBounds;
  beginMove(x: number, y: number): void;
  endMove(): void;
  setTheme(theme: WindowTheme): void;
  setBackgroundColour(r: number, g: number, b: number, a: number): void;
  getAppZoom(): Promise<number>;
  setAppZoom(factor: number): Promise<number>;
  resetAppZoom(): Promise<number>;
}

// The user-driven browser panel: no grant is involved because the user is
// the one acting, but every call is still gated on the trusted sender.
export interface BrowserRendererApi {
  list(): BrowserTabView[];
  open(url: string, options: Required<BrowserOpenOptions>): Promise<BrowserTabView>;
  close(tabId: string): void;
  activate(tabId: string | null): void;
  navigate(tabId: string, target: BrowserNavigateTarget): Promise<void>;
  setZoom(tabId: string, factor: number): void;
  toggleDevTools(tabId: string): void;
  resume(tabId: string): void;
  takeover(tabId: string): void;
  setLayout(rect: BrowserLayoutRect | null, senderId: number): void;
  setOverlay(active: boolean): void;
}

// The independent browser window: opening it is a user action, so it carries no
// grant; like the panel, every call is gated on the trusted sender.
export interface BrowserWindowRendererApi {
  open(taskId?: string): void;
  close(): void;
}

export interface RendererIpcDeps {
  ipcMain: IpcMain;
  contract: LoadedContract;
  window: RendererWindowApi;
  invoke(method: string, args: unknown[]): Promise<unknown>;
  serviceState(): ServiceState;
  clipboard: { writeText(text: string): Promise<void> | void; readText(): Promise<string> | string };
  graphics?: GraphicsSettingsStore;
  openExternal(url: string): Promise<void>;
  browser?: BrowserRendererApi;
  browserWindow?: BrowserWindowRendererApi;
  log: Logger;
}

const NAVIGATE_ACTIONS = new Set(["back", "forward", "reload", "stop"]);

export function parseNavigateTarget(value: unknown): BrowserNavigateTarget {
  const target = record(value);
  const action = str(target, "action");
  if (NAVIGATE_ACTIONS.has(action)) return { action: action as BrowserNavigateTarget["action"] };
  return { url: str(target, "url") };
}

export function parseLayout(value: unknown): BrowserLayoutRect | null {
  if (value === null || value === undefined) return null;
  const rect = record(value);
  return { x: finite(rect.x, Number.NaN), y: finite(rect.y, Number.NaN), width: finite(rect.width, Number.NaN), height: finite(rect.height, Number.NaN) };
}

const EXTERNAL_PROTOCOLS = new Set(["http:", "https:", "mailto:"]);

export function isOpenableExternalURL(value: unknown): value is string {
  if (typeof value !== "string") return false;
  try {
    return EXTERNAL_PROTOCOLS.has(new URL(value).protocol);
  } catch {
    return false;
  }
}

export function registerRendererIpc(deps: RendererIpcDeps): void {
  const trusted = (event: IpcMainEvent | IpcMainInvokeEvent): boolean => {
    const ok = deps.window.isTrustedSender(event.sender, event.senderFrame);
    if (!ok) deps.log.warn(`rejected IPC from untrusted sender (webContents ${event.sender.id})`);
    return ok;
  };
  // handleFrom is the form that knows which renderer called: a layout report
  // must be attributed to a window, and only the surface owner's is accepted.
  const handleFrom = (channel: string, run: (senderId: number, ...args: unknown[]) => Promise<unknown> | unknown) => {
    deps.ipcMain.handle(channel, async (event, ...args: unknown[]): Promise<IpcResult> => {
      if (!trusted(event)) return { ok: false, message: "untrusted sender" };
      try {
        return { ok: true, value: await run(event.sender.id, ...args) };
      } catch (error) {
        return { ok: false, message: errorText(error) };
      }
    });
  };
  const handle = (channel: string, run: (...args: unknown[]) => Promise<unknown> | unknown) => {
    handleFrom(channel, (_senderId, ...args) => run(...args));
  };

  deps.ipcMain.on(IPC.contract, (event) => {
    event.returnValue = trusted(event)
      ? { protocolVersion: deps.contract.protocolVersion, digest: deps.contract.digest, commands: [...deps.contract.commands] }
      : null;
  });

  handle(IPC.invoke, (method, args) => {
    if (!isAllowedCommand(deps.contract, method)) {
      throw new RpcError(-32601, `-32601 method not found: ${typeof method === "string" ? method : typeof method}`);
    }
    return deps.invoke(method, Array.isArray(args) ? args : []);
  });
  handle(IPC.serviceStateGet, () => deps.serviceState());
  handle(IPC.openExternal, (url) => {
    if (!isOpenableExternalURL(url)) throw new Error(`refusing to open ${typeof url === "string" ? url : typeof url}`);
    return deps.openExternal(url);
  });
  handle(IPC.clipboardWrite, async (text) => {
    await deps.clipboard.writeText(typeof text === "string" ? text : "");
    return true;
  });
  handle(IPC.clipboardRead, () => deps.clipboard.readText());
  handle(IPC.windowMinimise, () => deps.window.minimise());
  handle(IPC.windowToggleMaximise, () => deps.window.toggleMaximise());
  handle(IPC.windowIsMaximised, () => deps.window.isMaximised());
  handle(IPC.windowClose, () => deps.window.close());
  handle(IPC.windowGetBounds, () => deps.window.bounds());
  handle(IPC.windowBeginMove, (x, y) => {
    if (typeof x !== "number" || typeof y !== "number" || !Number.isFinite(x) || !Number.isFinite(y)) {
      throw new Error("window move needs finite coordinates");
    }
    deps.window.beginMove(x, y);
  });
  handle(IPC.windowEndMove, () => deps.window.endMove());
  handle(IPC.windowSetTheme, (theme) => deps.window.setTheme(theme === "light" || theme === "dark" ? theme : "system"));
  handle(IPC.windowSetBackground, (r, g, b, a) => deps.window.setBackgroundColour(finite(r), finite(g), finite(b), finite(a, 255)));
  handle(IPC.appZoomGet, () => deps.window.getAppZoom());
  handle(IPC.appZoomSet, (factor) => deps.window.setAppZoom(finite(factor, Number.NaN)));
  handle(IPC.appZoomReset, () => deps.window.resetAppZoom());
  handle(IPC.graphicsGet, () => deps.graphics?.current ?? { hardwareAcceleration: true, startupEnabled: true, override: "none", restartRequired: false, writable: false, warning: null });
  handle(IPC.graphicsSet, (enabled) => {
    if (typeof enabled !== "boolean") throw new Error("hardwareAcceleration must be boolean");
    if (!deps.graphics) throw new Error("graphics settings unavailable");
    return deps.graphics.setHardwareAcceleration(enabled);
  });

  // Registered ahead of the panel's early return: the window host stands on its
  // own and the panel is optional.
  const browserWindow = deps.browserWindow;
  if (browserWindow) {
    handle(IPC.browserWindowOpen, (taskId) => browserWindow.open(typeof taskId === "string" && taskId !== "" ? taskId : undefined));
    handle(IPC.browserWindowClose, () => browserWindow.close());
  }

  const browser = deps.browser;
  if (!browser) return;
  const tabId = (value: unknown): string => {
    if (typeof value !== "string" || value === "") throw new Error("tabId must be a non-empty string");
    return value;
  };
  handle(IPC.browserList, () => browser.list());
  handle(IPC.browserOpen, (url, options) => {
    if (typeof url !== "string") throw new Error("url must be a string");
    const opts = record(options);
    return browser.open(url, { temporary: bool(opts, "temporary"), taskId: str(opts, "taskId", "user") || "user" });
  });
  handle(IPC.browserClose, (id) => browser.close(tabId(id)));
  handle(IPC.browserActivate, (id) => browser.activate(id === null || id === undefined ? null : tabId(id)));
  handle(IPC.browserNavigate, (id, target) => browser.navigate(tabId(id), parseNavigateTarget(target)));
  handle(IPC.browserSetZoom, (id, factor) => browser.setZoom(tabId(id), finite(factor, Number.NaN)));
  handle(IPC.browserToggleDevTools, (id) => browser.toggleDevTools(tabId(id)));
  handle(IPC.browserResume, (id) => browser.resume(tabId(id)));
  handle(IPC.browserUserTakeover, (id) => browser.takeover(tabId(id)));
  handleFrom(IPC.browserSetLayout, (senderId, rect) => browser.setLayout(parseLayout(rect), senderId));
  handle(IPC.browserSetOverlay, (active) => browser.setOverlay(active === true));
}
