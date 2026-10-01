// Run: tsx src/__tests__/use-frameless-bar-drag.test.tsx
// Windows leaves the bar without a native drag region, so the hook owns a press
// on the bar: it must claim it, release it on mouseup or blur, and never claim a
// press on a control inside the bar or on a non-frameless platform.

import assert from "node:assert/strict";
import { JSDOM } from "jsdom";

const dom = new JSDOM("<div id='root'></div>", { pretendToBeVisual: true });
Object.assign(globalThis, {
  window: dom.window,
  document: dom.window.document,
  HTMLElement: dom.window.HTMLElement,
  MouseEvent: dom.window.MouseEvent,
  Event: dom.window.Event,
  IS_REACT_ACT_ENVIRONMENT: true,
});

const { default: React, act } = await import("react");
const { createRoot } = await import("react-dom/client");
const { useFramelessBarDrag } = await import("../app-runtime/useNativeWindowController");
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

press("blank");
assert.deepEqual(moves, [{ x: 640, y: 24 }], "a press on the bar starts a shell-driven move");
release();
assert.equal(ends, 1, "releasing ends the move");

press("control");
assert.equal(moves.length, 1, "a press on a control inside the bar never moves the window");
release();
assert.equal(ends, 1, "a press that started no move ends none");

press("blank");
assert.equal(moves.length, 2, "the bar is draggable again after a release");
window.dispatchEvent(new dom.window.Event("blur"));
assert.equal(ends, 2, "losing focus ends the move");

await act(async () => {
  root.render(<Harness enabled={false} />);
});
press("blank");
assert.equal(moves.length, 2, "a non-frameless platform issues no move");

await act(async () => {
  root.unmount();
});
window.reasonixDesktop = previousHost;
console.log("\nuse-frameless-bar-drag ok");
