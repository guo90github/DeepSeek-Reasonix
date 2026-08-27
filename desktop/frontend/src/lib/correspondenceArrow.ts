// CorrespondenceArrow measures where the flow-chart arrow should connect the
// focused conversation rows to the focused process turn. It is dynamically
// imported (on demand) so its code never lands in the initial bundle — the
// split desktop style is not the default.

export interface ArrowCoords {
  fx: number;
  fy: number;
  tx: number;
  ty: number;
}

export function measureArrow(container: HTMLElement): ArrowCoords | null {
  const convs = container.querySelectorAll(".conversation-pane__turn[data-focused]");
  const conv = convs[convs.length - 1] as HTMLElement | undefined;
  const proc = container.querySelector<HTMLElement>(".process-pane__turn[data-focused] .process-pane__turn-header");
  if (!conv || !proc) return null;
  const c = container.getBoundingClientRect();
  const a = conv.getBoundingClientRect();
  const b = proc.getBoundingClientRect();
  return { fx: a.right - c.left, fy: a.top + a.height / 2 - c.top, tx: b.left - c.left, ty: b.top + b.height / 2 - c.top };
}
