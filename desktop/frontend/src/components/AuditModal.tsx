import { useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import type { CSSProperties, KeyboardEvent as ReactKeyboardEvent, PointerEvent as ReactPointerEvent } from "react";
import { createPortal } from "react-dom";
import { app } from "../lib/bridge";
import { onAuditChunk, onAuditDone, onAuditRequest, type AuditChunkEvent, type AuditRequestPayload } from "../lib/auditStream";
import { useT } from "../lib/i18n";
import { loadLayoutSize, saveLayoutSize } from "../lib/layoutPreferences";
import { createRafResizeUpdater } from "../lib/resizeDrag";
import type { event } from "../../wailsjs/go/models";

type AuditStatus = "loading" | "streaming" | "done" | "error";

const AUDIT_DIALOG_MIN_W = 440;
const AUDIT_DIALOG_MAX_W = 1080;
const AUDIT_DIALOG_MIN_H = 360;
const AUDIT_DIALOG_MAX_H = 900;
const AUDIT_DIALOG_MAX_RATIO = 0.9;

function clampAuditSize(width: number, height: number, viewportW = 1440, viewportH = 900): { width: number; height: number } {
  const maxW = Math.max(AUDIT_DIALOG_MIN_W, Math.min(AUDIT_DIALOG_MAX_W, Math.floor(viewportW * AUDIT_DIALOG_MAX_RATIO)));
  const maxH = Math.max(AUDIT_DIALOG_MIN_H, Math.min(AUDIT_DIALOG_MAX_H, Math.floor(viewportH * AUDIT_DIALOG_MAX_RATIO)));
  return {
    width: Math.min(maxW, Math.max(AUDIT_DIALOG_MIN_W, Math.round(width))),
    height: Math.min(maxH, Math.max(AUDIT_DIALOG_MIN_H, Math.round(height))),
  };
}

const FINDING_KEYS = {
  contradiction: "audit.typeContradiction",
  factual_error: "audit.typeFactualError",
  invalid_inference: "audit.typeInvalidInference",
  redundancy: "audit.typeRedundancy",
  instruction_drift: "audit.typeDrift",
  omission: "audit.typeOmission",
} as const;

// AuditSection is a reusable collapsed block (header + chevron + optional body).
function AuditSection({
  title,
  open,
  onToggle,
  extra,
  children,
}: {
  title: string;
  open: boolean;
  onToggle: () => void;
  extra?: ReactNode;
  children: ReactNode;
}) {
  return (
    <section className="audit-section">
      <button type="button" className="audit-section__head" onClick={onToggle} aria-expanded={open}>
        <span className={`audit-section__chevron${open ? " is-open" : ""}`} aria-hidden="true" />
        <span className="audit-section__title">{title}</span>
        {extra}
      </button>
      {open && <div className="audit-section__body">{children}</div>}
    </section>
  );
}

// AuditMeta renders the evaluator cost line (tokens / cost / elapsed).
function AuditMeta({ totals, t }: { totals: event.ReasoningAuditTotals; t: ReturnType<typeof useT> }) {
  const parts: string[] = [];
  if (typeof totals.evalTokens === "number" && totals.evalTokens > 0) {
    parts.push(t("audit.tokens", { n: String(totals.evalTokens) }));
  }
  if (typeof totals.evalCost === "number" && totals.evalCost > 0) {
    parts.push(t("audit.cost", { c: totals.evalCost.toFixed(4) }));
  }
  if (totals.elapsedMs > 0) parts.push(`${(totals.elapsedMs / 1000).toFixed(1)}s`);
  if (parts.length === 0) return null;
  return <span className="audit__meta">{parts.join(" · ")}</span>;
}

// AuditVerdict is the audit deliverable: score + bar + pass/attention badge +
// rationale (explanation) + per-issue findings.
function AuditVerdict({ totals, threshold, t }: { totals: event.ReasoningAuditTotals; threshold: number; t: ReturnType<typeof useT> }) {
  const low = totals.score < threshold;
  return (
    <div className="audit-result">
      <div className="audit-result__row">
        <span className={`audit-result__score${low ? " is-low" : ""}`} title={t("audit.scoreHint")}>
          {totals.score.toFixed(2)}
        </span>
        <span className={`audit-badge${low ? " audit-badge--warn" : ""}`}>{low ? t("audit.attention") : t("audit.pass")}</span>
        <span className="audit__issues">
          {t("audit.contradiction", { n: String(totals.contradiction ?? 0) })} ·{" "}
          {t("audit.factualError", { n: String(totals.factualError ?? 0) })} ·{" "}
          {t("audit.invalidInference", { n: String(totals.invalidInference ?? 0) })} ·{" "}
          {t("audit.redundancy", { n: String(totals.redundancy ?? 0) })} ·{" "}
          {t("audit.drift", { n: String(totals.instructionDrift ?? 0) })} ·{" "}
          {t("audit.omission", { n: String(totals.omission ?? 0) })}
        </span>
        <AuditMeta totals={totals} t={t} />
      </div>
      <div
        className="audit-result__bar"
        style={{ background: `linear-gradient(to right, var(--accent) ${totals.score * 100}%, var(--border) ${totals.score * 100}%)` }}
        aria-hidden="true"
      />
      {totals.explanation && <p className="audit-result__evidence">{totals.explanation}</p>}
      {Array.isArray(totals.findings) && totals.findings.length > 0 && (
        <ul className="audit-findings">
          {totals.findings.map((f, i) => (
            <li key={i} className={`audit-finding is-${f.type}`}>
              <span className="audit-finding__tag">{t(FINDING_KEYS[f.type as keyof typeof FINDING_KEYS] ?? "audit.typeDrift")}</span>
              <span className="audit-finding__quote">“{f.quote}”</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

// AuditModal shows one audit run in a centered modal. It owns the stream
// lifecycle: subscribes on mount, cleans up on unmount (Escape / backdrop /
// close button). The deliverable (verdict + evidence) leads, then the audited
// input, then the technical trace (collapsed, live output) — so the run is
// transparent without raw mechanics dominating.
export function AuditModal({ reasoning, onClose }: { reasoning: string; onClose: () => void }) {
  const t = useT();
  const [status, setStatus] = useState<AuditStatus>("loading");
  const [request, setRequest] = useState<AuditRequestPayload | null>(null);
  const [think, setThink] = useState("");
  const [text, setText] = useState("");
  const [totals, setTotals] = useState<event.ReasoningAuditTotals | null>(null);
  const [error, setError] = useState("");
  const [showInput, setShowInput] = useState(false);
  const [showTech, setShowTech] = useState(false);
  const [threshold, setThreshold] = useState(0.6);
  const [dialogSize, setDialogSize] = useState(() => {
    const width = loadLayoutSize("auditDialogWidth", 680, (v) => clampAuditSize(v, 0).width);
    const height = loadLayoutSize("auditDialogHeight", 560, (v) => clampAuditSize(0, v).height);
    return { width, height };
  });
  const [resizing, setResizing] = useState(false);
  const dialogRef = useRef<HTMLDivElement>(null);
  const resizeRef = useRef<HTMLButtonElement>(null);

  const dialogStyle = useMemo(
    () => ({ "--audit-dialog-w": `${dialogSize.width}px`, "--audit-dialog-h": `${dialogSize.height}px` }) as CSSProperties,
    [dialogSize],
  );

  const closeRef = useRef<HTMLButtonElement>(null);
  const streaming = status === "streaming";

  // Load the configured audit threshold for the pass / needs-attention badge.
  useEffect(() => {
    let cancelled = false;
    app
      .GetAuditThreshold()
      .then((v) => {
        if (!cancelled && Number.isFinite(v)) setThreshold(v);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, []);

  // Escape closes the modal.
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

  // Focus the close button so keyboard users land somewhere sensible.
  useEffect(() => {
    closeRef.current?.focus();
  }, []);

  // Keep the technical trace open while the model output streams, so progress
  // is visible; it stays open afterwards but the user can collapse it.
  useEffect(() => {
    if (streaming) setShowTech(true);
  }, [streaming]);

  // Run the audit: subscribe to the stream and call the binding. The stream
  // events carry request/chunk/done; the resolved promise only means no error.
  // On unmount we cancel in-flight handlers and unsubscribe.
  useEffect(() => {
    let cancelled = false;
    const offRequest = onAuditRequest((_tabId, ev) => {
      if (cancelled) return;
      setRequest(ev);
      setStatus("streaming");
    });
    const offChunk = onAuditChunk((_tabId, ev: AuditChunkEvent) => {
      if (cancelled) return;
      if (ev.kind === "reasoning") setThink((p) => p + ev.chunk);
      else setText((p) => p + ev.chunk);
    });
    const offDone = onAuditDone((_tabId, ev) => {
      if (cancelled) return;
      setTotals(ev);
      setStatus("done");
    });
    app.AuditTurn(reasoning).catch((err) => {
      if (cancelled) return;
      setError(err instanceof Error ? err.message : String(err));
      setStatus("error");
    });
    return () => {
      cancelled = true;
      offRequest();
      offChunk();
      offDone();
    };
  }, [reasoning]);

  const hasThink = think.trim().length > 0;

  // startResize drags the dialog's bottom-right corner to resize width+height.
  // Two RAF updaters (one per dimension) keep the live resize cheap; the final
  // size is clamped and persisted on release.
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
      next = clampAuditSize(
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
      saveLayoutSize("auditDialogWidth", next.width);
      saveLayoutSize("auditDialogHeight", next.height);
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
      const next = clampAuditSize(dialogSize.width + delta, dialogSize.height + delta, window.innerWidth, window.innerHeight);
      setDialogSize(next);
      saveLayoutSize("auditDialogWidth", next.width);
      saveLayoutSize("auditDialogHeight", next.height);
    } else if (e.key === "Home" || e.key === "End") {
      e.preventDefault();
      const next = e.key === "Home" ? { width: AUDIT_DIALOG_MIN_W, height: AUDIT_DIALOG_MIN_H } : clampAuditSize(AUDIT_DIALOG_MAX_W, AUDIT_DIALOG_MAX_H);
      setDialogSize(next);
      saveLayoutSize("auditDialogWidth", next.width);
      saveLayoutSize("auditDialogHeight", next.height);
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
        aria-label={t("audit.modalTitle")}
        style={dialogStyle}
      >
        <header className="reasonix-audit-dialog__header">
          <span className="modal__title">{t("audit.modalTitle")}</span>
          <button ref={closeRef} type="button" className="reasonix-audit-dialog__close" onClick={onClose} aria-label={t("common.close")}>
            ✕
          </button>
        </header>

        <div className="reasonix-audit-dialog__body">
          {status === "loading" && (
            <div className="audit-card audit-card--loading">
              <span className="audit-card__spinner" aria-hidden="true" />
              <span>{t("audit.running")}</span>
            </div>
          )}

          {status === "error" && (
            <div className="audit-card audit-card--error" role="alert">
              {t("audit.failed")}: {error}
            </div>
          )}

          {(status === "streaming" || status === "done") && (
            <>
              {totals && <AuditVerdict totals={totals} threshold={threshold} t={t} />}

              {request && (
                <AuditSection
                  title={t("audit.input")}
                  open={showInput}
                  onToggle={() => setShowInput((v) => !v)}
                  extra={request.truncated ? <span className="audit-truncated">{t("audit.truncated")}</span> : null}
                >
                  <pre className="audit-stage__pre">{request.input}</pre>
                </AuditSection>
              )}

              <AuditSection title={t("audit.technical")} open={showTech} onToggle={() => setShowTech((v) => !v)}>
                {request && (
                  <>
                    <div className="audit-stage__label">{t("audit.requestPrompt")}</div>
                    <pre className="audit-stage__pre">{request.systemPrompt}</pre>
                  </>
                )}
                {hasThink && (
                  <>
                    <div className="audit-stage__label">{t("audit.processReasoning")}</div>
                    <pre className="audit-stage__pre">{think}</pre>
                  </>
                )}
                <div className="audit-stage__label">{t("audit.processOutput")}</div>
                <pre className="audit-stage__pre audit-stage__pre--output">
                  {text}
                  {streaming && <span className="audit-card__cursor" aria-hidden="true" />}
                </pre>
                {!text && streaming && <span className="audit-stage__hint">{t("audit.streamingHint")}</span>}
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
          aria-valuemin={AUDIT_DIALOG_MIN_W}
          aria-valuemax={AUDIT_DIALOG_MAX_W}
          aria-valuenow={dialogSize.width}
          onPointerDown={startResize}
          onKeyDown={onResizeKey}
          onDoubleClick={() => {
            const next = { width: 680, height: 560 };
            setDialogSize(next);
            saveLayoutSize("auditDialogWidth", next.width);
            saveLayoutSize("auditDialogHeight", next.height);
          }}
        >
          <span aria-hidden="true" />
        </button>
      </div>
    </div>,
    document.body,
  );
}
