// Run: tsx src/__tests__/composer-optimize-prompt.test.tsx
//
// Regression coverage for the composer "optimize prompt" button:
// - The panel opens immediately in a visible loading state; chunks stream in
//   incrementally and the result becomes editable once the stream completes.
// - It must be disabled while the draft is empty and while an optimization is
//   in flight, and never fire on a second click; it stays enabled while the
//   agent is running (independent of the turn stream).
// - Applying uses the (possibly edited) result; cancel keeps the draft, and a
//   failed optimization keeps the draft and surfaces a toast instead.

import { JSDOM } from "jsdom";
import React from "react";
import { act } from "react";
import type { Root } from "react-dom/client";
import { Composer } from "../components/Composer";
import { LocaleProvider } from "../lib/i18n";
import { ToastProvider } from "../lib/toast";
import { __emitMockOptimizePromptChunk, __emitMockOptimizePromptDone } from "../lib/bridge";
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
  // react-dom/client must load AFTER the JSDOM globals exist: statically
  // importing it at module top leaves React's event system (value tracking for
  // controlled inputs) initialized without a document, so portal onChange never
  // fires. Dynamic import after installDom matches the repo's working
  // portal-input tests (blank-project-dialog.test.tsx).
  const { createRoot } = await import("react-dom/client");
  const root: Root = createRoot(rootEl);
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

function previewDialog(): HTMLElement | null {
  return document.querySelector(".reasonix-optimize-dialog");
}

function previewApplyButton(): HTMLButtonElement {
  const node = document.querySelector(".reasonix-optimize-dialog__actions .btn--primary") as HTMLButtonElement | null;
  if (!node) throw new Error("preview apply button did not render");
  return node;
}

