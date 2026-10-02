// Run: tsx src/__tests__/agentbus-section.test.tsx
//
// The section owns two things only: it fetches, and it distinguishes "cannot read"
// from "nothing wrong". The card rendering itself is covered by agentbus-panel.test.

import { JSDOM } from "jsdom";
import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { WorkspaceAgentBusSection } from "../components/WorkspaceAgentBusSection";
import { LocaleProvider } from "../lib/i18n";

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

const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>");
(globalThis as unknown as { document: Document }).document = dom.window.document;
(globalThis as unknown as { window: Window }).window = dom.window as unknown as Window;
(globalThis as unknown as { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

async function renderSection(node: HTMLElement) {
  const root = createRoot(node);
  await act(async () => {
    root.render(
      <LocaleProvider>
        <WorkspaceAgentBusSection />
      </LocaleProvider>,
    );
  });
  return root;
}

const quiet = dom.window.document.createElement("div");
const first = await renderSection(quiet);
ok(
  quiet.textContent !== null && quiet.textContent.length > 0,
  "a readable briefing renders something visible",
);
ok(
  quiet.textContent?.includes("unavailable") !== true,
  "a readable briefing is not reported as unavailable",
);
await act(async () => {
  first.unmount();
});

const broken = dom.window.document.createElement("div");
const second = createRoot(broken);
await act(async () => {
  second.render(
    <LocaleProvider>
      <WorkspaceAgentBusSection
        load={async () => {
          throw new Error("host unreachable");
        }}
      />
    </LocaleProvider>,
  );
});
ok(
  broken.textContent?.includes("unavailable") === true ||
    broken.textContent?.includes("取不到") === true,
  "an unreadable briefing says so instead of drawing an empty board",
);
await act(async () => {
  second.unmount();
});

process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exit(1);
