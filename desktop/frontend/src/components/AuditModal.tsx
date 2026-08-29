import { useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { app } from "../lib/bridge";
import { onAuditChunk, onAuditDone, onAuditRequest, type AuditChunkEvent, type AuditRequestPayload } from "../lib/auditStream";
import { useT } from "../lib/i18n";
import type { event } from "../../wailsjs/go/models";

type AuditStatus = "loading" | "streaming" | "done" | "error";

function findingLabel(t: ReturnType<typeof useT>, type: string): string {
  switch (type) {
    case "contradiction":
      return t("audit.typeContradiction");
    case "hallucination":
      return t("audit.typeHallucination");
    case "redundancy":
      return t("audit.typeRedundancy");
    case "instruction_drift":
      return t("audit.typeDrift");
    default:
      return type;
  }
}

// AuditModal shows one audit run in a centered modal. It leads with the audit
// deliverable — the verdict + its rationale (explanation) — then the audited
// input as context, and finally the technical trace (request params + the live
// model output) collapsed by default so raw mechanics do not dominate the
// result. It owns the stream lifecycle: subscribes on mount, cleans up on
// unmount (Escape / backdrop / close button).
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

  return createPortal(
    <div
      className="modal-backdrop reasonix-audit-backdrop"
      role="presentation"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div className="modal reasonix-audit-dialog" role="dialog" aria-modal="true" aria-label={t("audit.modalTitle")}>
        <div className="reasonix-audit-dialog__header">
          <span className="modal__title">{t("audit.modalTitle")}</span>
          <button ref={closeRef} type="button" className="reasonix-audit-dialog__close" onClick={onClose} aria-label={t("common.close")}>
            ✕
          </button>
        </div>

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
              {/* ① 结论 + 依据（headline，done 后出现） */}
              {totals && (
                <div className="audit-result">
                  <div className="audit-result__row">
                    <span
                      className={`audit__score audit-result__score${totals.score < threshold ? " audit__score--warn" : ""}`}
                      title={t("audit.scoreHint")}
                    >
                      {totals.score.toFixed(2)}
                    </span>
                    {totals.score < threshold ? (
                      <span className="audit-badge audit-badge--warn">{t("audit.attention")}</span>
                    ) : (
                      <span className="audit-badge">{t("audit.pass")}</span>
                    )}
                    <span className="audit__issues">
                      {t("audit.contradiction", { n: String(totals.contradiction ?? 0) })} ·{" "}
                      {t("audit.hallucination", { n: String(totals.hallucination ?? 0) })} ·{" "}
                      {t("audit.redundancy", { n: String(totals.redundancy ?? 0) })} ·{" "}
                      {t("audit.drift", { n: String(totals.instructionDrift ?? 0) })}
                    </span>
                    {typeof totals.evalTokens === "number" && totals.evalTokens > 0 && (
                      <span className="audit__meta">
                        {t("audit.tokens", { n: String(totals.evalTokens) })}
                        {typeof totals.evalCost === "number" && totals.evalCost > 0
                          ? ` · ${t("audit.cost", { c: totals.evalCost.toFixed(4) })}` : ""}
                        {totals.elapsedMs > 0 ? ` · ${(totals.elapsedMs / 1000).toFixed(1)}s` : ""}
                      </span>
                    )}
                  </div>
                  {totals.explanation && <p className="audit-result__evidence">{totals.explanation}</p>}
                  {Array.isArray(totals.findings) && totals.findings.length > 0 && (
                    <ul className="audit-findings">
                      {totals.findings.map((f, i) => (
                        <li key={i} className="audit-finding">
                          <span className={`audit-finding__tag audit-finding__tag--${f.type}`}>{findingLabel(t, f.type)}</span>
                          <span className="audit-finding__quote">“{f.quote}”</span>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              )}

              {/* ② 被审计的输入 */}
              {request && (
                <>
                  <button type="button" className="audit-stage__head" onClick={() => setShowInput((v) => !v)} aria-expanded={showInput}>
                    <span className={`audit-stage__chevron${showInput ? " audit-stage__chevron--open" : ""}`} aria-hidden="true" />
                    {t("audit.input")}
                    {request.truncated && <span className="audit-truncated">{t("audit.truncated")}</span>}
                  </button>
                  {showInput && (
                    <div className="audit-stage__body">
                      <pre className="audit-stage__pre audit-stage__pre--input">{request.input}</pre>
                    </div>
                  )}
                </>
              )}

              {/* ③ 技术详情（流式归入；流式中自动展开） */}
              <button type="button" className="audit-stage__head" onClick={() => setShowTech((v) => !v)} aria-expanded={showTech}>
                <span className={`audit-stage__chevron${showTech ? " audit-stage__chevron--open" : ""}`} aria-hidden="true" />
                {t("audit.technical")}
              </button>
              {showTech && (
                <div className="audit-stage__body">
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
                </div>
              )}
            </>
          )}
        </div>
      </div>
    </div>,
    document.body,
  );
}
