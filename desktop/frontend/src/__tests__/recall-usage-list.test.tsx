// Run: node --import ./scripts/css-stub-register.mjs --import ./scripts/svg-stub-register.mjs --import tsx src/__tests__/recall-usage-list.test.tsx
//
// 第十六 全局级 (docs/50 §2.2): the usage list must name the fact, count its uses,
// and mark a used revision the store has since moved past — the "old conclusion
// quoted again" case the aggregate exists for. Rendering is asserted, not assumed:
// the panel is where a wrong chip would silently mislead.

import { JSDOM } from "jsdom";
import React from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { LocaleProvider } from "../lib/i18n";
import type { RecallUsage } from "../generated/desktopContract.generated";

let passed = 0;
let failed = 0;

function ok(value: boolean, label: string) {
  if (value) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

const dom = new JSDOM("<!doctype html><html><body></body></html>", {
  pretendToBeVisual: true,
  url: "http://localhost/",
});
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
globalThis.window = dom.window as unknown as Window & typeof globalThis;
globalThis.document = dom.window.document;
globalThis.Node = dom.window.Node;
globalThis.Element = dom.window.Element;
globalThis.HTMLElement = dom.window.HTMLElement;
globalThis.SVGElement = dom.window.SVGElement;
globalThis.Event = dom.window.Event;

const { RecallUsageList } = await import("../components/RecallUsageList");

const record: RecallUsage = {
  available: true,
  sessions: 3,
  facts: [
    {
      id: "mem-old",
      name: "old-fact",
      description: "发布目标仍写 v1",
      uses: 4,
      injected: 3,
      dropped: 1,
      lastTurnSeq: 12,
      usedRevision: 1,
      currentRevision: 3,
      superseded: true,
      live: true,
    },
    { id: "mem-gone", name: "gone-fact", uses: 1, injected: 1, lastTurnSeq: 5, live: false },
    { id: "mem-ok", name: "ok-fact", uses: 2, injected: 2, lastTurnSeq: 9, usedRevision: 2, currentRevision: 2, live: true },
  ],
};

async function render(load: () => Promise<RecallUsage>) {
  const host = document.createElement("div");
  document.body.appendChild(host);
  const root = createRoot(host);
  await act(async () => {
    root.render(
      <LocaleProvider>
        <RecallUsageList tabId="tab-1" load={load} />
      </LocaleProvider>,
    );
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  const text = host.textContent ?? "";
  const html = host.innerHTML;
  await act(async () => {
    root.unmount();
  });
  host.remove();
  return { text, html };
}

const rendered = await render(async () => record);
ok(rendered.text.includes("3"), `the header reports the scanned sessions: ${rendered.text.trim()}`);
ok(rendered.text.includes("发布目标仍写 v1"), "a row leads with the fact's own description");
ok(rendered.text.includes("命中 4 次"), "a row counts the uses");
ok(rendered.text.includes("注入 3 次·正文未见使用"), "an injected-but-unused fact carries the proxy signal");
ok(
  rendered.text.includes("已被新版本取代（用 r1，现为 r3）"),
  "a used revision the store moved past is marked superseded",
);
ok(rendered.text.includes("已不在记忆库"), "a fact no longer in the store says so");
ok(
  rendered.html.split("chip--warn").length - 1 === 1,
  "exactly the superseded fact carries the warn chip",
);
ok(
  rendered.html.split('class="chip"').length - 1 === 1,
  "exactly the missing fact carries the neutral chip",
);

const empty = await render(async () => ({ available: false }));
ok(empty.text === "", "an unavailable aggregate renders nothing");

const failing = await render(async () => {
  throw new Error("no bridge");
});
ok(failing.text === "", "a failed read renders nothing rather than an error");


// 第十六 全局级: the other half of the record — a query that keeps being asked.
const withQueries = await render(async () => ({
  available: true,
  sessions: 2,
  facts: [{ id: "mem-a", name: "a-fact", uses: 3, injected: 0, dropped: 3, live: true }],
  queries: [
    { hash: "aaaa11111111", turns: 3, sessions: 2, injected: 0, dropped: 3, facts: ["mem-a"] },
    { hash: "bbbb22222222", turns: 4, sessions: 1, injected: 4, dropped: 0, facts: ["mem-b"] },
  ],
}));
ok(withQueries.text.includes("重复提问"), "a repeated query gets its own section");
ok(withQueries.text.includes("aaaa1111"), "a query row leads with its hash");
ok(withQueries.text.includes("问过 3 次 · 跨 2 个会话"), "a query row counts turns and sessions");
ok(
  withQueries.html.split("chip--warn").length - 1 === 1,
  "only the query that never injected carries the warn chip",
);
ok(rendered.text.includes("命中 3 次·从未注入"), "a fact that never injected carries the curation signal");

process.stdout.write(`\nrecall usage list: ${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exit(1);
