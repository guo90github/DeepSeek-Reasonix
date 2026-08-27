// ContextPanel shows the active tab's context gauge and token usage.
// All visible text is routed through the i18n dictionary.
import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { asArray } from "../lib/array";
import { app } from "../lib/bridge";
import { contextWindowPercentages } from "../lib/contextWindow";
import { useI18n, type Locale, type Translator } from "../lib/i18n";
import { formatMoneyLocalized } from "../lib/money";
import { formatTokens } from "../lib/format";
import { normalizeRateBand, rateBandLabel, type DisplayRateBand } from "../lib/costRateBand";
import type { DictKey } from "../locales/en";
import type { BalanceInfo, ContextInfo, ContextPanelInfo, UsageSourceStats, WireUsage } from "../lib/types";
interface ContextPanelProps {
  tabId?: string;
  context?: ContextInfo;
  usage?: WireUsage;
  sessionTokens?: number;
  sessionCost?: number;
  sessionCurrency?: string;
  sessionTurns?: number;
  turnTokens?: number;
  turnCost?: number;
  turnRateBand?: string;
  balance?: BalanceInfo;
  sessionGen?: number;
  refreshKey?: number;
  // Monotonic counter bumped by EVERY usage event (executor and subagent).
  // The executor-gated `usage` prop freezes during sub-agent runs, which used
  // to pin 会话指标/用量分析 for minutes; this keeps the snapshot ticking.
  usageSeq?: number;
}

