import { useState } from "react";
import { Check, ChevronRight } from "lucide-react";
import { useT } from "../lib/i18n";
import type { ReasoningStepsResult, ReasoningStepView } from "../lib/useReasoningSteps";
import { Markdown } from "./Markdown";
import { StreamingReasoningText } from "./StreamingReasoningText";

function durationLabel(durationMs: number | undefined): string {
  if (typeof durationMs !== "number" || !Number.isFinite(durationMs) || durationMs <= 0) return "";
  return `[${Math.max(1, Math.round(durationMs / 1000))}s]`;
}

function StepRow({
  step,
  open,
  onToggle,
}: {
  step: ReasoningStepView;
  open: boolean;
  onToggle: () => void;
}) {
  const running = step.status === "streaming";
  const meta = running ? "…" : durationLabel(step.durationMs);
  return (
    <div className={`reasoning-step reasoning-step--${step.status}`} data-step-index={step.index}>
      <button
        type="button"
        className="reasoning-step__head"
        data-running={running ? "" : undefined}
        aria-expanded={open}
        onClick={onToggle}
      >
        <span className="reasoning-step__icon">
          {running ? <span className="reasoning-step__dot" /> : <Check size={12} />}
        </span>
        <span className="reasoning-step__num">Step {step.index}</span>
        <span className="reasoning-step__title">{step.title}</span>
        {meta && <span className="reasoning-step__meta">{meta}</span>}
        <ChevronRight size={12} className={`reasoning-step__chevron${open ? " reasoning-step__chevron--open" : ""}`} />
      </button>
      {open && step.content.trim() !== "" && (
        <div className="reasoning-step__body">
          {running ? <StreamingReasoningText text={step.content} /> : <Markdown text={step.content} streaming={false} />}
        </div>
      )}
    </div>
  );
}

/**
 * Renders the segmented reasoning stream as expandable step cards. Default
 * openness follows the display mode: the active step tracks the stream, and
 * completed steps stay folded unless the user (or expanded mode) opens them.
 * User toggles always win; "collapse completed" folds every opened step.
 */
export function StructuredReasoningSteps({
  result,
  followStreaming,
  defaultAllOpen,
}: {
  result: ReasoningStepsResult;
  /** Auto-open the active step while streaming (auto/expanded display modes). */
  followStreaming: boolean;
  /** Keep every completed step open (expanded display mode). */
  defaultAllOpen: boolean;
}) {
  const t = useT();
  const { steps } = result;
  const complete = steps.every((step) => step.status === "complete");
  const [userOpen, setUserOpen] = useState<ReadonlySet<number>>(new Set());
  const [userClosed, setUserClosed] = useState<ReadonlySet<number>>(new Set());

  const toggle = (index: number) => {
    setUserOpen((prev) => {
      const next = new Set(prev);
      if (next.has(index)) next.delete(index);
      else next.add(index);
      return next;
    });
    setUserClosed((prev) => {
      const next = new Set(prev);
      next.delete(index);
      return next;
    });
  };

  const collapseCompleted = () => {
    const completed = steps.filter((step) => step.status === "complete").map((step) => step.index);
    setUserOpen(new Set());
    setUserClosed(new Set(completed));
  };

  const isOpen = (step: ReasoningStepView): boolean => {
    if (userOpen.has(step.index)) return true;
    if (userClosed.has(step.index)) return false;
    if (step.status === "streaming") return followStreaming;
    return defaultAllOpen;
  };

  const hasOpenCompleted = steps.some((step) => step.status === "complete" && userOpen.has(step.index));

  return (
    <div className="reasoning-steps">
      <div className="reasoning-steps__summary">
        <span>
          {complete
            ? t("reasoning.stepsComplete", { n: steps.length, total: steps.length })
            : t("reasoning.stepsDetected", { n: steps.length })}
        </span>
        {hasOpenCompleted && (
          <button type="button" className="reasoning-steps__fold" onClick={collapseCompleted}>
            {t("reasoning.collapseCompleted")}
          </button>
        )}
      </div>
      {steps.map((step) => (
        <StepRow key={step.index} step={step} open={isOpen(step)} onToggle={() => toggle(step.index)} />
      ))}
    </div>
  );
}
