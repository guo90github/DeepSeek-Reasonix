// Run: tsx src/__tests__/footer-memory-module.test.tsx

// The memory module is the panel's second module: it must show the recalled
// facts and the notes actually loaded for the workspace, and it must render
// nothing at all when the store reports nothing to show.

import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { FooterPanel } from "../components/FooterPanel";
import { FOOTER_PANEL_MODULES } from "../components/footerPanelModules";
import type { AppBindings } from "../lib/bridge";
import { LocaleProvider } from "../lib/i18n";
import type { MemoryView } from "../lib/types";
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

const view: MemoryView = {
  docs: [
    { path: "REASONIX.md", scope: "project", body: "# Reasonix project memory", imports: [], depth: 0, order: 0, precedence: 0 },
    { path: "AGENTS.md", scope: "user", body: "# Evolution Declaration", imports: [{ path: "x.md", sourcePath: "AGENTS.md" }], depth: 0, order: 1, precedence: 1 },
  ],
  facts: [
    {
      name: "deepseek-费用计算机制",
      title: "DeepSeek 费用计算机制",
      description: "token 来源=provider 终态 usage，按官方内置 catalog 计价。",
      type: "project",
      scope: "project",
      body: "…",
      freshness: "fresh",
    },
    {
      name: "old-note",
      description: "一条已经过时的笔记",
      type: "reference",
      scope: "global",
      body: "…",
      freshness: "stale",
    },
  ],
  archives: [],
  scopes: [{ scope: "project", path: "/repo/REASONIX.md" }],
  instructionDiagnostics: [],
  conflicts: [],
  lastRecall: { query: "分栏 输入框", hits: [], omitted: 2, charBudget: 6000, usedChars: 1500 },
  storeDir: "/home/.reasonix/memory",
  available: true,
};

async function renderPanel(memory: MemoryView) {
  const dom = installDom();
  const asked = { count: 0 };
  installDesktopHostStub(({
    main: {
      App: {
        // Parked: this file asserts on the memory module only, and a module with
        // no answer renders nothing.
        WorkspaceChanges: () => new Promise(() => {}),
        MemoryForTab: async () => {
          asked.count += 1;
          return memory;
        },
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

console.log("\nfooter memory module");

{
  const { dom, root, asked } = await renderPanel(view);
  await act(async () => {
    await waitFor("memory answer", () => asked.count === 1);
  });
  ok(document.querySelector(".footer-memory") !== null, "the memory section renders for an available store");
  ok(document.body.textContent?.includes("DeepSeek 费用计算机制") === true, "the section lists the recalled fact titles");
  ok(document.body.textContent?.includes("一条已经过时的笔记") === true, "a stale fact still renders, marked stale");
  ok(document.querySelector(".footer-memory__row--stale") !== null, "a stale fact carries the stale marker");
  ok(document.body.textContent?.includes("REASONIX.md") === true, "the notes section lists the loaded instruction files");
  ok(document.body.textContent?.includes("1.5k/6.0k") === true, "the recall bar shows the character budget in use");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

{
  const { dom, root, asked } = await renderPanel({
    docs: [],
    facts: [],
    archives: [],
    scopes: [],
    instructionDiagnostics: [],
    conflicts: [],
    lastRecall: { query: "", hits: [], omitted: 0, charBudget: 0, usedChars: 0 },
    storeDir: "",
    available: false,
  });
  await act(async () => {
    await waitFor("empty memory answer", () => asked.count === 1);
  });
  ok(document.querySelector(".footer-memory") === null, "an unavailable, empty store renders nothing at all");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exitCode = 1;
