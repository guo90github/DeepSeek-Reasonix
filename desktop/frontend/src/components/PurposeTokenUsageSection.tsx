// PurposeTokenUsageSection lists the range's token split by usage_source: what
// each request was for (executor, title, compaction, session-recap, ...). The
// panel's other dimensions are by model and by entry point, so this is the one
// that answers "how much did session recaps cost".
import { formatUsageTokens as formatTokens } from "../lib/usageStatsFormat";
import type { PurposeTokenUsage } from "../lib/types";

export function PurposeTokenUsageSection({ purposes, t }: {
  purposes?: PurposeTokenUsage[];
  t: (key: "settings.stats.purpose") => string;
}) {
  if (!purposes || purposes.length === 0) return null;
  return (
    <section className="usage-stats__section">
      <h3 className="usage-stats__section-title">{t("settings.stats.purpose")}</h3>
      <ul className="usage-stats__model-list">
        {purposes.map((p) => (
          <li key={p.purpose} className="usage-stats__model-row">
            <span className="usage-stats__model-name">{p.purpose}</span>
            <span className="usage-stats__model-tokens">{formatTokens(p.tokens)}</span>
            <span className="usage-stats__model-pct">{percentLabel(p.percent)}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}

function percentLabel(p: number): string {
  return (Math.round(p * 10) / 10).toFixed(1) + "%";
}
