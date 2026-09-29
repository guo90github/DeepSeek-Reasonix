// Footer-panel module: the git-managed uncommitted files of this session's
// workspace root — the project's working-tree state, which every session of that
// workspace reads the same (unlike the per-session checkpoint list).
//
// It asks `WorkspaceGitStatsForTab(tabID, workspaceRoot)`, which is git only
// (desktop/workspace_git_branches.go) and takes the root explicitly, rather than
// the union view the session module reads. A workspace git cannot answer for
// renders nothing at all.

import { useCallback, useEffect, useState } from "react";
import { ChevronDown, RotateCw } from "lucide-react";
import { useT } from "../lib/i18n";
import type { WorkspaceChangesView } from "../lib/types";
import { loadWorkspaceGitStats } from "../lib/workspaceGitStats";
import { workspaceBasename } from "../lib/workspacePanelFormat";
import { FooterPanelSection, type FooterPanelModuleProps } from "./FooterPanel";

export const FOOTER_GIT_UNCOMMITTED_INITIAL = 5;
export const FOOTER_GIT_UNCOMMITTED_PAGE = 5;

export function FooterGitUncommittedModule({ tabId, workspaceRoot }: FooterPanelModuleProps) {
  const t = useT();
  const [changes, setChanges] = useState<WorkspaceChangesView | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [visibleCount, setVisibleCount] = useState(FOOTER_GIT_UNCOMMITTED_INITIAL);
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    if (!tabId || !workspaceRoot) return;
    let cancelled = false;
    setLoading(true);
    void Promise.resolve()
      .then(() => loadWorkspaceGitStats(tabId, workspaceRoot, () => !cancelled))
      .then((next) => {
        if (cancelled || !next) return;
        setChanges(next);
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
  }, [tabId, workspaceRoot, revision]);

  useEffect(() => {
    setVisibleCount(FOOTER_GIT_UNCOMMITTED_INITIAL);
  }, [tabId, workspaceRoot]);

  const refresh = useCallback(() => setRevision((current) => current + 1), []);
  const togglePage = useCallback((total: number) => {
    setVisibleCount((current) => (current >= total ? FOOTER_GIT_UNCOMMITTED_INITIAL : Math.min(current + FOOTER_GIT_UNCOMMITTED_PAGE, total)));
  }, []);

  if (!tabId || !workspaceRoot) return null;
  // Unanswered, or a workspace where git cannot answer: nothing to report, and
  // the session module still covers whatever this session touched itself.
  if (!changes || !changes.gitAvailable) return null;

  const files = changes.files ?? [];
  const shown = files.slice(0, visibleCount);
  const remaining = files.length - shown.length;
  const step = Math.min(FOOTER_GIT_UNCOMMITTED_PAGE, remaining);

  return (
    <FooterPanelSection title="footerPanel.gitUncommitted">
      <div className="footer-changed">
        <div className="footer-changed__bar">
          <span className="footer-changed__workspace" title={workspaceRoot}>{workspaceBasename(workspaceRoot)}</span>
          {changes.gitBranch ? <span className="footer-changed__branch">{changes.gitBranch}</span> : null}
          <span className="footer-changed__count">{t("footerPanel.fileCount", { n: files.length })}</span>
          {typeof changes.added === "number" && changes.added > 0 ? (
            <span className="footer-changed__stat footer-changed__stat--add">{`+${changes.added}`}</span>
          ) : null}
          {typeof changes.removed === "number" && changes.removed > 0 ? (
            <span className="footer-changed__stat footer-changed__stat--del">{`-${changes.removed}`}</span>
          ) : null}
          <button
            type="button"
            className="footer-changed__refresh"
            title={t("footerPanel.refresh")}
            aria-label={t("footerPanel.refresh")}
            onClick={refresh}
          >
            <RotateCw className={loading ? "footer-changed__spin" : undefined} size={13} aria-hidden="true" />
          </button>
        </div>
        {error ? (
          <p className="footer-panel__note footer-panel__note--error">{error}</p>
        ) : files.length === 0 ? (
          <p className="footer-panel__note">{t("footerPanel.noChanges")}</p>
        ) : (
          <>
            <ul className="footer-changed__list">
              {shown.map((file) => (
                <li className="footer-changed__row" key={file.path} title={file.path}>
                  <span className="footer-changed__status">{file.gitStatus?.trim() || "·"}</span>
                  <span className="footer-changed__path">{file.path}</span>
                </li>
              ))}
            </ul>
            {files.length > FOOTER_GIT_UNCOMMITTED_INITIAL ? (
              <button type="button" className="footer-panel__more" onClick={() => togglePage(files.length)}>
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
