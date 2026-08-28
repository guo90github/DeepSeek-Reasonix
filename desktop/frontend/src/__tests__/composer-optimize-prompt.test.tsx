// Run: tsx src/__tests__/composer-optimize-prompt.test.tsx
//
// Regression coverage for the composer "optimize prompt" button:
// - It must be disabled while the draft is empty and while an optimization is
//   in flight, and never fire on a second click.
// - A successful optimization replaces the draft with the result, caret at end.
// - A failed optimization keeps the draft and surfaces a toast instead.

import { JSDOM } from "jsdom";
import React from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";
import { Composer } from "../components/Composer";
import { LocaleProvider } from "../lib/i18n";
import { ToastProvider } from "../lib/toast";
import type { CollaborationMode, ToolApprovalMode } from "../lib/types";

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

function eq(actual: unknown, expected: unknown, label: string) {
  if (actual === expected) ok(true, label);
  else ok(false, `${label}: expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)}`);
}

function flushTimers(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, 0));
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
  Object.defineProperty(globalThis, "navigator", { configurable: true, value: dom.window.navigator });
  globalThis.Node = dom.window.Node;
  globalThis.HTMLElement = dom.window.HTMLElement;
  globalThis.HTMLTextAreaElement = dom.window.HTMLTextAreaElement;
  globalThis.Event = dom.window.Event;
  globalThis.CustomEvent = dom.window.CustomEvent;
  globalThis.KeyboardEvent = dom.window.KeyboardEvent;
  globalThis.InputEvent = dom.window.InputEvent;
  globalThis.MouseEvent = dom.window.MouseEvent;
  globalThis.File = dom.window.File;
  globalThis.FileReader = dom.window.FileReader;
  globalThis.PointerEvent = dom.window.MouseEvent as unknown as typeof PointerEvent;
  globalThis.MutationObserver = dom.window.MutationObserver;
  globalThis.localStorage = dom.window.localStorage;
  globalThis.requestAnimationFrame = dom.window.requestAnimationFrame.bind(dom.window);
  globalThis.cancelAnimationFrame = dom.window.cancelAnimationFrame.bind(dom.window);
  globalThis.ResizeObserver = TestResizeObserver;
  Object.defineProperty(dom.window.HTMLElement.prototype, "attachEvent", { configurable: true, value: () => {} });
  Object.defineProperty(dom.window.HTMLElement.prototype, "detachEvent", { configurable: true, value: () => {} });
  Object.defineProperty(window, "matchMedia", {
    configurable: true,
    value: () => ({
      matches: true,
      media: "(prefers-reduced-motion: reduce)",
      onchange: null,
      addEventListener() {},
      removeEventListener() {},
      addListener() {},
      removeListener() {},
      dispatchEvent: () => false,
    }),
  });
  return dom;
}

function installBridgeApp(methods: Record<string, unknown>) {
  (window as unknown as { go: { main: { App: Record<string, unknown> } } }).go = {
    go: undefined,
    main: {
      App: {
        Commands: async () => [],
        Models: async () => [],
        ModelsForTab: async () => [],
        ...methods,
      },
    },
  } as never;
}

async function renderComposer(props: Partial<Parameters<typeof Composer>[0]> = {}) {
  const rootEl = document.getElementById("root");
  if (!rootEl) throw new Error("missing root");
  const root = createRoot(rootEl);
  const currentProps: Parameters<typeof Composer>[0] = {
    running: false,
    collaborationMode: "normal",
    toolApprovalMode: "ask" as ToolApprovalMode,
    goal: "",
    cwd: "/repo",
    modelLabel: "DeepSeek-R1",
    imageInputEnabled: true,
    tabId: "optimize-tab",
    sessionKey: "session:project:/repo:topic-a:session-a",
    onSend: () => {},
    onCancel: async () => ({ discardedItemIds: [] }),
    onCycleMode: () => {},
    onSetMode: () => {},
    onSetCollaborationMode: (_mode: CollaborationMode) => {},
    onSetToolApprovalMode: () => {},
    onToggleYoloApprovalMode: () => {},
    onClearGoal: () => {},
    onSwitchModel: () => {},
    onSetEffort: () => {},
    ready: true,
    ...props,
  };
  await act(async () => {
    root.render(
      <LocaleProvider>
        <ToastProvider>
          <div className="chat-pane">
            <Composer {...currentProps} />
          </div>
        </ToastProvider>
      </LocaleProvider>,
    );
    await flushTimers();
  });
  return { root };
}

