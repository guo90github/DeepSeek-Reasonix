// Run: tsx src/__tests__/use-frameless-bar-drag.test.tsx
// A maximised frameless window gets no native caption drag on Windows, so the
// bar drops its drag region and the shell moves the window instead. The hook
// owns that press: it must claim a press on the bar itself, release it on
// mouseup, and never claim a press on a control or on an unmaximised window.

import assert from "node:assert/strict";
import { JSDOM } from "jsdom";

const dom = new JSDOM("<div id='root'></div>", { pretendToBeVisual: true });
Object.assign(globalThis, {
  window: dom.window,
  document: dom.window.document,
  HTMLElement: dom.window.HTMLElement,
  MouseEvent: dom.window.MouseEvent,
  IS_REACT_ACT_ENVIRONMENT: true,
});

const { default: React, act } = await import("react");
const { createRoot } = await import("react-dom/client");
const { useFramelessBarDrag } = await import("../app-runtime/useNativeWindowController");
const { setMainWindowMaximised } = await import("../store/windowChrome");
const { desktopHost } = await import("../lib/desktopHost");

const moves: Array<{ x: number; y: number }> = [];
let ends = 0;
const previousHost = window.reasonixDesktop;
const shell = desktopHost();
window.reasonixDesktop = {
  ...(previousHost ?? {}),
  kind: "electron",
  native: {
    window: {
      beginMove: (x: number, y: number) => {
        moves.push({ x, y });
        return Promise.resolve();
      },
      endMove: () => {
        ends += 1;
        return Promise.resolve();
      },
    },
  },
} as unknown as typeof window.reasonixDesktop;
assert.equal(typeof desktopHost().native.beginWindowMove, "function", "the stub must reach the real desktopHost() path");
assert.ok(shell);

function Harness({ enabled }: { enabled: boolean }) {
  useFramelessBarDrag(enabled);
  return <div className="topicbar">
    <div id="blank" />
    <button id="control" type="button">x</button>
  </div>;
}

const root = createRoot(document.getElementById("root")!);
await act(async () => {
  root.render(<Harness enabled />);
});

const press = (id: string, x = 640, y = 24) => {
  const node = document.getElementById(id)!;
  node.dispatchEvent(new dom.window.MouseEvent("mousedown", { bubbles: true, button: 0, clientX: x, clientY: y }));
};
const release = () => window.dispatchEvent(new dom.window.MouseEvent("mouseup", { bubbles: true, button: 0 }));

setMainWindowMaximised(true);
press("blank");
assert.deepEqual(moves, [{ x: 640, y: 24 }], "a press on the bar starts a shell-driven move");
assert.equal(document.documentElement.dataset.windowDrag, "js", "the bar drops its drag region for the drag");
release();
assert.equal(ends, 1, "releasing ends the move");
assert.equal(document.documentElement.dataset.windowDrag, undefined, "the drag region comes back after the move");

press("control");
assert.equal(moves.length, 1, "a press on a control inside the bar never moves the window");
assert.equal(document.documentElement.dataset.windowDrag, undefined);

setMainWindowMaximised(false);
press("blank");
assert.equal(moves.length, 1, "an unmaximised window keeps the native drag region and is left alone");
assert.equal(ends, 1);

setMainWindowMaximised(true);
await act(async () => {
  root.render(<Harness enabled={false} />);
});
press("blank");
assert.equal(moves.length, 1, "a non-frameless platform issues no move");

await act(async () => {
  root.unmount();
});
window.reasonixDesktop = previousHost;
console.log("\nuse-frameless-bar-drag ok");
