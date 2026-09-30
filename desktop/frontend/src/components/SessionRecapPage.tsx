import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ExternalLink, RotateCw, Search } from "lucide-react";
import type { RecapOpenItem, SessionMeta, SessionRecap, SessionRecapEntry } from "../lib/types";
import { useT } from "../lib/i18n";
import { useManagementT } from "../lib/managementLocale";
import { ManagementPageShell } from "./ManagementPageShell";

type RecapSort = "newest" | "oldest" | "session";

type RecapKindKey = "history.recapKindFact" | "history.recapKindRootCause" | "history.recapKindRefuted" | "history.recapKindHandoff";

// kindKey names a distilled note's kind; a kind this page does not know is shown
// as it arrived rather than dropped.
function kindKey(kind: string): RecapKindKey | null {
  switch (kind) {
    case "fact": return "history.recapKindFact";
    case "root-cause": return "history.recapKindRootCause";
    case "refuted": return "history.recapKindRefuted";
    case "handoff": return "history.recapKindHandoff";
    default: return null;
  }
}

// Notes are written by the backend when a session closes; what this page adds is
// the person's answer to each one — accept, edit-then-accept, or drop — plus the
// one kind that outlives the session: an unfinished item is kept for its project
// and offered to a later session that continues the same subject.
//
// Only the current project can be settled: an accepted note becomes a fact in the
// active project's memory, and an unfinished item belongs to the project its
// session lives in. A session from another project is read-only here.
export function SessionRecapPage({ active, onBack, list, listSessions, resume, accept, reject, undo, listOpenItems, keep, close, reopen }: {
  active: boolean;
  onBack: () => void;
  list: () => Promise<SessionRecap[]>;
  listSessions: () => Promise<SessionMeta[]>;
  resume: (session: SessionMeta) => void | Promise<void>;
  accept: (kind: string, body: string, editedBody: string) => Promise<string>;
  reject: (kind: string, body: string) => Promise<void>;
  undo: (kind: string, body: string) => Promise<void>;
  listOpenItems: () => Promise<RecapOpenItem[]>;
  keep: (sessionPath: string, body: string, evidence: string) => Promise<string>;
  close: (id: string) => Promise<void>;
  reopen: (id: string) => Promise<void>;
}) {
  const t = useT(); const m = useManagementT();
  const [recaps, setRecaps] = useState<SessionRecap[]>([]);
  const [sessions, setSessions] = useState<SessionMeta[]>([]);
  const [openItems, setOpenItems] = useState<RecapOpenItem[]>([]);
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<RecapSort>("newest");
  const [loading, setLoading] = useState(true);
  const [loadFailed, setLoadFailed] = useState(false);
  const [busy, setBusy] = useState("");
  const [failure, setFailure] = useState("");
  const [editing, setEditing] = useState<{ id: string; text: string } | null>(null);
  const seq = useRef(0);
  const refresh = useCallback(async () => {
    const generation = ++seq.current;
    setLoading(true);
    // Session metadata and the unfinished list are niceties, so losing either
    // must not hide the recaps themselves.
    const [recapValue, sessionValue, itemValue] = await Promise.all([
      list().catch(() => null),
      listSessions().catch(() => [] as SessionMeta[]),
      listOpenItems().catch(() => [] as RecapOpenItem[]),
    ]);
    if (generation !== seq.current) return;
    setRecaps(recapValue ?? []);
    setSessions(sessionValue ?? []);
    setOpenItems(itemValue ?? []);
    setLoadFailed(recapValue === null);
    setLoading(false);
  }, [list, listOpenItems, listSessions]);
  useEffect(() => { if (active) void refresh(); }, [active, refresh]);
  useEffect(() => () => { seq.current++; }, []);

  const byPath = useMemo(() => new Map(sessions.map((meta) => [meta.path, meta])), [sessions]);
  const titleOf = useCallback((recap: SessionRecap) => {
    const meta = byPath.get(recap.path);
    return (meta?.title ?? "").trim() || (meta?.preview ?? "").trim() || recap.path.split(/[/\\]/).pop() || recap.path;
  }, [byPath]);
  const rows = useMemo(() => {
    const needle = query.trim().toLowerCase();
    const searchable = (recap: SessionRecap) => [
      titleOf(recap), recap.model,
      ...recap.entries.map((entry) => `${entry.body}\n${entry.evidence ?? ""}`),
    ].join("\n").toLowerCase();
    const matched = needle === "" ? recaps.slice() : recaps.filter((recap) => searchable(recap).includes(needle));
    matched.sort((left, right) => {
      if (sort === "session") return titleOf(left).localeCompare(titleOf(right));
      const order = left.generatedAt < right.generatedAt ? -1 : left.generatedAt > right.generatedAt ? 1 : 0;
      return sort === "oldest" ? order : -order;
    });
    return matched;
  }, [recaps, query, sort, titleOf]);

  // settle marks one note with the choice the backend just recorded, so the page
  // reflects it without a second round trip.
  const settle = useCallback((id: string, choice: string) => {
    setRecaps((current) => current.map((recap) => ({
      ...recap,
      entries: recap.entries.map((entry) => (entry.id === id ? { ...entry, decision: choice } : entry)),
    })));
  }, []);
  const run = useCallback(async (id: string, choice: string, work: () => Promise<void>) => {
    setBusy(id);
    setFailure("");
    try {
      await work();
      settle(id, choice);
    } catch {
      setFailure(m("operationFailed"));
    } finally {
      setBusy("");
    }
  }, [m, settle]);

  // keepHandoff records one handoff note as an unfinished item of its project.
  // The item keeps the note's own id, so the entry and the list entry agree.
  const keepHandoff = useCallback(async (sessionPath: string, entry: SessionRecapEntry) => {
    setBusy(entry.id);
    setFailure("");
    try {
      const id = await keep(sessionPath, entry.body, entry.evidence ?? "");
      setOpenItems((current) => [
        { id, body: entry.body, evidence: entry.evidence, from: sessionPath, openedAt: new Date().toISOString(), closed: false },
        ...current.filter((item) => item.id !== id),
      ]);
    } catch {
      setFailure(m("operationFailed"));
    } finally {
      setBusy("");
    }
  }, [keep, m]);

  const setItemClosed = useCallback(async (item: RecapOpenItem, closed: boolean) => {
    setBusy(item.id);
    setFailure("");
    try {
      await (closed ? close(item.id) : reopen(item.id));
      setOpenItems((current) => current.map((candidate) =>
        candidate.id === item.id ? { ...candidate, closed } : candidate));
    } catch {
      setFailure(m("operationFailed"));
    } finally {
      setBusy("");
    }
  }, [close, m, reopen]);

  const labelStyle = { color: "var(--fg-dim)" } as const;
  const waiting = openItems.filter((item) => !item.closed).length;
  const stale = openItems.filter((item) => !item.closed && item.stale === true).length;
  const sorts: { id: RecapSort; label: string }[] = [
    { id: "newest", label: m("recapSortNewest") },
    { id: "oldest", label: m("recapSortOldest") },
    { id: "session", label: m("recapSortSession") },
  ];
  return <ManagementPageShell active={active} onBack={onBack} title={t("history.recapTitle")}
    description={`${m("recapDescription")} ${m("recapAcceptHint")}`}
    actions={<button className="btn btn--small" disabled={loading} onClick={() => void refresh()}><RotateCw size={14} />{m("refresh")}</button>}>
    {loadFailed && <div className="management-notice" role="alert">{m("loadFailed")}<button className="btn btn--small" onClick={() => void refresh()}>{m("retry")}</button></div>}
    {failure !== "" && <div className="management-notice" role="alert">{failure}</div>}
    {loading && <div className="management-notice" role="status">{m("loading")}</div>}
    {!loading && !loadFailed && recaps.length === 0 && <div className="management-notice" role="status">{t("history.recapEmpty")}</div>}
    {!loading && openItems.length > 0 && (
      <div className="management-notice" style={{ flexDirection: "column", alignItems: "stretch", gap: 4 }}>
        <strong>{m("recapOpenItemsTitle", { n: waiting })}</strong>
        {stale > 0 && <span style={{ ...labelStyle, fontSize: 12 }}>{m("recapOpenStaleCount", { n: stale })}</span>}
        <ul style={{ listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: 4 }}>
          {openItems.map((item) => (
            <li key={item.id} style={{ display: "flex", gap: 6, alignItems: "baseline", flexWrap: "wrap", opacity: item.closed ? 0.55 : 1 }}>
              <span>{item.body}</span>
              {item.evidence && <span style={{ ...labelStyle, fontSize: 12 }}>（{item.evidence}）</span>}
              {item.closed && <span style={{ ...labelStyle, fontSize: 12 }}>{m("recapOpenHandled")}</span>}
              {!item.closed && item.stale === true && <span style={{ ...labelStyle, fontSize: 12 }}>{m("recapOpenStale")}</span>}
              <button className="btn btn--small" type="button" disabled={busy !== ""}
                onClick={() => void setItemClosed(item, !item.closed)}>
                {item.closed ? m("recapUndo") : m("recapMarkHandled")}</button>
            </li>
          ))}
        </ul>
      </div>
    )}
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
            {recap.entries.length === 0 && <p style={{ margin: 0, ...labelStyle }}>{m("recapNoEntries")}</p>}
            {recap.entries.map((entry) => {
              const key = kindKey(entry.kind);
              const reviewable = meta !== undefined && entry.target !== "display";
              // The host omits an unset decision, so anything but an explicit
              // choice counts as untouched.
              const decision = entry.decision ?? "";
              const item = openItems.find((candidate) => candidate.id === entry.id);
              return <div key={entry.id} style={{ display: "flex", flexDirection: "column", gap: 4 }}>
                <p style={{ margin: 0, display: "flex", gap: 6, alignItems: "baseline", flexWrap: "wrap" }}>
                  <span style={{ ...labelStyle, fontSize: 12 }}>
                    {key ? t(key) : entry.kind} · {entry.target === "display" ? m("recapSinkDisplay") : m("recapSinkMemory")}
                  </span>
                  <span>{entry.body}</span>
                  {entry.evidence && <span style={{ ...labelStyle, fontSize: 12 }}>（{entry.evidence}）</span>}
                  {decision === "accept" && <span style={{ ...labelStyle, fontSize: 12 }}>{m("recapAccepted")}</span>}
                  {decision === "reject" && <span style={{ ...labelStyle, fontSize: 12 }}>{m("recapRejected")}</span>}
                  {reviewable && <span style={{ display: "flex", gap: 6 }}>
                    {decision === "" && <button className="btn btn--small" type="button" disabled={busy !== ""}
                      onClick={() => void run(entry.id, "accept", async () => { await accept(entry.kind, entry.body, ""); })}>{m("recapAccept")}</button>}
                    {decision === "" && <button className="btn btn--small" type="button" disabled={busy !== ""}
                      onClick={() => setEditing({ id: entry.id, text: entry.body })}>{m("recapAcceptEdited")}</button>}
                    {decision === "" && <button className="btn btn--small" type="button" disabled={busy !== ""}
                      onClick={() => void run(entry.id, "reject", async () => { await reject(entry.kind, entry.body); })}>{m("recapReject")}</button>}
                    {decision !== "" && <button className="btn btn--small" type="button" disabled={busy !== ""}
                      onClick={() => void run(entry.id, "", async () => { await undo(entry.kind, entry.body); })}>{m("recapUndo")}</button>}
                  </span>}
                  {meta !== undefined && entry.kind === "handoff" && <span style={{ display: "flex", gap: 6 }}>
                    {item === undefined
                      ? <button className="btn btn--small" type="button" disabled={busy !== ""}
                          onClick={() => void keepHandoff(recap.path, entry)}>{m("recapKeepOpen")}</button>
                      : <span style={{ ...labelStyle, fontSize: 12 }}>{item.closed ? m("recapOpenHandled") : m("recapOpenKept")}</span>}
                  </span>}
                </p>
                {editing?.id === entry.id && <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
                  <textarea value={editing.text} rows={3} style={{ width: "100%" }}
                    onChange={(event) => setEditing({ id: entry.id, text: event.target.value })} />
                  <span style={{ display: "flex", gap: 6 }}>
                    <button className="btn btn--small" type="button" disabled={busy !== "" || editing.text.trim() === ""}
                      onClick={() => { const text = editing.text; setEditing(null); void run(entry.id, "accept", async () => { await accept(entry.kind, entry.body, text); }); }}>
                      {m("recapSaveAccept")}</button>
                    <button className="btn btn--small" type="button" onClick={() => setEditing(null)}>{m("cancel")}</button>
                  </span>
                </div>}
              </div>;
            })}
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
