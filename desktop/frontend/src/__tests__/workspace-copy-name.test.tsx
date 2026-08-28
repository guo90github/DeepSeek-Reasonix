// Run: tsx src/__tests__/workspace-copy-name.test.tsx

import { JSDOM } from "jsdom";
import React from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { WorkspacePanel } from "../components/WorkspacePanel";
import type { AppBindings } from "../lib/bridge";
import { LocaleProvider } from "../lib/i18n";
import { resetWorkspaceTreeMemoryForTests } from "../lib/workspaceTreeMemory";

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

function flushTimers(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
}

async function waitFor(label: string, predicate: () => boolean) {
  for (let attempt = 0; attempt < 20; attempt += 1) {
    await act(async () => {
      await flushTimers();
    });
    if (predicate()) return;
  }
  throw new Error(`timed out waiting for ${label}`);
}

class TestResizeObserver {
  observe() {}
  unobserve() {}
  disconnect() {}
}

function installDom() {
  const dom = new JSDOM("<!doctype html><html><body><div id=\"root\"></div></body></html>", {
    pretendToBeVisual: true,
    url: "http://localhost/",
  });
  (globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
  globalThis.window = dom.window as unknown as Window & typeof globalThis;
  globalThis.document = dom.window.document;
  Object.defineProperty(dom.window.navigator, "language", { configurable: true, value: "en-US" });
  Object.defineProperty(globalThis, "navigator", { configurable: true, value: dom.window.navigator });
  globalThis.Node = dom.window.Node;
  globalThis.Element = dom.window.Element;
  globalThis.HTMLElement = dom.window.HTMLElement;
  globalThis.Event = dom.window.Event;
  globalThis.CustomEvent = dom.window.CustomEvent;
  globalThis.KeyboardEvent = dom.window.KeyboardEvent;
  globalThis.MouseEvent = dom.window.MouseEvent;
  globalThis.PointerEvent = dom.window.MouseEvent as unknown as typeof PointerEvent;
  globalThis.MutationObserver = dom.window.MutationObserver;
  globalThis.ResizeObserver = TestResizeObserver;
  dom.window.ResizeObserver = TestResizeObserver;
  globalThis.localStorage = dom.window.localStorage;
  globalThis.requestAnimationFrame = dom.window.requestAnimationFrame.bind(dom.window);
  globalThis.cancelAnimationFrame = dom.window.cancelAnimationFrame.bind(dom.window);
  Object.defineProperty(dom.window.HTMLElement.prototype, "scrollIntoView", { configurable: true, value: () => {} });
  Object.defineProperty(dom.window.HTMLElement.prototype, "offsetWidth", { configurable: true, get: () => 320 });
  Object.defineProperty(dom.window.HTMLElement.prototype, "offsetHeight", {
    configurable: true,
    get: function offsetHeight(this: HTMLElement) {
      return this.classList.contains("workspace-tree") ? 300 : this.dataset.index ? 24 : 0;
    },
  });
  Object.defineProperty(dom.window.HTMLElement.prototype, "getBoundingClientRect", {
    configurable: true,
    value: function getBoundingClientRect(this: HTMLElement) {
      const width = 320;
      const height = this.classList.contains("workspace-tree") ? 300 : this.dataset.index ? 24 : 0;
      return { x: 0, y: 0, top: 0, left: 0, right: width, bottom: height, width, height, toJSON: () => ({}) } as DOMRect;
    },
  });
  return dom;
}

function clickRow(path: string) {
  const row = document.querySelector<HTMLButtonElement>(`[data-workspace-path="${path}"]`);
  if (!row) throw new Error(`missing workspace row ${path}`);
  return act(async () => {
    row.dispatchEvent(new window.MouseEvent("click", { bubbles: true }));
    await flushTimers();
  });
}

async function openRowMenu(path: string) {
  const row = document.querySelector<HTMLButtonElement>(`[data-workspace-path="${path}"]`);
  if (!row) throw new Error(`missing workspace row ${path}`);
  await act(async () => {
    row.dispatchEvent(new window.MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: 30, clientY: 30 }));
    await flushTimers();
  });
}

console.log("\nworkspace copy file name on explicit click");

resetWorkspaceTreeMemoryForTests();
const dom = installDom();
const clipboardWrites: string[] = [];
Object.defineProperty(dom.window.navigator, "clipboard", {
  configurable: true,
  value: {
    writeText: async (value: string) => {
      clipboardWrites.push(value);
    },
  },
});
window.go = {
  main: {
    App: {
      ListDirForTab: async (_tabId, dir) => dir === ""
        ? [
            { name: "docs", isDir: true },
            { name: "notes.txt", isDir: false },
          ]
        : dir === "docs/"
          ? [{ name: "guide.txt", isDir: false }]
          : [],
      SearchFileRefsForTab: async () => [],
      WorkspaceGitHistory: async () => [],
      WorkspaceChanges: async () => ({ files: [], gitAvailable: true }),
      WorkspaceChangeDetail: async () => ({}),
      ReadFileForTab: async (_tabId, path) => ({ path, body: "", size: 0, truncated: false, binary: false }),
    } as Partial<AppBindings> as AppBindings,
  },
};

const rootElement = document.getElementById("root");
if (!rootElement) throw new Error("missing root");
const root = createRoot(rootElement);
await act(async () => {
  root.render(
    <LocaleProvider>
      <WorkspacePanel
        open
        tabId="workspace-tab"
        cwd="/repo"
        maximized={false}
        initialViewMode="files"
        onClose={() => {}}
        onToggleMaximized={() => {}}
      />
    </LocaleProvider>,
  );
  await flushTimers();
});

await waitFor("workspace rows", () => document.querySelector('[data-workspace-path="notes.txt"]') != null);

await clickRow("notes.txt");
ok(
  JSON.stringify(clipboardWrites) === JSON.stringify(["notes.txt"]),
  "clicking a file row copies the bare file name once",
);

await openRowMenu("notes.txt");
ok(document.querySelector(".workspace-tree-menu") != null, "right-click still opens the file context menu");
ok(
  JSON.stringify(clipboardWrites) === JSON.stringify(["notes.txt"]),
  "right-click does not copy (left/right click stay isolated)",
);

await clickRow("notes.txt");
ok(
  JSON.stringify(clipboardWrites) === JSON.stringify(["notes.txt", "notes.txt"]),
  "clicking the already-selected file again (deselect) still copies the name",
);

await clickRow("docs/");
ok(
  JSON.stringify(clipboardWrites) === JSON.stringify(["notes.txt", "notes.txt", "docs"]),
  "clicking a folder row copies its bare name while toggling it",
);

await waitFor("nested row", () => document.querySelector('[data-workspace-path="docs/guide.txt"]') != null);
await clickRow("docs/guide.txt");
ok(
  JSON.stringify(clipboardWrites) === JSON.stringify(["notes.txt", "notes.txt", "docs", "guide.txt"]),
  "nested file rows copy their name without the directory prefix",
);

await act(async () => {
  root.unmount();
});
dom.window.close();

console.log(`\n${passed} passed, ${failed} failed, ${passed + failed} total`);
if (failed > 0) process.exit(1);
