// SessionAuditModal shows one whole-session reasoning audit: the two-pass run
// streams step by step (segment scoring, then the cross-turn review), so the
// modal shows progress plus the live model output instead of a black box. The
// deliverables lead: the session verdict, the cross-turn issues, then the
// per-turn score table; the exact requests and outputs follow as collapsed
// evidence sections. Results are one-shot and never persisted.
import { useCallback, useEffect, useMemo, useRef, useState, type CSSProperties, type KeyboardEvent as ReactKeyboardEvent, type PointerEvent as ReactPointerEvent } from "react";
import { createPortal } from "react-dom";
import { app } from "../lib/bridge";
import { onSessionAuditDone, onSessionAuditEvent, type SessionAuditEvent } from "../lib/sessionAuditStream";
import { useT } from "../lib/i18n";
import { loadLayoutSize, saveLayoutSize } from "../lib/layoutPreferences";
import { createRafResizeUpdater } from "../lib/resizeDrag";
import { AuditSection } from "./AuditModal";
import type { DictKey } from "../locales/en";
import type { SessionAuditTotals, SessionAuditTurn, SessionAuditTurnResult } from "../generated/desktopContract.generated";

type AuditStatus = "loading" | "streaming" | "done" | "error";

type StepState = {
  key: string;
  stage: "segment" | "review";
  index: number;
  total: number;
  turnFrom?: number;
  turnTo?: number;
  systemPrompt: string;
  input: string;
  thinking: string;
  text: string;
  error?: string;
  done: boolean;
};

const DIALOG_MIN_W = 520;
const DIALOG_MAX_W = 1120;
const DIALOG_MIN_H = 400;
const DIALOG_MAX_H = 920;
const DIALOG_MAX_RATIO = 0.92;

function clampDialogSize(width: number, height: number, viewportW = 1440, viewportH = 900): { width: number; height: number } {
  const maxW = Math.max(DIALOG_MIN_W, Math.min(DIALOG_MAX_W, Math.floor(viewportW * DIALOG_MAX_RATIO)));
  const maxH = Math.max(DIALOG_MIN_H, Math.min(DIALOG_MAX_H, Math.floor(viewportH * DIALOG_MAX_RATIO)));
  return {
    width: Math.min(maxW, Math.max(DIALOG_MIN_W, Math.round(width))),
    height: Math.min(maxH, Math.max(DIALOG_MIN_H, Math.round(height))),
  };
}

const FINDING_KEYS: Record<string, DictKey> = {
  contradiction: "audit.typeContradiction",
  factual_error: "audit.typeFactualError",
  invalid_inference: "audit.typeInvalidInference",
  redundancy: "audit.typeRedundancy",
  instruction_drift: "audit.typeDrift",
  omission: "audit.typeOmission",
};

const CROSS_TURN_KEYS: Record<string, DictKey> = {
  cross_turn_contradiction: "sessionAudit.issueContradiction",
  cross_turn_drift: "sessionAudit.issueDrift",
  repeated_dead_end: "sessionAudit.issueDeadEnd",
  unmet_commitment: "sessionAudit.issueUnmet",
  error_propagation: "sessionAudit.issuePropagation",
};

const TREND_KEYS: Record<string, DictKey> = {
  improving: "sessionAudit.trendImproving",
  stable: "sessionAudit.trendStable",
  degrading: "sessionAudit.trendDegrading",
};

function stepKey(stage: string, index: number): string {
  return `${stage}:${index}`;
}

function TurnRow({ row, t }: { row: SessionAuditTurnResult; t: ReturnType<typeof useT> }) {
  const findings = Array.isArray(row.findings) ? row.findings : [];
  return (
    <li className="session-audit__turn">
      <div className="session-audit__turn-head">
        <span className="session-audit__turn-no">{t("sessionAudit.turnColumn", { n: String(row.turn) })}</span>
        <span className={`session-audit__turn-score${row.score < 0.6 ? " is-low" : ""}`}>{row.score.toFixed(2)}</span>
        <span className="session-audit__turn-issues">{t("sessionAudit.issuesColumn", { n: String(row.issues ?? 0) })}</span>
        {row.truncated && <span className="audit-truncated">{t("sessionAudit.truncatedTurn")}</span>}
      </div>
      {row.conclusion && (
        <p className="session-audit__turn-conclusion">
          <span className="session-audit__turn-label">{t("sessionAudit.conclusionLabel")}</span>{row.conclusion}
        </p>
      )}
      {row.explanation && <p className="session-audit__turn-explanation">{row.explanation}</p>}
      {findings.length > 0 && (
        <ul className="audit-findings">
          {findings.map((finding, index) => (
            <li key={index} className={`audit-finding is-${finding.type}`}>
              <span className="audit-finding__tag">{t(FINDING_KEYS[finding.type] ?? "audit.typeDrift")}</span>
              <span className="audit-finding__quote">“{finding.quote}”</span>
            </li>
          ))}
        </ul>
      )}
    </li>
  );
}

