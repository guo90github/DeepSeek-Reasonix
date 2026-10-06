// 召回记录 module of the bottom-band card ("面板"). The card's rule is one
// ellipsised line per row plus ONE shared detail modal, so the record renders as
// rows that open the whole entry: the recap page's in-page fingerprint strip is
// too tall and too wide for a band this narrow, and nothing in it was clickable.
//
// The record carries ids, counters and the label each hit had when it was written
// — never a fact's body — so a row is named from the record's own label, then
// from the tab's live fact list, and only then from the bare id.

import { useCallback, useEffect, useMemo, useState } from "react";
import { ChevronDown } from "lucide-react";

import type { MemoryFact, RecallRecordView } from "../generated/desktopContract.generated";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { buildRecallLabels, countRecallHitStates, recallHitState, recallHitStateKey } from "../lib/recallLabels";
import { FooterPanelSection, type FooterPanelModuleProps } from "./FooterPanel";
import { PanelRowButton, type PanelDetail } from "./FooterPanelDetail";

export const FOOTER_RECALL_INITIAL = 6;
export const FOOTER_RECALL_PAGE = 6;

type RecallTurn = NonNullable<RecallRecordView["turns"]>[number];
type RecallHit = NonNullable<RecallTurn["hits"]>[number];

type RecallRow =
  | {
      kind: "hit";
      key: string;
      turnSeq: number;
      label: string;
      /** Where the label came from (the slug or the live description), as a tooltip. */
      hint: string | null;
      fingerprint: string | null;
      state: string;
      detail: PanelDetail;
    }
  | { kind: "skill"; key: string; turnSeq: number; label: string; detail: PanelDetail }
  | { kind: "note"; key: string; text: string };

function factIndex(facts: readonly MemoryFact[]): ReadonlyMap<string, MemoryFact> {
  const index = new Map<string, MemoryFact>();
  for (const fact of facts) {
    if (fact.id) index.set(fact.id, fact);
    if (fact.name) index.set(fact.name, fact);
  }
  return index;
}

