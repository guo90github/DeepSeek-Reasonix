import assert from "node:assert/strict";
import { test } from "node:test";
import { BrowserSurfaceRouting } from "./browserSurfaceRouting.js";
import type { BrowserWindow } from "electron";

function window(id: number, destroyed = false): BrowserWindow {
  return { isDestroyed: () => destroyed, webContents: { id } } as unknown as BrowserWindow;
}

test("the main window owns the views until a browser window claims them", () => {
  const routing = new BrowserSurfaceRouting(() => 7);
  assert.equal(routing.ownerId(), 7, "ownership falls back to the main window");
  assert.equal(routing.accepts(7), true);
  assert.equal(routing.accepts(9), false, "a second renderer cannot move the views");
});

test("the browser window takes over and gives ownership back when it closes", () => {
  const routing = new BrowserSurfaceRouting(() => 7);
  routing.setOwner(window(9));
  assert.equal(routing.ownerId(), 9);
  assert.equal(routing.accepts(9), true);
  assert.equal(routing.accepts(7), false, "the main window's panel no longer reports layout");
  routing.setOwner(null);
  assert.equal(routing.ownerId(), 7, "closing the window hands the views back");
  assert.equal(routing.accepts(7), true);
});

test("a destroyed browser window never keeps ownership and a missing main window accepts nothing", () => {
  const dead = new BrowserSurfaceRouting(() => 7);
  dead.setOwner(window(9, true));
  assert.equal(dead.ownerId(), 7, "a destroyed window is not an owner");

  const headless = new BrowserSurfaceRouting(() => null);
  assert.equal(headless.ownerId(), null);
  assert.equal(headless.accepts(7), false, "with no window at all nothing is accepted");
  assert.equal(headless.accepts(0), false);
});
