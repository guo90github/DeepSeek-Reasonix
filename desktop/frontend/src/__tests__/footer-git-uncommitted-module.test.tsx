// Run: tsx src/__tests__/footer-git-uncommitted-module.test.tsx

// The git-uncommitted module answers about the WORKSPACE, not the session: it
// reads WorkspaceGitStatsForTab(tabID, workspaceRoot) (git only), so every
// session of one workspace reads the same list, and a workspace without git
// renders nothing.

import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { FooterGitUncommittedModule } from "../components/FooterGitUncommittedModule";
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

const stats: WorkspaceChangesView = {
  files: [
    { path: "src/a.ts", sources: ["git"], gitStatus: "M" },
    { path: "src/b.ts", sources: ["git"], gitStatus: "??" },
  ],
  gitAvailable: true,
  gitBranch: "dev-2",
  added: 7,
  removed: 1,
};

async function renderModule(workspaceRoot: string | undefined, answer: WorkspaceChangesView) {
  const dom = installDom();
  const asked = { count: 0, roots: [] as string[] };
  installDesktopHostStub(({
    main: {
      App: {
        WorkspaceGitStatsForTab: async (_tabID: string, root: string) => {
          asked.count += 1;
          asked.roots.push(root);
          return answer;
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
        <FooterGitUncommittedModule tabId="tab-a" workspaceScopeKey="scope-a" workspaceRoot={workspaceRoot} />
      </LocaleProvider>,
    );
    await flushPromises();
  });
  return { dom, root, asked };
}

console.log("\nfooter git uncommitted module");

{
  const { dom, root, asked } = await renderModule("/Users/guosj/proj/DeepSeek-Reasonix", stats);
  await act(async () => {
    await waitFor("git stats answer", () => asked.count === 1);
  });
  ok(document.querySelector(".footer-changed") !== null, "a git-backed workspace renders the section");
  ok(document.querySelectorAll(".footer-changed__row").length === 2, "every uncommitted file is listed");
  ok(document.body.textContent?.includes("src/b.ts") === true, "an untracked file is listed too");
  ok(document.body.textContent?.includes("dev-2") === true, "the bar shows the branch");
  ok(document.body.textContent?.includes("+7") === true, "the bar shows the diff tally");
  ok(document.querySelector(".footer-changed__workspace")?.textContent === "DeepSeek-Reasonix", "the bar names the workspace asked about");
  ok(asked.roots[0] === "/Users/guosj/proj/DeepSeek-Reasonix", "the workspace root is passed to git, not read from the tab");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

{
  const { dom, root, asked } = await renderModule("/Users/guosj/proj/not-a-repo", {
    files: [],
    gitAvailable: false,
    gitErr: "fatal: not a git repository",
  });
  await act(async () => {
    await waitFor("git stats answer without git", () => asked.count === 1);
  });
  ok(document.querySelector(".footer-changed") === null, "a workspace where git cannot answer renders no section");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

{
  const { dom, root, asked } = await renderModule(undefined, stats);
  await act(async () => {
    await flushPromises();
  });
  ok(asked.count === 0, "without a workspace root nothing is asked of git");
  ok(document.querySelector(".footer-changed") === null, "without a workspace root no section renders");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exitCode = 1;
