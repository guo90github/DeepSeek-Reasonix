// Run: tsx src/__tests__/footer-panel-visibility.test.tsx

// The reported defects: a workspace without git still showed a "changed files"
// section, and switching project blanked the section while its answer was in
// flight. Both are asserted on the section's presence and on the answer having
// arrived — not on the panel's box, which is CSS and jsdom has no layout for.

import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { FooterPanel } from "../components/FooterPanel";
import { FOOTER_PANEL_MODULES } from "../components/footerPanelModules";
import type { AppBindings } from "../lib/bridge";
import { LocaleProvider } from "../lib/i18n";
import type { WorkspaceChangesView } from "../lib/types";
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

// The tabId default is deliberately absent: passing `undefined` must mean "no
// session", not "use tab-a" (a default parameter would swallow it).
async function renderPanel(changes: WorkspaceChangesView, tabId: string | undefined) {
  const dom = installDom();
  const answers = { count: 0 };
  installDesktopHostStub(({
    main: {
      App: {
        WorkspaceChanges: async () => {
          answers.count += 1;
          return changes;
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
        <FooterPanel modules={FOOTER_PANEL_MODULES} context={{ tabId, workspaceScopeKey: "scope-a" }} />
      </LocaleProvider>,
    );
    await flushPromises();
  });
  return { dom, root, answers };
}

console.log("\nfooter panel module visibility");

{
  const { dom, root, answers } = await renderPanel({
    files: [
      { path: "src/a.ts", sources: ["git"], gitStatus: "M" },
      { path: "src/b.ts", sources: ["git"], gitStatus: "A" },
    ],
    gitAvailable: true,
    gitBranch: "dev-2",
    added: 7,
    removed: 1,
  }, "tab-a");
  await act(async () => {
    await waitFor("changed files answer", () => answers.count === 1);
  });
  ok(document.querySelector(".footer-panel__section") !== null, "a git-backed workspace renders the changed-files section");
  ok(document.body.textContent?.includes("src/a.ts") === true, "the section lists the changed file paths");
  ok(document.body.textContent?.includes("dev-2") === true, "the section shows the branch");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

{
  const { dom, root, answers } = await renderPanel({ files: [], gitAvailable: false }, "tab-a");
  await act(async () => {
    await waitFor("changes answer without git", () => answers.count === 1);
  });
  ok(document.querySelector(".footer-panel__section") === null, "a workspace without git renders no section at all");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

{
  const { dom, root, answers } = await renderPanel({ files: [], gitAvailable: true }, undefined);
  await act(async () => {
    await flushPromises();
  });
  ok(answers.count === 0, "no session tab asks for changes at all");
  ok(document.querySelector(".footer-panel__section") === null, "no session tab renders no section");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

{
  // workspaceScopeKey carries the tab id and the session generation, so a
  // switch changes it and the resource drops to "no answer yet". The section
  // must survive that window: it is what the user was reading.
  const dom = installDom();
  let releaseSecond: (() => void) | null = null;
  let calls = 0;
  installDesktopHostStub(({
    main: {
      App: {
        WorkspaceChanges: async () => {
          calls += 1;
          if (calls === 1) return { files: [{ path: "src/a.ts", sources: ["git"] }], gitAvailable: true, gitBranch: "dev-2" };
          await new Promise<void>((resolve) => {
            releaseSecond = resolve;
          });
          return { files: [{ path: "src/b.ts", sources: ["git"] }], gitAvailable: true, gitBranch: "dev-3" };
        },
      } as Partial<AppBindings> as AppBindings,
    },
  }).main.App);
  const rootEl = document.getElementById("root");
  if (!rootEl) throw new Error("missing root");
  const root = createRoot(rootEl);
  const renderAt = (tabId: string, workspaceScopeKey: string) => (
    <LocaleProvider>
      <FooterPanel modules={FOOTER_PANEL_MODULES} context={{ tabId, workspaceScopeKey }} />
    </LocaleProvider>
  );

  await act(async () => {
    root.render(renderAt("tab-a", "scope-a"));
    await flushPromises();
  });
  await act(async () => {
    await waitFor("first changed files answer", () => document.body.textContent?.includes("src/a.ts") === true);
  });
  await act(async () => {
    root.render(renderAt("tab-b", "scope-b"));
    await flushPromises();
  });
  ok(document.querySelector(".footer-panel__section") !== null, "a scope switch keeps the section while its reload is in flight");

  await act(async () => {
    releaseSecond?.();
    await flushPromises();
  });
  await act(async () => {
    await waitFor("second changed files answer", () => document.body.textContent?.includes("src/b.ts") === true);
  });
  ok(document.body.textContent?.includes("src/b.ts") === true, "the section repaints with the new workspace's files");

  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exitCode = 1;
