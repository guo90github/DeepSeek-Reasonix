// Run: tsx src/__tests__/footer-panel-detail.test.tsx

// The row-detail modal: a card row is one ellipsised line, so clicking it must
// open the whole record — over the document, closable three ways — and a file row
// opens that file's patch, not just its name.

import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { FooterChangedFilesModule } from "../components/FooterChangedFilesModule";
import { PanelDetailProvider, PanelRowButton } from "../components/FooterPanelDetail";
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

// The stub must land after installDom: the harness creates the global window the
// host stub binds to, so installing it first is silently dropped.
async function render(element: React.ReactElement, install?: () => void) {
  const dom = installDom();
  install?.();
  const rootEl = document.getElementById("root");
  if (!rootEl) throw new Error("missing root");
  const root = createRoot(rootEl);
  await act(async () => {
    root.render(<LocaleProvider>{element}</LocaleProvider>);
    await flushPromises();
  });
  return { dom, root };
}

const longPath = "desktop/frontend/src/components/FooterPanelDetail.tsx";
const patch = "@@ -1,1 +1,2 @@\n-old line\n+new line\n";

function installSessionChanges(): void {
  const changes: WorkspaceChangesView = {
    files: [{ path: longPath, sources: ["session"], gitStatus: "M" }],
    gitAvailable: true,
    gitBranch: "dev-2",
  };
  installDesktopHostStub(({
    main: {
      App: {
        WorkspaceChanges: async () => changes,
        WorkspaceChangeDetail: async () => ({ diff: patch, source: "git" as const, added: 1, removed: 1 }),
      } as Partial<AppBindings> as AppBindings,
    },
  }).main.App);
}

async function renderSessionChanges() {
  return render(
    <PanelDetailProvider>
      <FooterChangedFilesModule tabId="tab-a" workspaceScopeKey="scope-a" workspaceRoot="/Users/guosj/proj/repo" />
    </PanelDetailProvider>,
    installSessionChanges,
  );
}

console.log("\nfooter panel row detail");

{
  const { dom, root } = await render(
    <PanelDetailProvider>
      <ul>
        <li>
          <PanelRowButton
            className="footer-changed__row"
            detail={{ title: "src/deep/file.ts", meta: ["dev-2", "M"], body: "src/deep/file.ts", bodyStyle: "mono" }}
          >
            <span>src/deep/file.ts</span>
          </PanelRowButton>
        </li>
      </ul>
    </PanelDetailProvider>,
  );
  ok(document.querySelector(".footer-detail") === null, "no detail is open before a click");

  await act(async () => {
    document.querySelector<HTMLButtonElement>(".footer-panel__row")?.click();
    await flushPromises();
  });
  ok(document.querySelector('[role="dialog"]') !== null, "clicking a row opens the dialog");
  ok(document.body.textContent?.includes("src/deep/file.ts") === true, "the dialog shows the record's whole name");
  ok(document.querySelectorAll(".footer-detail__tag").length === 2, "the dialog shows the row's meta as tags");

  await act(async () => {
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    await flushPromises();
  });
  ok(document.querySelector('[role="dialog"]') === null, "Escape closes the dialog");

  await act(async () => {
    document.querySelector<HTMLButtonElement>(".footer-panel__row")?.click();
    await flushPromises();
  });
  await act(async () => {
    document.querySelector<HTMLElement>(".footer-detail-backdrop")?.dispatchEvent(
      new dom.window.MouseEvent("mousedown", { bubbles: true }),
    );
    await flushPromises();
  });
  ok(document.querySelector('[role="dialog"]') === null, "clicking the backdrop closes the dialog");

  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

{
  // A file row carries the file's patch: the row truncates a path, the dialog
  // answers with the change itself.
  const { dom, root } = await renderSessionChanges();
  await act(async () => {
    await waitFor("session changes answer", () => document.querySelector(".footer-panel__row") !== null);
  });
  ok(document.querySelectorAll(".footer-panel__row").length === 1, "the module's row is a clickable record");

  await act(async () => {
    document.querySelector<HTMLButtonElement>(".footer-panel__row")?.click();
    await flushPromises();
  });
  ok(document.querySelector('[role="dialog"]') !== null, "the module's row opens the dialog");
  ok(document.querySelector(".footer-detail__raw") !== null, "the patch body is a raw element, not pre-wrapped text");
  ok(document.body.textContent?.includes("+new line") === true, "the dialog carries the file's patch");
  ok(document.querySelectorAll(".footer-detail__tag").length === 3, "workspace, branch and status ride as tags");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exitCode = 1;
