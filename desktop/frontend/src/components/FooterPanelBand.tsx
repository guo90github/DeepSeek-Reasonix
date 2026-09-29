// The split layout's bottom band: the composer and the footer panel side by
// side, separated by a draggable rail. The band wrapper always mounts (its
// display changes, never its presence) because a conditional wrapper would
// remount the composer and drop the draft the user is typing.
//
// Without a panel both wrappers are display:contents, so the single-column
// footer keeps exactly the box it had before this band existed.

import {
  useCallback,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type PointerEvent as ReactPointerEvent,
  type ReactNode,
} from "react";
import { useT } from "../lib/i18n";
import { loadLayoutSize, saveLayoutSize } from "../lib/layoutPreferences";
import {
  FOOTER_PANEL_KEYBOARD_STEP,
  FOOTER_PANEL_SHARE_DEFAULT,
  FOOTER_PANEL_SHARE_MAX,
  FOOTER_PANEL_SHARE_MIN,
  clampFooterPanelShare,
  footerPanelShareFromPointer,
  stepFooterPanelShare,
} from "../lib/footerPanel";
import { FooterPanel, type FooterPanelModule, type FooterPanelModuleProps } from "./FooterPanel";

const FOOTER_PANEL_SHARE_STORAGE_KEY = "footerPanelShare";

export type FooterPanelBandProps = {
  children: ReactNode;
  panel: (FooterPanelModuleProps & { modules: readonly FooterPanelModule[] }) | null;
};

export function FooterPanelBand({ children, panel }: FooterPanelBandProps) {
  const t = useT();
  const [share, setShare] = useState(() =>
    clampFooterPanelShare(loadLayoutSize(FOOTER_PANEL_SHARE_STORAGE_KEY, FOOTER_PANEL_SHARE_DEFAULT, clampFooterPanelShare)),
  );
  const [resizing, setResizing] = useState(false);
  const bandRef = useRef<HTMLDivElement>(null);
  const shareRef = useRef(share);

  // The panel only exists in split mode, so its styles ride a lazy chunk and
  // leave the app-shell budget flat — same reason splitWorkspace.css does.
  useEffect(() => {
    if (!panel) return;
    void import("./footerPanel.css");
  }, [panel]);

  useEffect(() => {
    bandRef.current?.style.setProperty("--footer-panel-share", `${share}%`);
  }, [share]);

  // The panel shares the composer CARD's row, not the whole composer box: a row
  // the card does not own (the @-reference row stacked above it) would otherwise
  // push the card down alone and break the pair. The card is the reference.
  useLayoutEffect(() => {
    if (!panel) return;
    const band = bandRef.current;
    const card = band?.querySelector(".composer-card");
    if (!band || !card) return;
    const measure = () => {
      const height = Math.round(card.getBoundingClientRect().height);
      band.style.setProperty("--footer-composer-card-height", `${height}px`);
    };
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(measure);
    observer.observe(card);
    return () => observer.disconnect();
  }, [panel]);

  const applyShare = useCallback((next: number) => {
    const clamped = clampFooterPanelShare(next);
    shareRef.current = clamped;
    setShare(clamped);
  }, []);

  const beginResize = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    setResizing(true);
    event.currentTarget.setPointerCapture(event.pointerId);
  }, []);

  const onResizeMove = useCallback(
    (event: ReactPointerEvent<HTMLDivElement>) => {
      if (!resizing) return;
      const rect = bandRef.current?.getBoundingClientRect();
      if (!rect) return;
      applyShare(footerPanelShareFromPointer({ clientX: event.clientX, bandLeft: rect.left, bandWidth: rect.width }));
    },
    [applyShare, resizing],
  );

  const persistShare = useCallback(() => {
    saveLayoutSize(FOOTER_PANEL_SHARE_STORAGE_KEY, shareRef.current, clampFooterPanelShare);
  }, []);

  const endResize = useCallback(() => {
    setResizing(false);
    persistShare();
  }, [persistShare]);

  const resetShare = useCallback(() => {
    applyShare(FOOTER_PANEL_SHARE_DEFAULT);
    saveLayoutSize(FOOTER_PANEL_SHARE_STORAGE_KEY, FOOTER_PANEL_SHARE_DEFAULT, clampFooterPanelShare);
  }, [applyShare]);

  const onDividerKeyDown = useCallback(
    (event: ReactKeyboardEvent<HTMLDivElement>) => {
      if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
      event.preventDefault();
      // The panel is the band's right-hand track: ArrowLeft widens it.
      applyShare(stepFooterPanelShare({
        current: shareRef.current,
        delta: event.key === "ArrowLeft" ? FOOTER_PANEL_KEYBOARD_STEP : -FOOTER_PANEL_KEYBOARD_STEP,
      }));
      persistShare();
    },
    [applyShare, persistShare],
  );

  return (
    <div
      ref={bandRef}
      className={[
        "footer-band",
        panel ? "footer-band--panel" : "",
        resizing ? "footer-band--resizing" : "",
      ].filter(Boolean).join(" ")}
    >
      <div className="footer-band__composer">{children}</div>
      {panel ? (
        <>
          <div
            className={["footer-band__divider", resizing ? "footer-band__divider--active" : ""].filter(Boolean).join(" ")}
            role="separator"
            aria-orientation="vertical"
            aria-label={t("footerPanel.resize")}
            aria-valuemin={FOOTER_PANEL_SHARE_MIN}
            aria-valuemax={FOOTER_PANEL_SHARE_MAX}
            aria-valuenow={Math.round(share)}
            tabIndex={0}
            onPointerDown={beginResize}
            onPointerMove={onResizeMove}
            onPointerUp={endResize}
            onPointerCancel={endResize}
            onKeyDown={onDividerKeyDown}
            onDoubleClick={resetShare}
          />
          <div className="footer-band__panel">
            <FooterPanel modules={panel.modules} context={panel} />
          </div>
        </>
      ) : null}
    </div>
  );
}
