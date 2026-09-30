// Run: tsx src/__tests__/answer-key-points.test.ts
//
// The key-point strip reads the emphasis marks out of the rendered DOM, so the
// marks plus this collector are the contract. Claims carry their whole sentence;
// labels carry only the bolded label, never the explanation behind it.

import { JSDOM } from "jsdom";
import { collectAnswerKeyPoints } from "../lib/answerKeyPoints";

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

console.log("\nanswer key points");

eq(
  collectAnswerKeyPoints(body(`
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
    </div>`)),
  [
    { kind: "claim", text: "结论先行" },
    { kind: "label", text: "标签一" },
    { kind: "label", text: "标签二" },
    { kind: "claim", text: "第二个主张" },
  ],
  "collects claims and label bullets in document order, labels by their bolded label only, duplicates once",
);

eq(collectAnswerKeyPoints(body("<div class=\"md\"><p>只有普通段落</p><ul><li>普通项</li></ul></div>")), [], "an answer with no marks yields no points");
eq(collectAnswerKeyPoints(body("<p class=\"md-p--claim\"><strong>无 .md 祖先</strong></p>")), [], "marks outside the answer body are ignored");
eq(collectAnswerKeyPoints(null), [], "a missing root is tolerated");

const longClaim = "很长的判断".repeat(60);
eq(
  collectAnswerKeyPoints(body(`<div class="md"><p class="md-p--claim">${longClaim}</p></div>`))[0].text.length,
  120,
  "a long claim is truncated to the strip's width budget",
);
ok(
  collectAnswerKeyPoints(body(`<div class="md"><p class="md-p--claim">${longClaim}</p></div>`))[0].text.endsWith("…"),
  "the truncation is visible as an ellipsis",
);

const many = Array.from({ length: 9 }, (_, index) => `<p class="md-p--claim">判断 ${index}</p>`).join("");
eq(collectAnswerKeyPoints(body(`<div class="md">${many}</div>`)).length, 6, "the strip caps the number of points");

eq(
  collectAnswerKeyPoints(body("<div class=\"md\"><p class=\"md-p--claim\">  多行\n   判断  </p></div>")),
  [{ kind: "claim", text: "多行 判断" }],
  "whitespace inside a claim collapses to single spaces",
);
eq(collectAnswerKeyPoints(body("<div class=\"md\"><p class=\"md-p--claim\">   </p></div>")), [], "an empty claim is dropped");

console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
