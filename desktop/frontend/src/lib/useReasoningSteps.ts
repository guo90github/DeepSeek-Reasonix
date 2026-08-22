import { useMemo, useRef } from "react";
import { reasoningStepMarkers, segmentReasoningSteps, type ReasoningStep } from "./reasoningSteps";

export interface ReasoningStepView extends ReasoningStep {
  status: "complete" | "streaming";
  /** Wall-clock ms the step took; undefined when unknowable (history restore). */
  durationMs?: number;
}

export interface ReasoningStepsResult {
  steps: ReasoningStepView[];
  /** Completed marker lines observed so far — the live step-count label. */
  detected: number;
}

/**
 * Derives step statuses and per-step durations from the append-only reasoning
 * stream without touching the reducer: the hook diffs marker lines across
 * renders and stamps each new marker with the frame time. Durations are
 * contiguous, so step i lasts until step i+1 starts; the final step's duration
 * is the total turn duration minus the elapsed head, exact up to frame
 * granularity. A shrunk marker list (stream retry/discard) restarts the clock.
 * Pass `enabled=false` when the reasoning body is not rendered so the stream
 * is never touched (hidden mode, closed panel).
 */
export function useReasoningSteps(
  text: string,
  opts: { reasoningComplete: boolean; totalDurationMs?: number },
  enabled = true,
): ReasoningStepsResult {
  const markers = useMemo(() => (enabled ? reasoningStepMarkers(text) : []), [text, enabled]);
  const segments = useMemo(() => (enabled ? segmentReasoningSteps(text) : []), [text, enabled]);
  const startedAtRef = useRef<number[]>([]);
  const observedRef = useRef<string[] | null>(null);
  const streaming = enabled && !opts.reasoningComplete;

  if (streaming) {
    const prev = observedRef.current;
    if (prev === null || markers.length < prev.length) {
      const stamp = Date.now();
      observedRef.current = markers;
      startedAtRef.current = markers.map(() => stamp);
    } else if (markers.length > prev.length) {
      const stamp = Date.now();
      const next = startedAtRef.current.slice();
      for (let i = prev.length; i < markers.length; i += 1) next[i] = stamp;
      startedAtRef.current = next;
      observedRef.current = markers;
    }
  } else {
    observedRef.current = markers;
  }

  const started = startedAtRef.current;
  const steps: ReasoningStepView[] = segments.map((seg, i) => {
    const isLast = i === segments.length - 1;
    const startedAt = started[i];
    let durationMs: number | undefined;
    if (isLast) {
      if (
        opts.reasoningComplete &&
        typeof startedAt === "number" &&
        typeof started[0] === "number" &&
        typeof opts.totalDurationMs === "number"
      ) {
        const d = opts.totalDurationMs - (startedAt - started[0]);
        durationMs = Number.isFinite(d) && d >= 0 ? d : undefined;
      }
    } else if (typeof startedAt === "number" && typeof started[i + 1] === "number") {
      durationMs = started[i + 1] - startedAt;
    }
    return { ...seg, status: streaming && isLast ? "streaming" : "complete", durationMs };
  });

  return { steps, detected: markers.length };
}
