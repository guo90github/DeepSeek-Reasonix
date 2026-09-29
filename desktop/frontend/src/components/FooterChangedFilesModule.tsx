// Footer-panel module: the files THIS SESSION changed. The host's changes view
// unions this tab's session checkpoints with `git status` of this tab's
// workspace root (desktop/workspace_changes.go), so the rows are split by
// source: this module keeps the session-sourced ones, and FooterGitUncommittedModule
// owns the git-sourced ones — one list, two meanings, no ambiguity.
//
// The bar names the session's workspace, because a session bound to the global
// scratch dir is exactly why two sessions of one project disagree here.

import { useCallback, useEffect, useState } from "react";
import { ChevronDown, RotateCw } from "lucide-react";
import { useT } from "../lib/i18n";
import { useWorkspaceChangesResource } from "../lib/useWorkspaceChangesResource";
import { workspaceBasename } from "../lib/workspacePanelFormat";
import { FooterPanelSection, type FooterPanelModuleProps } from "./FooterPanel";
import { PanelRowButton } from "./FooterPanelDetail";

export const FOOTER_CHANGED_FILES_INITIAL = 5;
export const FOOTER_CHANGED_FILES_PAGE = 5;

export function FooterChangedFilesModule({ tabId, workspaceScopeKey, workspaceRoot }: FooterPanelModuleProps) {
  const t = useT();
  const [revision, setRevision] = useState(0);
  const [hadSessionRows, setHadSessionRows] = useState(false);
  const [visibleCount, setVisibleCount] = useState(FOOTER_CHANGED_FILES_INITIAL);
  const { workspaceChanges, loadingWorkspaceChanges, workspaceChangesErr, loadWorkspaceChanges } =
    useWorkspaceChangesResource(tabId ?? "", workspaceScopeKey, revision);

  useEffect(() => {
    if (!tabId) return;
    void loadWorkspaceChanges();
  }, [loadWorkspaceChanges, tabId]);

  useEffect(() => {
    if (!workspaceChanges) return;
    setHadSessionRows(workspaceChanges.files.some((file) => file.sources?.includes("session")));
  }, [workspaceChanges]);

  // A new session starts its own page window; a refresh keeps the one in view.
  useEffect(() => {
    setVisibleCount(FOOTER_CHANGED_FILES_INITIAL);
  }, [tabId, workspaceScopeKey]);

  const refresh = useCallback(() => setRevision((current) => current + 1), []);
  const togglePage = useCallback((total: number) => {
    setVisibleCount((current) => (current >= total ? FOOTER_CHANGED_FILES_INITIAL : Math.min(current + FOOTER_CHANGED_FILES_PAGE, total)));
  }, []);

  // A header that can never fill is worse than no header. The flag is monotonic
  // across switches on purpose: workspaceScopeKey carries the tab and the session
  // generation, so a per-key memo could never hit and the section would blank out
  // on every project switch. A failure keeps the section (and its error line).
  if (!tabId) return null;
  if (!hadSessionRows && !workspaceChangesErr) return null;

  const files = (workspaceChanges?.files ?? []).filter((file) => file.sources?.includes("session"));
  // A long session would otherwise push the composer's own row off the band; the
  // rest pages in five at a time, exactly like the commit list.
  const shown = files.slice(0, visibleCount);
  const remaining = files.length - shown.length;
  const step = Math.min(FOOTER_CHANGED_FILES_PAGE, remaining);
  const workspaceLabel = workspaceRoot ? workspaceBasename(workspaceRoot) : "";

  return (
    <FooterPanelSection title="footerPanel.sessionChanges">
      <div className="footer-changed">
        <div className="footer-changed__bar">
          {workspaceLabel ? (
            <span className="footer-changed__workspace" title={workspaceRoot}>{workspaceLabel}</span>
          ) : null}
          <span className="footer-changed__count">{t("footerPanel.fileCount", { n: files.length })}</span>
          <button
            type="button"
            className="footer-changed__refresh"
            title={t("footerPanel.refresh")}
            aria-label={t("footerPanel.refresh")}
            onClick={refresh}
          >
            <RotateCw className={loadingWorkspaceChanges ? "footer-changed__spin" : undefined} size={13} aria-hidden="true" />
          </button>
        </div>
        {workspaceChangesErr ? (
          <p className="footer-panel__note footer-panel__note--error">{workspaceChangesErr}</p>
        ) : files.length === 0 ? (
          <p className="footer-panel__note">{t("footerPanel.noChanges")}</p>
        ) : (
          <>
            <ul className="footer-changed__list">
              {shown.map((file) => (
                <li key={file.path}>
                  <PanelRowButton
                    className="footer-changed__row"
                    detail={{
                      title: file.path,
                      meta: [workspaceLabel, file.gitStatus?.trim() || "·", ...(file.sources ?? [])].filter(Boolean),
                      body: file.path,
                      mono: true,
                    }}
                  >
                    <span className="footer-changed__status">{file.gitStatus?.trim() || "·"}</span>
                    <span className="footer-changed__path">{file.path}</span>
                  </PanelRowButton>
                </li>
              ))}
            </ul>
            {files.length > FOOTER_CHANGED_FILES_INITIAL ? (
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
