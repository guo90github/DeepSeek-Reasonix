// Run: tsx src/__tests__/footer-panel.test.ts

import {
  FOOTER_PANEL_KEYBOARD_STEP,
  FOOTER_PANEL_SHARE_DEFAULT,
  FOOTER_PANEL_SHARE_MAX,
  FOOTER_PANEL_SHARE_MIN,
  clampFooterPanelShare,
  footerPanelShareFromPointer,
  stepFooterPanelShare,
} from "../lib/footerPanel";

let passed = 0;
let failed = 0;

function eq<T>(a: T, b: T, label: string) {
  if (a === b) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}: expected ${JSON.stringify(b)}, got ${JSON.stringify(a)}\n`);
    failed += 1;
  }
}

eq(clampFooterPanelShare(38), 38, "an in-range share is kept");
eq(clampFooterPanelShare(5), FOOTER_PANEL_SHARE_MIN, "a share below the floor clamps up");
eq(clampFooterPanelShare(95), FOOTER_PANEL_SHARE_MAX, "a share above the ceiling clamps down");
eq(clampFooterPanelShare(Number.NaN), FOOTER_PANEL_SHARE_DEFAULT, "a non-finite share falls back to the default");
eq(clampFooterPanelShare(37.6), 38, "the share rounds to a whole percent");

// The panel is the band's right-hand track: the pointer's distance to the right
// edge is the share it gets.
eq(footerPanelShareFromPointer({ clientX: 620, bandLeft: 0, bandWidth: 1000 }), 38, "a pointer at 62% of the band gives the panel 38%");
eq(footerPanelShareFromPointer({ clientX: 0, bandLeft: 0, bandWidth: 1000 }), FOOTER_PANEL_SHARE_MAX, "a pointer at the left edge clamps to the ceiling");
eq(footerPanelShareFromPointer({ clientX: 1000, bandLeft: 0, bandWidth: 1000 }), FOOTER_PANEL_SHARE_MIN, "a pointer at the right edge clamps to the floor");
eq(footerPanelShareFromPointer({ clientX: 700, bandLeft: 200, bandWidth: 1000 }), 50, "a shifted band measures from its own left edge");
eq(footerPanelShareFromPointer({ clientX: 700, bandLeft: 200, bandWidth: 0 }), FOOTER_PANEL_SHARE_DEFAULT, "a zero-width band keeps the default");

eq(stepFooterPanelShare({ current: 38, delta: FOOTER_PANEL_KEYBOARD_STEP }), 42, "a positive step widens the panel");
eq(stepFooterPanelShare({ current: 38, delta: -FOOTER_PANEL_KEYBOARD_STEP }), 34, "a negative step narrows the panel");
eq(stepFooterPanelShare({ current: FOOTER_PANEL_SHARE_MIN, delta: -FOOTER_PANEL_KEYBOARD_STEP }), FOOTER_PANEL_SHARE_MIN, "stepping past the floor sticks to the floor");
eq(stepFooterPanelShare({ current: FOOTER_PANEL_SHARE_MAX, delta: FOOTER_PANEL_KEYBOARD_STEP }), FOOTER_PANEL_SHARE_MAX, "stepping past the ceiling sticks to the ceiling");

process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exitCode = 1;
