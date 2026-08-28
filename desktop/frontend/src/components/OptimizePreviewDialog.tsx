import { useCallback, useEffect, useId, useLayoutEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import { createPortal } from "react-dom";
import { useI18n } from "../lib/i18n";

// One prompt-optimization run: the panel opens while status is "streaming" and
// the accumulated text grows with each chunk; "done" flips the result into an
// editable textarea. The original draft is kept for side-by-side review.
export type OptimizeRun = {
  status: "streaming" | "done";
  original: string;
  text: string;
};

type ResizeHandle = "n" | "s" | "e" | "w" | "ne" | "nw" | "se" | "sw";
const RESIZE_HANDLES: readonly ResizeHandle[] = ["n", "s", "e", "w", "ne", "nw", "se", "sw"];
const RESIZE_CURSOR: Record<ResizeHandle, string> = {
  n: "ns-resize",
  s: "ns-resize",
  e: "ew-resize",
  w: "ew-resize",
  ne: "nesw-resize",
  sw: "nesw-resize",
  nw: "nwse-resize",
  se: "nwse-resize",
};

// The dialog grows/shrinks around the backdrop's flex center, so dragging an
// edge moves both edges symmetrically. The user's size is remembered across
// opens within the session; CSS provides the default until first drag.
const OPTIMIZE_DIALOG_MIN_WIDTH = 440;
const OPTIMIZE_DIALOG_MIN_HEIGHT = 300;
const OPTIMIZE_DIALOG_VIEWPORT_MARGIN = 96;
let lastDialogSize: { width: number; height: number } | null = null;

// Preview/confirm panel for the composer's prompt-optimization utility: the
// optimized draft is streamed in and reviewed (and edited) here before it ever
// touches the composer, so the user approves the rewrite instead of being
// surprised by an in-place replace.
export function OptimizePreviewDialog({
  run,
  onApply,
  onCancel,
}: {
  run: OptimizeRun;
  onApply: (edited: string) => void;
  onCancel: () => void;
}) {
  const { t } = useI18n();
  const titleId = useId();
  const bodyId = useId();
  const cancelRef = useRef<HTMLButtonElement>(null);
  const applyRef = useRef<HTMLButtonElement>(null);
  const restoreFocusRef = useRef<HTMLElement | null>(null);
  const streamBodyRef = useRef<HTMLDivElement>(null);
  const dialogRef = useRef<HTMLDivElement>(null);
  const [showOriginal, setShowOriginal] = useState(false);
  const [edited, setEdited] = useState<string | null>(null);

  const streaming = run.status === "streaming";
  const displayText = streaming ? run.text : (edited ?? run.text);

  // Re-apply the user's last resized dimensions (clamped to the viewport); CSS
  // provides the default size until the first drag.
  useLayoutEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog || !lastDialogSize) return;
    const maxWidth = Math.max(OPTIMIZE_DIALOG_MIN_WIDTH, window.innerWidth - OPTIMIZE_DIALOG_VIEWPORT_MARGIN);
    const maxHeight = Math.max(OPTIMIZE_DIALOG_MIN_HEIGHT, window.innerHeight - OPTIMIZE_DIALOG_VIEWPORT_MARGIN);
    dialog.style.width = `${Math.min(lastDialogSize.width, maxWidth)}px`;
    dialog.style.height = `${Math.min(lastDialogSize.height, maxHeight)}px`;
  }, []);

  // Pointer-drag resize on the eight edge/corner handles. Same lifecycle idiom
  // as ResizableDrawer: window pointer listeners + rAF-batched style writes.
  const startResize = useCallback((handle: ResizeHandle) => (event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.button !== 0) return;
    const dialog = dialogRef.current;
    if (!dialog) return;
    event.preventDefault();
    const rect = dialog.getBoundingClientRect();
    const start = { x: event.clientX, y: event.clientY, width: rect.width, height: rect.height };
    let nextWidth = start.width;
    let nextHeight = start.height;
    let frame: number | null = null;
    const applySize = () => {
      frame = null;
      dialog.style.width = `${nextWidth}px`;
      dialog.style.height = `${nextHeight}px`;
    };
    const onMove = (moveEvent: PointerEvent) => {
      if (handle.includes("e")) nextWidth = start.width + (moveEvent.clientX - start.x);
      if (handle.includes("w")) nextWidth = start.width - (moveEvent.clientX - start.x);
      if (handle.includes("s")) nextHeight = start.height + (moveEvent.clientY - start.y);
      if (handle.includes("n")) nextHeight = start.height - (moveEvent.clientY - start.y);
      const maxWidth = Math.max(OPTIMIZE_DIALOG_MIN_WIDTH, window.innerWidth - OPTIMIZE_DIALOG_VIEWPORT_MARGIN);
      const maxHeight = Math.max(OPTIMIZE_DIALOG_MIN_HEIGHT, window.innerHeight - OPTIMIZE_DIALOG_VIEWPORT_MARGIN);
      nextWidth = Math.min(Math.max(nextWidth, OPTIMIZE_DIALOG_MIN_WIDTH), maxWidth);
      nextHeight = Math.min(Math.max(nextHeight, OPTIMIZE_DIALOG_MIN_HEIGHT), maxHeight);
      if (frame === null) frame = requestAnimationFrame(applySize);
    };
    const onDone = () => {
      if (frame !== null) {
        cancelAnimationFrame(frame);
        frame = null;
      }
      applySize();
      lastDialogSize = { width: nextWidth, height: nextHeight };
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onDone);
      window.removeEventListener("pointercancel", onDone);
    };
    document.body.style.cursor = RESIZE_CURSOR[handle];
    document.body.style.userSelect = "none";
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onDone);
    window.addEventListener("pointercancel", onDone);
  }, []);

  // Keep the latest streamed text visible while the optimizer is producing.
  useEffect(() => {
    if (!streaming || !streamBodyRef.current) return;
    streamBodyRef.current.scrollTop = streamBodyRef.current.scrollHeight;
  }, [run.text, streaming]);

  useLayoutEffect(() => {
    restoreFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    cancelRef.current?.focus();
    return () => {
      if (restoreFocusRef.current?.isConnected) restoreFocusRef.current.focus();
    };
  }, []);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        onCancel();
        return;
      }
      if (event.key !== "Tab") return;
      const first = cancelRef.current;
      const last = applyRef.current;
      if (!first || !last) return;
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKeyDown, { capture: true });
    return () => document.removeEventListener("keydown", onKeyDown, { capture: true });
  }, [onCancel]);

  return createPortal(
    <div
      className="modal-backdrop reasonix-optimize-backdrop"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onCancel();
      }}
    >
      <div
        ref={dialogRef}
        className="modal reasonix-optimize-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={bodyId}
      >
        <div className="modal__title reasonix-optimize-dialog__title" id={titleId}>
          {t("composer.optimizePreviewTitle")}
        </div>
        <div className="reasonix-optimize-dialog__body" id={bodyId}>
          {showOriginal && (
            <div className="reasonix-optimize-dialog__block">
              <div className="reasonix-optimize-dialog__label">{t("composer.optimizePreviewOriginal")}</div>
              <pre className="reasonix-optimize-dialog__text reasonix-optimize-dialog__text--original">{run.original}</pre>
            </div>
          )}
          <div className="reasonix-optimize-dialog__block">
            <div className="reasonix-optimize-dialog__label">
              {streaming ? t("composer.optimizePrompting") : t("composer.optimizePreviewResult")}
              <button
                type="button"
                className="reasonix-optimize-dialog__toggle"
                onClick={() => setShowOriginal((v) => !v)}
                aria-expanded={showOriginal}
              >
                {showOriginal ? t("composer.optimizePreviewHideOriginal") : t("composer.optimizePreviewShowOriginal")}
              </button>
            </div>
            {streaming ? (
              <div className="reasonix-optimize-dialog__streaming" ref={streamBodyRef}>
                {run.text ? (
                  <pre className="reasonix-optimize-dialog__text reasonix-optimize-dialog__text--stream">{run.text}</pre>
                ) : (
                  <span className="reasonix-optimize-dialog__placeholder">{t("composer.optimizePreviewStreamingHint")}</span>
                )}
                <span className="reasonix-optimize-dialog__spinner" aria-hidden="true" />
              </div>
            ) : (
              <textarea
                className="reasonix-optimize-dialog__text reasonix-optimize-dialog__textarea"
                value={displayText}
                onChange={(event) => setEdited(event.target.value)}
                spellCheck={false}
                aria-label={t("composer.optimizePreviewResult")}
              />
            )}
          </div>
        </div>
        <div className="modal__actions reasonix-optimize-dialog__actions">
          <button ref={cancelRef} className="btn btn--small" type="button" onClick={onCancel}>
            {t("common.cancel")}
          </button>
          <button
            ref={applyRef}
            className="btn btn--small btn--primary"
            type="button"
            disabled={streaming}
            onClick={() => onApply(displayText)}
          >
            {t("composer.optimizePreviewApply")}
          </button>
        </div>
        {RESIZE_HANDLES.map((handle) => (
          <div
            key={handle}
            className={`reasonix-optimize-dialog__handle reasonix-optimize-dialog__handle--${handle}`}
            onPointerDown={startResize(handle)}
            aria-hidden="true"
          />
        ))}
      </div>
    </div>,
    document.body,
  );
}
