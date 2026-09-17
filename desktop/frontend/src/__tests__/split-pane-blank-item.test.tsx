// Run: node --import ./scripts/css-stub-register.mjs --import ./scripts/svg-stub-register.mjs --import tsx src/__tests__/split-pane-blank-item.test.tsx
//
// Virtuoso measures every rendered row through its default div and logs
// "Zero-sized element" for one that measures 0, on every measurement pass. A
// turn that paints nothing must therefore still mount a measurable row: 1px and
// empty, rather than 0px or no element at all.

import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { ConversationPaneItem } from "../components/ConversationPane";
import { ProcessPaneItem } from "../components/ProcessPane";
import type { ConversationPaneTurn, ProcessPaneTurn } from "../lib/transcriptPanes";

let passed = 0;
let failed = 0;

function check(condition: unknown, label: string) {
  if (condition) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

console.log("\nsplit pane blank items");

const listProps = { "data-index": 3, "data-item-index": 3, "data-known-size": 0, style: { height: 0 } };

const blankConversation: ConversationPaneTurn = {
  key: "k-blank",
  turn: undefined,
  user: undefined,
  answers: [],
  hasShownContent: false,
  isActive: false,
};
const shownConversation: ConversationPaneTurn = { ...blankConversation, key: "k-shown", turn: 0 };
const blankProcess: ProcessPaneTurn = {
  key: "k-blank",
  turn: undefined,
  question: "",
  segments: [],
  hasShownContent: false,
  isActive: false,
  durationMs: 0,
};
const shownProcess: ProcessPaneTurn = { ...blankProcess, key: "k-shown", turn: 0, question: "q" };

const card = createElement("article", { className: "card" });
const conversationBlank = renderToStaticMarkup(createElement(ConversationPaneItem, { ...listProps, item: blankConversation }, card));
const conversationShown = renderToStaticMarkup(createElement(ConversationPaneItem, { ...listProps, item: shownConversation }, card));
const processBlank = renderToStaticMarkup(createElement(ProcessPaneItem, { ...listProps, item: blankProcess }, card));
const processShown = renderToStaticMarkup(createElement(ProcessPaneItem, { ...listProps, item: shownProcess }, card));

check(conversationBlank.includes('data-index="3"'), "a blank conversation turn stays measurable for Virtuoso");
check(conversationBlank.includes("min-height:1px"), "the blank conversation row is pinned above zero height");
check(conversationBlank.includes('aria-hidden="true"'), "the blank conversation row is hidden from assistive tech");
check(!conversationBlank.includes("card"), "the blank conversation row paints no card");
check(processBlank.includes('data-index="3"'), "a blank process turn stays measurable for Virtuoso");
check(processBlank.includes("min-height:1px"), "the blank process row is pinned above zero height");
check(processBlank.includes('aria-hidden="true"'), "the blank process row is hidden from assistive tech");
check(!processBlank.includes("card"), "the blank process row paints no card");
check(conversationShown.includes('data-index="3"'), "a painted conversation turn keeps Virtuoso's measurement attributes");
check(processShown.includes('data-index="3"'), "a painted process turn keeps Virtuoso's measurement attributes");
check(conversationShown.includes('data-known-size="0"'), "the known-size attribute reaches the row element");
check(conversationShown.includes("card"), "the painted turn still renders its card");
check(!conversationShown.includes("min-height:1px"), "a painted turn keeps its own height, not the blank row's placeholder");

if (failed > 0) {
  process.stdout.write(`\nsplit-pane-blank-item: ${failed} FAILED, ${passed} passed\n`);
  process.exitCode = 1;
} else {
  process.stdout.write(`\nsplit-pane-blank-item: ${passed} passed\n`);
}
