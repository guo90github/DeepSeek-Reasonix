// Run: tsx src/__tests__/footer-paging-steps.test.tsx

// Both list modules page in five rows per click and collapse once the whole
// answer is on screen — a 100-commit `git log` must never land in one click.

import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { FooterChangedFilesModule, FOOTER_CHANGED_FILES_INITIAL, FOOTER_CHANGED_FILES_PAGE } from "../components/FooterChangedFilesModule";
import { FooterGitHistoryModule, FOOTER_COMMITS_INITIAL, FOOTER_COMMITS_PAGE } from "../components/FooterGitHistoryModule";
import type { AppBindings } from "../lib/bridge";
import { LocaleProvider } from "../lib/i18n";
import type { GitCommitView, WorkspaceChangesView } from "../lib/types";
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

const TOTAL = 13;
const commits: GitCommitView[] = Array.from({ length: TOTAL }, (_unused, index) => ({
  hash: `${index}`.padStart(7, "b"),
  author: "guosj",
  date: "Mon Sep 29 13:20:00 2026 +0800",
  message: `提交 ${index}`,
}));
const changes: WorkspaceChangesView = {
  files: Array.from({ length: TOTAL }, (_unused, index) => ({ path: `src/p${index}.ts`, sources: ["git"], gitStatus: "M" })),
  gitAvailable: true,
  gitBranch: "dev-2",
};

async function renderModule(element: React.ReactElement) {
  const dom = installDom();
  installDesktopHostStub(({
    main: {
      App: {
        WorkspaceGitHistory: async () => commits,
        WorkspaceChanges: async () => changes,
      } as Partial<AppBindings> as AppBindings,
    },
  }).main.App);
  const rootEl = document.getElementById("root");
  if (!rootEl) throw new Error("missing root");
  const root = createRoot(rootEl);
  await act(async () => {
    root.render(<LocaleProvider>{element}</LocaleProvider>);
    await flushPromises();
  });
  return { dom, root };
}

async function clickMore() {
  await act(async () => {
    document.querySelector<HTMLButtonElement>(".footer-panel__more")?.click();
    await flushPromises();
  });
}

console.log("\nfooter list paging steps");

{
  const { dom, root } = await renderModule(<FooterGitHistoryModule tabId="tab-a" workspaceScopeKey="scope-a" />);
  await act(async () => {
    await waitFor("commit answer", () => document.querySelector(".footer-git__row") !== null);
  });
  const rowCount = () => document.querySelectorAll(".footer-git__row").length;
  ok(rowCount() === FOOTER_COMMITS_INITIAL, "commits start at the initial page");
  await clickMore();
  ok(rowCount() === FOOTER_COMMITS_INITIAL + FOOTER_COMMITS_PAGE, "one click pages in exactly one more page");
  ok(document.querySelector(".footer-panel__more .footer-panel__chevron--closed") !== null, "a partly paged list keeps the expand affordance");
  await clickMore();
  ok(rowCount() === TOTAL, "paging continues to the end of the answer");
  ok(document.querySelector(".footer-panel__more .footer-panel__chevron--closed") === null, "a fully shown list flips the affordance");
  await clickMore();
  ok(rowCount() === FOOTER_COMMITS_INITIAL, "the control collapses back to the first page");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

{
  const { dom, root } = await renderModule(<FooterChangedFilesModule tabId="tab-a" workspaceScopeKey="scope-a" />);
  await act(async () => {
    await waitFor("changes answer", () => document.querySelector(".footer-changed__row") !== null);
  });
  const rowCount = () => document.querySelectorAll(".footer-changed__row").length;
  ok(rowCount() === FOOTER_CHANGED_FILES_INITIAL, "changed files start at the initial page");
  await clickMore();
  ok(rowCount() === FOOTER_CHANGED_FILES_INITIAL + FOOTER_CHANGED_FILES_PAGE, "one click pages in exactly one more page");
  await clickMore();
  ok(rowCount() === TOTAL, "paging continues to the end of the list");
  await clickMore();
  ok(rowCount() === FOOTER_CHANGED_FILES_INITIAL, "the control collapses back to the first page");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exitCode = 1;
