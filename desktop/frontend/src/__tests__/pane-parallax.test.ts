// Run: tsx src/__tests__/pane-parallax.test.ts

import { JSDOM } from "jsdom";
import { attachPaneParallax } from "../lib/paneParallax";

let passed = 0;
let failed = 0;

function eq<T>(actual: T, expected: T, label: string) {
  if (actual === expected) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}: expected ${String(expected)}, got ${String(actual)}\n`);
    failed += 1;
  }
}

function makeDom(): { dom: JSDOM; scroller: HTMLElement; root: HTMLElement } {
  const dom = new JSDOM(
    '<div class="conversation-pane"><div data-virtuoso-scroller style="overflow-y: auto;"></div></div>',
    { pretendToBeVisual: true },
  );
  const root = dom.window.document.querySelector(".conversation-pane") as HTMLElement;
  const scroller = dom.window.document.querySelector("[data-virtuoso-scroller]") as HTMLElement;
  Object.defineProperty(scroller, "scrollTop", { value: 0, writable: true, configurable: true });
  return { dom, scroller, root };
}

console.log("\npane parallax");

// Scroll events write the (scaled) offset onto the pane root as a CSS var.
{
  const { dom, scroller, root } = makeDom();
  const detach = attachPaneParallax(scroller);
  scroller.scrollTop = 100;
  scroller.dispatchEvent(new dom.window.Event("scroll"));
  eq(root.style.getPropertyValue("--pane-scroll-y"), "30", "scroll offset is scaled by the parallax factor");
  eq(detach !== undefined, true, "attach returns a detach");
  detach?.();
}

// The variable is written immediately on attach (before any scroll).
{
  const { scroller, root } = makeDom();
  attachPaneParallax(scroller);
  eq(root.style.getPropertyValue("--pane-scroll-y"), "0", "initial offset is written on attach");
}

// Detach removes the listener and the CSS variable.
{
  const { dom, scroller, root } = makeDom();
  const detach = attachPaneParallax(scroller);
  detach?.();
  scroller.scrollTop = 50;
  scroller.dispatchEvent(new dom.window.Event("scroll"));
  eq(root.style.getPropertyValue("--pane-scroll-y"), "", "detach clears the CSS variable");
}

// A scroller without a matching pane root is a no-op.
{
  const dom = new JSDOM('<div><div data-virtuoso-scroller></div></div>', { pretendToBeVisual: true });
  const scroller = dom.window.document.querySelector("[data-virtuoso-scroller]") as HTMLElement;
  eq(attachPaneParallax(scroller), undefined, "non-pane scroller returns no detach");
}

// Null scroller is a no-op.
eq(attachPaneParallax(null), undefined, "null scroller returns no detach");

console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