function fmtDuration(ms: number, t: Translator): string {
  if (ms <= 0) return "-";
  const totalSeconds = Math.max(1, Math.round(ms / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  if (minutes <= 0) return t("context.durationSeconds", { seconds });
  return t("context.durationMinutesSeconds", { minutes, seconds });
}

interface MetricTokenDisplay {
  display: string;
  exact: string;
}

function numberLocale(locale: Locale | string): string {
  if (locale === "zh") return "zh-CN";
  if (locale === "zh-TW") return "zh-TW";
  return "en";
}

export function formatMetricTokens(tokens: number | undefined, locale: Locale | string): MetricTokenDisplay {
  if (typeof tokens !== "number" || tokens <= 0) {
    return { display: "-", exact: "-" };
  }
  const tag = numberLocale(locale);
  const exact = tokens.toLocaleString(tag);
  return { display: exact, exact };
}

export function formatCacheHitRate(hitTokens: number, missTokens: number): string {
  const denom = hitTokens + missTokens;
  if (denom <= 0) return "-";
  return `${((hitTokens / denom) * 100).toFixed(2)}%`;
}

type MetricTone = "accent" | "good" | "notice" | "warn";
type ContextUsageRefreshFields = Pick<
  WireUsage,
  "totalTokens" | "promptTokens" | "completionTokens" | "reasoningTokens" | "sessionCacheHitTokens" | "sessionCacheMissTokens"
>;

export function contextUsageRefreshKey(usage?: ContextUsageRefreshFields): string {
  if (!usage) return "";
  return [
    usage.totalTokens ?? 0,
    usage.promptTokens ?? 0,
    usage.completionTokens ?? 0,
    usage.reasoningTokens ?? 0,
    usage.sessionCacheHitTokens ?? 0,
    usage.sessionCacheMissTokens ?? 0,
  ].join(":");
}

export function cacheHitTone(hitTokens: number, missTokens: number): MetricTone | undefined {
  const denom = hitTokens + missTokens;
  if (denom <= 0) return undefined;
  const pct = (hitTokens / denom) * 100;
  if (pct >= 80) return "good";
  if (pct >= 60) return "notice";
  return "warn";
}

export function formatSharePercent(value: number, total: number): string {
  if (total <= 0 || value <= 0) return "-";
  const pct = (value / total) * 100;
  if (pct > 0 && pct < 1) return "<1%";
  return `${Math.round(pct)}%`;
}

interface ContextWindowStatus {
  tone: "good" | "notice" | "warn";
  key: DictKey;
}

export function contextCostDisplay({
  info,
  sessionCost,
  sessionCurrency,
  usage,
}: {
  info?: Pick<ContextPanelInfo, "sessionCost" | "sessionCurrency" | "sessionCostUsd" | "sessionCostComplete" | "sessionCostEstimated" | "sessionBillingMode" | "sessionCostQuote"> | null;
  sessionCost?: number;
  sessionCurrency?: string;
  usage?: Pick<WireUsage, "cost" | "costUsd" | "currency" | "currencyCode" | "costQuote">;
}): {
  amount: number;
  currency?: string;
  estimated?: boolean;
  complete?: boolean;
  billingMode?: string;
  labelKind?: "estimated" | "payg_equivalent" | "fallback" | "bucketed" | "unavailable";
} {
  // Prefer structured session quote, then per-usage quote.
  const quote = info?.sessionCostQuote || usage?.costQuote;
  if (quote?.displayStatus === "bucketed" || quote?.aggregateMode === "currency_buckets") {
    return {
      amount: 0,
      currency: undefined,
      estimated: true,
      complete: false,
      labelKind: "bucketed",
    };
  }
  const fallbackOriginal = quote?.displayStatus === "fallback_original";
  if (!fallbackOriginal && (info?.sessionCostComplete === false || quote?.displayStatus === "unavailable" || quote?.costComplete === false)) {
    return {
      amount: 0,
      currency: info?.sessionCurrency || sessionCurrency || usage?.currencyCode || usage?.currency,
      estimated: true,
      complete: false,
      labelKind: "unavailable",
    };
  }
  const selected = quote?.selected;
  if (selected?.amount) {
    const n = Number(selected.amount);
    if (Number.isFinite(n) && n > 0) {
      const mode = quote?.billingMode || info?.sessionBillingMode;
      return {
        amount: n,
        currency: selected.currency || usage?.currencyCode || usage?.currency || info?.sessionCurrency,
        estimated: quote?.estimated !== false,
        complete: quote?.displayComplete !== false,
        billingMode: mode,
        labelKind: fallbackOriginal ? "fallback" : mode === "subscription_equivalent" ? "payg_equivalent" : "estimated",
      };
    }
  }
  // Session-scoped scalar fallbacks (legacy telemetry).
  if (info?.sessionCost && info.sessionCost > 0) {
    return {
      amount: info.sessionCost,
      currency: info.sessionCurrency || sessionCurrency || usage?.currencyCode || usage?.currency,
      estimated: true,
      complete: true,
      labelKind: "estimated",
    };
  }
  if (sessionCost && sessionCost > 0) {
    return {
      amount: sessionCost,
      currency: sessionCurrency || info?.sessionCurrency || usage?.currencyCode || usage?.currency,
      estimated: true,
      complete: true,
      labelKind: "estimated",
    };
  }
  if (info?.sessionCostUsd && info.sessionCostUsd > 0) {
    return {
      amount: info.sessionCostUsd,
      currency: info.sessionCurrency || sessionCurrency || usage?.currencyCode || usage?.currency,
      estimated: true,
      complete: true,
      labelKind: "estimated",
    };
  }
  return {
    amount: 0,
    currency: info?.sessionCurrency || sessionCurrency || usage?.currencyCode || usage?.currency,
    estimated: true,
    complete: false,
    labelKind: "unavailable",
  };
}

// contextSessionCache picks the session-cumulative cache hit/miss pair for the
// panel's session average. The shared ContextInfo is refreshed after every
// usage event and also drives StatusBar, so prefer it over the panel's
// independently throttled snapshot. Panel telemetry remains the all-sources
// fallback for callers without live context; executor-only wire counters only
// bridge the pre-refresh gap. The pair always comes from one source so the
// computed rate never mixes scopes.
export function contextSessionCache(
  info?: Pick<ContextPanelInfo, "sessionCacheHitTokens" | "sessionCacheMissTokens"> | null,
  context?: Pick<ContextInfo, "cacheHitTokens" | "cacheMissTokens">,
  usage?: Pick<WireUsage, "sessionCacheHitTokens" | "sessionCacheMissTokens">,
): { hit: number; miss: number } {
  const ctxHit = context?.cacheHitTokens ?? 0;
  const ctxMiss = context?.cacheMissTokens ?? 0;
  if (ctxHit + ctxMiss > 0) return { hit: ctxHit, miss: ctxMiss };
  const infoHit = info?.sessionCacheHitTokens ?? 0;
  const infoMiss = info?.sessionCacheMissTokens ?? 0;
  if (infoHit + infoMiss > 0) return { hit: infoHit, miss: infoMiss };
  return { hit: usage?.sessionCacheHitTokens ?? 0, miss: usage?.sessionCacheMissTokens ?? 0 };
}

interface ContextBreakdown {
  promptTokens: number;
  completionTokens: number;
  reasoningTokens: number;
  otherTokens: number;
  promptPct: number;
  completionPct: number;
  reasoningPct: number;
  otherPct: number;
}

function nonNegativeTokenCount(value: number): number {
  return Number.isFinite(value) ? Math.max(0, value) : 0;
}

/** Prefer Context* (latest attempt) over billable aggregates for turn panels. */
export function liveTurnUsageBreakdown(
  usage?: WireUsage | null,
  info?: Pick<ContextPanelInfo, "promptTokens" | "completionTokens" | "reasoningTokens"> | null,
): { promptTokens: number; completionTokens: number; reasoningTokens: number } {
  if (usage) {
    const hasContext =
      (usage.contextPromptTokens ?? 0) > 0 || (usage.contextCompletionTokens ?? 0) > 0;
    if (hasContext) {
      return {
        promptTokens: usage.contextPromptTokens ?? 0,
        completionTokens: usage.contextCompletionTokens ?? 0,
        reasoningTokens: usage.contextReasoningTokens ?? 0,
      };
    }
    return {
      promptTokens: usage.promptTokens ?? 0,
      completionTokens: usage.completionTokens ?? 0,
      reasoningTokens: usage.reasoningTokens ?? 0,
    };
  }
  return {
    promptTokens: info?.promptTokens ?? 0,
    completionTokens: info?.completionTokens ?? 0,
    reasoningTokens: info?.reasoningTokens ?? 0,
  };
}

export function contextBreakdown(
  usedTokens: number,
  windowTokens: number,
  promptTokens: number,
  completionTokens: number,
  reasoningTokens: number,
): ContextBreakdown {
  const used = nonNegativeTokenCount(usedTokens);
  const window = nonNegativeTokenCount(windowTokens);
  let prompt = nonNegativeTokenCount(promptTokens);
  let reasoning = Math.min(nonNegativeTokenCount(reasoningTokens), nonNegativeTokenCount(completionTokens));
  let completion = Math.max(0, nonNegativeTokenCount(completionTokens) - reasoning);
  const known = prompt + completion + reasoning;

  if (known > used && known > 0) {
    const scale = used / known;
    prompt *= scale;
    completion *= scale;
    reasoning *= scale;
  }

  const normalizedKnown = Math.min(used, prompt + completion + reasoning);
  const other = Math.max(0, used - normalizedKnown);
  const hasWindow = window > 0;
  const promptPct = hasWindow ? Math.min(100, (prompt / window) * 100) : 0;
  const completionPct = hasWindow ? Math.min(100, ((prompt + completion) / window) * 100) : 0;
  const reasoningPct = hasWindow ? Math.min(100, ((prompt + completion + reasoning) / window) * 100) : 0;
  const otherPct = hasWindow ? Math.min(100, (used / window) * 100) : 0;

  return {
    promptTokens: Math.round(prompt),
    completionTokens: Math.round(completion),
    reasoningTokens: Math.round(reasoning),
    otherTokens: Math.round(other),
    promptPct,
    completionPct,
    reasoningPct,
    otherPct,
  };
}

export function contextWindowStatus(rawUsagePct: number, compactPct: number): ContextWindowStatus {
  if (rawUsagePct > 100) return { tone: "warn", key: "context.windowStatusOverLimit" };
  const usagePct = Math.min(100, Math.max(0, rawUsagePct));
  if (usagePct >= 90) return { tone: "warn", key: "context.windowStatusNearLimit" };
  if (compactPct > 0 && usagePct >= compactPct) return { tone: "warn", key: "context.windowStatusPastCompact" };
  if (compactPct > 0 && usagePct >= Math.max(0, compactPct - 10)) return { tone: "notice", key: "context.windowStatusWatch" };
  return { tone: "good", key: "context.windowStatusHealthy" };
}

const SOURCE_ORDER = ["executor", "planner", "subagent", "compaction", "classifier", "title"];

function sourceCost(stats: UsageSourceStats): number {
  return stats.sessionCost && stats.sessionCost > 0 ? stats.sessionCost : stats.sessionCostUsd ?? 0;
}

export interface ContextSourceRow {
  source: string;
  label: string;
  promptTokens: number;
  completionTokens: number;
  cacheHitTokens: number;
  cacheMissTokens: number;
  totalTokens: number;
  cost: number;
  currency?: string;
  requests: number;
  estimated: boolean;
}

export function contextSourceRows(info: ContextPanelInfo | null, sessionCurrency?: string): ContextSourceRow[] {
  const entries = Object.entries(info?.sources ?? {});
  if (entries.length === 0) return [];
  return entries
    .filter(([, stats]) =>
      (stats.requestCount ?? 0) > 0 ||
      (stats.promptTokens ?? 0) > 0 ||
      (stats.completionTokens ?? 0) > 0 ||
      (stats.cacheHitTokens ?? 0) > 0 ||
      (stats.cacheMissTokens ?? 0) > 0 ||
      sourceCost(stats) > 0
    )
    .sort(([a], [b]) => {
      const ia = SOURCE_ORDER.indexOf(a);
      const ib = SOURCE_ORDER.indexOf(b);
      if (ia >= 0 || ib >= 0) return (ia >= 0 ? ia : SOURCE_ORDER.length) - (ib >= 0 ? ib : SOURCE_ORDER.length);
      return a.localeCompare(b);
    })
    .map(([source, stats]) => ({
      source,
      label: source,
      promptTokens: stats.promptTokens ?? 0,
      completionTokens: stats.completionTokens ?? 0,
      cacheHitTokens: stats.cacheHitTokens ?? 0,
      cacheMissTokens: stats.cacheMissTokens ?? 0,
      totalTokens: stats.totalTokens ?? 0,
      cost: sourceCost(stats),
      currency: stats.sessionCurrency || sessionCurrency || info?.sessionCurrency,
      requests: stats.requestCount ?? 0,
      estimated: stats.estimated === true,
    }));
}

export function ContextPanel({
  tabId,
  context,
  usage,
  sessionTokens,
  sessionCost,
  sessionCurrency,
  sessionGen,
  refreshKey,
  usageSeq,
}: ContextPanelProps) {
  const { locale, t } = useI18n();
  const [info, setInfo] = useState<ContextPanelInfo | null>(null);
  const refreshSeq = useRef(0);
  const lastRefreshTime = useRef(0);
  const usageRefreshKey = contextUsageRefreshKey(usage);

  const refresh = useCallback(async () => {
    if (!tabId) return;
    const seq = ++refreshSeq.current;
    try {
      const next = await app.ContextPanel(tabId);
      if (refreshSeq.current === seq) {
        setInfo(next);
      }
    } catch {
      /* bridge unavailable */
    }
  }, [tabId]);

  useEffect(() => {
    refreshSeq.current += 1;
    setInfo(null);
    void refresh();
  }, [refresh, sessionGen]);

  useEffect(() => {
    void refresh();
  }, [refresh, refreshKey]);

  // Refresh the panel snapshot while usage events stream — from any source:
  // usageSeq covers sub-agent/title requests the executor-gated usage prop
  // never reflects, and usageRefreshKey keeps ticking for providers whose
  // events lack a seq. Throttled to once per second.
  useEffect(() => {
    if (!usageRefreshKey && !usageSeq) return;
    const now = Date.now();
    if (now - lastRefreshTime.current >= 1000) {
      lastRefreshTime.current = now;
      void refresh();
    }
  }, [usageRefreshKey, usageSeq, refresh]);

  const usedTokens = context?.used && context.used > 0 ? context.used : info?.usedTokens ?? 0;
  const windowTokens = context?.window && context.window > 0 ? context.window : info?.windowTokens ?? 0;
  // Prefer live usage props (updated in real-time by the reducer during streaming)
  // over the async-fetched info snapshot (only refreshed on turn_done). Multi-
  // attempt stream recovery reports billable aggregates on prompt/completion
  // and latest-attempt shape on Context* — use the latter for turn breakdown.
  const turnBreakdown = liveTurnUsageBreakdown(usage, info);
  const promptTokens = turnBreakdown.promptTokens;
  const completionTokens = turnBreakdown.completionTokens;
  const totalTokens = info?.totalTokens && info.totalTokens > 0
    ? info.totalTokens
    : sessionTokens && sessionTokens > 0
      ? sessionTokens
      : usage?.totalTokens && usage.totalTokens > 0
        ? usage.totalTokens
        : promptTokens + completionTokens;

  // Session-cumulative cache tokens for the top summary: all-sources telemetry
  // first (matching the session cost and per-source rows in this panel — the
  // wire session counters are executor-only), with the live counters bridging
  // only a fresh session's first turn before the telemetry refresh. Hit and
  // miss come as a pair from one source so the rate cannot mix scopes.
  const { hit: sessionCacheHit, miss: sessionCacheMiss } = contextSessionCache(info, context, usage);
  const totalTokensMetric = formatMetricTokens(totalTokens, locale);
  const cost = contextCostDisplay({ info, sessionCost, sessionCurrency, usage });
  const readFiles = asArray(info?.readFiles);
  const changedFiles = asArray(info?.changedFiles);

  const usagePercentages = contextWindowPercentages(usedTokens, windowTokens);
  const rawUsagePct = usagePercentages.raw;
  const usagePct = usagePercentages.display;
  const compactRatio = context?.compactRatio && context.compactRatio > 0 ? context.compactRatio : 0.80;
  const compactPct = Math.round(compactRatio * 100);
  const reportedTriggerTokens = context?.maintenance?.triggerTokens ?? 0;
  const triggerTokens = reportedTriggerTokens > 0
    ? reportedTriggerTokens
    : windowTokens > 0
      ? Math.round(windowTokens * compactRatio)
      : 0;
  const compactTokens = triggerTokens > 0 ? triggerTokens : (windowTokens > 0 ? Math.round(windowTokens * compactRatio) : 0);
  const tokensUntilCompact = compactTokens > usedTokens ? compactTokens - usedTokens : 0;
  const eventTimes = [
    ...readFiles.map((file) => file.time),
    ...changedFiles.map((file) => file.latestTime ?? 0),
  ].filter((time) => time > 0);
  const derivedElapsed = eventTimes.length > 1 ? Math.max(...eventTimes) - Math.min(...eventTimes) : 0;
  const elapsed = info?.elapsedMs && info.elapsedMs > 0 ? info.elapsedMs : derivedElapsed;
  const derivedRequestCount = Math.max(readFiles.length + changedFiles.length, 0);
  const requestCount = info?.requestCount && info.requestCount > 0 ? info.requestCount : derivedRequestCount;
  const windowStatus = contextWindowStatus(rawUsagePct, compactPct);
  const sessionEstimated = info?.sessionEstimated === true || context?.estimated === true;
  const markEstimated = (value: string, estimated: boolean) => estimated && value !== "-" ? `≈${value}` : value;
  const rawSessionCostLabel = cost.labelKind === "bucketed"
    ? t("context.sessionCostBucketed")
    : cost.labelKind === "unavailable"
      ? t("context.sessionCostUnavailable")
      : cost.labelKind === "fallback"
        ? `${markEstimated(formatMoneyLocalized(cost.amount, cost.currency, { locale, empty: "dash" }), sessionEstimated)} (${t("context.sessionCostFallback")})`
        : markEstimated(formatMoneyLocalized(cost.amount, cost.currency, { locale, empty: "dash" }), sessionEstimated);
  const sessionCostLabel = rawSessionCostLabel;
  const sessionRateBand = normalizeRateBand(info?.sessionCostQuote?.rateBand);
  const sessionRateBandTitle = sessionRateBand ? t("billing.rateBand.tooltip") : undefined;
  const sessionRateBandBadge = sessionRateBand
    ? { label: rateBandLabel(sessionRateBand, t) ?? sessionRateBand, tone: sessionRateBand, title: sessionRateBandTitle }
    : undefined;
  const totalTokensTitle = totalTokensMetric.exact === "-" ? "-" : t("context.tokensValue", { value: totalTokensMetric.exact });
  const usedLabel = formatTokens(usedTokens);
  const windowLabel = formatTokens(windowTokens);
  const compactRemainingLabel = tokensUntilCompact > 0 ? formatTokens(tokensUntilCompact) : "0";
  const compactMarkerPct = Math.max(0, Math.min(100, compactPct));
  const usageMarkerPct = Math.max(6, Math.min(94, usagePct));
  const compactLabelPct = Math.max(6, Math.min(94, compactMarkerPct));
  const usageSummary = t("context.windowUsageSummary", { used: usedLabel, window: windowLabel, pct: rawUsagePct });
  const compactSummary = t("context.windowCompactRemaining", { used: usedLabel, window: windowLabel, tokens: compactRemainingLabel, pct: compactPct });
  return (
    <div className="context-panel">
      <div className="context-panel__body">
        <section className="context-panel__overview">
          <section className="context-panel__usage">
            <SectionHeading title={t("context.windowTitle")} />
            <div className={`context-panel__capacity-card context-panel__capacity-card--${windowStatus.tone}`}>
              <div className="context-panel__capacity-top">
                <span className="context-panel__capacity-status">{t(windowStatus.key)}</span>
                <strong>{usedLabel}/{windowLabel}</strong>
              </div>
              <div className="context-panel__usage-progress context-panel__capacity-meter" aria-label={`${t(windowStatus.key)}. ${usageSummary}. ${compactSummary}`}>
                <div className="context-panel__capacity-scale" aria-hidden="true">
                  <span className="context-panel__capacity-pin context-panel__capacity-pin--used" style={{ left: `${usageMarkerPct}%` }}>{rawUsagePct}%</span>
                  <span className="context-panel__capacity-pin context-panel__capacity-pin--compact" style={{ left: `${compactLabelPct}%` }}>{compactPct}%</span>
                </div>
                <div className="context-panel__progress-track" aria-hidden="true">
                  <span className="context-panel__progress-fill" style={{ width: `${usagePct}%` }} />
                  <span className="context-panel__compact-marker" style={{ left: `${compactMarkerPct}%` }} />
                </div>
              </div>
              <div className="context-panel__capacity-foot">
                <span>{t("context.windowUsedLabel")}</span>
                <span className="context-panel__capacity-remaining">
                  <span>{t("context.windowCompactDistance")}</span>
                  <strong>{compactRemainingLabel}</strong>
                </span>
              </div>
            </div>
          </section>
          <section className="context-panel__section context-panel__session-section">
            <SectionHeading title={t("context.sessionMetrics")} />
            <div className="context-panel__session-metrics">
              <div className="context-panel__summary-rows">
                <MiniStat label={t("status.cacheAvgLabel")} value={formatCacheHitRate(sessionCacheHit, sessionCacheMiss)} tone={cacheHitTone(sessionCacheHit, sessionCacheMiss)} />
                <MiniStat label={t("context.sessionCost")} value={sessionCostLabel} title={sessionRateBandTitle} badge={sessionRateBandBadge} />
                <MiniStat label={t("context.time")} value={fmtDuration(elapsed, t)} />
                <MiniStat label={t("context.requests")} value={requestCount > 0 ? String(requestCount) : "-"} />
                <MiniStat label={t("context.sessionTokensShort")} value={markEstimated(totalTokensMetric.display, sessionEstimated)} title={totalTokensTitle} wide />
              </div>
            </div>
          </section>
        </section>
      </div>

    </div>
  );
}

function SectionHeading({ title, meta, children }: { title: string; meta?: string; children?: ReactNode }) {
  return (
    <header className="context-panel__section-head">
      <h3>{title}</h3>
      {meta && <span>{meta}</span>}
      {children}
    </header>
  );
}

interface MiniStatBadge {
  label: string;
  tone: DisplayRateBand;
  title?: string;
}

function MiniStat({ label, value, title, tone, wide, badge }: { label: string; value: string; title?: string; tone?: MetricTone; wide?: boolean; badge?: MiniStatBadge }) {
  const toneClass = tone ? ` context-panel__mini-stat--${tone}` : "";
  const wideClass = wide ? " context-panel__mini-stat--wide" : "";
  const exactTitle = title && title !== value ? title : undefined;
  const accessibleLabel = badge || exactTitle
    ? `${label}: ${value}${badge ? `, ${badge.label}` : ""}${exactTitle ? `. ${exactTitle}` : ""}`
    : undefined;
  return (
    <div className={`context-panel__mini-stat${toneClass}${wideClass}`} aria-label={accessibleLabel}>
      <div className="context-panel__mini-stat-head">
        <span className="context-panel__mini-stat-label">{label}</span>
        {badge && (
          <span className={`context-panel__rate-band context-panel__rate-band--${badge.tone}`} title={badge.title}>
            {badge.label}
          </span>
        )}
      </div>
      <strong title={exactTitle}>{value}</strong>
    </div>
  );
}

