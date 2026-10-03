// Run: node --import ./scripts/css-stub-register.mjs --import ./scripts/svg-stub-register.mjs --import tsx src/__tests__/footer-recall-module.test.tsx

// 用户要求把召回记录放进桌面「面板」（底部面板带的卡片，dict 里 footerPanel.title=面板）。
// 两条断言：① 记录渲染本身走受控模式（调用方已经拿到记录时不再重复问宿主，且没有记录就什么都不渲染——
// 卡片的规矩是"不适用的模块不许留一个永远填不满的表头"）；② 接线：registry 里确实注册了这个模块，
// 模块用按 tab 的读取器、套 FooterPanelSection。少挂一个可选模块在类型检查上是无声的。

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { JSDOM } from "jsdom";
import React from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";
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

const dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost/" });
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
globalThis.window = dom.window as unknown as Window & typeof globalThis;
globalThis.document = dom.window.document;
globalThis.HTMLElement = dom.window.HTMLElement;
globalThis.Node = dom.window.Node;
globalThis.Event = dom.window.Event;

const { RecapRecallStrip } = await import("../components/RecapRecallStrip");

async function renderControlled(record: RecallRecordView | null): Promise<string> {
  const host = document.createElement("div");
  document.body.appendChild(host);
  const root = createRoot(host);
  await act(async () => {
    root.render(
      <LocaleProvider>
        <RecapRecallStrip record={record} />
      </LocaleProvider>,
    );
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  const text = host.textContent ?? "";
  await act(async () => {
    root.unmount();
  });
  host.remove();
  return text;
}

const withRecord: RecallRecordView = {
  available: true,
  sessionPath: "C:/sessions/panel.jsonl",
  turns: [
    {
      turnSeq: 4,
      omitted: 2,
      hits: [
        { id: "mem-injected", revision: 3, score: 0.8, injected: true },
        { id: "mem-dropped", revision: 1, score: 0.2, injected: false },
      ],
    },
  ],
  skills: [{ turnSeq: 4, name: "review", contentHash: "abc", catalogDigest: "def" }],
};

const shown = await renderControlled(withRecord);
ok(shown.includes("召回") && shown.includes("技能"), "a held record renders without asking the host again");
ok(!shown.includes("mem-injected"), "the folded view lists no fingerprints");

const empty = await renderControlled(null);
ok(empty.trim() === "", "no record renders nothing, so the card never leaves an empty header");
const unavailable = await renderControlled({ available: false });
ok(unavailable.trim() === "", "an unavailable record renders nothing");

const testDir = fileURLToPath(new URL(".", import.meta.url));
const source = (relative: string) => readFileSync(resolve(testDir, relative), "utf8");
const registry = source("../components/footerPanelModules.tsx");
const moduleSource = source("../components/FooterRecallModule.tsx");
ok(registry.includes('import { FooterRecallModule } from "./FooterRecallModule"'), "the module registry imports it");
ok(registry.includes('id: "recall-record"'), "the footer panel registers the recall-record module");
ok(registry.indexOf('id: "recall-record"') > registry.indexOf('id: "memory"'), "it sits with the memory family, in reading order");
ok(/\bapp\s*\.\s*RecallRecordForTab\(tabId\)/.test(moduleSource), "the module reads the record for its tab");
ok(moduleSource.includes('title="memory.activity"'), "the section reuses the 召回记录 label");
ok(moduleSource.includes("<RecapRecallStrip record={record} />"), "the module hands the record to the strip instead of re-reading it");
ok(moduleSource.includes("record === null || record.available !== true) return null"), "an unavailable record keeps the module (and its header) off the card");

dom.window.close();

console.log(`\nfooter recall module: ${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
