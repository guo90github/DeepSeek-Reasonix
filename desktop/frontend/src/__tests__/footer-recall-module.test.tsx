// Run: tsx src/__tests__/footer-recall-module.test.tsx

// 用户在桌面「面板」（底部面板带的卡片）里看召回记录：先反馈"只有 id 给人看很不友好"，
// 这一轮又反馈"展示混乱、点了看不到详情"。后者钉在这里：记录按卡片的行渲染（一行一条、
// 省略号收尾），点任意一行开卡片的共用详情弹层。记录自身仍 content-free —— 行上的名字优先
// 取记录随行的标签，缺了才回落当前事实清单，兜底是 id。
//
// ① 有记录才出现（没记录 / 不可用整块不出现——卡片的规矩是不留空表头）；
// ② 行模型：命中项 = 第N轮 · 名字 · id · 已注入/被挤掉；技能行自带「技能」，不与命中项混同；
//    每轮「未列出 / 未召回」是提示行，不是可点的记录行；
// ③ 点行 → [role=dialog]：标题是名字，meta 带轮次/指纹/版本/分值/状态，body 是 id
//    （当前事实清单能解析到时，body 换成它的正文，并打上「现行记忆」标记）；
// ④ 记录随会话变长、卡片不会：超过一页先只出 FOOTER_RECALL_INITIAL 行，点「再显示」才继续；
// ⑤ 接线：registry 注册了它、模块用 PanelRowButton 与 .footer-recall__row，
//    不再挂回顾页那条会在窄栏里折行的指纹条。
//
// 文案一律用 `t(...)` 取，不写死某一种语言：jsdom 里 detectLocale 落回 en，而记录本身的
// 名字（事实标题、slug）是数据，照旧按断言里的原文比对。

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { FooterPanel } from "../components/FooterPanel";
import { FOOTER_PANEL_MODULES } from "../components/footerPanelModules";
import { FOOTER_RECALL_INITIAL } from "../components/FooterRecallModule";
import type { AppBindings } from "../lib/bridge";
import { LocaleProvider, t } from "../lib/i18n";
import type { MemoryView } from "../lib/types";
import type { MemoryFact, RecallRecordView } from "../generated/desktopContract.generated";
import { installDesktopHostStub } from "./desktopHostStub";
import { flushPromises, installDom, waitFor } from "./workspace-panel-test-harness";

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

const facts: readonly MemoryFact[] = [
  {
    id: "mem-injected",
    name: "recall-ledger",
    title: "方案与决策记录",
    description: "为什么选方案A",
    type: "project",
    scope: "project",
    body: "整条事实的正文",
    freshness: "fresh",
  },
];

function memoryView(items: readonly MemoryFact[]): MemoryView {
  return {
    docs: [],
    facts: [...items],
    archives: [],
    scopes: [{ scope: "project", path: "/repo/REASONIX.md" }],
    instructionDiagnostics: [],
    conflicts: [],
    lastRecall: { query: "", hits: [], omitted: 0, charBudget: 6000, usedChars: 1500 },
    storeDir: "/home/.reasonix/memory",
    available: true,
  };
}

const record: RecallRecordView = {
  available: true,
  sessionPath: "C:/sessions/panel.jsonl",
  turns: [
    {
      turnSeq: 4,
      omitted: 2,
      hits: [
        {
          id: "mem-injected",
          revision: 3,
          score: 0.81,
          injected: true,
          reason: "matched 方案与决策 in label fields (6 of 4 runes needed); project scope",
        },
        { id: "mem-dropped", revision: 1, score: 0.22, injected: false },
        { id: "mem-labeled", name: "slug-only", title: "只读落盘也能读的名字", injected: true },
      ],
      suppressed: "预算",
    },
  ],
  skills: [{ turnSeq: 4, name: "review", contentHash: "abc", catalogDigest: "def" }],
};

