// Run: tsx src/__tests__/footer-changed-files-paging.test.tsx

// A long session must not push the band out of shape: the session-changes list
// starts with five rows and pages the rest in on demand, like the commits.

import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { FooterChangedFilesModule, FOOTER_CHANGED_FILES_INITIAL } from "../components/FooterChangedFilesModule";
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
  files: [1, 2, 3, 4, 5, 6, 7].map((index) => ({ path: `src/f${index}.ts`, sources: ["session"], gitStatus: "M" })),
  gitAvailable: true,
  gitBranch: "dev-2",
};

async function renderModule() {
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
        <FooterChangedFilesModule tabId="tab-a" workspaceScopeKey="scope-a" />
      </LocaleProvider>,
    );
    await flushPromises();
  });
  return { dom, root };
}

console.log("\nfooter session-changes paging");

{
  const { dom, root } = await renderModule();
  await act(async () => {
    await waitFor("session changes answer", () => document.querySelector(".footer-changed__row") !== null);
  });
  ok(
    document.querySelectorAll(".footer-changed__row").length === FOOTER_CHANGED_FILES_INITIAL,
    `the list starts with ${FOOTER_CHANGED_FILES_INITIAL} files`,
  );
  ok(document.body.textContent?.includes("src/f7.ts") === false, "files past the initial window stay hidden");

  const more = document.querySelector<HTMLButtonElement>(".footer-panel__more");
  ok(more !== null, "a longer session offers a show-more control");

  await act(async () => {
    more?.click();
    await flushPromises();
  });
  ok(document.querySelectorAll(".footer-changed__row").length === 7, "show more reveals every file");
  ok(document.body.textContent?.includes("src/f7.ts") === true, "the last file of the answer is revealed");

  await act(async () => {
    document.querySelector<HTMLButtonElement>(".footer-panel__more")?.click();
    await flushPromises();
  });
  ok(
    document.querySelectorAll(".footer-changed__row").length === FOOTER_CHANGED_FILES_INITIAL,
    "the control collapses back again",
  );

  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exitCode = 1;
