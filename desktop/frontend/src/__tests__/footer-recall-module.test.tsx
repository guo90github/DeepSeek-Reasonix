// Run: node --import ./scripts/css-stub-register.mjs --import ./scripts/svg-stub-register.mjs --import tsx src/__tests__/footer-recall-module.test.tsx

// 用户要求把召回记录放进桌面「面板」（底部面板带的卡片，dict 里 footerPanel.title=面板）。
// 用户随后反馈"只有 id 给人看很不友好、没有内容会看不懂" ⇒ 记录仍保持 content-free（指纹），
// 显示时用调用方手上的 facts 清单把 id 解析成事实名。这里钉住：
// ① 受控渲染（调用方已拿到记录就不再重复问宿主；没有记录整块不出现——卡片的规矩是不留空表头）；
// ② 展开后有标签就显示事实名、同时保留 id 以便追溯，没标签就退回只显示 id（历史会话仍可读）；
// ③ 接线：registry 注册了它、模块读了记录**和** facts、套 FooterPanelSection。

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { JSDOM } from "jsdom";
import React from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { LocaleProvider } from "../lib/i18n";
import type { MemoryFact, RecallRecordView } from "../generated/desktopContract.generated";

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
globalThis.MouseEvent = dom.window.MouseEvent;

const { RecapRecallStrip } = await import("../components/RecapRecallStrip");
const { buildRecallLabels } = await import("../lib/recallLabels");

async function renderControlled(
  record: RecallRecordView | null,
  options: { facts?: readonly MemoryFact[]; expand?: boolean } = {},
): Promise<string> {
  const host = document.createElement("div");
  document.body.appendChild(host);
  const root = createRoot(host);
  await act(async () => {
    root.render(
      <LocaleProvider>
        <RecapRecallStrip record={record} facts={options.facts} />
      </LocaleProvider>,
    );
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  if (options.expand === true) {
    const head = host.querySelector<HTMLButtonElement>(".recap-recall__head");
    await act(async () => {
      head?.dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true, cancelable: true }));
    });
  }
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

const facts: readonly MemoryFact[] = [
  {
    id: "mem-injected",
    name: "recall-ledger",
    title: "方案与决策记录",
    description: "为什么选方案A",
    type: "project",
    scope: "project",
    body: "…",
    freshness: "fresh",
  },
];

const shown = await renderControlled(withRecord);
ok(shown.includes("召回") && shown.includes("技能"), "a held record renders without asking the host again");
ok(!shown.includes("mem-injected"), "the folded view lists no fingerprints");

const named = await renderControlled(withRecord, { facts, expand: true });
ok(named.includes("方案与决策记录"), "with the caller's fact list the row leads with a readable name");
ok(named.includes("mem-injected"), "the id stays beside the name, so the row is still traceable");
ok(!named.includes("recall-ledger"), "the slug-ish name is not what a person reads");

const unnamed = await renderControlled(withRecord, { expand: true });
ok(unnamed.includes("mem-injected"), "without a fact list the row falls back to the id alone");
ok(!unnamed.includes("方案与决策记录"), "an unknown fact never invents a name");

const labels = buildRecallLabels([
  { id: "mem-a", name: "slug-a", title: "标题A", description: "摘要A", type: "project", scope: "project", body: "", freshness: "fresh" },
  { id: "mem-b", name: "slug-b", description: "摘要B", type: "global", scope: "global", body: "", freshness: "stale" },
]);
ok(labels.get("mem-a")?.label === "标题A", "a titled fact is keyed by id and reads by its title");
ok(labels.get("slug-a")?.label === "标题A", "the fact's name is a key too");
ok(labels.get("mem-b")?.label === "slug-b", "a fact without a title falls back to its name");
ok((labels.get("mem-b")?.hint ?? "").includes("摘要B"), "the hint carries the description the model saw");

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
ok(/\bapp\s*\.\s*MemoryForTab\(tabId\)/.test(moduleSource), "the module reads the tab's fact list for the names");
ok(moduleSource.includes('title="memory.activity"'), "the section reuses the 召回记录 label");
ok(moduleSource.includes("<RecapRecallStrip record={record} facts={facts} />"), "the module hands over both the record and the facts instead of re-reading them");
ok(moduleSource.includes("record === null || record.available !== true) return null"), "an unavailable record keeps the module (and its header) off the card");

dom.window.close();

console.log(`\nfooter recall module: ${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