async function renderPanel(rec: RecallRecordView, items: readonly MemoryFact[] = []) {
  const dom = installDom();
  const asked = { recall: 0 };
  installDesktopHostStub(({
    main: {
      App: {
        // Parked: the other modules of this card must not add rows to the panel.
        WorkspaceChanges: () => new Promise(() => {}),
        WorkspaceGitHistory: () => new Promise(() => {}),
        MemorySuggestionsForTab: () => new Promise(() => {}),
        RecallRecordForTab: async () => {
          asked.recall += 1;
          return rec;
        },
        MemoryForTab: async () => memoryView(items),
      } as Partial<AppBindings> as AppBindings,
    },
  }).main.App);
  const rootEl = document.getElementById("root");
  if (!rootEl) throw new Error("missing root");
  const root = createRoot(rootEl);
  await act(async () => {
    root.render(
      <LocaleProvider>
        <FooterPanel modules={FOOTER_PANEL_MODULES} context={{ tabId: "tab-a", workspaceScopeKey: "scope-a" }} />
      </LocaleProvider>,
    );
    await flushPromises();
  });
  return { dom, root, asked };
}

function rows(): HTMLElement[] {
  return [...document.querySelectorAll<HTMLElement>(".footer-recall__row")];
}

async function click(element: Element | null | undefined) {
  await act(async () => {
    (element as HTMLElement | null)?.click();
    await flushPromises();
  });
}

async function escape() {
  await act(async () => {
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await flushPromises();
  });
}

console.log("\nfooter recall module");