function SessionVerdict({ totals, t }: { totals: SessionAuditTotals; t: ReturnType<typeof useT> }) {
  const issues = Array.isArray(totals.issues) ? totals.issues : [];
  const meta: string[] = [];
  if (totals.evalTokens > 0) meta.push(t("audit.tokens", { n: String(totals.evalTokens) }));
  if (totals.evalCost > 0) meta.push(t("audit.cost", { c: totals.evalCost.toFixed(4) }));
  if (totals.elapsedMs > 0) meta.push(`${(totals.elapsedMs / 1000).toFixed(1)}s`);
  return (
    <div className="audit-result">
      <div className="audit-result__row">
        <span className="audit-result__score" title={t("sessionAudit.scoreHint")}>
          {totals.score.toFixed(2)}
        </span>
        <span className="audit-badge">{t(TREND_KEYS[totals.trend] ?? "sessionAudit.trendStable")}</span>
        <span className="audit__meta">
          {t("sessionAudit.scope", { turns: String(totals.turnCount), segments: String(totals.segmentCount) })}
          {meta.length > 0 ? ` · ${meta.join(" · ")}` : ""}
        </span>
      </div>
      <div
        className="audit-result__bar"
        style={{ background: `linear-gradient(to right, var(--accent) ${totals.score * 100}%, var(--border) ${totals.score * 100}%)` }}
        aria-hidden="true"
      />
      {totals.explanation && <p className="audit-result__evidence">{totals.explanation}</p>}
      <h4 className="session-audit__subtitle">{t("sessionAudit.crossTurnTitle")}</h4>
      {issues.length === 0 ? (
        <p className="audit-findings__empty">{t("sessionAudit.crossTurnNone")}</p>
      ) : (
        <ul className="session-audit__issues">
          {issues.map((issue, index) => (
            <li key={index} className="session-audit__issue">
              <span className="audit-finding__tag">{t(CROSS_TURN_KEYS[issue.type] ?? "sessionAudit.issueContradiction")}</span>
              <span className="session-audit__issue-turns">
                {t("sessionAudit.crossTurnTurns", { turns: (issue.turns ?? []).map((turn) => `#${turn}`).join(" → ") })}
              </span>
              <span className="session-audit__issue-note">{issue.note}</span>
              {issue.quote && <span className="audit-finding__quote">“{issue.quote}”</span>}
            </li>
          ))}
        </ul>
      )}
      <h4 className="session-audit__subtitle">{t("sessionAudit.turnsTitle")}</h4>
      <ul className="session-audit__turns">
        {(totals.turns ?? []).map((row) => (
          <TurnRow key={row.turn} row={row} t={t} />
        ))}
      </ul>
    </div>
  );
}

