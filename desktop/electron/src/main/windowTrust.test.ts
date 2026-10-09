import assert from "node:assert/strict";
import { test } from "node:test";
import type { WebContents, WebFrameMain } from "electron";
import { RendererTrust } from "./windowTrust.js";

const frame = (): WebFrameMain => ({}) as WebFrameMain;

function contents(destroyed = false): WebContents {
  const mainFrame = frame();
  return { mainFrame, isDestroyed: () => destroyed } as unknown as WebContents;
}

test("the main window's own main frame is the only trusted sender by default", () => {
  const main = contents();
  const trust = new RendererTrust(() => ({ webContents: main }));
  assert.equal(trust.isTrusted(main, main.mainFrame), true);
  assert.equal(trust.isTrusted(main, frame()), false, "a sub-frame of the main window is not trusted");
  assert.equal(trust.isTrusted(main, null), false, "a sender without a frame is never trusted");
  assert.equal(trust.isTrusted(contents(), frame()), false, "another window is not trusted until it is added");
});

test("a second window becomes trusted for its main frame only", () => {
  const main = contents();
  const second = contents();
  const trust = new RendererTrust(() => ({ webContents: main }));
  trust.trust(second);
  assert.equal(trust.isTrusted(second, second.mainFrame), true, "the browser window's own frame may call the shell");
  assert.equal(trust.isTrusted(second, frame()), false, "an iframe inside that window may not");
  assert.equal(trust.isTrusted(main, main.mainFrame), true, "the main window stays trusted");
});

test("forget revokes a closed window and destroyed contents are never trusted", () => {
  const main = contents();
  const second = contents();
  const trust = new RendererTrust(() => ({ webContents: main }));
  trust.trust(second);
  trust.forget(second);
  assert.equal(trust.isTrusted(second, second.mainFrame), false, "the closed window loses its access");

  const dead = contents(true);
  trust.trust(dead);
  assert.equal(trust.isTrusted(dead, dead.mainFrame), false, "a destroyed renderer is not registered");

  const headless = new RendererTrust(() => null);
  assert.equal(headless.isTrusted(second, second.mainFrame), false, "with no main window nothing is trusted");
  headless.trust(second);
  assert.equal(headless.isTrusted(second, second.mainFrame), true, "a trusted extra still works without the main window");
});
