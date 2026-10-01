// View placement: which surface carries a workspace tab's view, and where a
// floating panel sits. Geometry is clamped the way layout sizes are
// (store/layout.ts): a panel never leaves the viewport, never shrinks below its
// minimum, and a gesture keeps the edge it is anchored on.

export type ViewSurface = "main" | "float";

export type FloatingRect = { x: number; y: number; w: number; h: number };

export type ViewPlacement = {
  viewId: string;
  surface: ViewSurface;
  /** Meaningful only for surface "float"; window coordinates. */
  rect: FloatingRect;
  /** Stacking order; higher is closer to the reader. */
  z: number;
};

export type ResizeHandle = "n" | "s" | "e" | "w" | "ne" | "nw" | "se" | "sw";

export type Viewport = { width: number; height: number };

export const FLOATING_MIN_W = 360;
export const FLOATING_MIN_H = 240;
/** How much of the viewport a floating panel must leave around itself. */
export const FLOATING_MARGIN = 8;
/** Offset between successive tear-offs so panels do not stack exactly. */
export const FLOATING_CASCADE = 24;

const STORAGE_KEY = "reasonix.viewPlacements.v1";

function clampNumber(value: number, min: number, max: number): number {
  if (!Number.isFinite(value)) return min;
  return Math.min(Math.max(value, min), max);
}

/** The width a panel may not shrink below in this viewport. */
export function minFloatingWidth(viewport: Viewport): number {
  return Math.min(FLOATING_MIN_W, Math.max(Math.floor(viewport.width - FLOATING_MARGIN * 2), 1));
}

/** The height a panel may not shrink below in this viewport. */
export function minFloatingHeight(viewport: Viewport): number {
  return Math.min(FLOATING_MIN_H, Math.max(Math.floor(viewport.height - FLOATING_MARGIN * 2), 1));
}

/** Keeps a rectangle inside the viewport and above its minimum size. */
export function clampFloatingRect(rect: FloatingRect, viewport: Viewport): FloatingRect {
  const minW = minFloatingWidth(viewport);
  const minH = minFloatingHeight(viewport);
  const maxW = Math.max(viewport.width - FLOATING_MARGIN * 2, minW);
  const maxH = Math.max(viewport.height - FLOATING_MARGIN * 2, minH);
  const w = clampNumber(rect.w, minW, maxW);
  const h = clampNumber(rect.h, minH, maxH);
  const maxX = Math.max(viewport.width - w - FLOATING_MARGIN, FLOATING_MARGIN);
  const maxY = Math.max(viewport.height - h - FLOATING_MARGIN, FLOATING_MARGIN);
  return {
    x: Math.round(clampNumber(rect.x, FLOATING_MARGIN, maxX)),
    y: Math.round(clampNumber(rect.y, FLOATING_MARGIN, maxY)),
    w: Math.round(w),
    h: Math.round(h),
  };
}

/** Moves a panel by a pointer delta, never out of the viewport. */
export function dragFloatingRect(rect: FloatingRect, dx: number, dy: number, viewport: Viewport): FloatingRect {
  return clampFloatingRect({ ...rect, x: rect.x + dx, y: rect.y + dy }, viewport);
}

/**
 * Resizes a panel by a pointer delta on one handle. The edge opposite the
 * handle stays put: the minimum size and the viewport bound the moving edge,
 * never the anchored one.
 */
