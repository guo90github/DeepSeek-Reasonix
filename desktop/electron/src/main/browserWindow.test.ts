import assert from "node:assert/strict";
import { test } from "node:test";
import { BrowserWindowHost, browserSurfaceURL, type BrowserWindowOptions, type BrowserWindowView } from "./browserWindow.js";
import { silentLog } from "./browser/fakeGuestViews.js";

class FakeWindow implements BrowserWindowView {
  static created: FakeWindow[] = [];
  static last(): FakeWindow {
    return FakeWindow.created[FakeWindow.created.length - 1];
  }
  static reset(): void {
    FakeWindow.created = [];
  }

  shows = 0;
  focuses = 0;
  closes = 0;
  destroys = 0;
  restores = 0;
  visible = false;
  minimized = false;
  menuBar = true;
  loaded: string[] = [];
  private listeners = new Map<string, (() => void)[]>();
  private dead = false;

  constructor(readonly options: BrowserWindowOptions) {
    FakeWindow.created.push(this);
  }

  isDestroyed(): boolean { return this.dead; }
  isMinimized(): boolean { return this.minimized; }
  isVisible(): boolean { return this.visible; }
  show(): void { this.visible = true; this.shows += 1; }
  focus(): void { this.focuses += 1; }
  restore(): void { this.restores += 1; this.minimized = false; }
  close(): void { this.closes += 1; this.kill(); }
  destroy(): void { this.destroys += 1; this.kill(); }
  loadURL(url: string): Promise<void> { this.loaded.push(url); return Promise.resolve(); }
  setMenuBarVisibility(visible: boolean): void { this.menuBar = visible; }
  on(event: "closed", listener: () => void): unknown { return this.add(event, listener); }
  once(event: "ready-to-show", listener: () => void): unknown { return this.add(event, listener); }

  emit(event: string): void {
    for (const listener of [...(this.listeners.get(event) ?? [])]) listener();
  }

  private add(event: string, listener: () => void): unknown {
    const list = this.listeners.get(event) ?? [];
    list.push(listener);
    this.listeners.set(event, list);
    return this;
  }

  private kill(): void {
    if (this.dead) return;
    this.dead = true;
    this.visible = false;
    this.emit("closed");
  }
}

function host(overrides: Partial<{ platform: NodeJS.Platform; icon: string }> = {}) {
  const closed: number[] = [];
  const opened: FakeWindow[] = [];
  const instance = new BrowserWindowHost({
    platform: overrides.platform ?? "win32",
    ...(overrides.icon ? { icon: overrides.icon } : {}),
    appURL: "reasonix://app/index.html",
    preloadPath: "/tmp/preload.cjs",
    createWindow: (options) => new FakeWindow(options as BrowserWindowOptions),
    log: silentLog,
    onOpened: (win) => opened.push(win as FakeWindow),
    onClosed: () => closed.push(1),
  });
  return { instance, closed, opened };
}

test("the browser surface marker survives an existing query", () => {
  assert.equal(browserSurfaceURL("reasonix://app/index.html"), "reasonix://app/index.html?surface=browser");
  assert.equal(browserSurfaceURL("http://localhost:5173/?platform=windows"), "http://localhost:5173/?platform=windows&surface=browser");
  assert.equal(browserSurfaceURL("reasonix://app/index.html?surface=browser"), "reasonix://app/index.html?surface=browser");
});

test("open creates one window, loads the browser surface and shows it when ready", async () => {
  FakeWindow.reset();
  const { instance, opened } = host({ icon: "/tmp/icon.png" });
  instance.open();
  assert.equal(FakeWindow.created.length, 1, "one window per open");
  const win = FakeWindow.last();
  assert.deepEqual(opened, [win], "the shell learns which window opened, so it can re-parent the views");
  assert.deepEqual(win.loaded, ["reasonix://app/index.html?surface=browser"], "the browser surface is selected by URL");
  assert.equal(win.visible, false, "the window stays hidden until the surface is paintable");
  assert.equal(win.options.show, false);
  assert.equal(win.options.width, 1180);
  assert.equal(win.options.webPreferences.preload, "/tmp/preload.cjs", "the shell preload is reused, so the surface gets the host API");
  assert.equal(win.options.icon, "/tmp/icon.png");
  assert.equal(win.menuBar, false, "windows hides the menu bar");
  win.emit("ready-to-show");
  assert.equal(win.shows, 1, "the first paint shows the window");
  assert.equal(win.visible, true);
  assert.equal(instance.isOpen(), true);
});

test("darwin keeps its menu bar and open focuses an existing window", () => {
  FakeWindow.reset();
  const { instance } = host({ platform: "darwin" });
  instance.open();
  const win = FakeWindow.last();
  assert.equal(win.menuBar, true, "macOS keeps the application menu");
  instance.open();
  assert.equal(FakeWindow.created.length, 1, "a second open never stacks a duplicate window");
  assert.equal(win.focuses, 1, "the existing window is focused instead");
  assert.equal(win.shows, 1);
  win.minimized = true;
  instance.open();
  assert.equal(win.restores, 1, "a minimized window is restored before it is focused");
  assert.equal(win.focuses, 2);
});

test("close runs the closed bookkeeping once and a later open starts a new window", () => {
  FakeWindow.reset();
  const { instance, closed, opened } = host();
  instance.open();
  const first = FakeWindow.last();
  instance.close();
  assert.equal(first.closes, 1);
  assert.equal(instance.isOpen(), false, "the closed window is forgotten");
  assert.deepEqual(closed, [1], "the shell hears about the close exactly once");
  instance.open();
  assert.equal(FakeWindow.created.length, 2, "reopening creates a fresh window");
  assert.equal(instance.isOpen(), true);
  assert.equal(opened.length, 2, "each open reports its window once");
});

test("closeAll destroys the window for the quit path and tolerates an empty host", () => {
  FakeWindow.reset();
  const { instance, closed } = host();
  instance.closeAll();
  assert.equal(FakeWindow.created.length, 0, "closing nothing opens nothing");
  instance.open();
  const win = FakeWindow.last();
  instance.closeAll();
  assert.equal(win.destroys, 1, "quit destroys rather than closes, so the page cannot block the exit");
  assert.equal(win.closes, 0);
  assert.equal(instance.isOpen(), false);
  assert.deepEqual(closed, [], "the quit path sends no close notification");
  instance.closeAll();
  assert.equal(win.destroys, 1, "a second closeAll is a no-op");
});
