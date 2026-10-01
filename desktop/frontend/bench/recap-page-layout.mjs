#!/usr/bin/env node
// Layout gate for the 会话回顾 page (U-5 of docs/60-第十四 §2.4): the page owns one
// bounded rail and one scroller, so no number of reports or notes may squeeze the
// card list out of the window. jsdom can only assert the inline values; the real
// contract is geometry, which needs a real engine.
//
// Usage: node bench/recap-page-layout.mjs   (uses the Vite dev server, no build)
// Screenshots land in the OS temp dir, never in the repo.
import assert from "node:assert/strict";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createServer } from "vite";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
process.env.PLAYWRIGHT_BROWSERS_PATH = !process.env.PLAYWRIGHT_BROWSERS_PATH || process.env.PLAYWRIGHT_BROWSERS_PATH === ".pw-browsers"
  ? path.join(root, ".pw-browsers")
  : process.env.PLAYWRIGHT_BROWSERS_PATH;
const { chromium } = await import("playwright");

const LIST_FLOOR = 160;
const SIZES = [[1280, 800], [720, 600]];

// The pinned browser build and the installed Playwright can drift apart (the vendored
// revision here is older than what this Playwright wants), so resolve a usable engine
// instead of failing on a download: explicit path, then the OS browser, then default.
async function launchChromium() {
  const explicit = process.env.REASONIX_BENCH_CHROMIUM;
  if (explicit) return chromium.launch({ headless: true, executablePath: explicit });
  for (const channel of ["chrome", "msedge"]) {
    try {
      return await chromium.launch({ headless: true, channel });
    } catch {
      // Not installed, or not launchable — try the next choice.
    }
  }
  return chromium.launch({ headless: true });
}

let browser;
let server;
const failures = [];
const check = (ok, message) => {
  if (ok) console.log(`PASS ${message}`);
  else { console.log(`FAIL ${message}`); failures.push(message); }
};
const frames = (page) => page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))));

const measure = (page) => page.evaluate(() => {
  const list = document.querySelector(".history-list.recap-page__list");
  const panel = document.querySelector(".recap-page__panel");
  const head = document.querySelector(".recap-page__head");
  const column = document.querySelector(".recap-page");
  const content = document.querySelector(".management-screen__content");
  const screen = document.querySelector(".management-screen");
  const rect = (element) => {
    const box = element.getBoundingClientRect();
    return { top: box.top, bottom: box.bottom, height: box.height };
  };
  return {
    viewportHeight: window.innerHeight,
    list: { ...rect(list), client: list.clientHeight, scroll: list.scrollHeight, scrollTop: list.scrollTop },
    panel: { ...rect(panel), client: panel.clientHeight },
    head: rect(head),
    column: rect(column),
    content: rect(content),
    screen: rect(screen),
    cards: list.querySelectorAll("li").length,
  };
});

