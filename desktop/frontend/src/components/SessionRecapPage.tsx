import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ExternalLink, RotateCw, Search } from "lucide-react";
import type { RecapOpenItem, RecapSkillDraft, SessionMeta, SessionRecap, SessionRecapEntry, SessionRecapInsight } from "../lib/types";
import { useT } from "../lib/i18n";
import { useManagementT } from "../lib/managementLocale";
import { ManagementPageShell } from "./ManagementPageShell";
import { groupByTopic } from "../lib/recapTopics";
import { RecapRow } from "./RecapRow";
import { RecapHeatmap } from "./RecapHeatmap";
import { RecapRecallStrip } from "./RecapRecallStrip";
import { matchesHeatmapDay } from "../lib/recapHeatmap";
import type { RecapPreviewView } from "../lib/types";
import type { RecallRecordView } from "../generated/desktopContract.generated";

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
// A preview is one model-written draft per button press: memory keeps one draft per
// note (each note becomes its own memory), a playbook is a single draft for the topic.
type PreviewDraft = { entry: SessionRecapEntry; view: RecapPreviewView; text: string };
type PreviewState = { kind: "memory" | "skill"; drafts: PreviewDraft[]; markdown: string; group: string };

export function SessionRecapPage({ active, onBack, list, listSessions, resume, accept, reject, undo, listOpenItems, keep, close, reopen, generate, draftSkill, draftTopicSkill, previewMemory, previewSkill, listInsights, recallRecord }: {
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
  close: (id: string, body: string, evidence: string, resolution: string) => Promise<void>;
  reopen: (id: string) => Promise<void>;
  // generate asks the host to (re)run one session's recap now. It queues into the
  // same lane the close path uses, so it returns as soon as the work is accepted.
  generate: (sessionPath: string) => Promise<boolean>;
  // draftSkill has the host write a playbook draft into this project's skill
  // directory. Only the kinds that are procedures can become one.
  draftSkill: (kind: string, body: string) => Promise<RecapSkillDraft>;
  // One call per topic: the host composes a single playbook from the notes the
  // page grouped, so a batch is one file or several depending on the topic.
  draftTopicSkill: (sources: { kind: string; body: string }[], markdown: string) => Promise<RecapSkillDraft>;
  previewMemory: (source: { kind: string; body: string }) => Promise<RecapPreviewView>;
  previewSkill: (sources: { kind: string; body: string }[]) => Promise<RecapPreviewView>;
  // listInsights is the projection read as a report: what more than one project
  // reached on its own. Read-only, and computed from the same evidence rule.
  listInsights: () => Promise<SessionRecapInsight[]>;
  // recallRecord reads one session's recall/skill fingerprints by transcript path
  // (the page lists sessions that are not open tabs). Optional: a host without it
  // simply shows no strip.
  recallRecord?: (sessionPath: string) => Promise<RecallRecordView>;
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
  const [queued, setQueued] = useState<string[]>([]);
  const [drafted, setDrafted] = useState<Record<string, string>>({});
  // Which notes of a same-topic group a bulk write should take; unset means yes.
  const [selected, setSelected] = useState<Record<string, boolean>>({});
  // Session cards start folded except the newest one: a page of cards is a list
  // first, and the reader's attention is at the top. Explicit toggles win.
  const [openCards, setOpenCards] = useState<Record<string, boolean>>({});
  const [openGroups, setOpenGroups] = useState<Record<string, boolean>>({});
  // A preview is a draft the model wrote under one of the two prompts, waiting for a
  // person to read it. Nothing is stored until it is confirmed, and the button's
  // no-model behaviour is always offered as the way out of a failed generation.
  const [preview, setPreview] = useState<PreviewState | null>(null);
  const [insights, setInsights] = useState<SessionRecapInsight[]>([]);
  // A heatmap cell filters the list below; "" means no filter.
  const [heatmapDay, setHeatmapDay] = useState("");
  const [editing, setEditing] = useState<{ id: string; text: string } | null>(null);
  const [closing, setClosing] = useState<{ id: string; text: string } | null>(null);
  const seq = useRef(0);
  const refresh = useCallback(async () => {
    const generation = ++seq.current;
    setLoading(true);
    // Session metadata and the unfinished list are niceties, so losing either
    // must not hide the recaps themselves.
    const [recapValue, sessionValue, itemValue, insightValue] = await Promise.all([
      list().catch(() => null),
      listSessions().catch(() => [] as SessionMeta[]),
      listOpenItems().catch(() => [] as RecapOpenItem[]),
      listInsights().catch(() => [] as SessionRecapInsight[]),
    ]);
    if (generation !== seq.current) return;
    const records = recapValue ?? [];
    setRecaps(records);
    setSessions(sessionValue ?? []);
    setOpenItems(itemValue ?? []);
    setInsights(insightValue ?? []);
    // A session that has since produced a record is no longer waiting in the lane.
    setQueued((current) => current.filter((path) => !records.some((record) => record.path === path)));
    setLoadFailed(recapValue === null);
    setLoading(false);
  }, [list, listInsights, listOpenItems, listSessions]);
  // Read through a ref: a caller passing fresh callbacks every render must not turn
  // this effect into a refresh loop.
  const refreshRef = useRef(refresh);
  useEffect(() => { refreshRef.current = refresh; }, [refresh]);
  useEffect(() => { if (active) void refreshRef.current(); }, [active]);
  useEffect(() => () => { seq.current++; }, []);
  // The generation lane is asynchronous, so a queued session is read back once
  // later; one timer for the whole page, cleared on the way out.
  const refreshTimer = useRef(0);
  const scheduleRefresh = useCallback(() => {
    if (refreshTimer.current !== 0) return;
    refreshTimer.current = window.setTimeout(() => { refreshTimer.current = 0; void refreshRef.current(); }, 3000);
  }, []);
  useEffect(() => () => { if (refreshTimer.current !== 0) window.clearTimeout(refreshTimer.current); }, []);

  const byPath = useMemo(() => new Map(sessions.map((meta) => [meta.path, meta])), [sessions]);
  const titleOf = useCallback((recap: { path: string }) => {
    const meta = byPath.get(recap.path);
    return (meta?.title ?? "").trim() || (meta?.preview ?? "").trim() || recap.path.split(/[/\\]/).pop() || recap.path;
  }, [byPath]);
  const rows = useMemo(() => {
    const needle = query.trim().toLowerCase();
    const searchable = (recap: SessionRecap) => [
      titleOf(recap), recap.model,
      ...recap.entries.map((entry) => `${entry.body}\n${entry.evidence ?? ""}`),
    ].join("\n").toLowerCase();
    const matched = recaps.filter((recap) => (needle === "" || searchable(recap).includes(needle)) && matchesHeatmapDay(recap, heatmapDay));
    matched.sort((left, right) => {
      if (sort === "session") return titleOf(left).localeCompare(titleOf(right));
      // A failed attempt has no generatedAt; its own last try is what orders it.
      const leftStamp = left.generatedAt || left.pending?.updatedAt || "";
      const rightStamp = right.generatedAt || right.pending?.updatedAt || "";
      const order = leftStamp < rightStamp ? -1 : leftStamp > rightStamp ? 1 : 0;
      return sort === "oldest" ? order : -order;
    });
    return matched;
  }, [heatmapDay, recaps, query, sort, titleOf]);

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
  // One button per topic writes every selected note of it, and each note still
  // lands on its own: the group is a render-time row, never a merged note.
  const runGroup = useCallback(async (groupId: string, choice: string, entries: SessionRecapEntry[]) => {
    if (entries.length === 0) return;
    setBusy(groupId);
    setFailure("");
    try {
      for (const entry of entries) {
        if (choice === "accept") await accept(entry.kind, entry.body, "");
        else await reject(entry.kind, entry.body);
        settle(entry.id, choice);
      }
    } catch {
      setFailure(m("operationFailed"));
    } finally {
      setBusy("");
    }
  }, [accept, m, reject, settle]);

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

  // makeDraft turns one note into a playbook draft inside this project. The host
  // refuses the kinds that are not procedures, so the button never promises more
  // than it can do.
  const makeDraft = useCallback(async (entry: SessionRecapEntry) => {
    setBusy(entry.id);
    setFailure("");
    try {
      const draft = await draftSkill(entry.kind, entry.body);
      setDrafted((current) => ({ ...current, [entry.id]: draft.path }));
    } catch {
      setFailure(m("operationFailed"));
    } finally {
      setBusy("");
    }
  }, [draftSkill, m]);

  // One draft per playbook-worthy note in the topic, because a playbook is only as
  // good as the steps behind it: this reuses each note's own cited ground rather
  // than inventing a procedure.
  const askMemoryPreview = useCallback(async (entries: SessionRecapEntry[]) => {
    setBusy(entries[0]?.id ?? "");
    setFailure("");
    try {
      const drafts: PreviewDraft[] = [];
      for (const entry of entries) {
        const view = await previewMemory({ kind: entry.kind, body: entry.body });
        drafts.push({ entry, view, text: view.text !== "" ? view.text : view.fallback });
      }
      setPreview({ kind: "memory", drafts, markdown: "", group: entries.map((entry) => entry.id).join("|") });
    } catch {
      setFailure(m("operationFailed"));
    } finally {
      setBusy("");
    }
  }, [m, previewMemory]);

  const askSkillPreview = useCallback(async (entries: SessionRecapEntry[]) => {
    setBusy(entries[0]?.id ?? "");
    setFailure("");
    try {
      const sources = entries.map((entry) => ({ kind: entry.kind, body: entry.body }));
      const view = await previewSkill(sources);
      setPreview({
        kind: "skill",
        drafts: entries.map((entry) => ({ entry, view, text: "" })),
        markdown: view.text !== "" ? view.text : view.fallback,
        group: entries.map((entry) => entry.id).join("|"),
      });
    } catch {
      setFailure(m("operationFailed"));
    } finally {
      setBusy("");
    }
  }, [m, previewSkill]);

  // Confirming is the only place that writes: the memory path stores exactly what the
  // preview shows (as an edited body), the skill path writes the reviewed markdown.
  const confirmPreview = useCallback(async () => {
    if (preview === null) return;
    const current = preview;
    setBusy(current.group);
    setFailure("");
    try {
      if (current.kind === "memory") {
        for (const draft of current.drafts) {
          await accept(draft.entry.kind, draft.entry.body, draft.text);
          settle(draft.entry.id, "accept");
        }
      } else {
        const sources = current.drafts.map((draft) => ({ kind: draft.entry.kind, body: draft.entry.body }));
        const written = await draftTopicSkill(sources, current.markdown);
        setDrafted((existing) => {
          const next = { ...existing };
          for (const draft of current.drafts) next[draft.entry.id] = written.path;
          return next;
        });
      }
      setPreview(null);
    } catch {
      setFailure(m("operationFailed"));
    } finally {
      setBusy("");
    }
  }, [accept, draftTopicSkill, m, preview, settle]);

  const draftGroupSkills = useCallback(async (entries: SessionRecapEntry[]) => {
    setBusy(entries[0]?.id ?? "");
    setFailure("");
    try {
      const draft = await draftTopicSkill(entries.map((entry) => ({ kind: entry.kind, body: entry.body })), "");
      setDrafted((current) => {
        const next = { ...current };
        for (const entry of entries) next[entry.id] = draft.path;
        return next;
      });
    } catch {
      setFailure(m("operationFailed"));
    } finally {
      setBusy("");
    }
  }, [draftTopicSkill, m]);

  const setItemClosed = useCallback(async (item: RecapOpenItem, closed: boolean, resolution = "") => {
    setBusy(item.id);
    setFailure("");
    try {
      await (closed ? close(item.id, item.body, item.evidence ?? "", resolution) : reopen(item.id));
      setOpenItems((current) => current.map((candidate) =>
        candidate.id === item.id ? { ...candidate, closed } : candidate));
    } catch {
      setFailure(m("operationFailed"));
    } finally {
      setBusy("");
    }
  }, [close, m, reopen]);

  const labelStyle = { color: "var(--fg-dim)" } as const;
  // One bounded panel above one scroller. The panel carries what the page already
  // knows (heatmap, reports, queues) and is capped, so growing note counts can only
  // scroll inside it — never squeeze the cards themselves out of the window.
  const pageStyle = { display: "flex", flexDirection: "column", flex: "1 1 auto", minHeight: 0 } as const;
  const headStyle = { flex: "0 0 auto", maxHeight: "33vh", overflowY: "auto" } as const;
  const panelStyle = { flex: "0 1 auto", minHeight: 0, maxHeight: "40vh", overflowY: "auto" } as const;
  const listStyle = { flex: "1 1 auto", minHeight: 160, maxHeight: "none", borderRightWidth: 0 } as const;
  const waiting = openItems.filter((item) => !item.closed).length;
  const stale = openItems.filter((item) => !item.closed && item.stale === true).length;
  const failed = recaps.filter((recap) => recap.state === "pending").length;
  // The card the page opens by itself: the first row under the default sort.
  const newestPath = rows[0]?.path ?? "";
  const cardOpen = (path: string) => openCards[path] ?? path === newestPath;
  const toggleCard = useCallback((path: string) => {
    setOpenCards((current) => ({ ...current, [path]: !(current[path] ?? path === newestPath) }));
  }, [newestPath]);
  // A session that never produced a recap appears in neither the record list nor
  // the failure list, so the newest few are offered here: yielding nothing must
  // still leave it one click from a retry. Older ones stay a bulk job
  // (`reasonix catalogs reindex session-recap`).
  // A stored note is a snapshot: these were produced by an older rule set than the
  // one running now, and only regenerating rewrites them.
  const staleRecaps = useMemo(() => recaps.filter((recap) => recap.stale === true), [recaps]);
  const known = useMemo(() => new Set(recaps.map((recap) => recap.path)), [recaps]);
  const ungenerated = useMemo(() => sessions
    .filter((meta) => !known.has(meta.path))
    .sort((left, right) => (right.lastActivityAt ?? right.modTime ?? 0) - (left.lastActivityAt ?? left.modTime ?? 0))
    .slice(0, 5), [known, sessions]);
  const askGenerate = useCallback(async (path: string) => {
    setBusy(path); setFailure("");
    try {
      const accepted = await generate(path);
      if (!accepted) { setFailure(m("operationFailed")); return; }
      setQueued((current) => [path, ...current.filter((candidate) => candidate !== path)]);
      // The lane is asynchronous, so the page cannot wait for the result: it says
      // the work is queued and reads back once, in case it is already done.
      scheduleRefresh();
    } catch {
      setFailure(m("operationFailed"));
    } finally {
      setBusy("");
    }
  }, [generate, m, scheduleRefresh]);
  // A batch is a loop over the same per-session request: the lane stays
  // one-session-at-a-time, so nothing here is a new host contract.
  const askGenerateAll = useCallback(async (paths: string[]) => {
    for (const path of paths) await askGenerate(path);
  }, [askGenerate]);

  const sorts: { id: RecapSort; label: string }[] = [
    { id: "newest", label: m("recapSortNewest") },
    { id: "oldest", label: m("recapSortOldest") },
    { id: "session", label: m("recapSortSession") },
  ];
  return <ManagementPageShell active={active} onBack={onBack} title={t("history.recapTitle")}
    description={`${m("recapDescription")} ${m("recapAcceptHint")}`}
    actions={<button className="btn btn--small" disabled={loading} onClick={() => void refresh()}><RotateCw size={14} />{m("refresh")}</button>}>
    <div className="recap-page" style={pageStyle}>
    <div className="recap-page__head" style={headStyle}>
    {loadFailed && <div className="management-notice" role="alert">{m("loadFailed")}<button className="btn btn--small" onClick={() => void refresh()}>{m("retry")}</button></div>}
    {failure !== "" && <div className="management-notice" role="alert">{failure}</div>}
    {loading && <div className="management-notice" role="status">{m("loading")}</div>}
    {/* The empty line is only the fallback: the ungenerated list below already
        offers those same sessions a generation. */}
    {!loading && !loadFailed && recaps.length === 0 && ungenerated.length === 0 && <div className="management-notice" role="status">{t("history.recapEmpty")}</div>}
    {!loading && failed > 0 && <div className="management-notice" role="status">{m("recapPendingNotice", { n: failed })}</div>}
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
        {/* A selected heatmap day filters the list the way the query does, so it
            stands here as one removable chip, not only as a highlighted cell. */}
        {heatmapDay !== "" && (
          <button type="button" className="history-filter__pill history-filter__pill--on"
            aria-label={`${m("clearFilters")}: ${heatmapDay}`} title={m("clearFilters")}
            onClick={() => setHeatmapDay("")}>{heatmapDay} ✕</button>
        )}
        <span style={{ ...labelStyle, fontSize: 12 }}>{m("recapCount", { shown: rows.length, total: recaps.length })}</span>
        <span style={{ display: "flex", gap: 6 }}>
          <button className="btn btn--small recap-expand-all" type="button"
            onClick={() => setOpenCards(Object.fromEntries(rows.map((recap) => [recap.path, true])))}>{m("recapExpandAll")}</button>
          <button className="btn btn--small recap-collapse-all" type="button"
            onClick={() => setOpenCards(Object.fromEntries(rows.map((recap) => [recap.path, false])))}>{m("recapCollapseAll")}</button>
        </span>
      </div>
    )}
    {!loading && recaps.length > 0 && rows.length === 0 && (
      <div className="management-notice" role="status">{m("recapNoMatch")}
        <button className="btn btn--small" onClick={() => { setQuery(""); setHeatmapDay(""); }}>{m("clearFilters")}</button></div>
    )}
    </div>
    {!loading && <div className="recap-page__panel" style={panelStyle}>
    <RecapHeatmap insights={insights} recaps={recaps} selectedDay={heatmapDay} onSelectDay={setHeatmapDay} />

    {!loading && insights.length > 0 && (
      <section style={{ marginBottom: 12 }}>
        <div style={{ ...labelStyle, fontSize: 12 }}>{m("recapInsightsTitle")}</div>
        <div style={{ marginTop: 6, display: "flex", flexDirection: "column", gap: 4 }}>
          {insights.map((insight) => (
            <div key={`${insight.kind}:${insight.body}`}>
              <span style={{ ...labelStyle, fontSize: 12 }}>{insight.kind}</span> {insight.body}{" "}
              <span style={{ ...labelStyle, fontSize: 12 }}>
                {insight.projects.length >= 2
                  ? m("recapInsightsLine", { n: insight.projects.length, list: insight.projects.join("、") })
                  : m("recapInsightsRepeat", { n: insight.occurrences })}
              </span>
            </div>
          ))}
        </div>
      </section>
    )}
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
              {item.closed ? (
                <button className="btn btn--small" type="button" disabled={busy !== ""}
                  onClick={() => void setItemClosed(item, false)}>{m("recapUndo")}</button>
              ) : closing?.id === item.id ? (
                <span style={{ display: "inline-flex", gap: 6, alignItems: "center" }}>
                  <input className="input" style={{ minWidth: 240 }} disabled={busy !== ""}
                    placeholder={m("recapOutcomePlaceholder")} value={closing.text}
                    onChange={(event) => setClosing({ id: item.id, text: event.target.value })} />
                  <button className="btn btn--small" type="button" disabled={busy !== ""}
                    onClick={() => { const text = closing.text; setClosing(null); void setItemClosed(item, true, text); }}>
                    {m("recapOutcomeSave")}</button>
                  <button className="btn btn--small" type="button" disabled={busy !== ""}
                    onClick={() => setClosing(null)}>{m("recapOutcomeCancel")}</button>
                </span>
              ) : (
                <button className="btn btn--small" type="button" disabled={busy !== ""}
                  onClick={() => setClosing({ id: item.id, text: "" })}>{m("recapMarkHandled")}</button>
              )}
            </li>
          ))}
        </ul>
      </div>
    )}
    {!loading && preview !== null && (
      <div className="management-notice" style={{ flexDirection: "column", alignItems: "stretch", gap: 6 }}>
        <strong>{preview.kind === "memory" ? m("recapPreviewMemoryTitle") : m("recapPreviewSkillTitle")}</strong>
        <span style={{ ...labelStyle, fontSize: 12 }}>
          {preview.kind === "memory" ? m("recapPreviewMemoryHint") : m("recapPreviewSkillHint")}
        </span>
        {preview.kind === "memory" ? preview.drafts.map((draft) => (
          <div key={draft.entry.id} style={{ display: "flex", flexDirection: "column", gap: 2 }}>
            <span style={{ ...labelStyle, fontSize: 12 }}>{draft.entry.body}</span>
            {draft.view.reason !== undefined && draft.view.text === "" && (
              <span style={{ ...labelStyle, fontSize: 12, color: "var(--warn, inherit)" }}>
                {m("recapPreviewFailed", { reason: draft.view.reason })}</span>
            )}
            <textarea rows={2} value={draft.text} style={{ width: "100%" }}
              onChange={(event) => setPreview((current) => current === null ? null : {
                ...current,
                drafts: current.drafts.map((item) => item.entry.id === draft.entry.id ? { ...item, text: event.target.value } : item),
              })} />
            {draft.view.promptTag !== "" && (
              <span style={{ ...labelStyle, fontSize: 12 }}>
                {m("recapPreviewProvenance", { tag: draft.view.promptTag, model: draft.view.model })}</span>
            )}
          </div>
        )) : (
          <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
            {preview.drafts[0] !== undefined && preview.drafts[0].view.reason !== undefined && preview.drafts[0].view.text === "" && (
              <span style={{ ...labelStyle, fontSize: 12, color: "var(--warn, inherit)" }}>
                {m("recapPreviewFailed", { reason: preview.drafts[0].view.reason })}</span>
            )}
            <textarea rows={14} value={preview.markdown} style={{ width: "100%", fontFamily: "monospace" }}
              onChange={(event) => setPreview((current) => current === null ? null : { ...current, markdown: event.target.value })} />
            {preview.drafts[0] !== undefined && preview.drafts[0].view.promptTag !== "" && (
              <span style={{ ...labelStyle, fontSize: 12 }}>
                {m("recapPreviewProvenance", { tag: preview.drafts[0].view.promptTag, model: preview.drafts[0].view.model })}</span>
            )}
          </div>
        )}
        <span style={{ display: "flex", gap: 6 }}>
          <button className="btn btn--small recap-preview-confirm" type="button" disabled={busy !== ""}
            onClick={() => void confirmPreview()}>{preview.kind === "memory" ? m("recapPreviewConfirmMemory") : m("recapPreviewConfirmSkill")}</button>
          {preview.kind === "skill" && (
            <button className="btn btn--small" type="button" disabled={busy !== ""}
              onClick={() => {
                const drafts = preview.drafts.map((draft) => draft.entry);
                setPreview(null);
                void (drafts.length === 1 ? makeDraft(drafts[0]) : draftGroupSkills(drafts));
              }}>
              {m("recapPreviewUseVerbatim")}</button>
          )}
          <button className="btn btn--small" type="button" onClick={() => setPreview(null)}>{m("cancel")}</button>
        </span>
      </div>
    )}
    {!loading && ungenerated.length > 0 && (
      <div className="management-notice" style={{ flexDirection: "column", alignItems: "stretch", gap: 4 }}>
        <strong>{m("recapUngeneratedTitle", { n: ungenerated.length })}</strong>
        <span style={{ ...labelStyle, fontSize: 12 }}>{m("recapUngeneratedHint")}</span>
        <ul style={{ listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: 4 }}>
          {groupByTopic(ungenerated.map((meta) => ({
            id: meta.path, kind: "session", body: titleOf({ path: meta.path }),
          }))).map((group) => (
            <li key={group.key} style={{ display: "flex", flexDirection: "column", gap: 4 }}>
              {group.entries.length > 1 && (
                <span style={{ display: "flex", gap: 6, alignItems: "baseline", flexWrap: "wrap" }}>
                  <span style={{ ...labelStyle, fontSize: 12 }} title={m("recapTopicGroupHint")}>{m("recapTopicGroupSessions", { n: group.entries.length })}</span>
                  <button className="btn btn--small" type="button" disabled={busy !== ""}
                    onClick={() => void askGenerateAll(group.entries.map((item) => item.id))}>{m("recapGenerate")}</button>
                </span>
              )}
              {group.entries.map((item) => (
                <RecapRow key={item.id} badge={m("recapUngeneratedBadge")}
                  text={item.body}
                  actions={<>
                    <button className="btn btn--small" type="button" disabled={busy !== ""}
                      onClick={() => void askGenerate(item.id)}>{m("recapGenerate")}</button>
                    {queued.includes(item.id) && <span style={{ ...labelStyle, fontSize: 12 }}>{m("recapQueued")}</span>}
                  </>} />
              ))}
            </li>
          ))}
        </ul>
      </div>
    )}
    {!loading && staleRecaps.length > 0 && (
      <div className="management-notice" style={{ flexDirection: "column", alignItems: "stretch", gap: 4 }}>
        <strong>{m("recapStaleTitle", { n: staleRecaps.length })}</strong>
        <span style={{ ...labelStyle, fontSize: 12 }}>{m("recapStaleHint")}</span>
        <ul style={{ listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: 4 }}>
          {groupByTopic(staleRecaps.map((recap) => ({
            id: recap.path, kind: "session", body: titleOf(recap),
          }))).map((group) => (
            <li key={group.key} style={{ display: "flex", flexDirection: "column", gap: 4 }}>
              {group.entries.length > 1 && (
                <span style={{ display: "flex", gap: 6, alignItems: "baseline", flexWrap: "wrap" }}>
                  <span style={{ ...labelStyle, fontSize: 12 }} title={m("recapTopicGroupHint")}>{m("recapTopicGroupSessions", { n: group.entries.length })}</span>
                  <button className="btn btn--small" type="button" disabled={busy !== ""}
                    onClick={() => void askGenerateAll(group.entries.map((item) => item.id))}>{m("recapGenerate")}</button>
                </span>
              )}
              {group.entries.map((item) => (
                <RecapRow key={item.id} badge={m("recapStaleBadge")}
                  text={item.body}
                  actions={<>
                    <button className="btn btn--small" type="button" disabled={busy !== ""}
                      onClick={() => void askGenerate(item.id)}>{m("recapGenerate")}</button>
                    {queued.includes(item.id) && <span style={{ ...labelStyle, fontSize: 12 }}>{m("recapQueued")}</span>}
                  </>} />
              ))}
            </li>
          ))}
        </ul>
      </div>
    )}
    </div>}
    {!loading && <div className="history-list recap-page__list" style={listStyle}>
      <ul style={{ listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: 8 }}>
        {rows.map((recap) => {
          const meta = byPath.get(recap.path);
          return <li key={recap.path} className="management-notice" style={{ flexDirection: "column", alignItems: "stretch", gap: 4 }}>
            <div style={{ display: "flex", gap: 12, alignItems: "baseline", flexWrap: "wrap", fontSize: 12 }}>
              <span role="button" tabIndex={0} data-recap-toggle="" title={m("recapFoldHint")}
                onClick={() => toggleCard(recap.path)}
                onKeyDown={(event) => { if (event.key === "Enter" || event.key === " ") { event.preventDefault(); toggleCard(recap.path); } }}
                style={{ cursor: "pointer" }}><strong>{titleOf(recap)}</strong></span>
              {recap.entries.length > 0 && <span style={labelStyle}>{m("recapNoteCount", { n: recap.entries.length })}</span>}
              {meta && <span style={labelStyle}>{t(meta.turns === 1 ? "history.turnOne" : "history.turnOther", { n: meta.turns })}</span>}
              {recap.generatedAt !== "" && <span style={labelStyle}>{t("history.recapGeneratedAt")}：{formatStamp(recap.generatedAt)}</span>}
              {recap.model !== "" && <span style={labelStyle}>{t("history.recapModel")}：{recap.model}</span>}
              {recap.pending && <span style={labelStyle}>{m("recapPendingSince")}：{formatStamp(recap.pending.updatedAt)}</span>}
              {meta && <button className="btn btn--small" type="button" onClick={() => { void resume(meta); onBack(); }}>
                <ExternalLink size={13} />{m("recapOpen")}</button>}
            </div>
            {recap.pending && <p style={{ margin: 0, display: "flex", gap: 6, alignItems: "baseline", flexWrap: "wrap", color: "var(--warn, inherit)" }}>
              <span>{m("recapPendingLine", { n: recap.pending.attempts, reason: recap.pending.reason })}</span>
              <button className="btn btn--small" type="button" disabled={busy !== ""}
                onClick={() => void askGenerate(recap.path)}>{m("recapRetryGenerate")}</button>
              {queued.includes(recap.path) && <span style={{ ...labelStyle, fontSize: 12 }}>{m("recapQueued")}</span>}
            </p>}
            {/* Notes fold away; the pending line above does not, because a failed
                attempt is a state the reader has to see without expanding. */}
            {cardOpen(recap.path) && <>
            {/* A failed attempt has no notes to speak of; "nothing reusable" would
                be the wrong one of the two silences. */}
            {recap.entries.length === 0 && recap.state !== "pending" && <p style={{ margin: 0, ...labelStyle }}>{m("recapNoEntries")}</p>}
            {groupByTopic(recap.entries).map((group) => {
              const grouped = group.entries.length > 1;
              const open = group.entries.filter((entry) => (entry.decision ?? "") === "" && entry.target !== "display" && meta !== undefined);
              const chosen = open.filter((entry) => selected[entry.id] ?? true);
              // Same gate the per-note button uses: the host refuses a playbook from
              // anything but a procedure, and a display-only note is not reviewable.
              const groupDraftable = group.entries.filter((entry) => meta !== undefined && entry.target !== "display"
                && (entry.kind === "root-cause" || entry.kind === "refuted"));
              return <div key={group.key} style={{ display: "flex", flexDirection: "column", gap: 6 }}>
              {grouped && <p style={{ margin: 0, display: "flex", gap: 6, alignItems: "baseline", flexWrap: "wrap" }}>
                <span style={{ ...labelStyle, fontSize: 12 }} title={m("recapTopicGroupHint")}>{m("recapTopicGroup", { n: group.entries.length })}</span>
                {chosen.length > 0 && <button className="btn btn--small" type="button" disabled={busy !== ""}
                  onClick={() => void askMemoryPreview(chosen)}>{m("recapAccept")}</button>}
                {/* Rejecting takes the same selection the accept button does: a note
                    the reader left unchecked must not be dropped by this row. */}
                {chosen.length > 0 && <button className="btn btn--small" type="button" disabled={busy !== ""}
                  onClick={() => void runGroup(group.key, "reject", chosen)}>{m("recapReject")}</button>}
                {groupDraftable.length > 0 && <button className="btn btn--small" type="button" disabled={busy !== ""}
                  title={m("recapDraftSkillBatchHint", { n: groupDraftable.length })}
                  onClick={() => void askSkillPreview(groupDraftable)}>{m("recapDraftSkill")}</button>}
              </p>}
            {(openGroups[group.key] || group.entries.length <= 6 ? group.entries : group.entries.slice(0, 6)).map((entry) => {
              const key = kindKey(entry.kind);
              const reviewable = meta !== undefined && entry.target !== "display";
              // The host omits an unset decision, so anything but an explicit
              // choice counts as untouched.
              const decision = entry.decision ?? "";
              const item = openItems.find((candidate) => candidate.id === entry.id);
              const actions = <>
                {decision === "accept" && <span style={{ ...labelStyle, fontSize: 12 }}>{m("recapAccepted")}</span>}
                {decision === "reject" && <span style={{ ...labelStyle, fontSize: 12 }}>{m("recapRejected")}</span>}
                {reviewable && !grouped && decision === "" && <>
                  <button className="btn btn--small" type="button" disabled={busy !== ""}
                    onClick={() => void askMemoryPreview([entry])}>{m("recapAccept")}</button>
                  <button className="btn btn--small" type="button" disabled={busy !== ""}
                    onClick={() => setEditing({ id: entry.id, text: entry.body })}>{m("recapAcceptEdited")}</button>
                  <button className="btn btn--small" type="button" disabled={busy !== ""}
                    onClick={() => void run(entry.id, "reject", async () => { await reject(entry.kind, entry.body); })}>{m("recapReject")}</button>
                </>}
                {reviewable && !grouped && decision !== "" && <button className="btn btn--small" type="button" disabled={busy !== ""}
                  onClick={() => void run(entry.id, "", async () => { await undo(entry.kind, entry.body); })}>{m("recapUndo")}</button>}
                {reviewable && grouped && decision === "" && <button className="btn btn--small" type="button" disabled={busy !== ""}
                  onClick={() => setEditing({ id: entry.id, text: entry.body })}>{m("recapAcceptEdited")}</button>}
                {reviewable && grouped && decision !== "" && <button className="btn btn--small" type="button" disabled={busy !== ""}
                  onClick={() => void run(entry.id, "", async () => { await undo(entry.kind, entry.body); })}>{m("recapUndo")}</button>}
                {meta !== undefined && entry.kind === "handoff" && (item === undefined
                  ? <button className="btn btn--small" type="button" disabled={busy !== ""}
                      onClick={() => void keepHandoff(recap.path, entry)}>{m("recapKeepOpen")}</button>
                  : <span style={{ ...labelStyle, fontSize: 12 }}>{item.closed ? m("recapOpenHandled") : m("recapOpenKept")}</span>)}
                {/* Only procedures become playbooks; the host refuses the rest, so
                    the button is offered only where it can do something. */}
                {reviewable && (entry.kind === "root-cause" || entry.kind === "refuted") && <>
                  <button className="btn btn--small" type="button" disabled={busy !== ""}
                    onClick={() => void askSkillPreview([entry])}>{m("recapDraftSkill")}</button>
                  {drafted[entry.id] !== undefined && (
                    <span style={{ ...labelStyle, fontSize: 12 }}>{m("recapSkillDrafted", { path: drafted[entry.id] })}</span>
                  )}
                </>}
              </>;
              const detail = <>
                {entry.evidence && <span style={{ ...labelStyle, fontSize: 12 }}>（{entry.evidence}）</span>}
                {entry.scope && <span style={{ ...labelStyle, fontSize: 12 }}>{m("recapScopeProposed", { level: entry.scope })}</span>}
                {entry.observedIn !== undefined && entry.observedIn.length > 0 && (
                  <span style={{ ...labelStyle, fontSize: 12 }}>
                    {m("recapObservedIn", { n: entry.observedIn.length, list: entry.observedIn.join("、") })}
                  </span>
                )}
                {entry.refs !== undefined && entry.refs.length > 0 && (
                  <span style={{ ...labelStyle, fontSize: 12 }}>
                    {entry.refs.map((ref) => `${ref.kind} ${ref.value}${ref.detail ? ` ${ref.detail}` : ""}`).join(" · ")}
                  </span>
                )}
              </>;
              return <RecapRow key={entry.id}
                leading={reviewable && grouped ? <input type="checkbox" aria-label={m("recapAccept")} checked={selected[entry.id] ?? true}
                  onChange={(event) => setSelected((current) => ({ ...current, [entry.id]: event.target.checked }))} /> : undefined}
                badge={`${key ? t(key) : entry.kind} · ${entry.target === "display" ? m("recapSinkDisplay") : m("recapSinkMemory")}`}
                text={entry.body}
                actions={actions}
                detail={detail}
                editor={editing?.id === entry.id ? <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
                  <textarea value={editing.text} rows={3} style={{ width: "100%" }}
                    onChange={(event) => setEditing({ id: entry.id, text: event.target.value })} />
                  <span style={{ display: "flex", gap: 6 }}>
                    <button className="btn btn--small" type="button" disabled={busy !== "" || editing.text.trim() === ""}
                      onClick={() => { const next = editing.text; setEditing(null); void run(entry.id, "accept", async () => { await accept(entry.kind, entry.body, next); }); }}>
                      {m("recapSaveAccept")}</button>
                    <button className="btn btn--small" type="button" onClick={() => setEditing(null)}>{m("cancel")}</button>
                  </span>
                </div> : undefined} />;
            })}
            {group.entries.length > 6 && (
              <button className="btn btn--small" type="button" style={{ alignSelf: "flex-start" }}
                onClick={() => setOpenGroups((current) => ({ ...current, [group.key]: !(current[group.key] ?? false) }))}>
                {openGroups[group.key] ? m("recapHideNotes") : m("recapMoreNotes", { n: group.entries.length - 6 })}</button>)}
            </div>;
            })}
            </>}
            {/* One host read per open card: a folded card shows no strip, so a page of
                cards no longer fires one read per row. */}
            {cardOpen(recap.path) && recallRecord !== undefined
              && <RecapRecallStrip sessionPath={recap.path} recallRecord={recallRecord} />}
          </li>;
        })}
      </ul>
    </div>}
    </div>
  </ManagementPageShell>;
}

// formatStamp renders the backend's RFC3339 stamp in the reader's own locale; a
// stamp that does not parse is shown as it arrived rather than dropped.
function formatStamp(stamp: string): string {
  const at = new Date(stamp);
  return Number.isNaN(at.getTime()) ? stamp : at.toLocaleString();
}