function previewCancelButton(): HTMLButtonElement {
  const node = document.querySelector(".reasonix-optimize-dialog__actions .btn:not(.btn--primary)") as HTMLButtonElement | null;
  if (!node) throw new Error("preview cancel button did not render");
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

// Stream simulation: the tsx harness has no window.runtime, so onOptimizePrompt
// chunk/done subscriptions land in the bridge's mock listener sets, which these
// helpers drive with the same (tabId, chunk) shape the Wails channel delivers.
const TEST_TAB_ID = "optimize-tab";

async function emitChunk(chunk: string) {
  await act(async () => {
    __emitMockOptimizePromptChunk(TEST_TAB_ID, chunk);
    await flushTimers();
  });
}

async function emitDone() {
  await act(async () => {
    __emitMockOptimizePromptDone(TEST_TAB_ID);
    await flushTimers();
  });
}

function streamText(): string {
  return document.querySelector(".reasonix-optimize-dialog__text--stream")?.textContent ?? "";
}

function dialogTextarea(): HTMLTextAreaElement {
  const node = document.querySelector(".reasonix-optimize-dialog__textarea") as HTMLTextAreaElement | null;
  if (!node) throw new Error("preview textarea did not render");
  return node;
}

function streamingHint(): string {
  return document.querySelector(".reasonix-optimize-dialog__placeholder")?.textContent ?? "";
}

async function main() {
  const dom = installDom();

  // --- disabled while the draft is empty ---
  {
    installBridgeApp({ OptimizePrompt: async (text: string) => `OPT:${text}` });
    await renderComposer();
    ok(optimizeButton().disabled, "optimize is disabled with an empty draft");
  }

  // --- click opens the panel in a visible loading state, streams chunks,
  // then becomes editable; applying uses the EDITED text ---
  {
    await typeDraft("帮我改个 bug");
    ok(!optimizeButton().disabled, "optimize is enabled once the draft is non-empty");
    await clickOptimize();
    ok(previewDialog() !== null, "preview dialog opens immediately");
    ok(previewApplyButton().disabled, "apply is disabled while streaming");
    ok(optimizeButton().disabled, "optimize button is disabled while streaming");
    ok(streamingHint().length > 0, "panel shows a streaming hint before the first chunk");
    eq(textarea().value, "帮我改个 bug", "draft is untouched while streaming");
    await emitChunk("请");
    eq(streamText(), "请", "first chunk renders immediately");
    await emitChunk("修复：");
    await emitChunk("改 bug");
    eq(streamText(), "请修复：改 bug", "chunks accumulate in arrival order");
    await emitDone();
    ok(!previewApplyButton().disabled, "apply enables once streaming completes");
    eq(dialogTextarea().value, "请修复：改 bug", "done swaps the stream for an editable textarea");
    await act(async () => {
      const node = dialogTextarea();
      Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, "value")?.set?.call(node, "请修复登录接口的鉴权漏洞");
      node.dispatchEvent(new window.Event("input", { bubbles: true }));
      await flushTimers();
    });
    await act(async () => {
      previewApplyButton().dispatchEvent(new window.MouseEvent("click", { bubbles: true, cancelable: true }));
      await flushTimers();
    });
    eq(textarea().value, "请修复登录接口的鉴权漏洞", "applying uses the EDITED text, not the raw stream");
    ok(previewDialog() === null, "preview dialog closes after apply");
    eq(optimizeButton().disabled, false, "optimize re-enables after completion");
  }

  // --- cancelling the preview keeps the draft ---
  {
    installBridgeApp({ OptimizePrompt: async (text: string) => `OPT:${text}` });
    await renderComposer();
    await typeDraft("不应用");
    await clickOptimize();
    ok(previewDialog() !== null, "preview dialog opens for cancellation");
    await emitDone();
    await act(async () => {
      previewCancelButton().dispatchEvent(new window.MouseEvent("click", { bubbles: true, cancelable: true }));
      await flushTimers();
    });
    eq(textarea().value, "不应用", "cancelling the preview keeps the draft");
    ok(previewDialog() === null, "preview dialog closes on cancel");
    eq(optimizeButton().disabled, false, "optimize re-enables after cancel");
  }

  // --- cancelling mid-stream keeps the draft; the button stays disabled until
  // the background run finishes ---
  {
    installBridgeApp({ OptimizePrompt: async (text: string) => `OPT:${text}` });
    await renderComposer();
    await typeDraft("流式中取消");
    await clickOptimize();
    ok(previewDialog() !== null, "panel is open while streaming");
    await emitChunk("部分");
    await act(async () => {
      previewCancelButton().dispatchEvent(new window.MouseEvent("click", { bubbles: true, cancelable: true }));
      await flushTimers();
    });
    ok(previewDialog() === null, "cancel closes the panel");
    eq(textarea().value, "流式中取消", "draft is untouched");
    ok(optimizeButton().disabled, "button stays disabled until the run finishes");
    await emitDone();
    eq(optimizeButton().disabled, false, "button re-enables once the background run completes");
  }

  // --- the dialog is resizable via its edge/corner handles ---
  {
    installBridgeApp({ OptimizePrompt: async (text: string) => `OPT:${text}` });
    await renderComposer();
    await typeDraft("缩放");
    await clickOptimize();
    await emitChunk("OPT:缩放");
    await emitDone();
    const dialog = document.querySelector(".reasonix-optimize-dialog") as HTMLElement | null;
    ok(dialog !== null, "dialog renders for resize");
    eq(document.querySelectorAll(".reasonix-optimize-dialog__handle").length, 8, "dialog exposes edge and corner resize handles");
    ok(dialog?.style.width === "", "no inline size before the first drag (CSS default applies)");
    const se = document.querySelector(".reasonix-optimize-dialog__handle--se") as HTMLElement | null;
    ok(se !== null, "south-east corner handle exists");
    await act(async () => {
      se?.dispatchEvent(new window.MouseEvent("pointerdown", { bubbles: true, cancelable: true, clientX: 300, clientY: 300, button: 0 }));
    });
    await act(async () => {
      window.dispatchEvent(new window.MouseEvent("pointermove", { bubbles: true, clientX: 420, clientY: 380 }));
      await new Promise((resolve) => setTimeout(resolve, 30)); // let the rAF frame apply
    });
    await act(async () => {
      window.dispatchEvent(new window.MouseEvent("pointerup", { bubbles: true }));
      await flushTimers();
    });
    // jsdom reports a zero rect, so the drag clamps to the dialog minimums.
    eq(dialog?.style.width, "440px", "drag applies the resized width (min-clamped in jsdom)");
    eq(dialog?.style.height, "300px", "drag applies the resized height (min-clamped in jsdom)");
    eq(document.body.style.cursor, "", "cursor restored after the gesture");
    await act(async () => {
      previewCancelButton().dispatchEvent(new window.MouseEvent("click", { bubbles: true, cancelable: true }));
      await flushTimers();
    });
  }

  // --- enabled while the agent is running: independent of the turn stream ---
  {
    installBridgeApp({ OptimizePrompt: async (text: string) => `OPT:${text}` });
    const { root } = await renderComposer({ running: true });
    await typeDraft("运行中也要能优化");
    ok(!optimizeButton().disabled, "optimize stays enabled while the agent is running");
    await clickOptimize();
    ok(previewDialog() !== null, "optimization runs concurrently with the active turn");
    await emitChunk("OPT:");
    await emitChunk("运行中也要能优化");
    await emitDone();
    await act(async () => {
      previewApplyButton().dispatchEvent(new window.MouseEvent("click", { bubbles: true, cancelable: true }));
      await flushTimers();
    });
    eq(textarea().value, "OPT:运行中也要能优化", "confirmed result applies while running");
    // Unmount so the running ticker (useTick(running) interval) cannot leak
    // into later blocks or keep the process alive.
    await act(async () => {
      root.unmount();
    });
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
    ok(previewDialog() === null, "panel closes on failure");
    eq(optimizeButton().disabled, false, "button re-enables after failure");
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
    ok(previewDialog() !== null, "panel stays open through the run");
    await emitChunk("OPT:并发");
    await emitDone();
    await act(async () => {
      previewApplyButton().dispatchEvent(new window.MouseEvent("click", { bubbles: true, cancelable: true }));
      await flushTimers();
    });
    eq(textarea().value, "OPT:并发", "first optimization result applies after confirm");
    eq(inflightCalls, 1, "no double-fire while optimizing");
  }

  process.stdout.write(`\n${passed} passed, ${failed} failed, ${passed + failed} total\n`);
  dom.window.close();
  if (failed > 0) process.exit(1);
}

void main();
