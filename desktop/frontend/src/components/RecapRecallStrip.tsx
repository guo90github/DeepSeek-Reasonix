import { useCallback, useEffect, useMemo, useState } from "react";

import type { MemoryFact, RecallRecordView } from "../generated/desktopContract.generated";
import { useT } from "../lib/i18n";
import {  countRecallHitStates, recallHitState, recallHitStateKey,buildRecallLabels } from "../lib/recallLabels";
import "./RecapRecallStrip.css";

/**
 * 第十六: what a session's turns asked memory for and which skill they ran. The
 * record carries ids, counters, digests and each hit's short label — never a
 * fact's body — so this panel renders without exposing memory or skill text.
 *
 * A hit names itself from the label the record wrote (a tab whose controller is
 * not loaded has no live fact list to resolve ids against); a record written
 * before that label existed falls back to `facts`, and only then to the bare id.
 *
 * It is read-only: folding is local, and nothing here reaches a prompt.
 */
export function RecapRecallStrip({
  sessionPath = "",
  recallRecord,
  load: providedLoad,
  record: controlled,
  facts,
}: {
  sessionPath?: string;
  recallRecord?: (sessionPath: string) => Promise<RecallRecordView>;
  load?: () => Promise<RecallRecordView>;
  /** When the caller already holds the record — a section that must hide itself
   *  when there is none — the strip renders that instead of asking again. */
  record?: RecallRecordView | null;
  /** The caller's live fact list, for naming the ids the record carries. */
  facts?: readonly MemoryFact[];
}) {
  const t = useT();
  const [fetched, setFetched] = useState<RecallRecordView | null>(null);
  const [open, setOpen] = useState(false);
  const labels = useMemo(() => buildRecallLabels(facts ?? []), [facts]);

  // Two surfaces read the same record: the recap page knows a session path, the
  // memory panel knows only its tab, so `load` is the tab-scoped shape.
  const fetchRecord = useCallback(async () => {
    try {
      if (providedLoad) return await providedLoad();
      if (!recallRecord || sessionPath.trim() === "") return null;
      return await recallRecord(sessionPath);
    } catch {
      return null;
    }
  }, [providedLoad, recallRecord, sessionPath]);

  useEffect(() => {
    if (controlled !== undefined) return;
    void fetchRecord().then(setFetched);
  }, [controlled, fetchRecord]);

  const record = controlled !== undefined ? controlled : fetched;
  if (record === null || record.available !== true) return null;
  const turns = record.turns ?? [];
  const skills = record.skills ?? [];
  const counts = countRecallHitStates(turns);
  const trimmed = record.droppedTurns ?? 0;
  let summary = t("history.recallStripSummary", {
    injected: String(counts.injected),
    dropped: String(counts.dropped),
    skills: String(skills.length),
  });
  if (counts.unrecorded > 0) {
    summary += ` · ${t("history.recallStripUnrecordedCount", { n: String(counts.unrecorded) })}`;
  }
  if (trimmed > 0) {
    summary += ` · ${t("history.recallStripTrimmed", { n: String(trimmed) })}`;
  }

  return (
    <div className="recap-recall">
      <button
        type="button"
        className="recap-recall__head"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
      >
        <span className={`recap-recall__chevron${open ? "" : " recap-recall__chevron--closed"}`}>▸</span>
        <span className="recap-recall__text">{summary}</span>
      </button>
      {open && (
        <div className="recap-recall__body">
          {turns.map((turn) => (
            <div key={`turn:${turn.turnSeq}`} className="recap-recall__turn">
              <div className="recap-recall__turn-head">
                {t("history.recallStripTurn", { turn: String(turn.turnSeq) })}
                {turn.omitted !== undefined && turn.omitted > 0
                  ? ` · ${t("history.recallStripOmitted", { n: String(turn.omitted) })}`
                  : ""}
              </div>
              {(turn.hits ?? []).map((hit) => {
                // The record's own label wins: it is the fact's name as of that turn
                // and it is there even when no controller is loaded to list facts.
                const own = (hit.title ?? "").trim() || (hit.name ?? "").trim();
                const slug = (hit.name ?? "").trim();
                const known =
                  own !== ""
                    ? { label: own, hint: slug !== "" && slug !== own ? slug : undefined }
                    : labels.get(hit.id);
                return (
                  <div key={`hit:${hit.id}`} className="recap-recall__hit">
                    {known !== undefined && (
                      <span className="recap-recall__name" title={known.hint}>
                        {known.label}
                      </span>
                    )}
                    <span className="recap-recall__fingerprint" title={hit.id}>
                      {hit.id}
                    </span>
                    <span className="recap-recall__meta">
                      {`r${hit.revision ?? 1} · ${(hit.score ?? 0).toFixed(2)} · ${t(recallHitStateKey(recallHitState(hit)))}`}
                    </span>
                  </div>
                );
              })}
              {turn.suppressed !== undefined && turn.suppressed !== "" && (
                <div className="recap-recall__meta">
                  {t("history.recallStripSuppressed", { reason: turn.suppressed })}
                </div>
              )}
            </div>
          ))}
          {skills.map((skill) => (
            <div key={`skill:${skill.turnSeq}:${skill.name}`} className="recap-recall__hit">
              <span className="recap-recall__fingerprint">{skill.name}</span>
              <span className="recap-recall__meta">
                {t("history.recallStripSkill", {
                  turn: String(skill.turnSeq),
                  contentHash: skill.contentHash ?? "",
                  catalogDigest: skill.catalogDigest ?? "",
                })}
              </span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