function textarea(): HTMLTextAreaElement {
  const node = document.querySelector("textarea") as HTMLTextAreaElement | null;
  if (!node) throw new Error("composer textarea did not render");
  return node;
}

function optimizeButton(): HTMLButtonElement {
  const node = document.querySelector(".composer__btn--optimize") as HTMLButtonElement | null;
  if (!node) throw new Error("optimize button did not render");
  return node;
}

function textPasteEvent(text: string): Event {
  const event = new window.Event("paste", { bubbles: true, cancelable: true });
  Object.defineProperty(event, "clipboardData", {
    configurable: true,
    value: {
      files: [],
      items: [],
      types: ["text/plain"],
      getData: (kind: string) => (kind === "text" || kind === "text/plain" ? text : ""),
    },
  });
  return event;
}

async function typeDraft(value: string) {
  await act(async () => {
    textarea().focus();
    textarea().dispatchEvent(textPasteEvent(value));
    await flushTimers();
  });
  if (textarea().value !== value) throw new Error(`composer text = ${JSON.stringify(textarea().value)}, want ${JSON.stringify(value)}`);
}

async function clickOptimize() {
  await act(async () => {
    optimizeButton().dispatchEvent(new window.MouseEvent("click", { bubbles: true, cancelable: true }));
    await flushTimers();
    await flushTimers();
  });
}

async function main() {
  installDom();

  // --- disabled while the draft is empty ---
  {
    installBridgeApp({ OptimizePrompt: async (text: string) => `OPT:${text}` });
    await renderComposer();
    ok(optimizeButton().disabled, "optimize is disabled with an empty draft");
  }

  // --- successful optimization replaces the draft ---
  {
    await typeDraft("帮我改个 bug");
    ok(!optimizeButton().disabled, "optimize is enabled once the draft is non-empty");
    await clickOptimize();
    eq(textarea().value, "OPT:帮我改个 bug", "optimized result replaces the draft");
    eq(optimizeButton().disabled, false, "optimize re-enables after completion");
  }

  // --- a failed optimization keeps the draft and shows a toast ---
  {
    installBridgeApp({
      OptimizePrompt: async () => {
        throw new Error("provider unavailable");
      },
    });
    await renderComposer();
    await typeDraft("保留我");
    await clickOptimize();
    eq(textarea().value, "保留我", "failed optimization must not touch the draft");
    const toast = Array.from(document.querySelectorAll(".toast")).map((node) => node.textContent ?? "").join("|");
    ok(toast.length > 0, "failure surfaces a toast");
  }

  // --- a second click during flight must not double-fire ---
  {
    let inflightCalls = 0;
    let resolveFirst: (value: string) => void = () => {};
    installBridgeApp({
      OptimizePrompt: () => {
        inflightCalls += 1;
        return new Promise<string>((resolve) => { resolveFirst = resolve; });
      },
    });
    await renderComposer();
    await typeDraft("并发");
    await act(async () => {
      optimizeButton().dispatchEvent(new window.MouseEvent("click", { bubbles: true, cancelable: true }));
      await flushTimers();
    });
    ok(optimizeButton().disabled, "optimize is disabled while an optimization is in flight");
    await act(async () => {
      optimizeButton().dispatchEvent(new window.MouseEvent("click", { bubbles: true, cancelable: true }));
      await flushTimers();
    });
    await act(async () => {
      resolveFirst("OPT:并发");
      await flushTimers();
    });
    eq(textarea().value, "OPT:并发", "first optimization result applies");
    eq(inflightCalls, 1, "no double-fire while optimizing");
  }

  process.stdout.write(`\n${passed} passed, ${failed} failed, ${passed + failed} total\n`);
  if (failed > 0) process.exit(1);
}

void main();
