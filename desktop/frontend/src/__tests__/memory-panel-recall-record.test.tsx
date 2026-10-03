// Run: node --import ./scripts/css-stub-register.mjs --import ./scripts/svg-stub-register.mjs --import tsx src/__tests__/memory-panel-recall-record.test.tsx
//
// 第十六 (docs/50 §2.2): the requirement asks for the recall record in the existing
// panel module. The memory panel's 召回记录 tab mounts the same strip the recap page
// uses, driven by the tab-scoped host reader (`RecallRecordForTab`) instead of a
// session path — an optional prop, so losing the mount would pass the type checker
// and be invisible at runtime. This pins both the behaviour and the mount.

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { JSDOM } from "jsdom";
import React from "react";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { LocaleProvider } from "../lib/i18n";
import type { RecallRecordView } from "../generated/desktopContract.generated";

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

const { RecapRecallStrip } = await import("../components/RecapRecallStrip");

let root: Root | null = null;

async function renderTabScopedStrip(load: () => Promise<RecallRecordView>): Promise<string> {
  const host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  await act(async () => {
    root?.render(
      <LocaleProvider>
        <RecapRecallStrip load={load} />
      </LocaleProvider>,
    );
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  const text = host.textContent ?? "";
  await act(async () => {
    root?.unmount();
  });
  host.remove();
  root = null;
  return text;
}

const withRecord: RecallRecordView = {
  available: true,
  sessionPath: "C:/sessions/tab.jsonl",
  turns: [
    {
      turnSeq: 3,
      omitted: 1,
      hits: [
        { id: "mem-injected", revision: 2, score: 0.5, injected: true },
        { id: "mem-dropped", revision: 1, score: 0.25, injected: false },
      ],
    },
  ],
  skills: [{ turnSeq: 3, name: "review", contentHash: "abc", catalogDigest: "def" }],
};
const withoutRecord: RecallRecordView = { available: false };

const folded = await renderTabScopedStrip(async () => withRecord);
ok(folded.includes("召回") && folded.includes("1") && folded.includes("技能"), "a tab-scoped loader renders the folded counts");
ok(!folded.includes("mem-injected"), "the folded view lists no fingerprints");

const empty = await renderTabScopedStrip(async () => withoutRecord);
ok(empty.trim() === "", "an unavailable record renders nothing");

const failing = await renderTabScopedStrip(async () => {
  throw new Error("host unavailable");
});
ok(failing.trim() === "", "a failed read renders nothing rather than an error");

const testDir = fileURLToPath(new URL(".", import.meta.url));
const panel = readFileSync(resolve(testDir, "../components/MemoryPanel.tsx"), "utf8");
const stripAt = panel.indexOf("<RecapRecallStrip");
const activityAt = panel.indexOf('{tab === "activity"');
ok(panel.includes('import { RecapRecallStrip } from "./RecapRecallStrip";'), "the memory panel imports the strip");
ok(panel.includes("app.RecallRecordForTab(effectiveTabId)"), "the panel drives it with the tab-scoped host reader");
ok(stripAt > 0 && activityAt > 0 && stripAt > activityAt, "the strip is mounted inside the 召回记录 tab, not elsewhere");

process.stdout.write(`\nmemory panel recall record: ${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exit(1);
