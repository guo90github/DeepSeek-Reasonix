// Run: tsx src/__tests__/answer-key-points.test.ts
//
// The key-point strip reads the emphasis marks out of the rendered DOM, so the
// marks plus this collector are the contract. Claims carry their whole sentence;
// labels carry only the bolded label, never the explanation behind it. Every
// mark gets a stable address (data-md-point) so a jump target is exact even when
// the strip dedupes or caps what it lists.

import { JSDOM } from "jsdom";
import { EMPTY_ANSWER_MARKS, collectAnswerKeyPoints, sameAnswerKeyPoints } from "../lib/answerKeyPoints";

let passed = 0;
let failed = 0;

function ok(value: unknown, label: string) {
  if (value) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

function eq(actual: unknown, expected: unknown, label: string) {
  const same = JSON.stringify(actual) === JSON.stringify(expected);
  ok(same, same ? label : `${label}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`);
}

function body(html: string): HTMLElement {
  return new JSDOM(`<!doctype html><body>${html}</body>`).window.document.body;
}

function markAddresses(root: ParentNode): (string | null)[] {
  return [...root.querySelectorAll(".md p.md-p--claim, .md li.md-li--label")].map((node) => node.getAttribute("data-md-point"));
}

console.log("\nanswer key points");

const mixed = body(`
  <div class="md">
    <p class="md-p--claim"><strong>结论先行</strong></p>
    <p>普通段落不该成为要点。</p>
    <ul>
      <li class="md-li--label"><strong>标签一</strong>：后面是解释文字，不该进要点</li>
      <li>普通列表项</li>
      <li class="md-li--label"><p><strong>标签二</strong>：松散列表里也一样</p></li>
    </ul>
    <p class="md-p--claim"><strong>第二个主张</strong></p>
    <p class="md-p--claim"><strong>结论先行</strong></p>
  </div>`);

eq(
  collectAnswerKeyPoints(mixed),
  {
    points: [
      { ordinal: 0, kind: "claim", text: "结论先行" },
      { ordinal: 1, kind: "label", text: "标签一" },
      { ordinal: 2, kind: "label", text: "标签二" },
      { ordinal: 3, kind: "claim", text: "第二个主张" },
    ],
    total: 4,
  },
  "collects claims and label bullets in document order, labels by their bolded label only, duplicates once",
);
eq(markAddresses(mixed), ["0", "1", "2", "3", "4"], "every mark carries its document-order address, duplicates included");
eq(
  collectAnswerKeyPoints(mixed).points.map((point) => point.ordinal),
  [0, 1, 2, 3],
  "a deduped point keeps the address of its first occurrence",
);

eq(collectAnswerKeyPoints(body("<div class=\"md\"><p>只有普通段落</p><ul><li>普通项</li></ul></div>")), EMPTY_ANSWER_MARKS, "an answer with no marks yields no points");
eq(collectAnswerKeyPoints(body("<p class=\"md-p--claim\"><strong>无 .md 祖先</strong></p>")), EMPTY_ANSWER_MARKS, "marks outside the answer body are ignored");
eq(collectAnswerKeyPoints(null), EMPTY_ANSWER_MARKS, "a missing root is tolerated");

// Renumbering must be idempotent: the strip re-reads the body on every DOM change.
const renumbered = body("<div class=\"md\"><p class=\"md-p--claim\">甲</p><p class=\"md-p--claim\">乙</p></div>");
collectAnswerKeyPoints(renumbered);
collectAnswerKeyPoints(renumbered);
eq(markAddresses(renumbered), ["0", "1"], "a second read leaves the same addresses");

const longClaim = "很长的判断".repeat(60);
const longRoot = body(`<div class="md"><p class="md-p--claim">${longClaim}</p></div>`);
eq(collectAnswerKeyPoints(longRoot).points[0].text.length, 120, "a long claim is truncated to the strip's width budget");
ok(collectAnswerKeyPoints(longRoot).points[0].text.endsWith("…"), "the truncation is visible as an ellipsis");

const many = Array.from({ length: 9 }, (_, index) => `<p class="md-p--claim">判断 ${index}</p>`).join("");
const manySummary = collectAnswerKeyPoints(body(`<div class="md">${many}</div>`));
eq(manySummary.points.length, 6, "the strip caps how many points it lists");
eq(manySummary.total, 9, "the header still reports every mark, so the list never overstates itself");

eq(
  collectAnswerKeyPoints(body("<div class=\"md\"><p class=\"md-p--claim\">  多行\n   判断  </p></div>")).points,
  [{ ordinal: 0, kind: "claim", text: "多行 判断" }],
  "whitespace inside a claim collapses to single spaces",
);
eq(collectAnswerKeyPoints(body("<div class=\"md\"><p class=\"md-p--claim\">   </p></div>")).points, [], "an empty claim is dropped");
eq(collectAnswerKeyPoints(body("<div class=\"md\"><p class=\"md-p--claim\">   </p></div>")).total, 0, "an empty claim is not counted as a key point");
const emptyThenReal = body("<div class=\"md\"><p class=\"md-p--claim\">   </p><p class=\"md-p--claim\">真要点</p></div>");
collectAnswerKeyPoints(emptyThenReal);
eq(markAddresses(emptyThenReal), ["0", "1"], "addresses count marks, not listable points");

ok(sameAnswerKeyPoints(EMPTY_ANSWER_MARKS, collectAnswerKeyPoints(null)), "identical summaries compare equal");
ok(
  !sameAnswerKeyPoints(mixed ? collectAnswerKeyPoints(mixed) : EMPTY_ANSWER_MARKS, { points: [], total: 4 }),
  "a changed point list is not mistaken for the same one",
);

console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
