// Run: tsx src/__tests__/view-placement.test.ts

import {
  FLOATING_CASCADE,
  FLOATING_MARGIN,
  clampFloatingRect,
  defaultFloatingRect,
  dragFloatingRect,
  loadViewPlacements,
  minFloatingWidth,
  normalizeViewPlacements,
  resizeFloatingRect,
  saveViewPlacements,
} from "../lib/viewPlacement";
import { VIEW_PLACEMENT_STORAGE_KEY } from "../lib/viewPlacement";

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

function rect(x: number, y: number, w: number, h: number) {
  return { x, y, w, h };
}

const backing = new Map<string, string>();
(globalThis as typeof globalThis & { window: unknown }).window = {
  localStorage: {
    getItem: (key: string) => backing.get(key) ?? null,
    setItem: (key: string, value: string) => {
      backing.set(key, value);
    },
    removeItem: (key: string) => {
      backing.delete(key);
    },
  },
};

const viewport = { width: 1000, height: 700 };

// --- clamping -------------------------------------------------------------

const shrunk = clampFloatingRect(rect(100, 100, 10, 10), viewport);
eq(shrunk.w >= 360, true, "a panel never shrinks below the minimum width");
eq(shrunk.h >= 240, true, "a panel never shrinks below the minimum height");
eq(shrunk.x, 100, "clamping a size leaves a legitimate position alone");

const offscreen = clampFloatingRect(rect(-500, 5000, 400, 300), viewport);
eq(offscreen.x, FLOATING_MARGIN, "a panel dragged off the left edge is pulled back in");
eq(offscreen.y, viewport.height - offscreen.h - FLOATING_MARGIN, "a panel dragged off the bottom is pulled back in");

const oversized = clampFloatingRect(rect(0, 0, 5000, 4000), viewport);
eq(oversized.w, viewport.width - FLOATING_MARGIN * 2, "a panel wider than the viewport is trimmed");
eq(oversized.h, viewport.height - FLOATING_MARGIN * 2, "a panel taller than the viewport is trimmed");

const tiny = clampFloatingRect(rect(0, 0, 100, 100), { width: 300, height: 200 });
eq(tiny.w, 284, "in a viewport narrower than the minimum the viewport minus its margins wins");
eq(tiny.h, 184, "the same rule applies to the height");
eq(minFloatingWidth({ width: 300, height: 200 }), 284, "the minimum width follows the viewport");

// --- dragging -------------------------------------------------------------

const dragged = dragFloatingRect(rect(200, 150, 400, 300), 50, 25, viewport);
eq(dragged.x, 250, "a drag moves the panel by the pointer delta (x)");
eq(dragged.y, 175, "a drag moves the panel by the pointer delta (y)");
eq(dragged.w, 400, "a drag never changes the size");

const draggedFar = dragFloatingRect(rect(200, 150, 400, 300), 10_000, 10_000, viewport);
eq(draggedFar.x, viewport.width - 400 - FLOATING_MARGIN, "a drag stops at the right edge");
eq(draggedFar.y, viewport.height - 300 - FLOATING_MARGIN, "a drag stops at the bottom edge");

// --- resizing -------------------------------------------------------------

const grown = resizeFloatingRect(rect(100, 100, 400, 300), "se", 60, 40, viewport);
eq(grown.w, 460, "the se handle grows the width");
eq(grown.h, 340, "the se handle grows the height");
eq(grown.x, 100, "the se handle keeps the left edge");
eq(grown.y, 100, "the se handle keeps the top edge");

const fromWest = resizeFloatingRect(rect(100, 100, 600, 300), "w", 60, 0, viewport);
eq(fromWest.x, 160, "the w handle moves the left edge");
eq(fromWest.w, 540, "the w handle keeps the right edge where it was");
eq(fromWest.x + fromWest.w, 700, "the right edge of a west resize is unchanged");

// The same gesture on a panel already near the floor pulls the left edge back
// instead of shrinking past the minimum.
const westAtFloor = resizeFloatingRect(rect(100, 100, 400, 300), "w", 60, 0, viewport);
eq(westAtFloor.w, 360, "a west resize stops at the minimum width before moving the left edge");
eq(westAtFloor.x + westAtFloor.w, 500, "the right edge still holds when the floor is hit");