{
  const { dom, root, asked } = await renderPanel(record, facts);
  await act(async () => {
    await waitFor("recall answer", () => document.querySelector(".footer-recall") !== null);
  });
  ok(asked.recall === 1, "the module reads its record once");

  const turn = t("history.recallStripTurn", { turn: "4" });
  const rendered = rows();
  ok(rendered.length === 4, "three hits and one skill are four rows, one line each");
  ok(
    rendered.every((row) => row.querySelector(".footer-recall__turn")?.textContent?.includes(turn) === true),
    "every row is anchored by its turn",
  );
  ok(
    document.body.textContent?.includes(t("history.recallStripSummary", { injected: "2", dropped: "1", skills: "1" })) === true,
    "the bar keeps the record's own summary",
  );
  ok(
    document.querySelectorAll(".footer-recall .footer-panel__note").length === 1 &&
      document.body.textContent?.includes(t("history.recallStripOmitted", { n: "2" })) === true &&
      document.body.textContent?.includes(t("history.recallStripSuppressed", { reason: "预算" })) === true,
    "a turn's omitted and suppressed reasons are one note line, not a clickable row",
  );

  ok(rendered[0].textContent?.includes("方案与决策记录") === true, "a hit without its own label is named from the tab's fact list");
  ok(rendered[0].textContent?.includes("mem-injected") === true, "the id stays beside the name, so the row is traceable");
  ok(rendered[0].textContent?.includes(t("history.recallStripInjected")) === true, "the row says whether the hit was injected");
  ok(rendered[1].textContent?.includes(t("history.recallStripDropped")) === true, "a dropped hit says so");
  ok(rendered[2].textContent?.includes("只读落盘也能读的名字") === true, "a record's own label wins over today's fact list");
  ok(rendered[2].textContent?.includes("方案与决策记录") === false, "the live fact list is only the fallback for unlabelled hits");
  ok(
    rendered[3].textContent?.includes("review") === true && rendered[3].textContent?.includes(t("footerPanel.recallSkills")) === true,
    "a skill gets its own row, marked as a skill",
  );

  await click(rendered[0]);
  ok(document.querySelector('[role="dialog"]') !== null, "clicking a row opens the card's shared detail dialog");
  ok(document.querySelector(".footer-detail__title")?.textContent === "方案与决策记录", "the dialog leads with the fact's name");
  ok(
    document.querySelectorAll(".footer-detail__tag").length === 6,
    "turn, fingerprint, revision, score, state and the live-fact marker ride as tags",
  );
  ok(document.body.textContent?.includes(t("footerPanel.recallLiveFact")) === true, "a body taken from today's fact list is marked as such");
  ok(document.body.textContent?.includes("整条事实的正文") === true, "the dialog carries the fact the id resolves to today");
  ok(
    document.body.textContent?.includes(
      t("footerPanel.recallEvidence", { reason: "matched 方案与决策 in label fields (6 of 4 runes needed); project scope" }),
    ) === true,
    "the dialog says why the hit was kept, in the record's own words",
  );
  ok(
    document.querySelectorAll(".footer-detail__body .footer-panel__note").length === 1,
    "the evidence rides one muted line of its own, not another tag in the state row",
  );
  await escape();
  ok(document.querySelector('[role="dialog"]') === null, "Escape closes the dialog");

  await click(rows()[1]);
  ok(document.querySelector(".footer-detail__title")?.textContent === "mem-dropped", "an unresolvable id is its own headline");
  ok(document.querySelector(".modal__subject") !== null, "an unresolvable id gets a mono body, not invented content");
  await escape();

  await click(rows()[3]);
  ok(
    document.body.textContent?.includes(
      t("history.recallStripSkill", { turn: "4", contentHash: "abc", catalogDigest: "def" }),
    ) === true,
    "a skill row opens with its content hash and catalog digest",
  );
  await escape();

  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

{
  const { dom, root } = await renderPanel({ available: false });
  await act(async () => {
    await waitFor("unavailable record", () => document.querySelector(".footer-memory") !== null);
  });
  ok(document.querySelector(".footer-recall") === null, "an unavailable record renders nothing");
  ok(document.body.textContent?.includes("召回记录") === false, "and leaves no header behind");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

{
  // 记录随会话变长，卡片不会：一页之后先收住，点「再显示」才继续。
  const many: RecallRecordView = {
    available: true,
    turns: [{ turnSeq: 1, hits: Array.from({ length: 9 }, (_, index) => ({ id: `mem-${index}`, injected: index % 2 === 0 })) }],
  };
  const { dom, root } = await renderPanel(many);
  await act(async () => {
    await waitFor("long record", () => document.querySelector(".footer-recall") !== null);
  });
  ok(rows().length === FOOTER_RECALL_INITIAL, "a long record shows one page of rows first");
  ok(document.querySelector(".footer-panel__more") !== null, "and offers the next page");
  await click(document.querySelector(".footer-panel__more"));
  ok(rows().length === 9, "the button carries the rest of the record into view");
  ok(
    document.querySelector(".footer-panel__more")?.textContent?.includes(t("footerPanel.showLess")) === true,
    "and turns into the collapse it now is",
  );
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

const testDir = fileURLToPath(new URL(".", import.meta.url));
const source = (relative: string) => readFileSync(resolve(testDir, relative), "utf8");
const registry = source("../components/footerPanelModules.tsx");
const moduleSource = source("../components/FooterRecallModule.tsx");
const css = source("../components/footerPanel.css");
const idRule = css.slice(css.indexOf(".footer-recall__id"));
ok(registry.includes('import { FooterRecallModule } from "./FooterRecallModule"'), "the module registry imports it");
ok(registry.includes('id: "recall-record"'), "the footer panel registers the recall-record module");
ok(registry.indexOf('id: "recall-record"') > registry.indexOf('id: "memory"'), "it sits with the memory family, in reading order");
ok(/\bPanelRowButton\b/.test(moduleSource), "the module renders its rows as the card's clickable records");
ok(moduleSource.includes('className="footer-recall__row"'), "the rows carry the recall row class, not the recap strip's");
ok(!moduleSource.includes("RecapRecallStrip"), "the band no longer mounts the recap page's in-band strip");
ok(moduleSource.includes('title="memory.activity"'), "the section reuses the 召回记录 label");
ok(
  moduleSource.includes("record === null || record.available !== true) return null"),
  "an unavailable record keeps the module (and its header) off the card",
);
ok(idRule.slice(0, idRule.indexOf("}")).includes("text-overflow: ellipsis"), "the fingerprint truncates instead of widening the row");

console.log(`\nfooter recall module: ${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
