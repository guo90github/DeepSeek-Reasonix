import { useCallback, useEffect, useState } from "react";

import type { RecallUsage } from "../generated/desktopContract.generated";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import "./RecallUsageList.css";

/**
 * 第十六 全局级 (docs/50 §2.2): which fact memory actually used across this
 * workspace's sessions, how often, and whether the revision it used is still the
 * current one. An old conclusion quoted again is the failure this surfaces. The
 * record carries ids, counters and each fact's own label — never its body.
 */
export function RecallUsageList({
  tabId,
  load: providedLoad,
}: {
  tabId: string;
  /** A caller-supplied record, so a test can render a fixture without the bridge. */
  load?: () => Promise<RecallUsage>;
}) {
  const t = useT();
  const [usage, setUsage] = useState<RecallUsage | null>(null);

  const load = useCallback(async () => {
    if (providedLoad) {
      try {
        return await providedLoad();
      } catch {
        return null;
      }
    }
    if (!tabId) return null;
    try {
      return await app.RecallUsageForTab(tabId);
    } catch {
      return null;
    }
  }, [providedLoad, tabId]);

  useEffect(() => {
    void load().then(setUsage);
  }, [load]);

  if (usage === null || usage.available !== true) return null;
  const facts = usage.facts ?? [];
  if (facts.length === 0) return null;

  return (
    <div className="mem-usage">
      <div className="mem-section__row">
        <div>
          <div className="mem-section__title">{t("memory.usageTitle")}</div>
          <div className="mem-note">{t("memory.usageHint", { sessions: String(usage.sessions ?? 0) })}</div>
        </div>
        <span className="mem-count">{facts.length}</span>
      </div>
      {facts.map((fact) => (
        <div className="mem-usage__row" key={fact.id}>
          <span className="mem-usage__label" title={fact.id}>
            {(fact.description ?? "").trim() || (fact.name ?? "").trim() || fact.id}
          </span>
          <span className="mem-usage__meta">
            {t("memory.usageUses", {
              uses: String(fact.uses ?? 0),
              injected: String(fact.injected ?? 0),
              dropped: String(fact.dropped ?? 0),
            })}
            {fact.lastTurnSeq ? ` · ${t("history.recallStripTurn", { turn: String(fact.lastTurnSeq) })}` : ""}
          </span>
          {fact.superseded === true && (
            <span className="chip chip--warn">
              {t("memory.usageSuperseded", {
                used: String(fact.usedRevision ?? 0),
                current: String(fact.currentRevision ?? 0),
              })}
            </span>
          )}
          {fact.live !== true && <span className="chip">{t("memory.usageGone")}</span>}
        </div>
      ))}
      {usage.truncated === true && <div className="mem-note">{t("memory.usageTruncated")}</div>}
    </div>
  );
}
