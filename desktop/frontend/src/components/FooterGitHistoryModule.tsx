// Footer-panel module: the workspace's recent commits, the same `git log` the
// dock's changes overview reads. Five rows to start, five more per click, and the
// control collapses once the whole answer is on screen.

import { useCallback, useEffect, useState } from "react";
import { ChevronDown, RotateCw } from "lucide-react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import type { GitCommitView } from "../lib/types";
import { workspaceFormatCommitDate } from "../lib/workspacePanelFormat";
import { FooterPanelSection, type FooterPanelModuleProps } from "./FooterPanel";
import { PanelRowButton } from "./FooterPanelDetail";
import { PanelDiff } from "./FooterPanelDiff";

export const FOOTER_COMMITS_INITIAL = 5;
export const FOOTER_COMMITS_PAGE = 5;

// The dock's formatter ("29 Sep 2026 13:20") is too wide for a card row; the full
// form rides the row's tooltip instead.
function compactCommitDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const pad = (part: number) => String(part).padStart(2, "0");
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

export function FooterGitHistoryModule({ tabId }: FooterPanelModuleProps) {
  const t = useT();
  const [commits, setCommits] = useState<GitCommitView[] | null>(null);
  const [visibleCount, setVisibleCount] = useState(FOOTER_COMMITS_INITIAL);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    if (!tabId) return;
    let cancelled = false;
    setLoading(true);
    // A synchronous bridge failure must land in the same catch as a rejected
    // call: `git log` in a workspace without git is exactly how "no history
    // here" reaches us, and that must not throw out of the effect.
    void Promise.resolve()
      .then(() => app.WorkspaceGitHistory(tabId, ""))
      .then((next) => {
        if (cancelled) return;
        setCommits(Array.isArray(next) ? next : []);
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

  // A new session starts its own page window; a refresh keeps the one in view.
  useEffect(() => {
    setVisibleCount(FOOTER_COMMITS_INITIAL);
  }, [tabId]);

  const refresh = useCallback(() => setRevision((current) => current + 1), []);
  const togglePage = useCallback((total: number) => {
    setVisibleCount((current) => (current >= total ? FOOTER_COMMITS_INITIAL : Math.min(current + FOOTER_COMMITS_PAGE, total)));
  }, []);

  if (!tabId) return null;
  // Unanswered, or a workspace where git has nothing: the section stays away
  // rather than reporting an empty history it never read.
  if (!commits) return null;

  const shown = commits.slice(0, visibleCount);
  const remaining = commits.length - shown.length;
  const step = Math.min(FOOTER_COMMITS_PAGE, remaining);

  return (
    <FooterPanelSection title="footerPanel.gitHistory">
      <div className="footer-git">
        <div className="footer-panel__bar">
          <span>{t("footerPanel.commitCount", { n: commits.length })}</span>
          {error ? <span className="footer-panel__warn" title={error}>{t("workspace.historyUnavailable")}</span> : null}
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
        {commits.length === 0 ? (
          <p className="footer-panel__note">{t("footerPanel.noCommits")}</p>
        ) : (
          <>
            <ul className="footer-git__list">
              {shown.map((commit) => (
                <li key={commit.hash}>
                  <PanelRowButton
                    className="footer-git__row"
                    detail={{
                      title: commit.message,
                      meta: [commit.hash, commit.author, workspaceFormatCommitDate(commit.date)],
                      body: <PanelDiff tabId={tabId} path="" commit={commit.hash} />,
                      bodyStyle: "raw",
                    }}
                  >
                    <span className="footer-git__hash">{commit.hash.slice(0, 7)}</span>
                    <span className="footer-git__message">{commit.message}</span>
                    <span className="footer-git__date">{compactCommitDate(commit.date)}</span>
                  </PanelRowButton>
                </li>
              ))}
            </ul>
            {commits.length > FOOTER_COMMITS_INITIAL ? (
              <button type="button" className="footer-panel__more" onClick={() => togglePage(commits.length)}>
                <ChevronDown
                  className={`footer-panel__chevron${remaining > 0 ? " footer-panel__chevron--closed" : ""}`}
                  size={12}
                  aria-hidden="true"
                />
                <span>{remaining > 0 ? t("footerPanel.showMore", { n: step }) : t("footerPanel.showLess")}</span>
              </button>
            ) : null}
          </>
        )}
      </div>
    </FooterPanelSection>
  );
}
