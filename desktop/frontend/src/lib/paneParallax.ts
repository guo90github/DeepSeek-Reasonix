// Pane parallax: a decorative ::before layer on the pane root drifts at a
// fraction of the scroll speed (CSS transform driven by --pane-scroll-y).
// Attached to the Virtuoso scroller so the offset tracks the real scroller;
// returns a detach that removes the listener and the CSS variable.

const PARALLAX_FACTOR = 0.3;

export function attachPaneParallax(scrollerEl: HTMLElement | null): (() => void) | undefined {
  if (!scrollerEl) return undefined;
  const root = scrollerEl.closest<HTMLElement>(".conversation-pane, .process-pane");
  if (!root) return undefined;

  let frame: number | null = null;
  const update = () => {
    frame = null;
    root.style.setProperty("--pane-scroll-y", String(scrollerEl.scrollTop * PARALLAX_FACTOR));
  };
  const onScroll = () => {
    if (frame !== null) return;
    // Guard for environments without rAF (jsdom tests): update inline.
    if (typeof requestAnimationFrame !== "undefined") {
      frame = requestAnimationFrame(update);
    } else {
      update();
    }
  };
  scrollerEl.addEventListener("scroll", onScroll, { passive: true });
  update();

  return () => {
    if (frame !== null && typeof cancelAnimationFrame !== "undefined") cancelAnimationFrame(frame);
    scrollerEl.removeEventListener("scroll", onScroll);
    root.style.removeProperty("--pane-scroll-y");
  };
}
