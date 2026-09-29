import { useCallback, useEffect, useRef, useState } from "react";
import { RotateCw } from "lucide-react";
import type { SessionRecap } from "../lib/types";
import { useT } from "../lib/i18n";
import { useManagementT } from "../lib/managementLocale";
import { ManagementPageShell } from "./ManagementPageShell";

// Read-only: recaps are written by the backend when a session closes, so this
// page exposes no delete/restore — those stay on the session in Trash.
export function SessionRecapPage({ active, onBack, list }: {
  active: boolean; onBack: () => void; list: () => Promise<SessionRecap[]>;
}) {
  const t = useT(); const m = useManagementT();
  const [recaps, setRecaps] = useState<SessionRecap[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadFailed, setLoadFailed] = useState(false);
  const seq = useRef(0);
  const refresh = useCallback(async () => {
    const generation = ++seq.current;
    setLoading(true);
    try {
      const value = await list();
      if (generation !== seq.current) return;
      setRecaps(value); setLoadFailed(false);
    } catch {
      if (generation !== seq.current) return;
      setLoadFailed(true);
    } finally { if (generation === seq.current) setLoading(false); }
  }, [list]);
  useEffect(() => { if (active) void refresh(); }, [active, refresh]);
  useEffect(() => () => { seq.current++; }, []);
  const labelStyle = { color: "var(--fg-dim)" } as const;
  return <ManagementPageShell active={active} onBack={onBack} title={t("history.recapTitle")}
    description={m("recapDescription")}
    actions={<button className="btn btn--small" disabled={loading} onClick={() => void refresh()}><RotateCw size={14} />{m("refresh")}</button>}>
    {loadFailed && <div className="management-notice" role="alert">{m("loadFailed")}<button className="btn btn--small" onClick={() => void refresh()}>{m("retry")}</button></div>}
    {loading && <div className="management-notice" role="status">{m("loading")}</div>}
    {!loading && !loadFailed && recaps.length === 0 && <div className="management-notice" role="status">{t("history.recapEmpty")}</div>}
    {!loading && <ul style={{ listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: 8 }}>
      {recaps.map((recap) => <li key={recap.path} className="management-notice" style={{ flexDirection: "column", alignItems: "stretch", gap: 4 }}>
        <div style={{ display: "flex", gap: 12, color: "var(--fg-dim)", fontSize: 12 }}>
          <span>{t("history.recapGeneratedAt")}：{recap.generatedAt}</span>
          <span>{t("history.recapModel")}：{recap.model}</span>
        </div>
        <p style={{ margin: 0 }}><strong style={labelStyle}>{t("history.recapGoal")}：</strong>{recap.goal}</p>
        <p style={{ margin: 0 }}><strong style={labelStyle}>{t("history.recapActions")}：</strong>{recap.actions}</p>
        <p style={{ margin: 0 }}><strong style={labelStyle}>{t("history.recapConclusion")}：</strong>{recap.conclusion}</p>
        {recap.todos && <p style={{ margin: 0 }}><strong style={labelStyle}>{t("history.recapTodos")}：</strong>{recap.todos}</p>}
      </li>)}
    </ul>}
  </ManagementPageShell>;
}
