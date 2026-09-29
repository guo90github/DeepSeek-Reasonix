// The row-detail modal's diff body: a file row asks for that file's patch, a
// commit row for that commit's. Git is authoritative for a tracked file, and the
// host falls back to session checkpoints for one only Reasonix touched
// (desktop/workspace_changes.go), so both kinds of row can answer.

import { useEffect, useState } from "react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { languageFor } from "../lib/selectedTextContext";
import { DiffView } from "./DiffView";

export function PanelDiff({ tabId, path, commit }: { tabId?: string; path: string; commit?: string }) {
  const t = useT();
  const [diff, setDiff] = useState<string | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!tabId) return;
    let cancelled = false;
    setDiff(null);
    setError("");
    // A synchronous bridge failure must land in the same catch as a rejected call.
    void Promise.resolve()
      .then(() => (commit ? app.WorkspaceGitCommitDetail(tabId, commit, path) : app.WorkspaceChangeDetail(tabId, path)))
      .then((next) => {
        if (cancelled) return;
        setDiff(next?.diff ?? "");
      })
      .catch((failure: unknown) => {
        if (cancelled) return;
        setError(failure instanceof Error ? failure.message : String(failure));
      });
    return () => {
      cancelled = true;
    };
  }, [tabId, path, commit]);

  if (!tabId) return null;
  if (error) return <p className="footer-panel__note footer-panel__note--error">{error}</p>;
  if (diff === null) return <p className="footer-panel__note">{t("common.loading")}</p>;
  if (diff.trim() === "") return <p className="footer-panel__note">{t("footerPanel.noChanges")}</p>;
  return <DiffView diff={diff} language={languageFor(path)} />;
}
