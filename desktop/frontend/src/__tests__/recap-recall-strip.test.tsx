// Run: node --import ./scripts/css-stub-register.mjs --import ./scripts/svg-stub-register.mjs --import tsx src/__tests__/recap-recall-strip.test.tsx
// 第十六 (docs/60 §2.2): the strip shows a session's recall/skill fingerprints —
// counts while folded, fingerprints when expanded — and nothing else.

import { readFileSync } from "node:fs";
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
let host: HTMLElement | null = null;

async function renderStrip(record: RecallRecordView | Error) {
  host = document.createElement("div");
  document.body.appendChild(host);
  root = createRoot(host);
  await act(async () => {
    root?.render(
      <LocaleProvider>
        <RecapRecallStrip
          sessionPath="C:\\sessions\\s.jsonl"
          recallRecord={async () => {
            if (record instanceof Error) throw record;
            return record;
          }}
        />
      </LocaleProvider>,
    );
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

const SENTINEL = "SENTINEL-BODY-TEXT";
const record: RecallRecordView = {
  available: true,
  sessionPath: "C:\\sessions\\s.jsonl",
  turns: [
    {
      turnSeq: 4,
      usedChars: 120,
      omitted: 2,
      hits: [
        { id: "mem-a", revision: 2, score: 0.9, injected: true },
        { id: "mem-b", revision: 1, score: 0.4 },
      ],
    },
    {
      turnSeq: 5,
      hits: [
        { id: "mem-c", revision: 1, score: 0.7, injected: true },
        { id: "mem-d", revision: 3, score: 0.6, injected: true },
      ],
    },
  ],
  skills: [
    { turnSeq: 4, name: "hot", contentHash: "abcdef0123456789", catalogDigest: "0123456789abcdef" },
    { turnSeq: 5, name: "warm", contentHash: "fedcba9876543210", catalogDigest: "fedcba9876543210" },
  ],
};

await renderStrip(record);
const head = host?.querySelector<HTMLButtonElement>(".recap-recall__head");
ok(head !== null, "the strip offers a fold control");
ok(head?.getAttribute("aria-expanded") === "false", "it starts folded");
const summary = head?.textContent ?? "";
// The record's own tri-state: mem-b carries no injected field, which is an
// unrecorded decision — counting it as dropped is the bug this pins.
ok(
  summary.includes("召回 3 条") && summary.includes("被挤掉 0 条") && summary.includes("另有 1 条未记录"),
  `the folded line separates a recorded drop from an unrecorded decision: ${summary}`,
);
ok(host?.querySelector(".recap-recall__body") === null, "a folded strip lists no fingerprints");
ok((host?.textContent ?? "").includes(SENTINEL) === false, "the folded view carries no body text");

await act(async () => {
  head?.click();
  await new Promise((resolve) => setTimeout(resolve, 0));
});
ok(host?.querySelector(".recap-recall__head")?.getAttribute("aria-expanded") === "true", "expanding flips the fold state");
const body = host?.querySelector(".recap-recall__body");
ok(body !== null, "the expanded strip lists the record");
const fingerprints = [...(host?.querySelectorAll(".recap-recall__fingerprint") ?? [])].map((node) => node.textContent ?? "");
ok(["mem-a", "mem-b", "mem-c", "mem-d"].every((id) => fingerprints.includes(id)), "every hit is listed by id");
const unrecordedRow = Array.from(host?.querySelectorAll(".recap-recall__hit") ?? []).find((row) =>
  (row.textContent ?? "").includes("mem-b"),
);
ok(
  (unrecordedRow?.textContent ?? "").includes("未记录"),
  "the row for an unrecorded decision says so instead of reading as dropped",
);
ok(fingerprints.includes("hot") && fingerprints.includes("warm"), "every skill is listed by name");
const meta = [...(host?.querySelectorAll(".recap-recall__meta") ?? [])].map((node) => node.textContent ?? "").join(" | ");
ok(meta.includes("r2") && meta.includes("0.90"), "a hit shows its revision and score");
ok(meta.includes("abcdef0123456789") && meta.includes("0123456789abcdef"), "a skill shows its content hash and catalog digest");
ok(meta.includes("2"), "the turn states how many hits were omitted");
ok((host?.textContent ?? "").includes(SENTINEL) === false, "the expanded view carries no body text");
const focused = host?.querySelector<HTMLButtonElement>(".recap-recall__head");
await act(async () => {
  focused?.click();
  await new Promise((resolve) => setTimeout(resolve, 0));
});
ok(host?.querySelector(".recap-recall__body") === null, "clicking again folds the strip");

// A session with nothing recorded, and a failed read, both render nothing.
await renderStrip({ available: false });
ok(host?.childElementCount === 0, "an unavailable record renders no strip");
await renderStrip(new Error("read failed"));
ok(host?.childElementCount === 0, "a failed read renders no strip rather than an error");

// The stylesheet must not reach for retired tokens.
const css = readFileSync("src/components/RecapRecallStrip.css", "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
for (const retired of ["--fg-muted", "--bg-elev-1", "--hover", "--border-strong", "--shadow"]) {
  ok(!css.includes(retired), `the strip stylesheet avoids the retired token ${retired}`);
}

process.stdout.write(`\nrecap recall strip: ${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exit(1);
