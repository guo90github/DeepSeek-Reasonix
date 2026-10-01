// FloatingViewLayer hosts the workspace tabs a reader tore off the main view.
// One panel per placement, each draggable by its header and resizable from its
// edges and corners; geometry stays inside the viewport (lib/viewPlacement).
// A view is rendered here only while its placement says surface="float", so it
// is never carried by two surfaces at once.

import { useCallback, useEffect, useRef, useState, type PointerEvent as ReactPointerEvent, type ReactNode } from "react";

import { useT } from "../lib/i18n";
import {
  dragFloatingRect,
  resizeFloatingRect,
  type FloatingRect,
  type ResizeHandle,
  type Viewport,
} from "../lib/viewPlacement";
import {
  bringViewToFront,
  clampAllFloatingRects,
  dockViewToMain,
  flushViewPlacements,
  selectFloatingPlacements,
  setFloatingRect,
  useViewPlacementStore,
} from "../store/viewPlacements";

export type FloatingViewLayerProps = {
  /** The view a panel carries. Called once per floating placement. */
  renderView: (viewId: string) => ReactNode;
  /** Panel title, usually the tab's own title. */
  titleOf: (viewId: string) => string;
};

const RESIZE_HANDLES: ResizeHandle[] = ["n", "s", "e", "w", "ne", "nw", "se", "sw"];

type Gesture =
  | { kind: "drag"; viewId: string; startX: number; startY: number; rect: FloatingRect }
  | { kind: "resize"; viewId: string; handle: ResizeHandle; startX: number; startY: number; rect: FloatingRect };

function currentViewport(): Viewport {
  if (typeof window === "undefined") return { width: 1280, height: 800 };
  return { width: window.innerWidth, height: window.innerHeight };
}

export function FloatingViewLayer({ renderView, titleOf }: FloatingViewLayerProps) {
  const t = useT();
  const placements = useViewPlacementStore((state) => state.placements);
  const floating = selectFloatingPlacements(placements);
  const [viewport, setViewport] = useState<Viewport>(currentViewport);
  const viewportRef = useRef(viewport);
  const gestureRef = useRef<Gesture | null>(null);
  viewportRef.current = viewport;

  useEffect(() => {
    const update = () => setViewport(currentViewport());
    window.addEventListener("resize", update);
    return () => window.removeEventListener("resize", update);
  }, []);

  useEffect(() => {
    clampAllFloatingRects(viewport);
  }, [viewport]);

  // A gesture listens on the window so a fast pointer cannot leave the panel
  // behind, and writes storage once on release instead of on every move.
  const beginGesture = useCallback((gesture: Gesture) => {
    gestureRef.current = gesture;
    const onMove = (event: PointerEvent) => {
      const active = gestureRef.current;
      if (!active) return;
      const dx = event.clientX - active.startX;
      const dy = event.clientY - active.startY;
      const bounds = viewportRef.current;
      const rect =
        active.kind === "drag"
          ? dragFloatingRect(active.rect, dx, dy, bounds)
          : resizeFloatingRect(active.rect, active.handle, dx, dy, bounds);
      setFloatingRect(active.viewId, rect, bounds);
    };
    const onEnd = () => {
      gestureRef.current = null;
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onEnd);
      window.removeEventListener("pointercancel", onEnd);
      flushViewPlacements();
    };
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onEnd);
    window.addEventListener("pointercancel", onEnd);
  }, []);

  if (floating.length === 0) return null;

  return (
    <div className="floating-view-layer" data-testid="floating-view-layer" aria-label={t("floatingView.layer")}>
      {floating.map((placement) => {
        const { viewId, rect, z } = placement;
        return (
          <section
            key={viewId}
            className="floating-view"
            role="dialog"
            aria-label={titleOf(viewId)}
            data-view-id={viewId}
            style={{
              left: rect.x,
              top: rect.y,
              width: rect.w,
              height: rect.h,
              // The layer's token plus this panel's own order: later panels
              // stack above earlier ones without inventing new z tokens.
              zIndex: `calc(var(--z-workspace-float) + ${z})`,
            }}
            onPointerDown={() => bringViewToFront(viewId)}
          >
            <header
              className="floating-view__header"
              onPointerDown={(event: ReactPointerEvent<HTMLElement>) => {
                event.preventDefault();
                bringViewToFront(viewId);
                beginGesture({ kind: "drag", viewId, startX: event.clientX, startY: event.clientY, rect });
              }}
            >
              <span className="floating-view__title">{titleOf(viewId)}</span>
              <button
                type="button"
                className="floating-view__action"
                onClick={() => {
                  dockViewToMain(viewId);
                  flushViewPlacements();
                }}
              >
                {t("floatingView.dockBack")}
              </button>
            </header>
            <div className="floating-view__body">{renderView(viewId)}</div>
            {RESIZE_HANDLES.map((handle) => (
              <span
                key={handle}
                className={`floating-view__handle floating-view__handle--${handle}`}
                data-handle={handle}
                role="presentation"
                onPointerDown={(event: ReactPointerEvent<HTMLElement>) => {
                  event.preventDefault();
                  event.stopPropagation();
                  bringViewToFront(viewId);
                  beginGesture({ kind: "resize", viewId, handle, startX: event.clientX, startY: event.clientY, rect });
                }}
              />
            ))}
          </section>
        );
      })}
    </div>
  );
}
