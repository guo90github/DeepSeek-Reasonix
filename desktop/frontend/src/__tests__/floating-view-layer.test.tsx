// Run: node --import ./scripts/css-stub-register.mjs --import tsx src/__tests__/floating-view-layer.test.tsx

import { JSDOM } from "jsdom";
import React from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";

import { FloatingViewLayer } from "../components/FloatingViewLayer";
import { LocaleProvider } from "../lib/i18n";
import { VIEW_PLACEMENT_STORAGE_KEY, type ViewPlacement } from "../lib/viewPlacement";
import { resetViewPlacementsForTests } from "../store/viewPlacements";

let passed = 0;
let failed = 0;

function eq<T>(actual: T, expected: T, label: string) {
  if (Object.is(actual, expected)) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}: expected ${String(expected)}, got ${String(actual)}\n`);
    failed += 1;
  }
}

function flushTimers(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

class TestResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

const dom = new JSDOM('<!doctype html><html><body><div id="root"></div></body></html>', {
  pretendToBeVisual: true,
  url: "http://localhost/",
});
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
globalThis.window = dom.window as unknown as typeof globalThis.window;
globalThis.document = dom.window.document;
Object.defineProperty(globalThis, "navigator", { configurable: true, value: dom.window.navigator });
globalThis.Node = dom.window.Node;
globalThis.Element = dom.window.Element;
globalThis.HTMLElement = dom.window.HTMLElement;
globalThis.Event = dom.window.Event;
globalThis.MouseEvent = dom.window.MouseEvent;
globalThis.PointerEvent = dom.window.MouseEvent as unknown as typeof PointerEvent;
globalThis.localStorage = dom.window.localStorage;
globalThis.ResizeObserver = TestResizeObserver as unknown as typeof ResizeObserver;
globalThis.requestAnimationFrame = dom.window.requestAnimationFrame.bind(dom.window);
globalThis.cancelAnimationFrame = dom.window.cancelAnimationFrame.bind(dom.window);

const domWindow = dom.window as unknown as Window & typeof globalThis;
const viewport = { width: domWindow.innerWidth, height: domWindow.innerHeight };

function floatPlacement(viewId: string, x: number, y: number, w = 420, h = 320, z = 1): ViewPlacement {
  return { viewId, surface: "float", rect: { x, y, w, h }, z };
}

const rendered: string[] = [];
function renderView(viewId: string) {
  rendered.push(viewId);
  return React.createElement("div", { "data-testid": `view-${viewId}` }, viewId);
}

function titleOf(viewId: string) {
  return `Title ${viewId}`;
}

let mountedRoot: ReturnType<typeof createRoot> | null = null;

async function mount() {
  if (mountedRoot) {
    const previous = mountedRoot;
    await act(async () => {
      previous.unmount();
    });
  }
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root = createRoot(container);
  mountedRoot = root;
  // Renders are counted per mount, so a previous tree's calls never leak in.
  rendered.length = 0;
  await act(async () => {
    root.render(
      <LocaleProvider>
        <FloatingViewLayer renderView={renderView} titleOf={titleOf} />
      </LocaleProvider>,
    );
    await flushTimers();
  });
  return root;
}

function panel(viewId: string): HTMLElement {
  const element = document.querySelector<HTMLElement>(`[data-view-id="${viewId}"]`);
  if (!element) throw new Error(`no panel for ${viewId}`);
  return element;
}

function rectOf(viewId: string) {
  const element = panel(viewId);
  return {
    x: Number.parseFloat(element.style.left),
    y: Number.parseFloat(element.style.top),
    w: Number.parseFloat(element.style.width),
    h: Number.parseFloat(element.style.height),
  };
}

function pointer(target: EventTarget, type: string, x: number, y: number) {
  target.dispatchEvent(new domWindow.MouseEvent(type, { bubbles: true, cancelable: true, clientX: x, clientY: y }));
}

// --- rendering ------------------------------------------------------------

await act(async () => {
  resetViewPlacementsForTests({ "tab-main": { viewId: "tab-main", surface: "main", rect: { x: 0, y: 0, w: 0, h: 0 }, z: 0 } });
});
let root = await mount();
eq(document.querySelectorAll(".floating-view").length, 0, "a view still on the main surface has no panel");
eq(rendered.length, 0, "the main surface's view is not rendered by the layer");

await act(async () => {
  resetViewPlacementsForTests({ "tab-a": floatPlacement("tab-a", 120, 90) });
});
root = await mount();
eq(document.querySelectorAll(".floating-view").length, 1, "a floating placement renders one panel");
eq(rendered.filter((id) => id === "tab-a").length, 1, "the floating view is rendered exactly once");
eq(rectOf("tab-a").x, 120, "the panel uses the stored x");
eq(rectOf("tab-a").y, 90, "the panel uses the stored y");
eq(panel("tab-a").style.zIndex.startsWith("calc("), true, "the panel stacks through the layout z token");

// --- dragging -------------------------------------------------------------

await act(async () => {
  pointer(panel("tab-a").querySelector(".floating-view__header")!, "pointerdown", 200, 200);
  pointer(domWindow, "pointermove", 260, 240);
  await flushTimers();
});
eq(rectOf("tab-a").x, 180, "a header drag moves the panel by the delta");
eq(rectOf("tab-a").y, 130, "a header drag moves the panel on both axes");

await act(async () => {
  pointer(domWindow, "pointerup", 260, 240);
  await flushTimers();
});
const stored = JSON.parse(domWindow.localStorage.getItem(VIEW_PLACEMENT_STORAGE_KEY) ?? "{}") as Record<string, ViewPlacement>;
eq(stored["tab-a"]?.rect.x, 180, "a finished drag writes the new geometry to storage");

await act(async () => {
  pointer(panel("tab-a").querySelector(".floating-view__header")!, "pointerdown", 260, 240);
  pointer(domWindow, "pointermove", 5000, 5000);
  pointer(domWindow, "pointerup", 5000, 5000);
  await flushTimers();
});
const clamped = rectOf("tab-a");
eq(clamped.x, viewport.width - clamped.w - 8, "a drag past the edge stops inside the viewport");
eq(clamped.y, viewport.height - clamped.h - 8, "the same applies vertically");

// --- resizing -------------------------------------------------------------

await act(async () => {
  resetViewPlacementsForTests({ "tab-a": floatPlacement("tab-a", 120, 90) });
  await flushTimers();
});
const beforeResize = rectOf("tab-a");
eq(beforeResize.x, 120, "the panel is back with room to grow before the resize checks");
await act(async () => {
  pointer(panel("tab-a").querySelector('[data-handle="se"]')!, "pointerdown", 500, 480);
  pointer(domWindow, "pointermove", 560, 520);
  pointer(domWindow, "pointerup", 560, 520);
  await flushTimers();
});
const afterResize = rectOf("tab-a");
eq(afterResize.w, beforeResize.w + 60, "the se handle grows the width");
eq(afterResize.h, beforeResize.h + 40, "the se handle grows the height");
eq(afterResize.x, beforeResize.x, "a resize leaves the anchored edge alone");

await act(async () => {
  pointer(panel("tab-a").querySelector('[data-handle="w"]')!, "pointerdown", 400, 300);
  pointer(domWindow, "pointermove", 800, 300);
  pointer(domWindow, "pointerup", 800, 300);
  await flushTimers();
});
eq(rectOf("tab-a").w, 360, "a resize stops at the minimum width");

// --- stacking -------------------------------------------------------------

await act(async () => {
  resetViewPlacementsForTests({
    "tab-a": floatPlacement("tab-a", 60, 60, 400, 300, 1),
    "tab-b": floatPlacement("tab-b", 300, 180, 400, 300, 2),
  });
});
root = await mount();
const topBefore = panel("tab-b").style.zIndex;
eq(panel("tab-a").style.zIndex < panel("tab-b").style.zIndex, true, "panels stack by their stored order");
await act(async () => {
  pointer(panel("tab-a").querySelector(".floating-view__header")!, "pointerdown", 100, 100);
  pointer(domWindow, "pointerup", 100, 100);
  await flushTimers();
});
eq(panel("tab-a").style.zIndex > topBefore, true, "touching a panel raises it above the others");
eq(document.querySelectorAll(".floating-view").length, 2, "raising a panel does not add or remove panels");

// --- docking back ---------------------------------------------------------

await act(async () => {
  (panel("tab-a").querySelector(".floating-view__action") as HTMLButtonElement).click();
  await flushTimers();
});
eq(document.querySelector('[data-view-id="tab-a"]'), null, "docking back removes the panel");
eq(document.querySelectorAll(".floating-view").length, 1, "the other panel stays floating");
eq(renderView.length > 0, true, "views were rendered while floating");

await act(async () => {
  root.unmount();
});
dom.window.close();

console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