export function resizeFloatingRect(
  rect: FloatingRect,
  handle: ResizeHandle,
  dx: number,
  dy: number,
  viewport: Viewport,
): FloatingRect {
  const minW = minFloatingWidth(viewport);
  const minH = minFloatingHeight(viewport);
  const left = rect.x;
  const top = rect.y;
  const right = rect.x + rect.w;
  const bottom = rect.y + rect.h;
  let nLeft = left;
  let nTop = top;
  let nRight = right;
  let nBottom = bottom;

  if (handle.includes("w")) nLeft = left + dx;
  if (handle.includes("e")) nRight = right + dx;
  if (handle.includes("n")) nTop = top + dy;
  if (handle.includes("s")) nBottom = bottom + dy;

  if (nRight - nLeft < minW) {
    if (handle.includes("w")) nLeft = nRight - minW;
    else nRight = nLeft + minW;
  }
  if (nBottom - nTop < minH) {
    if (handle.includes("n")) nTop = nBottom - minH;
    else nBottom = nTop + minH;
  }

  const maxRight = Math.max(viewport.width - FLOATING_MARGIN, FLOATING_MARGIN + minW);
  const maxBottom = Math.max(viewport.height - FLOATING_MARGIN, FLOATING_MARGIN + minH);
  if (handle.includes("w")) nLeft = Math.max(FLOATING_MARGIN, Math.min(nLeft, nRight - minW));
  else nRight = Math.min(maxRight, Math.max(nRight, nLeft + minW));
  if (handle.includes("n")) nTop = Math.max(FLOATING_MARGIN, Math.min(nTop, nBottom - minH));
  else nBottom = Math.min(maxBottom, Math.max(nBottom, nTop + minH));

  return {
    x: Math.round(nLeft),
    y: Math.round(nTop),
    w: Math.round(nRight - nLeft),
    h: Math.round(nBottom - nTop),
  };
}

/**
 * The first rectangle a tear-off uses: readable by default, offset from the
 * panels already open so two tear-offs never cover each other exactly.
 */
export function defaultFloatingRect(viewport: Viewport, taken: readonly FloatingRect[] = []): FloatingRect {
  const minW = minFloatingWidth(viewport);
  const minH = minFloatingHeight(viewport);
  const w = Math.round(clampNumber(viewport.width * 0.6, minW, Math.max(viewport.width - FLOATING_MARGIN * 2, minW)));
  const h = Math.round(clampNumber(viewport.height * 0.6, minH, Math.max(viewport.height - FLOATING_MARGIN * 2, minH)));
  const centered = { x: Math.round((viewport.width - w) / 2), y: Math.round((viewport.height - h) / 2), w, h };
  let rect = clampFloatingRect(centered, viewport);
  let guard = 0;
  while (taken.some((other) => other.x === rect.x && other.y === rect.y) && guard < taken.length + 1) {
    rect = clampFloatingRect({ ...rect, x: rect.x + FLOATING_CASCADE, y: rect.y + FLOATING_CASCADE }, viewport);
    guard += 1;
  }
  return rect;
}

function isRect(value: unknown): value is FloatingRect {
  if (!value || typeof value !== "object") return false;
  const rect = value as Partial<FloatingRect>;
  return [rect.x, rect.y, rect.w, rect.h].every((part) => typeof part === "number" && Number.isFinite(part));
}

/** Accepts only well-formed placements; anything else is dropped on load. */
export function normalizeViewPlacements(raw: unknown): Record<string, ViewPlacement> {
  if (!raw || typeof raw !== "object") return {};
  const out: Record<string, ViewPlacement> = {};
  for (const [viewId, candidate] of Object.entries(raw as Record<string, unknown>)) {
    if (!viewId || !candidate || typeof candidate !== "object") continue;
    const entry = candidate as Partial<ViewPlacement>;
    if (entry.surface !== "main" && entry.surface !== "float") continue;
    if (!isRect(entry.rect)) continue;
    out[viewId] = {
      viewId,
      surface: entry.surface,
      rect: { x: entry.rect.x, y: entry.rect.y, w: entry.rect.w, h: entry.rect.h },
      z: typeof entry.z === "number" && Number.isFinite(entry.z) ? entry.z : 0,
    };
  }
  return out;
}

export function loadViewPlacements(): Record<string, ViewPlacement> {
  if (typeof window === "undefined") return {};
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    if (!raw) return {};
    return normalizeViewPlacements(JSON.parse(raw));
  } catch {
    return {};
  }
}

export function saveViewPlacements(placements: Record<string, ViewPlacement>): void {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(placements));
  } catch {
    // Storage may be unavailable or full; placement is a preference, not data.
  }
}

export const VIEW_PLACEMENT_STORAGE_KEY = STORAGE_KEY;
