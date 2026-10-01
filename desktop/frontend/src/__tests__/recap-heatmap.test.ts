// Run: npx tsx src/__tests__/recap-heatmap.test.ts
// 第十四 (docs/60 §2.1): the heatmap aggregates what the page already has — insight
// occurrence counts by project and day, plus recaps by their generation day.

import {
  RECAP_HEATMAP_UNLABELED,
  RECAP_HEATMAP_WINDOW_DAYS,
  buildRecapHeatmap,
  dayKey,
} from "../lib/recapHeatmap";

let passed = 0;
let failed = 0;

function eq<T>(got: T, want: T, label: string) {
  const ok = JSON.stringify(got) === JSON.stringify(want);
  if (ok) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n    got  ${JSON.stringify(got)}\n    want ${JSON.stringify(want)}\n`);
    failed += 1;
  }
}

const now = "2026-10-01T12:00:00Z";

// The window ends today and covers the days before it, oldest first.
const empty = buildRecapHeatmap({ insights: [], recaps: [], now });
eq(empty.days.length, RECAP_HEATMAP_WINDOW_DAYS, "an empty input still describes the window");
eq(empty.from, "2026-09-02", "the window starts 29 days before the end");
eq(empty.to, "2026-10-01", "the window ends on the given day");
eq(empty.rows.length, 0, "no data means no rows");
eq(empty.max, 0, "no data means no intensity");

// Insights weight a project's day by how many records reached it.
const insights = buildRecapHeatmap({
  insights: [
    { projects: ["reasonix"], occurrences: 3, seenAt: "2026-09-30T08:00:00Z" },
    { projects: ["reasonix"], occurrences: 1, seenAt: "2026-10-01T09:30:00Z" },
    { projects: ["chatting"], occurrences: 2, seenAt: "2026-09-28T23:59:00Z" },
  ],
  recaps: [],
  now,
});
eq(insights.rows.map((row) => row.project), ["reasonix", "chatting"], "rows are ordered by total intensity");
eq(insights.rows[0].total, 4, "a project's total sums its occurrences");
eq(insights.max, 3, "the strongest cell is the 3-occurrence day");
const reasonix = insights.rows[0].cells;
eq(reasonix.filter((cell) => cell.count > 0).map((cell) => cell.day), ["2026-09-30", "2026-10-01"], "only days with data carry intensity");
eq(reasonix[reasonix.length - 1].count, 1, "today's cell counts today's insight");

// Recaps count once per project per day; an unlabelled recap is kept, not dropped.
const withRecaps = buildRecapHeatmap({
  insights: [],
  recaps: [
    { generatedAt: "2026-10-01T01:00:00Z", projects: ["reasonix"] },
    { generatedAt: "2026-10-01T02:00:00Z", projects: ["reasonix"] },
    { generatedAt: "2026-10-01T03:00:00Z" },
    { generatedAt: "2026-08-01T03:00:00Z", projects: ["reasonix"] },
  ],
  now,
});
eq(withRecaps.rows.length, 2, "labelled and unlabelled recaps both produce a row");
eq(withRecaps.rows.map((row) => row.project), ["reasonix", RECAP_HEATMAP_UNLABELED], "the busier project comes first, the unlabelled row last");
eq(withRecaps.rows.find((row) => row.project === "reasonix")?.total, 2, "recaps outside the window are dropped");
eq(withRecaps.rows.find((row) => row.project === RECAP_HEATMAP_UNLABELED)?.total, 1, "a recap with no project lands in the unlabelled row");

// The window is honoured at both edges.
const windowed = buildRecapHeatmap({ insights: [], recaps: [{ generatedAt: "2026-09-02T00:00:00Z", projects: ["p"] }], now });
eq(windowed.rows[0]?.total, 1, "the first day of the window is inside it");
const outside = buildRecapHeatmap({ insights: [], recaps: [{ generatedAt: "2026-09-01T23:59:00Z", projects: ["p"] }], now });
eq(outside.rows.length, 0, "the day before the window is outside it");

// A shorter window is respected, and day keys are UTC calendar days.
const short = buildRecapHeatmap({ insights: [], recaps: [], now, windowDays: 7 });
eq(short.days.length, 7, "windowDays is honoured");
eq(short.from, "2026-09-25", "a short window starts later");
eq(dayKey("2026-10-01T23:30:00Z"), "2026-10-01", "a stamp late in the day keeps its UTC day");
eq(dayKey(""), "", "an empty stamp has no day");
eq(dayKey("not a date"), "", "an unparseable stamp has no day");

process.stdout.write(`\nrecap heatmap: ${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exit(1);
