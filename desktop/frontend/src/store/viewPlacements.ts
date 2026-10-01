// Placement of every workspace tab's view: the main surface carries it, or a
// floating panel does. Setters are pure (state only) exactly like layout.ts;
// callers flush to storage with saveFloatingPlacements when a gesture ends.
import { create } from "zustand";

import {
  clampFloatingRect,
  defaultFloatingRect,
  loadViewPlacements,
  saveViewPlacements,
  type FloatingRect,
  type ViewPlacement,
  type Viewport,
} from "../lib/viewPlacement";

export type ViewPlacementState = {
  placements: Record<string, ViewPlacement>;
};

export const useViewPlacementStore = create<ViewPlacementState>(() => ({
  placements: loadViewPlacements(),
}));

function withPlacement(viewId: string, next: (current: ViewPlacement | undefined) => ViewPlacement): void {
  useViewPlacementStore.setState((state) => ({
    placements: { ...state.placements, [viewId]: next(state.placements[viewId]) },
  }));
}

/** The next stacking order above everything currently placed. */
export function nextFloatingZ(placements: Record<string, ViewPlacement>): number {
  let top = 0;
  for (const placement of Object.values(placements)) top = Math.max(top, placement.z);
  return top + 1;
}

/** Floating placements in stacking order, lowest first. */
export function selectFloatingPlacements(placements: Record<string, ViewPlacement>): ViewPlacement[] {
  return Object.values(placements)
    .filter((placement) => placement.surface === "float")
    .sort((a, b) => a.z - b.z || a.viewId.localeCompare(b.viewId));
}

export function placementFor(viewId: string): ViewPlacement | undefined {
  return useViewPlacementStore.getState().placements[viewId];
}

/** Tears one tab's view off the main surface into a floating panel. */
export function tearOffView(viewId: string, viewport: Viewport): FloatingRect {
  const state = useViewPlacementStore.getState();
  const taken = selectFloatingPlacements(state.placements).map((placement) => placement.rect);
  const rect = defaultFloatingRect(viewport, taken);
  const z = nextFloatingZ(state.placements);
  withPlacement(viewId, () => ({ viewId, surface: "float", rect, z }));
  return rect;
}

/** Hands a view back to the main surface. */
export function dockViewToMain(viewId: string): void {
  withPlacement(viewId, (current) => ({
    viewId,
    surface: "main",
    rect: current?.rect ?? { x: 0, y: 0, w: 0, h: 0 },
    z: current?.z ?? 0,
  }));
}

export function setFloatingRect(viewId: string, rect: FloatingRect, viewport: Viewport): void {
  withPlacement(viewId, (current) => ({
    viewId,
    surface: "float",
    rect: clampFloatingRect(rect, viewport),
    z: current?.z ?? 0,
  }));
}

/** Raises a panel above the others without moving it. */
export function bringViewToFront(viewId: string): void {
  const placements = useViewPlacementStore.getState().placements;
  const current = placements[viewId];
  if (!current || current.surface !== "float") return;
  if (current.z === nextFloatingZ(placements) - 1) return;
  withPlacement(viewId, (entry) => ({ ...(entry ?? current), z: nextFloatingZ(placements) }));
}

/** Keeps every floating panel inside a viewport that just changed. */
export function clampAllFloatingRects(viewport: Viewport): void {
  const placements = useViewPlacementStore.getState().placements;
  const next: Record<string, ViewPlacement> = {};
  let changed = false;
  for (const [viewId, placement] of Object.entries(placements)) {
    if (placement.surface !== "float") {
      next[viewId] = placement;
      continue;
    }
    const rect = clampFloatingRect(placement.rect, viewport);
    const same = rect.x === placement.rect.x && rect.y === placement.rect.y && rect.w === placement.rect.w && rect.h === placement.rect.h;
    if (!same) changed = true;
    next[viewId] = { ...placement, rect };
  }
  if (changed) useViewPlacementStore.setState({ placements: next });
}

export function flushViewPlacements(): void {
  saveViewPlacements(useViewPlacementStore.getState().placements);
}

/** Test seam: the store is module state, so suite isolation needs a reset. */
export function resetViewPlacementsForTests(placements: Record<string, ViewPlacement> = {}): void {
  useViewPlacementStore.setState({ placements });
}