try {
  server = await createServer({ root, logLevel: "error", server: { host: "127.0.0.1", port: 0 } });
  await server.listen();
  browser = await launchChromium();
  const base = server.resolvedUrls.local[0];
  const pageErrors = [];

  for (const [width, height] of SIZES) {
    const context = await browser.newContext({ viewport: { width, height } });
    const page = await context.newPage();
    page.on("pageerror", (error) => pageErrors.push(`${width}x${height}: ${error.message}`));
    await page.goto(`${base}bench/recap-page-layout.html`);
    await page.waitForSelector(".history-list.recap-page__list li", { timeout: 20_000 });
    await frames(page);

    const before = await measure(page);
    const label = `${width}x${height}`;
    check(before.cards > 0 && before.list.scroll > before.list.client,
      `${label}: a ${before.cards}-card list actually scrolls (${before.list.scroll} > ${before.list.client})`);
    check(before.list.client >= LIST_FLOOR,
      `${label}: the card list keeps its ${LIST_FLOOR}px floor (${before.list.client}px)`);
    check(before.panel.client <= Math.ceil(height * 0.4) + 1,
      `${label}: the rail is capped at 40vh (${before.panel.client}px)`);
    check(before.panel.client >= 40,
      `${label}: the rail keeps a readable strip instead of a sliver (${before.panel.client}px)`);
    check(before.panel.bottom <= before.list.top + 1,
      `${label}: the rail never covers the list (rail bottom ${Math.round(before.panel.bottom)} <= list top ${Math.round(before.list.top)})`);
    check(before.list.top >= before.head.bottom - 1,
      `${label}: the list starts below the fixed head (${Math.round(before.list.top)} >= ${Math.round(before.head.bottom)})`);
    check(before.column.bottom <= before.content.bottom + 1,
      `${label}: the recap column fits its content box (column ${Math.round(before.column.top)}..${Math.round(before.column.bottom)} in ${Math.round(before.content.top)}..${Math.round(before.content.bottom)})`);
    check(before.list.bottom <= before.column.bottom + 1,
      `${label}: the list is not clipped by the column (list bottom ${Math.round(before.list.bottom)} <= column bottom ${Math.round(before.column.bottom)})`);
    check(Math.round(before.screen.height) === before.viewportHeight,
      `${label}: the page fills the window exactly (${Math.round(before.screen.height)} of ${before.viewportHeight})`);
    // The rail's own first report must be fully readable: with a squeezed rail the
    // summary row is cut in half, which is what the rail floor above guards.
    const firstSummary = await page.evaluate(() => {
      const summary = document.querySelector("details.recap-fold summary").getBoundingClientRect();
      const panel = document.querySelector(".recap-page__panel").getBoundingClientRect();
      return { summaryTop: summary.top, summaryBottom: summary.bottom, panelTop: panel.top, panelBottom: panel.bottom };
    });
    check(firstSummary.summaryTop >= firstSummary.panelTop - 1 && firstSummary.summaryBottom <= firstSummary.panelBottom + 1,
      `${label}: the rail's first report is readable (${Math.round(firstSummary.summaryTop)}..${Math.round(firstSummary.summaryBottom)} inside ${Math.round(firstSummary.panelTop)}..${Math.round(firstSummary.panelBottom)})`);

    // Scroll to the very bottom: the last card must be inside the list viewport,
    // and the head must not have moved (it is not part of the scroller).
    await page.evaluate(() => {
      const list = document.querySelector(".history-list.recap-page__list");
      list.scrollTop = list.scrollHeight;
    });
    await frames(page);
    const tail = await page.evaluate(() => {
      const list = document.querySelector(".history-list.recap-page__list");
      const cards = [...list.querySelectorAll("li")];
      const last = cards[cards.length - 1].getBoundingClientRect();
      const first = cards[0].getBoundingClientRect();
      const box = list.getBoundingClientRect();
      const head = document.querySelector(".recap-page__head").getBoundingClientRect();
      return { lastTop: last.top, lastBottom: last.bottom, firstTop: first.top, listTop: box.top, listBottom: box.bottom, headTop: head.top };
    });
    check(tail.lastBottom <= tail.listBottom + 1 && tail.lastTop >= tail.listTop - 1,
      `${label}: the last card is reachable by scrolling (${Math.round(tail.lastTop)}..${Math.round(tail.lastBottom)} inside ${Math.round(tail.listTop)}..${Math.round(tail.listBottom)})`);
    check(Math.abs(tail.headTop - before.head.top) <= 1,
      `${label}: the head stays put while the list scrolls (${Math.round(tail.headTop)} vs ${Math.round(before.head.top)})`);

    // Back to the top: the first card is reachable, and folding the rail open must
    // not steal height from the list.
    await page.evaluate(() => {
      const list = document.querySelector(".history-list.recap-page__list");
      list.scrollTop = 0;
    });
    await frames(page);
    const head = await page.evaluate(() => {
      const list = document.querySelector(".history-list.recap-page__list");
      const first = list.querySelector("li").getBoundingClientRect();
      const box = list.getBoundingClientRect();
      return { firstTop: first.top, listTop: box.top, listBottom: box.bottom };
    });
    check(head.firstTop >= head.listTop - 1 && head.firstTop < head.listBottom,
      `${label}: the first card is reachable at the top (${Math.round(head.firstTop)})`);

    const folds = await page.locator("details.recap-fold").count();
    check(folds >= 4, `${label}: the rail folds its four reports (${folds})`);
    const closed = await page.evaluate(() => [...document.querySelectorAll("details.recap-fold")].every((fold) => fold.open === false));
    check(closed, `${label}: every rail report starts folded`);
    await page.locator("details.recap-fold summary").first().click();
    await frames(page);
    const opened = await page.evaluate(() => {
      const fold = document.querySelector("details.recap-fold");
      const list = document.querySelector(".history-list.recap-page__list");
      const panel = document.querySelector(".recap-page__panel");
      return { open: fold.open, listClient: list.clientHeight, panelClient: panel.clientHeight };
    });
    check(opened.open === true, `${label}: clicking a summary opens the folded report`);
    check(opened.listClient >= LIST_FLOOR,
      `${label}: an opened report still leaves the list its floor (${opened.listClient}px)`);
    check(opened.panelClient <= Math.ceil(height * 0.4) + 1,
      `${label}: an opened report stays inside the 40vh cap (${opened.panelClient}px)`);

    const shot = path.join(os.tmpdir(), `reasonix-recap-page-${width}x${height}.png`);
    await page.screenshot({ path: shot });
    console.log(`     screenshot: ${shot}`);
    await context.close();
  }

  check(pageErrors.length === 0, `no browser errors${pageErrors.length ? ": " + pageErrors.join("; ") : ""}`);
} finally {
  await browser?.close();
  await server?.close();
}

if (failures.length > 0) {
  console.error(`\nrecap page layout: ${failures.length} failed`);
  process.exit(1);
}
console.log("\nrecap page layout: OK");
