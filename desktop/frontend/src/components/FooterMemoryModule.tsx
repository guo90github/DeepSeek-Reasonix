// Footer-panel module: the session's memory — the facts that can be recalled,
// and the notes (standing instruction files) actually loaded for this workspace.
// Both sections come from one MemoryForTab answer.

import { useCallback, useEffect, useState } from "react";
import { RotateCw } from "lucide-react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import type { MemoryView } from "../lib/types";
import { FooterPanelSection, type FooterPanelModuleProps } from "./FooterPanel";
import { PanelRowButton } from "./FooterPanelDetail";

function compactChars(chars: number): string {
  return chars >= 1000 ? `${(chars / 1000).toFixed(1)}k` : String(chars);
}

export function FooterMemoryModule({ tabId }: FooterPanelModuleProps) {
  const t = useT();
  const [view, setView] = useState<MemoryView | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    if (!tabId) return;
    let cancelled = false;
    setLoading(true);
    void app.MemoryForTab(tabId)
      .then((next) => {
        if (cancelled) return;
        setView(next);
        setError("");
      })
      .catch((failure: unknown) => {
        if (cancelled) return;
        setError(failure instanceof Error ? failure.message : String(failure));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [tabId, revision]);

  const refresh = useCallback(() => setRevision((current) => current + 1), []);

  // Same rule as the session-changes module: an unanswered or empty store renders
  // nothing rather than a header that can never fill. The loaded answer survives a
  // session switch, so a refresh never blanks what the user is reading.
  if (!tabId) return null;
  if (!view && !error) return null;
  if (view && !view.available && view.facts.length === 0 && view.docs.length === 0) return null;

  const facts = view?.facts ?? [];
  const docs = view?.docs ?? [];
  const recall = view?.lastRecall;
  const conflicts = view?.conflicts?.length ?? 0;
  const diagnostics = view?.instructionDiagnostics?.length ?? 0;

  return (
    <>
      <FooterPanelSection title="footerPanel.memory">
        <div className="footer-memory">
          <div className="footer-memory__bar">
            <span>{t("footerPanel.factCount", { n: facts.length })}</span>
            {recall && recall.charBudget > 0 ? (
              <span className="footer-memory__recall" title={recall.query}>
                {t("footerPanel.recallUsage", { used: compactChars(recall.usedChars), budget: compactChars(recall.charBudget) })}
              </span>
            ) : null}
            {conflicts > 0 ? <span className="footer-memory__warn">{t("footerPanel.conflictCount", { n: conflicts })}</span> : null}
            <button
              type="button"
              className="footer-panel__refresh"
              title={t("footerPanel.refreshMemory")}
              aria-label={t("footerPanel.refreshMemory")}
              onClick={refresh}
            >
              <RotateCw className={loading ? "footer-panel__spin" : undefined} size={13} aria-hidden="true" />
            </button>
          </div>
          {error ? (
            <p className="footer-panel__note footer-panel__note--error">{error}</p>
          ) : facts.length === 0 ? (
            <p className="footer-panel__note">{t("footerPanel.noMemory")}</p>
          ) : (
            <ul className="footer-memory__list">
              {facts.map((fact) => (
                <li key={fact.name}>
                  <PanelRowButton
                    className={`footer-memory__row${fact.freshness === "stale" ? " footer-memory__row--stale" : ""}`}
                    detail={{
                      title: fact.title || fact.name,
                      meta: [fact.name, fact.type, fact.scope, fact.freshness],
                      body: [fact.description, fact.body].filter(Boolean).join("\n\n"),
                    }}
                  >
                    <span className="footer-memory__type">{fact.type}</span>
                    <span className="footer-memory__name">{fact.title || fact.name}</span>
                    <span className="footer-memory__note">{fact.description}</span>
                    {fact.scope === "global" ? <span className="footer-memory__tag">G</span> : null}
                  </PanelRowButton>
                </li>
              ))}
            </ul>
          )}
        </div>
      </FooterPanelSection>
      <FooterPanelSection title="footerPanel.instructions">
        <div className="footer-memory">
          <div className="footer-memory__bar">
            <span>{t("footerPanel.docCount", { n: docs.length })}</span>
            {diagnostics > 0 ? <span className="footer-memory__warn">{t("footerPanel.diagnostics", { n: diagnostics })}</span> : null}
          </div>
          {docs.length === 0 ? (
            <p className="footer-panel__note">{t("footerPanel.noNotes")}</p>
          ) : (
            <ul className="footer-memory__list">
              {docs.map((doc) => (
                <li key={`${doc.scope}:${doc.path}`}>
                  <PanelRowButton
                    className="footer-memory__row"
                    detail={{
                      title: doc.path,
                      meta: [doc.scope, ...doc.imports.map((entry) => entry.path)],
                      body: doc.body,
                    }}
                  >
                    <span className="footer-memory__type">{doc.scope}</span>
                    <span className="footer-memory__note">{doc.path}</span>
                    {doc.imports.length > 0 ? <span className="footer-memory__tag">{`+${doc.imports.length}`}</span> : null}
                  </PanelRowButton>
                </li>
              ))}
            </ul>
          )}
        </div>
      </FooterPanelSection>
    </>
  );
}
