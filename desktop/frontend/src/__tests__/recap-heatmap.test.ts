// Run: npx tsx src/__tests__/recap-heatmap.test.ts
// 第十四 (docs/60 §2.1): the heatmap aggregates what the page already has — insight
// occurrence counts by project and day, plus recaps by the day they were generated.
// A day is a local calendar day: the day the page prints for that record.

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

// Fixtures are derived from the page's own local-day arithmetic, so the suite holds
// in whatever timezone it runs in; a hardcoded hour-based stamp would not.
const atLocalNoon = (daysAgo: number) => {
  const at = new Date(now);
  at.setDate(at.getDate() - daysAgo);
  at.setHours(12, 0, 0, 0);
  return at.toISOString();
};

// The window ends today and covers the days before it, oldest first.
const empty = buildRecapHeatmap({ insights: [], recaps: [], now });
eq(empty.days.length, RECAP_HEATMAP_WINDOW_DAYS, "an empty input still describes the window");
eq(empty.from, dayKey(atLocalNoon(29)), "the window starts 29 days before the end");
eq(empty.to, dayKey(now), "the window ends on the given day");
eq(empty.rows.length, 0, "no data means no rows");
eq(empty.max, 0, "no data means no intensity");

// Insights weight a project's day by how many records reached it.
const insights = buildRecapHeatmap({
  insights: [
    { projects: ["reasonix"], occurrences: 3, seenAt: atLocalNoon(1) },
    { projects: ["reasonix"], occurrences: 1, seenAt: atLocalNoon(0) },
    { projects: ["chatting"], occurrences: 2, seenAt: atLocalNoon(3) },
  ],
  recaps: [],
  now,
});
eq(insights.rows.map((row) => row.project), ["reasonix", "chatting"], "rows are ordered by total intensity");
eq(insights.rows[0].total, 4, "a project's total sums its occurrences");
eq(insights.max, 3, "the strongest cell is the 3-occurrence day");
const reasonix = insights.rows[0].cells;
eq(reasonix.filter((cell) => cell.count > 0).map((cell) => cell.day), [dayKey(atLocalNoon(1)), dayKey(atLocalNoon(0))],
  "only days with data carry intensity");
eq(reasonix[reasonix.length - 1].count, 1, "today's cell counts today's insight");

// Recaps count once per project per day; an unlabelled recap is kept, not dropped.
const withRecaps = buildRecapHeatmap({
  insights: [],
  recaps: [
    { generatedAt: atLocalNoon(0), projects: ["reasonix"] },
    { generatedAt: atLocalNoon(0), projects: ["reasonix"] },
    { generatedAt: atLocalNoon(0) },
    { generatedAt: atLocalNoon(40), projects: ["reasonix"] },
  ],
  now,
});
eq(withRecaps.rows.length, 2, "labelled and unlabelled recaps both produce a row");
eq(withRecaps.rows.map((row) => row.project), ["reasonix", RECAP_HEATMAP_UNLABELED], "the busier project comes first, the unlabelled row last");
eq(withRecaps.rows.find((row) => row.project === "reasonix")?.total, 2, "recaps outside the window are dropped");
eq(withRecaps.rows.find((row) => row.project === RECAP_HEATMAP_UNLABELED)?.total, 1, "a recap with no project lands in the unlabelled row");

// The window is honoured at both edges.
const windowed = buildRecapHeatmap({ insights: [], recaps: [{ generatedAt: atLocalNoon(0), projects: ["p"] }], now });
eq(windowed.rows[0]?.total, 1, "the first day of the window is inside it");
const outside = buildRecapHeatmap({ insights: [], recaps: [{ generatedAt: atLocalNoon(30), projects: ["p"] }], now });
eq(outside.rows.length, 0, "the day before the window is outside it");

// A shorter window is respected.
const short = buildRecapHeatmap({ insights: [], recaps: [], now, windowDays: 7 });
eq(short.days.length, 7, "windowDays is honoured");
eq(short.from, dayKey(atLocalNoon(6)), "a short window starts later");

// A day key is the stamp's local calendar day, which is the day the page prints
// beside it. Bucketing in UTC would file a recap generated before 08:00 in UTC+8
// under the previous cell, next to a card stating the later date.
eq(dayKey(atLocalNoon(0)), dayKey(now), "a stamp on the window's day keys the window's own day");
const lateStamp = "2026-10-01T23:30:00Z";
const lateLocal = new Date(lateStamp);
eq(
  dayKey(lateStamp),
  `${lateLocal.getFullYear()}-${String(lateLocal.getMonth() + 1).padStart(2, "0")}-${String(lateLocal.getDate()).padStart(2, "0")}`,
  "a stamp late in the day keys the day the page shows it on",
);
eq(dayKey(""), "", "an empty stamp has no day");
eq(dayKey("not a date"), "", "an unparseable stamp has no day");

process.stdout.write(`\nrecap heatmap: ${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exit(1);
