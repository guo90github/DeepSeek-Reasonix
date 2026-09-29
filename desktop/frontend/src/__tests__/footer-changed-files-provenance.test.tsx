// Run: tsx src/__tests__/footer-changed-files-provenance.test.tsx

// The session module lists what THIS session changed, and nothing else: the
// host's answer unions the tab's checkpoints with git of its workspace root, so
// the git-sourced rows belong to FooterGitUncommittedModule instead. The bar
// names the workspace, because a session bound elsewhere is exactly why two
// sessions of one project used to disagree.

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
    { path: "src/git-only.ts", sources: ["git"], gitStatus: "M" },
    { path: "src/session-only.ts", sources: ["session"] },
    { path: "src/both.ts", sources: ["git", "session"], gitStatus: "A" },
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

console.log("\nfooter session-changes provenance");

{
  const { dom, root } = await renderModule("/Users/guosj/Reasonix/global-workspace");
  await act(async () => {
    await waitFor("session changes answer", () => document.querySelector(".footer-changed__row") !== null);
  });
  const rows = Array.from(document.querySelectorAll(".footer-changed__row"));
  ok(rows.length === 2, "only the rows this session touched are listed");
  ok(document.body.textContent?.includes("src/session-only.ts") === true, "a checkpoint-only file is listed");
  ok(document.body.textContent?.includes("src/both.ts") === true, "a file both git and this session touched is listed");
  ok(document.body.textContent?.includes("src/git-only.ts") === false, "a git-only file belongs to the git module, not here");

  const workspace = document.querySelector(".footer-changed__workspace");
  ok(workspace !== null, "the bar names the workspace the session ran in");
  ok(workspace?.textContent === "global-workspace", "the name is the session's own workspace directory");
  ok(workspace?.getAttribute("title") === "/Users/guosj/Reasonix/global-workspace", "the full path rides the tooltip");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

{
  const { dom, root } = await renderModule(undefined);
  await act(async () => {
    await waitFor("session changes answer without a root", () => document.querySelector(".footer-changed__row") !== null);
  });
  ok(document.querySelector(".footer-changed__workspace") === null, "no workspace name is invented when the host does not report one");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exitCode = 1;