export function FooterRecallModule({ tabId }: FooterPanelModuleProps) {
  const t = useT();
  const [record, setRecord] = useState<RecallRecordView | null>(null);
  const [facts, setFacts] = useState<readonly MemoryFact[]>([]);
  const [visibleCount, setVisibleCount] = useState(FOOTER_RECALL_INITIAL);

  useEffect(() => {
    if (!tabId) {
      setRecord(null);
      setFacts([]);
      return;
    }
    let cancelled = false;
    // Deferred calls: a host (or a test stub) without a command must leave the
    // module hidden, not fail the whole card.
    void Promise.resolve()
      .then(() => app.RecallRecordForTab(tabId))
      .then((next) => {
        if (!cancelled) setRecord(next);
      })
      .catch(() => {
        if (!cancelled) setRecord(null);
      });
    void Promise.resolve()
      .then(() => app.MemoryForTab(tabId))
      .then((view) => {
        if (!cancelled) setFacts(view.facts);
      })
      .catch(() => {
        if (!cancelled) setFacts([]);
      });
    return () => {
      cancelled = true;
    };
  }, [tabId]);

  // A new session starts its own page window.
  useEffect(() => {
    setVisibleCount(FOOTER_RECALL_INITIAL);
  }, [tabId]);

  const labels = useMemo(() => buildRecallLabels(facts), [facts]);
  const liveFacts = useMemo(() => factIndex(facts), [facts]);
  const togglePage = useCallback((total: number) => {
    setVisibleCount((current) => (current >= total ? FOOTER_RECALL_INITIAL : Math.min(current + FOOTER_RECALL_PAGE, total)));
  }, []);

  if (!tabId || record === null || record.available !== true) return null;

  const hitRow = (turn: RecallTurn, hit: RecallHit): RecallRow => {
    const own = (hit.title ?? "").trim() || (hit.name ?? "").trim();
    const resolved = labels.get(hit.id);
    const label = own !== "" ? own : (resolved?.label ?? hit.id);
    const live = liveFacts.get(hit.id);
    const liveBody = live ? [live.description, live.body].filter(Boolean).join("\n\n").trim() : "";
    const slug = (hit.name ?? "").trim();
    // The record's own label wins; the live fact list only supplies this row's
    // tooltip and the body the modal can still resolve today.
    const hint = own === "" ? (resolved?.hint ?? null) : slug === "" || slug === own ? (resolved?.hint ?? null) : slug;
    return {
      kind: "hit",
      key: `hit:${turn.turnSeq}:${hit.id}`,
      turnSeq: turn.turnSeq,
      label,
      fingerprint: label === hit.id ? null : hit.id,
      state: t(recallHitStateKey(recallHitState(hit))),
      detail: {
        title: label,
        meta: [
          t("history.recallStripTurn", { turn: String(turn.turnSeq) }),
          hit.id,
          `r${hit.revision ?? 1}`,
          (hit.score ?? 0).toFixed(2),
          t(recallHitStateKey(recallHitState(hit))),
          ...(liveBody === "" ? [] : [t("footerPanel.recallLiveFact")]),
        ],
        body: liveBody === "" ? hit.id : liveBody,
        bodyStyle: liveBody === "" ? "mono" : "prose",
      },
      hint,
    };
  };

  const rows: RecallRow[] = [];
  const counts = countRecallHitStates(record.turns ?? []);
  const trimmed = record.droppedTurns ?? 0;
  let summary = t("history.recallStripSummary", {
    injected: String(counts.injected),
    dropped: String(counts.dropped),
    skills: String((record.skills ?? []).length),
  });
  if (counts.unrecorded > 0) summary += ` · ${t("history.recallStripUnrecordedCount", { n: String(counts.unrecorded) })}`;
  if (trimmed > 0) summary += ` · ${t("history.recallStripTrimmed", { n: String(trimmed) })}`;
  for (const turn of record.turns ?? []) {
    for (const hit of turn.hits ?? []) {
      rows.push(hitRow(turn, hit));
    }
    const turnLabel = t("history.recallStripTurn", { turn: String(turn.turnSeq) });
    const notes = [
      turn.omitted !== undefined && turn.omitted > 0 ? t("history.recallStripOmitted", { n: String(turn.omitted) }) : "",
      turn.suppressed !== undefined && turn.suppressed !== "" ? t("history.recallStripSuppressed", { reason: turn.suppressed }) : "",
    ].filter((part) => part !== "");
    if (notes.length > 0) rows.push({ kind: "note", key: `note:${turn.turnSeq}`, text: [turnLabel, ...notes].join(" · ") });
  }
  const skills = record.skills ?? [];
  for (const skill of skills) {
    rows.push({
      kind: "skill",
      key: `skill:${skill.turnSeq}:${skill.name}`,
      turnSeq: skill.turnSeq,
      label: skill.name,
      detail: {
        title: skill.name,
        meta: [
          t("history.recallStripSkill", {
            turn: String(skill.turnSeq),
            contentHash: skill.contentHash ?? "",
            catalogDigest: skill.catalogDigest ?? "",
          }),
        ],
      },
    });
  }

  const shown = rows.slice(0, visibleCount);
  const remaining = rows.length - shown.length;

  return (
    <FooterPanelSection title="memory.activity">
      <div className="footer-recall">
        <div className="footer-panel__bar">
          <span>{summary}</span>
        </div>
        <ul className="footer-recall__list">
          {shown.map((row) =>
            row.kind === "note" ? (
              <li key={row.key}>
                <p className="footer-panel__note">{row.text}</p>
              </li>
            ) : (
              <li key={row.key}>
                <PanelRowButton className="footer-recall__row" detail={row.detail}>
                  <span className="footer-recall__turn">{t("history.recallStripTurn", { turn: String(row.turnSeq) })}</span>
                  <span className="footer-recall__name" title={row.kind === "hit" ? (row.hint ?? row.label) : row.label}>
                    {row.label}
                  </span>
                  {row.kind === "hit" && row.fingerprint !== null ? (
                    <span className="footer-recall__id" title={row.fingerprint}>
                      {row.fingerprint}
                    </span>
                  ) : null}
                  <span className="footer-recall__state">
                    {row.kind === "hit" ? row.state : t("footerPanel.recallSkills")}
                  </span>
                </PanelRowButton>
              </li>
            ),
          )}
        </ul>
        {rows.length > FOOTER_RECALL_INITIAL ? (
          <button type="button" className="footer-panel__more" onClick={() => togglePage(rows.length)}>
            <ChevronDown
              className={`footer-panel__chevron${remaining > 0 ? " footer-panel__chevron--closed" : ""}`}
              size={12}
              aria-hidden="true"
            />
            <span>
              {remaining > 0
                ? t("footerPanel.showMore", { n: String(Math.min(FOOTER_RECALL_PAGE, remaining)) })
                : t("footerPanel.showLess")}
            </span>
          </button>
        ) : null}
      </div>
    </FooterPanelSection>
  );
}
