// Run: tsx src/__tests__/footer-git-history-module.test.tsx

// The git-history module: five commits to start, the rest behind one button, and
// nothing at all where git cannot answer (a workspace without git fails the
// `git log` call the module makes).

import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { FooterGitHistoryModule, FOOTER_COMMITS_INITIAL } from "../components/FooterGitHistoryModule";
import type { AppBindings } from "../lib/bridge";
import { LocaleProvider } from "../lib/i18n";
import type { GitCommitView } from "../lib/types";
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

function commit(index: number): GitCommitView {
  return {
    hash: `${index}`.padStart(7, "a"),
    author: "guosj",
    date: "Mon Sep 29 13:20:00 2026 +0800",
    message: `第 ${index} 次提交`,
  };
}

async function renderModule(history: () => Promise<GitCommitView[]>) {
  const dom = installDom();
  const asked = { count: 0 };
  installDesktopHostStub(({
    main: {
      App: {
        WorkspaceGitHistory: async () => {
          asked.count += 1;
          return history();
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
        <FooterGitHistoryModule tabId="tab-a" workspaceScopeKey="scope-a" />
      </LocaleProvider>,
    );
    await flushPromises();
  });
  return { dom, root, asked };
}

console.log("\nfooter git history module");

{
  const { dom, root, asked } = await renderModule(async () => [1, 2, 3, 4, 5, 6, 7].map(commit));
  await act(async () => {
    await waitFor("commit history answer", () => asked.count === 1);
  });
  ok(document.querySelectorAll(".footer-git__row").length === FOOTER_COMMITS_INITIAL, `the section starts with ${FOOTER_COMMITS_INITIAL} commits`);
  ok(document.body.textContent?.includes("第 1 次提交") === true, "the first entry of the answer is on screen");
  ok(document.body.textContent?.includes("第 6 次提交") === false, "entries past the initial window stay hidden");
  const more = document.querySelector<HTMLButtonElement>(".footer-git__more");
  ok(more !== null, "a workspace with more commits offers a show-more control");

  await act(async () => {
    more?.click();
    await flushPromises();
  });
  ok(document.querySelectorAll(".footer-git__row").length === 7, "show more reveals every commit");
  ok(document.body.textContent?.includes("第 7 次提交") === true, "the last entry of the answer is revealed");

  await act(async () => {
    document.querySelector<HTMLButtonElement>(".footer-git__more")?.click();
    await flushPromises();
  });
  ok(document.querySelectorAll(".footer-git__row").length === FOOTER_COMMITS_INITIAL, "the control collapses back again");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

{
  const { dom, root, asked } = await renderModule(async () => []);
  await act(async () => {
    await waitFor("empty history answer", () => asked.count === 1);
  });
  ok(document.querySelector(".footer-git") !== null, "a git-backed workspace with no commits still shows the section");
  ok(document.querySelectorAll(".footer-git__row").length === 0, "an empty history lists no rows");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

{
  const { dom, root, asked } = await renderModule(async () => {
    throw new Error("fatal: not a git repository (or any of the parent directories): .git");
  });
  await act(async () => {
    await waitFor("failed history answer", () => asked.count === 1);
  });
  ok(document.querySelector(".footer-git") === null, "a workspace without git renders no history section");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exitCode = 1;
