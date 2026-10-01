import { useMemo } from "react";

import {
  RECAP_HEATMAP_UNLABELED,
  RECAP_HEATMAP_WINDOW_DAYS,
  buildRecapHeatmap,
  type RecapHeatmapInput,
} from "../lib/recapHeatmap";
import { useT } from "../lib/i18n";
import "./RecapHeatmap.css";

/**
 * 第十四: a calendar heatmap over the recap page's own data — insight occurrence
 * counts per project and day, plus recaps by the day they were generated. It is a
 * read-only view: selecting a cell only filters the page's list, and nothing here
 * reaches a model or a prompt.
 */
export function RecapHeatmap({
  insights,
  recaps,
  now,
  selectedDay,
  onSelectDay,
}: {
  insights: RecapHeatmapInput["insights"];
  recaps: RecapHeatmapInput["recaps"];
  now?: string | Date;
  /** The selected day, or "" for no filter. */
  selectedDay?: string;
  onSelectDay?: (day: string) => void;
}) {
  const t = useT();
  const heatmap = useMemo(
    () => buildRecapHeatmap({ insights, recaps, now: now ?? new Date() }),
    [insights, recaps, now],
  );
  if (heatmap.rows.length === 0) return null;

  const active = String(selectedDay ?? "");
  return (
    <section className="recap-heatmap" aria-label={t("history.recapHeatmapTitle")}>
      <div className="recap-heatmap__head">
        <span className="recap-heatmap__title">{t("history.recapHeatmapTitle")}</span>
        {/* The window is bounded on purpose (30 days), so an empty column means
            "outside the window", not "nothing happened" — say so. */}
        <span className="recap-heatmap__window">
          {t("history.recapHeatmapWindow", {
            from: heatmap.from,
            to: heatmap.to,
            days: String(RECAP_HEATMAP_WINDOW_DAYS),
          })}
        </span>
      </div>
      <div className="recap-heatmap__rows">
        {heatmap.rows.map((row) => (
          <div key={row.project} className="recap-heatmap__row">
            <span className="recap-heatmap__project" title={row.project}>
              {row.project === RECAP_HEATMAP_UNLABELED ? t("history.recapHeatmapUnlabelled") : row.project}
            </span>
            <div className="recap-heatmap__cells" role="group" aria-label={row.project}>
              {row.cells.map((cell) => (
                <button
                  key={cell.day}
                  type="button"
                  className={`recap-heatmap__cell${cell.count > 0 ? " recap-heatmap__cell--filled" : ""}${active === cell.day ? " recap-heatmap__cell--active" : ""}`}
                  style={cell.count > 0 ? { "--recap-heatmap-weight": String(cell.count / heatmap.max) } as React.CSSProperties : undefined}
                  aria-pressed={active === cell.day}
                  aria-label={t("history.recapHeatmapCell", { day: cell.day, n: String(cell.count) })}
                  title={t("history.recapHeatmapCell", { day: cell.day, n: String(cell.count) })}
                  onClick={() => onSelectDay?.(active === cell.day ? "" : cell.day)}
                >
                  {cell.count > 0 ? cell.count : ""}
                </button>
              ))}
            </div>
            <span className="recap-heatmap__total">{row.total}</span>
          </div>
        ))}
      </div>
    </section>
  );
}
