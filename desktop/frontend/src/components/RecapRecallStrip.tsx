import { useCallback, useEffect, useState } from "react";

import type { RecallRecordView } from "../generated/desktopContract.generated";
import { useT } from "../lib/i18n";
import "./RecapRecallStrip.css";

/**
 * 第十六: what a session's turns asked memory for and which skill they ran. The
 * record is content-free by construction — identifiers, counters and digests —
 * so this panel can show it without exposing any memory or skill text.
 *
 * It is read-only: folding is local, and nothing here reaches a prompt.
 */
export function RecapRecallStrip({
  sessionPath = "",
  recallRecord,
  load: providedLoad,
  record: controlled,
}: {
  sessionPath?: string;
  recallRecord?: (sessionPath: string) => Promise<RecallRecordView>;
  load?: () => Promise<RecallRecordView>;
  /** When the caller already holds the record — a section that must hide itself
   *  when there is none — the strip renders that instead of asking again. */
  record?: RecallRecordView | null;
}) {
  const t = useT();
  const [fetched, setFetched] = useState<RecallRecordView | null>(null);
  const [open, setOpen] = useState(false);

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
  let injected = 0;
  let dropped = 0;
  for (const turn of turns) {
    for (const hit of turn.hits ?? []) {
      if (hit.injected === true) injected += 1;
      else dropped += 1;
    }
  }
  const summary = t("history.recallStripSummary", {
    injected: String(injected),
    dropped: String(dropped),
    skills: String(skills.length),
  });

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
              {(turn.hits ?? []).map((hit) => (
                <div key={`hit:${hit.id}`} className="recap-recall__hit">
                  <span className="recap-recall__fingerprint">{hit.id}</span>
                  <span className="recap-recall__meta">
                    {`r${hit.revision ?? 1} · ${(hit.score ?? 0).toFixed(2)} · ${t(
                      hit.injected === true ? "history.recallStripInjected" : "history.recallStripDropped",
                    )}`}
                  </span>
                </div>
              ))}
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
