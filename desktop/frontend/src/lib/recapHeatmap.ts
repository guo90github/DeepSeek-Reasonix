// Calendar heatmap for the 会话回顾 page (docs/60 §2.1).
//
// The axes come from data the page already has: insights carry projects and a
// seen-at stamp with an occurrence count, recap records carry the day they were
// generated. Aggregation is a pure function so it can be tested without a DOM,
// and it never sees memory or skill text.

export type RecapHeatmapInput = {
  /** Recorded insights: a project list, an occurrence count, and when it was seen. */
  insights: { projects?: string[] | null; occurrences?: number | null; seenAt?: string | null }[];
  /** Recap records: when they were generated, and (when known) which projects. */
  recaps: { generatedAt?: string | null; projects?: string[] | null }[];
  /** The window's end; the window covers it and the days before it. */
  now: string | Date;
  /** Window length in days, inclusive of today. Defaults to 30 (insightWindow). */
  windowDays?: number;
};

export type HeatmapCell = { day: string; count: number };
export type HeatmapRow = { project: string; total: number; cells: HeatmapCell[] };
export type RecapHeatmap = { from: string; to: string; days: string[]; rows: HeatmapRow[]; max: number };

export const RECAP_HEATMAP_WINDOW_DAYS = 30;
/** Recaps whose projects are unknown share this row rather than being dropped. */
export const RECAP_HEATMAP_UNLABELED = "未标注";

export function buildRecapHeatmap(input: RecapHeatmapInput): RecapHeatmap {
  const windowDays = Math.max(1, Math.trunc(input.windowDays ?? RECAP_HEATMAP_WINDOW_DAYS));
  const to = dayKey(input.now);
  const days = trailingDays(to, windowDays);
  const index = new Map<string, number>();
  days.forEach((day, i) => index.set(day, i));

  const rows = new Map<string, number[]>();
  const row = (project: string): number[] => {
    let cells = rows.get(project);
    if (!cells) {
      cells = new Array(days.length).fill(0);
      rows.set(project, cells);
    }
    return cells;
  };

  for (const insight of input.insights) {
    const at = index.get(dayKey(insight.seenAt));
    if (at === undefined) continue;
    const weight = Math.max(1, Math.trunc(insight.occurrences ?? 1));
    for (const project of labels(insight.projects)) {
      row(project)[at] += weight;
    }
  }
  for (const recap of input.recaps) {
    const at = index.get(dayKey(recap.generatedAt));
    if (at === undefined) continue;
    const projects = labels(recap.projects);
    if (projects.length === 0) {
      row(RECAP_HEATMAP_UNLABELED)[at] += 1;
      continue;
    }
    for (const project of projects) {
      row(project)[at] += 1;
    }
  }

  let max = 0;
  const built: HeatmapRow[] = [];
  for (const [project, cells] of rows) {
    const total = cells.reduce((sum, n) => sum + n, 0);
    max = Math.max(max, ...cells);
    built.push({ project, total, cells: days.map((day, i) => ({ day, count: cells[i] })) });
  }
  built.sort((a, b) => (b.total - a.total) || a.project.localeCompare(b.project));
  return { from: days[0], to, days, rows: built, max };
}

/**
 * dayKey is a stamp's local calendar day — the day the page prints for it.
 * Bucketing in UTC would file a recap generated before 08:00 in UTC+8 under the
 * previous cell, right next to a card that states the later date.
 */
export function dayKey(stamp: string | Date | null | undefined): string {
  if (stamp === null || stamp === undefined) return "";
  const date = stamp instanceof Date ? stamp : new Date(String(stamp).trim());
  if (Number.isNaN(date.getTime())) return "";
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${date.getFullYear()}-${month}-${day}`;
}

function labels(projects: string[] | null | undefined): string[] {
  const out: string[] = [];
  for (const project of projects ?? []) {
    const label = String(project ?? "").trim();
    if (label !== "") out.push(label);
  }
  return out;
}

function trailingDays(to: string, days: number): string[] {
  const anchor = new Date(`${to}T00:00:00Z`);
  const out: string[] = [];
  for (let offset = days - 1; offset >= 0; offset -= 1) {
    const day = new Date(anchor);
    day.setUTCDate(anchor.getUTCDate() - offset);
    out.push(day.toISOString().slice(0, 10));
  }
  return out;
}

/**
 * Whether one recap record belongs to a heatmap day. The page uses this to filter
 * its list when a cell is selected; an empty day means "no filter".
 */
export function matchesHeatmapDay(recap: { generatedAt?: string | null }, day: string): boolean {
  const wanted = String(day ?? "").trim();
  if (wanted === "") return true;
  return dayKey(recap.generatedAt) === wanted;
}

