// Run: tsx src/__tests__/footer-changed-files-provenance.test.tsx

// Real-machine report: the same project's sessions disagreed (23 files vs 0) and
// refresh could not help. The list is per SESSION (the host unions this tab's
// checkpoints with git of this tab's root), so the module must name the
// workspace it inspected and mark the rows this session alone touched.

import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { FooterChangedFilesModule } from "../components/FooterChangedFilesModule";
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

const changes: WorkspaceChangesView = {
  files: [
    { path: "src/a.ts", sources: ["git"], gitStatus: "M" },
    { path: "src/b.ts", sources: ["session"] },
    { path: "src/c.ts", sources: ["git", "session"], gitStatus: "A" },
  ],
  gitAvailable: true,
  gitBranch: "dev-2",
};

async function renderModule(workspaceRoot: string | undefined) {
  const dom = installDom();
  installDesktopHostStub(({
    main: { App: { WorkspaceChanges: async () => changes } as Partial<AppBindings> as AppBindings },
  }).main.App);
  const rootEl = document.getElementById("root");
  if (!rootEl) throw new Error("missing root");
  const root = createRoot(rootEl);
  await act(async () => {
    root.render(
      <LocaleProvider>
        <FooterChangedFilesModule tabId="tab-a" workspaceScopeKey="scope-a" workspaceRoot={workspaceRoot} />
      </LocaleProvider>,
    );
    await flushPromises();
  });
  return { dom, root };
}

console.log("\nfooter changed-files provenance");

{
  const { dom, root } = await renderModule("/Users/guosj/Reasonix/global-workspace");
  await act(async () => {
    await waitFor("changes answer", () => document.querySelector(".footer-changed__row") !== null);
  });
  const workspace = document.querySelector(".footer-changed__workspace");
  ok(workspace !== null, "the bar names the workspace the list came from");
  ok(workspace?.textContent === "global-workspace", "the name is the workspace's own directory, not the project's");
  ok(workspace?.getAttribute("title") === "/Users/guosj/Reasonix/global-workspace", "the full inspected path rides the tooltip");
  ok(document.querySelectorAll(".footer-changed__row").length === 3, "every row still renders");
  ok(document.querySelectorAll(".footer-changed__source").length === 1, "only the session-only row is marked as this session's");
  ok(document.querySelectorAll(".footer-changed__row")[0]?.querySelector(".footer-changed__source") === null, "a git-reported row carries no session mark");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

{
  const { dom, root } = await renderModule(undefined);
  await act(async () => {
    await waitFor("changes answer without a root", () => document.querySelector(".footer-changed__row") !== null);
  });
  ok(document.querySelector(".footer-changed__workspace") === null, "no workspace name is invented when the host does not report one");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exitCode = 1;