export function SessionAuditModal({ turns, tabId, onClose }: { turns: SessionAuditTurn[]; tabId?: string; onClose: () => void }) {
  const t = useT();
  const [status, setStatus] = useState<AuditStatus>("loading");
  const [steps, setSteps] = useState<StepState[]>([]);
  const [activeKey, setActiveKey] = useState("");
  const [totals, setTotals] = useState<SessionAuditTotals | null>(null);
  const [error, setError] = useState("");
  const [showInput, setShowInput] = useState(false);
  const [showPrompt, setShowPrompt] = useState(false);
  const [showOutput, setShowOutput] = useState(true);
  const [dialogSize, setDialogSize] = useState(() => {
    const width = loadLayoutSize("sessionAuditDialogWidth", 760, (v) => clampDialogSize(v, 0).width);
    const height = loadLayoutSize("sessionAuditDialogHeight", 620, (v) => clampDialogSize(0, v).height);
    return { width, height };
  });
  const [resizing, setResizing] = useState(false);
  const dialogRef = useRef<HTMLDivElement>(null);
  const resizeRef = useRef<HTMLButtonElement>(null);
  const closeRef = useRef<HTMLButtonElement>(null);

  const dialogStyle = useMemo(
    () => ({ "--audit-dialog-w": `${dialogSize.width}px`, "--audit-dialog-h": `${dialogSize.height}px` }) as CSSProperties,
    [dialogSize],
  );
  const streaming = status === "streaming";
  const busy = status === "loading" || streaming;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        onClose();
      }
    };
    document.addEventListener("keydown", onKey, { capture: true });
    return () => document.removeEventListener("keydown", onKey, { capture: true });
  }, [onClose]);

  useEffect(() => {
    closeRef.current?.focus();
  }, []);

  // Run the audit: subscribe to the step stream and call the binding. The
  // resolved promise only means "no error"; sessionaudit:done is the
  // completion signal.
  const run = useCallback(() => {
    setStatus("loading");
    setSteps([]);
    setActiveKey("");
    setTotals(null);
    setError("");
    app.AuditSession(turns).then(
      () => setStatus((current) => (current === "error" ? current : "done")),
      (err: unknown) => {
        setError(err instanceof Error ? err.message : String(err));
        setStatus("error");
      },
    );
  }, [turns]);

  useEffect(() => {
    let cancelled = false;
    const offEvent = onSessionAuditEvent((eventTabId, ev: SessionAuditEvent) => {
      if (cancelled) return;
      // A run belongs to the tab that started it: another tab's step stream must
      // not paint into this modal.
      if (tabId && eventTabId && eventTabId !== tabId) return;
      const key = stepKey(ev.stage, ev.index);
      if (ev.kind === "request") {
        setSteps((prev) => prev.some((step) => step.key === key)
          ? prev
          : [...prev, {
              key, stage: ev.stage, index: ev.index, total: ev.total,
              turnFrom: ev.turnFrom, turnTo: ev.turnTo,
              systemPrompt: ev.systemPrompt ?? "", input: ev.input ?? "",
              thinking: "", text: "", done: false,
            }]);
        setActiveKey(key);
        setStatus("streaming");
        return;
      }
      if (ev.kind === "reasoning" || ev.kind === "text") {
        setSteps((prev) => prev.map((step) => step.key === key
          ? (ev.kind === "reasoning" ? { ...step, thinking: step.thinking + (ev.text ?? "") } : { ...step, text: step.text + (ev.text ?? "") })
          : step));
        return;
      }
      if (ev.kind === "step_done") {
        setSteps((prev) => prev.map((step) => (step.key === key ? { ...step, done: true } : step)));
        return;
      }
      if (ev.kind === "step_failed") {
        const message = ev.error ?? "";
        setSteps((prev) => prev.map((step) => (step.key === key ? { ...step, done: true, error: message } : step)));
        setError(message);
        setStatus("error");
      }
    });
    const offDone = onSessionAuditDone((eventTabId, ev) => {
      if (cancelled) return;
      if (tabId && eventTabId && eventTabId !== tabId) return;
      setTotals(ev);
      setStatus("done");
    });
    run();
    return () => {
      cancelled = true;
      offEvent();
      offDone();
    };
    // A new turn list (another session) restarts the run; same-session reruns
    // go through the rerun button, which calls run() directly.
  }, [run, tabId]);

  const activeStep = steps.find((step) => step.key === activeKey);
  const prompts = useMemo(() => {
    const seen = new Set<string>();
    const out: Array<{ key: string; label: string; prompt: string }> = [];
    for (const step of steps) {
      if (!step.systemPrompt || seen.has(step.systemPrompt)) continue;
      seen.add(step.systemPrompt);
      out.push({ key: step.key, label: stepLabel(step, t), prompt: step.systemPrompt });
    }
    return out;
  }, [steps, t]);

  const startResize = (event: ReactPointerEvent<HTMLButtonElement>) => {
    if (event.button !== 0) return;
    const dialog = dialogRef.current;
    if (!dialog) return;
    event.preventDefault();
    setResizing(true);
    let next = { ...dialogSize };
    const liveW = createRafResizeUpdater({ target: dialog, separator: resizeRef.current, cssVar: "--audit-dialog-w" });
    const liveH = createRafResizeUpdater({ target: dialog, separator: resizeRef.current, cssVar: "--audit-dialog-h" });
    const startX = event.clientX;
    const startY = event.clientY;
    const onMove = (moveEvent: PointerEvent) => {
      next = clampDialogSize(
        dialogSize.width + (moveEvent.clientX - startX),
        dialogSize.height + (moveEvent.clientY - startY),
        window.innerWidth,
        window.innerHeight,
      );
      liveW.schedule(next.width);
      liveH.schedule(next.height);
    };
    const onDone = () => {
      liveW.flush();
      liveH.flush();
      setDialogSize(next);
      saveLayoutSize("sessionAuditDialogWidth", next.width);
      saveLayoutSize("sessionAuditDialogHeight", next.height);
      setResizing(false);
      window.removeEventListener("pointermove", onMove);
      window.removeEventListener("pointerup", onDone);
      window.removeEventListener("pointercancel", onDone);
      document.body.style.cursor = "";
      document.body.style.userSelect = "";
    };
    document.body.style.cursor = "nwse-resize";
    document.body.style.userSelect = "none";
    window.addEventListener("pointermove", onMove);
    window.addEventListener("pointerup", onDone);
    window.addEventListener("pointercancel", onDone);
  };

  const onResizeKey = (e: ReactKeyboardEvent<HTMLButtonElement>) => {
    const step = e.shiftKey ? 40 : 12;
    const grow = e.key === "ArrowRight" || e.key === "ArrowDown";
    if (e.key === "ArrowRight" || e.key === "ArrowDown" || e.key === "ArrowLeft" || e.key === "ArrowUp") {
      e.preventDefault();
      const delta = grow ? step : -step;
      const next = clampDialogSize(dialogSize.width + delta, dialogSize.height + delta, window.innerWidth, window.innerHeight);
      setDialogSize(next);
      saveLayoutSize("sessionAuditDialogWidth", next.width);
      saveLayoutSize("sessionAuditDialogHeight", next.height);
    }
  };

  return createPortal(
    <div
      className="modal-backdrop reasonix-audit-backdrop"
      role="presentation"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        ref={dialogRef}
        className={`modal reasonix-audit-dialog${resizing ? " is-resizing" : ""}`}
        role="dialog"
        aria-modal="true"
        aria-label={t("sessionAudit.modalTitle")}
        style={dialogStyle}
      >
        <header className="reasonix-audit-dialog__header">
          <span className="modal__title">{t("sessionAudit.modalTitle")}</span>
          <button ref={closeRef} type="button" className="reasonix-audit-dialog__close" onClick={onClose} aria-label={t("common.close")}>
            ✕
          </button>
        </header>

        <div className="reasonix-audit-dialog__body">
          {busy && (
            <div className="audit-card audit-card--loading">
              <span className="audit-card__spinner" aria-hidden="true" />
              <span>{activeStep ? stepLabel(activeStep, t) : t("sessionAudit.running")}</span>
            </div>
          )}

          {status === "error" && (
            <div className="audit-card audit-card--error" role="alert">
              {t("sessionAudit.failed")}: {error}
              <button type="button" className="audit-prompt-editor__rerun" onClick={run} disabled={busy || turns.length === 0}>
                {t("sessionAudit.rerun")}
              </button>
            </div>
          )}

          {(streaming || status === "done") && (
            <>
              {totals && <SessionVerdict totals={totals} t={t} />}

              <AuditSection title={t("sessionAudit.inputSection")} open={showInput} onToggle={() => setShowInput((v) => !v)}>
                {steps.map((step) => (
                  <div key={step.key} className="session-audit__stage">
                    <div className="audit-stage__label">{stepLabel(step, t)}</div>
                    <pre className="audit-stage__pre">{step.input}</pre>
                  </div>
                ))}
              </AuditSection>

              <AuditSection title={t("audit.prompt")} open={showPrompt} onToggle={() => setShowPrompt((v) => !v)}>
                {prompts.map((entry) => (
                  <div key={entry.key} className="session-audit__stage">
                    <div className="audit-stage__label">{entry.label}</div>
                    <pre className="audit-stage__pre">{entry.prompt}</pre>
                  </div>
                ))}
              </AuditSection>

              <AuditSection title={t("sessionAudit.outputSection")} open={showOutput} onToggle={() => setShowOutput((v) => !v)}>
                {steps.map((step) => (
                  <div key={step.key} className="session-audit__stage">
                    <div className="audit-stage__label">{stepLabel(step, t)}</div>
                    {step.thinking.trim() !== "" && <pre className="audit-stage__pre">{step.thinking}</pre>}
                    <pre className="audit-stage__pre audit-stage__pre--output">
                      {step.text}
                      {!step.done && step.key === activeKey && <span className="audit-card__cursor" aria-hidden="true" />}
                    </pre>
                    {step.error && <div className="audit-card audit-card--error">{step.error}</div>}
                  </div>
                ))}
              </AuditSection>
            </>
          )}
        </div>

        <button
          ref={resizeRef}
          type="button"
          className="reasonix-audit-dialog__resize"
          role="separator"
          aria-orientation="horizontal"
          aria-label={t("audit.resize")}
          aria-valuemin={DIALOG_MIN_W}
          aria-valuemax={DIALOG_MAX_W}
          aria-valuenow={dialogSize.width}
          onPointerDown={startResize}
          onKeyDown={onResizeKey}
          onDoubleClick={() => {
            const next = { width: 760, height: 620 };
            setDialogSize(next);
            saveLayoutSize("sessionAuditDialogWidth", next.width);
            saveLayoutSize("sessionAuditDialogHeight", next.height);
          }}
        >
          <span aria-hidden="true" />
        </button>
      </div>
    </div>,
    document.body,
  );
}

function stepLabel(step: StepState, t: ReturnType<typeof useT>): string {
  const label = step.stage === "review"
    ? t("sessionAudit.stepReview", { i: String(step.index), total: String(step.total) })
    : t("sessionAudit.stepSegment", {
        i: String(step.index),
        total: String(step.total),
        from: String(step.turnFrom ?? 0),
        to: String(step.turnTo ?? 0),
      });
  return step.error ? `${label} · ${t("sessionAudit.stepFailed")}` : label;
}
