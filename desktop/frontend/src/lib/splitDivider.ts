// Pure split-divider geometry: magnetic snap points and keyboard stepping for
// the conversation/process pane divider. Kept out of the component so the
// behavior is unit-testable without a DOM.

export function snapSplitWidth({
  next,
  panelWidth,
  thresholdPx,
  snapPoints,
}: {
  next: number;
  panelWidth: number;
  thresholdPx: number;
  snapPoints: readonly number[];
}): number {
  if (panelWidth <= 0) return next;
  const threshold = thresholdPx / panelWidth;
  let best = next;
  let bestDistance = threshold;
  for (const point of snapPoints) {
    const distance = Math.abs(point - next);
    if (distance < bestDistance) {
      best = point;
      bestDistance = distance;
    }
  }
  return best;
}

export function stepSplitWidth({
  current,
  direction,
  step,
  min,
  max,
}: {
  current: number;
  direction: -1 | 1;
  step: number;
  min: number;
  max: number;
}): number {
  return Math.min(max, Math.max(min, current + direction * step));
}

export function resolveAutoSplitProcessWidth({
  hasReasoning,
  reasoningComplete,
  activeWidth,
  idleWidth,
}: {
  hasReasoning: boolean;
  reasoningComplete: boolean;
  activeWidth: number;
  idleWidth: number;
}): number {
  // The pane widens only while reasoning is actually streaming — a completed
  // or absent reasoning pass falls back to the idle width so the conversation
  // keeps the majority of the surface.
  return hasReasoning && !reasoningComplete ? activeWidth : idleWidth;
}
