// Pure geometry for the split layout's footer band: how much of the band the
// panel owns, and how a pointer or a key step moves that share. Kept out of the
// component so the clamp and drag math are unit-testable without a DOM.

export const FOOTER_PANEL_SHARE_DEFAULT = 38;
export const FOOTER_PANEL_SHARE_MIN = 22;
export const FOOTER_PANEL_SHARE_MAX = 62;
export const FOOTER_PANEL_KEYBOARD_STEP = 4;

export function clampFooterPanelShare(share: number): number {
  if (!Number.isFinite(share)) return FOOTER_PANEL_SHARE_DEFAULT;
  return Math.min(FOOTER_PANEL_SHARE_MAX, Math.max(FOOTER_PANEL_SHARE_MIN, Math.round(share)));
}

/** The panel is the band's right-hand track, so its share is the distance from
 *  the pointer to the band's right edge. */
export function footerPanelShareFromPointer({
  clientX,
  bandLeft,
  bandWidth,
}: {
  clientX: number;
  bandLeft: number;
  bandWidth: number;
}): number {
  if (!Number.isFinite(bandWidth) || bandWidth <= 0) return FOOTER_PANEL_SHARE_DEFAULT;
  return clampFooterPanelShare(((bandLeft + bandWidth - clientX) / bandWidth) * 100);
}

/** Positive delta widens the panel (ArrowLeft on a right-hand panel). */
export function stepFooterPanelShare({ current, delta }: { current: number; delta: number }): number {
  return clampFooterPanelShare(current + delta);
}
