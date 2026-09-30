import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ExternalLink, RotateCw, Search } from "lucide-react";
import type { SessionMeta, SessionRecap } from "../lib/types";
import { useT } from "../lib/i18n";
import { useManagementT } from "../lib/managementLocale";
import { ManagementPageShell } from "./ManagementPageShell";

type RecapSort = "newest" | "oldest" | "session";

// Read-only: recaps are written by the backend when a session closes, so this
// page exposes no delete/restore — those stay on the session in Trash. Search
// and sort run on the loaded page: the list is one IPC call either way, and每行
// 带上会话标题，长列表才找得着。
export function SessionRecapPage({ active, onBack, list, listSessions, resume }: {
  active: boolean;
  onBack: () => void;
  list: () => Promise<SessionRecap[]>;
  listSessions: () => Promise<SessionMeta[]>;
  resume: (session: SessionMeta) => void | Promise<void>;
}) {
  const t = useT(); const m = useManagementT();
  const [recaps, setRecaps] = useState<SessionRecap[]>([]);
  const [sessions, setSessions] = useState<SessionMeta[]>([]);
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<RecapSort>("newest");
  const [loading, setLoading] = useState(true);
  const [loadFailed, setLoadFailed] = useState(false);
  const seq = useRef(0);
  const refresh = useCallback(async () => {
    const generation = ++seq.current;
    setLoading(true);
    // Session metadata is a nicety (title, turn count, jump target), so losing it
    // must not hide the recaps themselves.
    const [recapValue, sessionValue] = await Promise.all([
      list().catch(() => null),
      listSessions().catch(() => [] as SessionMeta[]),
    ]);
    if (generation !== seq.current) return;
    setRecaps(recapValue ?? []);
    setSessions(sessionValue ?? []);
    setLoadFailed(recapValue === null);
    setLoading(false);
  }, [list, listSessions]);
  useEffect(() => { if (active) void refresh(); }, [active, refresh]);
  useEffect(() => () => { seq.current++; }, []);

  const byPath = useMemo(() => new Map(sessions.map((meta) => [meta.path, meta])), [sessions]);
  const titleOf = useCallback((recap: SessionRecap) => {
    const meta = byPath.get(recap.path);
    return (meta?.title ?? "").trim() || (meta?.preview ?? "").trim() || recap.path.split(/[/\\]/).pop() || recap.path;
  }, [byPath]);
  const rows = useMemo(() => {
    const needle = query.trim().toLowerCase();
    const matched = needle === "" ? recaps.slice() : recaps.filter((recap) =>
      [titleOf(recap), recap.goal, recap.actions, recap.conclusion, recap.todos ?? "", recap.model]
        .join("\n").toLowerCase().includes(needle));
    matched.sort((left, right) => {
      if (sort === "session") return titleOf(left).localeCompare(titleOf(right));
      const order = left.generatedAt < right.generatedAt ? -1 : left.generatedAt > right.generatedAt ? 1 : 0;
      return sort === "oldest" ? order : -order;
    });
    return matched;
  }, [recaps, query, sort, titleOf]);

  const labelStyle = { color: "var(--fg-dim)" } as const;
  const sorts: { id: RecapSort; label: string }[] = [
    { id: "newest", label: m("recapSortNewest") },
    { id: "oldest", label: m("recapSortOldest") },
    { id: "session", label: m("recapSortSession") },
  ];
  return <ManagementPageShell active={active} onBack={onBack} title={t("history.recapTitle")}
    description={m("recapDescription")}
    actions={<button className="btn btn--small" disabled={loading} onClick={() => void refresh()}><RotateCw size={14} />{m("refresh")}</button>}>
    {loadFailed && <div className="management-notice" role="alert">{m("loadFailed")}<button className="btn btn--small" onClick={() => void refresh()}>{m("retry")}</button></div>}
    {loading && <div className="management-notice" role="status">{m("loading")}</div>}
    {!loading && !loadFailed && recaps.length === 0 && <div className="management-notice" role="status">{t("history.recapEmpty")}</div>}
    {!loading && recaps.length > 0 && (
      <div className="history-toolbar">
        <label className="mem-search history-search">
          <Search size={13} />
          <input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={m("recapSearch")} />
        </label>
        <div className="history-filter" role="group" aria-label={m("recapSort")}>
          {sorts.map((option) => (
            <button key={option.id} type="button" aria-pressed={sort === option.id}
              className={`history-filter__pill${sort === option.id ? " history-filter__pill--on" : ""}`}
              onClick={() => setSort(option.id)}>{option.label}</button>
          ))}
        </div>
        <span style={{ ...labelStyle, fontSize: 12 }}>{m("recapCount", { shown: rows.length, total: recaps.length })}</span>
      </div>
    )}
    {!loading && recaps.length > 0 && rows.length === 0 && (
      <div className="management-notice" role="status">{m("recapNoMatch")}
        <button className="btn btn--small" onClick={() => setQuery("")}>{m("clearFilters")}</button></div>
    )}
    {!loading && <div className="history-list" style={{ flex: 1, minHeight: 0 }}>
      <ul style={{ listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: 8 }}>
        {rows.map((recap) => {
          const meta = byPath.get(recap.path);
          return <li key={recap.path} className="management-notice" style={{ flexDirection: "column", alignItems: "stretch", gap: 4 }}>
            <div style={{ display: "flex", gap: 12, alignItems: "baseline", flexWrap: "wrap", fontSize: 12 }}>
              <strong>{titleOf(recap)}</strong>
              {meta && <span style={labelStyle}>{t(meta.turns === 1 ? "history.turnOne" : "history.turnOther", { n: meta.turns })}</span>}
              <span style={labelStyle}>{t("history.recapGeneratedAt")}：{formatStamp(recap.generatedAt)}</span>
              <span style={labelStyle}>{t("history.recapModel")}：{recap.model}</span>
              {meta && <button className="btn btn--small" type="button" onClick={() => { void resume(meta); onBack(); }}>
                <ExternalLink size={13} />{m("recapOpen")}</button>}
            </div>
            <p style={{ margin: 0 }}><strong style={labelStyle}>{t("history.recapGoal")}：</strong>{recap.goal}</p>
            <p style={{ margin: 0 }}><strong style={labelStyle}>{t("history.recapActions")}：</strong>{recap.actions}</p>
            <p style={{ margin: 0 }}><strong style={labelStyle}>{t("history.recapConclusion")}：</strong>{recap.conclusion}</p>
            {recap.todos && <p style={{ margin: 0 }}><strong style={labelStyle}>{t("history.recapTodos")}：</strong>{recap.todos}</p>}
          </li>;
        })}
      </ul>
    </div>}
  </ManagementPageShell>;
}

// formatStamp renders the backend's RFC3339 stamp in the reader's own locale; a
// stamp that does not parse is shown as it arrived rather than dropped.
function formatStamp(stamp: string): string {
  const at = new Date(stamp);
  return Number.isNaN(at.getTime()) ? stamp : at.toLocaleString();
}
