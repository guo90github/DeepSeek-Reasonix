// Sample footer-panel module: the session's working-tree changes, reusing the
// dock's WorkspaceChanges resource instead of a second git reader. Reloads on
// session switch and on demand; it never polls. It renders nothing at all where
// git does not back the workspace.

import { useCallback, useEffect, useState } from "react";
import { RotateCw } from "lucide-react";
import { useT } from "../lib/i18n";
import { useWorkspaceChangesResource } from "../lib/useWorkspaceChangesResource";
import { FooterPanelSection, type FooterPanelModuleProps } from "./FooterPanel";

export function FooterChangedFilesModule({ tabId, workspaceScopeKey }: FooterPanelModuleProps) {
  const t = useT();
  const [revision, setRevision] = useState(0);
  const [gitBacked, setGitBacked] = useState(false);
  const { workspaceChanges, loadingWorkspaceChanges, workspaceChangesErr, loadWorkspaceChanges } =
    useWorkspaceChangesResource(tabId ?? "", workspaceScopeKey, revision);

  useEffect(() => {
    if (!tabId) return;
    void loadWorkspaceChanges();
  }, [loadWorkspaceChanges, tabId]);

  useEffect(() => {
    if (workspaceChanges) setGitBacked(workspaceChanges.gitAvailable);
  }, [workspaceChanges]);

  const refresh = useCallback(() => setRevision((current) => current + 1), []);

  // A header that can never fill is worse than no header. The flag is monotonic
  // across switches on purpose: workspaceScopeKey carries the tab and the session
  // generation, so a per-key memo could never hit and the section would blank out
  // on every project switch. A failure keeps the section (and its error line).
  if (!tabId) return null;
  if (!gitBacked && !workspaceChangesErr) return null;

  const files = workspaceChanges?.files ?? [];

  return (
    <FooterPanelSection title="footerPanel.changedFiles">
      <div className="footer-changed">
        <div className="footer-changed__bar">
          {workspaceChanges?.gitBranch ? <span className="footer-changed__branch">{workspaceChanges.gitBranch}</span> : null}
          <span className="footer-changed__count">{t("footerPanel.fileCount", { n: files.length })}</span>
          {typeof workspaceChanges?.added === "number" && workspaceChanges.added > 0 ? (
            <span className="footer-changed__stat footer-changed__stat--add">{`+${workspaceChanges.added}`}</span>
          ) : null}
          {typeof workspaceChanges?.removed === "number" && workspaceChanges.removed > 0 ? (
            <span className="footer-changed__stat footer-changed__stat--del">{`-${workspaceChanges.removed}`}</span>
          ) : null}
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
        ) : workspaceChanges === null ? (
          <p className="footer-panel__note">{t("common.loading")}</p>
        ) : workspaceChanges.gitErr ? (
          <p className="footer-panel__note">{workspaceChanges.gitErr}</p>
        ) : files.length === 0 ? (
          <p className="footer-panel__note">{t("footerPanel.noChanges")}</p>
        ) : (
          <ul className="footer-changed__list">
            {files.map((file) => (
              <li className="footer-changed__row" key={file.path} title={file.path}>
                <span className="footer-changed__status">{file.gitStatus?.trim() || "·"}</span>
                <span className="footer-changed__path">{file.path}</span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </FooterPanelSection>
  );
}
