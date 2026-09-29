// The panel's row detail: a card row is one ellipsised line, so clicking it opens
// the whole record. The modal base is deliberately unbound — it portals into the
// document by default and takes any container — and the card hosts ONE instance
// for every module through PanelDetailProvider, so a module only has to hand its
// row over with PanelRowButton.

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { X } from "lucide-react";
import { useT } from "../lib/i18n";

export type PanelDetail = {
  /** Headline: the record's own name (a path, a commit subject, a note title). */
  title: string;
  /** Short facts under the headline: branch, status, author, scope… */
  meta?: string[];
  /** The part the row truncates: a path, a description, or a live element. */
  body?: ReactNode;
  /** prose (default) wraps text; mono boxes a path/hash; raw leaves it alone. */
  bodyStyle?: "prose" | "mono" | "raw";
};

type PanelDetailSink = (detail: PanelDetail) => void;

const PanelDetailContext = createContext<PanelDetailSink | null>(null);

export function DetailModal({
  detail,
  onClose,
  container,
}: {
  detail: PanelDetail | null;
  onClose: () => void;
  container?: HTMLElement | null;
}) {
  const t = useT();
  const titleId = useId();
  const closeRef = useRef<HTMLButtonElement>(null);
  const restoreFocusRef = useRef<HTMLElement | null>(null);

  useLayoutEffect(() => {
    if (!detail) return;
    restoreFocusRef.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    closeRef.current?.focus();
    return () => {
      if (restoreFocusRef.current?.isConnected) restoreFocusRef.current.focus();
    };
  }, [detail]);

  useEffect(() => {
    if (!detail) return;
    // Escape closes and does not reach the shell; Tab stays on the close button
    // because the dialog holds nothing else focusable.
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        event.stopPropagation();
        onClose();
        return;
      }
      if (event.key === "Tab") {
        event.preventDefault();
        closeRef.current?.focus();
      }
    };
    document.addEventListener("keydown", onKeyDown, { capture: true });
    return () => document.removeEventListener("keydown", onKeyDown, { capture: true });
  }, [detail, onClose]);

  if (!detail) return null;

  return createPortal(
    <div
      data-app-overlay=""
      className="modal-backdrop footer-detail-backdrop"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div className="modal footer-detail" role="dialog" aria-modal="true" aria-labelledby={titleId}>
        <div className="footer-detail__head">
          <div className="modal__title footer-detail__title" id={titleId}>
            {detail.title}
          </div>
          <button
            ref={closeRef}
            type="button"
            className="modal-close-button"
            aria-label={t("common.close")}
            onClick={onClose}
          >
            <X size={14} aria-hidden="true" />
          </button>
        </div>
        {detail.meta && detail.meta.length > 0 ? (
          <div className="footer-detail__meta">
            {detail.meta.map((item) => (
              <span className="footer-detail__tag" key={item}>{item}</span>
            ))}
          </div>
        ) : null}
        {detail.body ? (
          detail.bodyStyle === "raw" ? (
            <div className="footer-detail__raw">{detail.body}</div>
          ) : (
            <div className={detail.bodyStyle === "mono" ? "modal__subject footer-detail__body" : "footer-detail__body"}>
              {detail.body}
            </div>
          )
        ) : null}
      </div>
    </div>,
    container ?? document.body,
  );
}

/** Hosts the one modal every row in the card opens. */
export function PanelDetailProvider({ children, container }: { children: ReactNode; container?: HTMLElement | null }) {
  const [detail, setDetail] = useState<PanelDetail | null>(null);
  const open = useCallback<PanelDetailSink>((next) => setDetail(next), []);
  const close = useCallback(() => setDetail(null), []);
  return (
    <PanelDetailContext.Provider value={open}>
      {children}
      <DetailModal detail={detail} onClose={close} container={container} />
    </PanelDetailContext.Provider>
  );
}

/** A module's row: the module's own row class plus the whole record on click. */
export function PanelRowButton({
  detail,
  className,
  children,
}: {
  detail: PanelDetail;
  className?: string;
  children: ReactNode;
}) {
  const open = useContext(PanelDetailContext);
  return (
    <button
      type="button"
      className={["footer-panel__row", className].filter(Boolean).join(" ")}
      onClick={() => open?.(detail)}
    >
      {children}
    </button>
  );
}