const fromNorth = resizeFloatingRect(rect(100, 100, 400, 300), "n", 0, 50, viewport);
eq(fromNorth.y, 150, "the n handle moves the top edge");
eq(fromNorth.y + fromNorth.h, 400, "the bottom edge of a north resize is unchanged");

const floored = resizeFloatingRect(rect(100, 100, 400, 300), "e", -1000, 0, viewport);
eq(floored.w, 360, "a resize stops at the minimum width");
eq(floored.x, 100, "a resize that hits the floor keeps its anchored edge");

const westFloor = resizeFloatingRect(rect(100, 100, 400, 300), "w", 1000, 0, viewport);
eq(westFloor.w, 360, "a west resize stops at the minimum width too");
eq(westFloor.x + westFloor.w, 500, "a west resize that hits the floor keeps the right edge");

const offRight = resizeFloatingRect(rect(600, 100, 300, 300), "e", 5000, 0, viewport);
eq(offRight.x + offRight.w, viewport.width - FLOATING_MARGIN, "a resize stops at the viewport edge");

const corner = resizeFloatingRect(rect(100, 100, 400, 300), "nw", -50, -50, viewport);
eq(corner.x, 50, "the nw handle moves the left edge");
eq(corner.y, 50, "the nw handle moves the top edge");
eq(corner.x + corner.w, 500, "the nw handle keeps the right edge");
eq(corner.y + corner.h, 400, "the nw handle keeps the bottom edge");

// --- defaults -------------------------------------------------------------

const first = defaultFloatingRect(viewport);
eq(first.w, 600, "the first tear-off takes 60% of the viewport width");
eq(first.h, 420, "the first tear-off takes 60% of the viewport height");
eq(first.x > FLOATING_MARGIN && first.y > FLOATING_MARGIN, true, "the first tear-off is centered, not pinned to a corner");

const second = defaultFloatingRect(viewport, [first]);
eq(second.x, first.x + FLOATING_CASCADE, "the second tear-off is offset from the first");
eq(second.y, first.y + FLOATING_CASCADE, "the offset applies on both axes");

const nearEdge = defaultFloatingRect(viewport, [rect(viewport.width - 500, viewport.height - 400, 600, 420)]);
eq(nearEdge.x + nearEdge.w <= viewport.width - FLOATING_MARGIN, true, "a cascaded tear-off stays inside the viewport");

// --- persistence ----------------------------------------------------------

saveViewPlacements({
  "tab-a": { viewId: "tab-a", surface: "float", rect: rect(10, 20, 400, 300), z: 3 },
  "tab-b": { viewId: "tab-b", surface: "main", rect: rect(0, 0, 0, 0), z: 0 },
});
const loaded = loadViewPlacements();
eq(Object.keys(loaded).length, 2, "both placements round-trip through storage");
eq(loaded["tab-a"].rect.x, 10, "geometry round-trips");
eq(loaded["tab-b"].surface, "main", "a docked view round-trips");

backing.set(
  VIEW_PLACEMENT_STORAGE_KEY,
  JSON.stringify({ "tab-a": { viewId: "tab-a", surface: "sideways", rect: rect(1, 2, 3, 4), z: 1 } }),
);
eq(Object.keys(loadViewPlacements()).length, 0, "an unknown surface is dropped on load");

backing.set(VIEW_PLACEMENT_STORAGE_KEY, JSON.stringify({ "tab-a": { surface: "float", rect: { x: 1, y: 2 } } }));
eq(Object.keys(loadViewPlacements()).length, 0, "a malformed rect is dropped on load");

backing.set(VIEW_PLACEMENT_STORAGE_KEY, "{not json");
eq(Object.keys(loadViewPlacements()).length, 0, "unreadable storage falls back to no placements");

eq(
  Object.keys(normalizeViewPlacements({ "tab-a": null, "": { surface: "float", rect: rect(0, 0, 1, 1), z: 0 } })).length,
  0,
  "normalize drops empty ids and null entries",
);

console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
