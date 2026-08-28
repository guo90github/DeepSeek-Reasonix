// Run: tsx src/__tests__/use-pane-tail-follow.test.tsx

import { JSDOM } from "jsdom";
import React, { act, useEffect, useRef, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import type { VirtuosoHandle } from "react-virtuoso";
import { usePaneTailFollow } from "../lib/usePaneTailFollow";

let passed = 0;
let failed = 0;

function check(condition: unknown, label: string) {
  if (condition) {
    process.stdout.write(`  PASS  ${label}\n`);
    passed += 1;
  } else {
    process.stdout.write(`  FAIL  ${label}\n`);
    failed += 1;
  }
}

console.log("\nusePaneTailFollow");

const dom = new JSDOM('<!doctype html><html><body><div id="root"></div></body></html>', {
  pretendToBeVisual: true,
  url: "http://localhost/",
});
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
globalThis.window = dom.window as unknown as Window & typeof globalThis;
globalThis.document = dom.window.document;
globalThis.HTMLElement = dom.window.HTMLElement;
globalThis.Element = dom.window.Element;
globalThis.Node = dom.window.Node;

let nextFrame = 1;
const frames = new Map<number, FrameRequestCallback>();
const requestFrame = (callback: FrameRequestCallback) => {
  const id = nextFrame;
  nextFrame += 1;
  frames.set(id, callback);
  return id;
};
const cancelFrame = (id: number) => void frames.delete(id);
globalThis.requestAnimationFrame = requestFrame;
globalThis.cancelAnimationFrame = cancelFrame;
dom.window.requestAnimationFrame = requestFrame;
dom.window.cancelAnimationFrame = cancelFrame;

async function flushFrames() {
  const pending = [...frames.values()];
  frames.clear();
  await act(async () => pending.forEach((callback) => callback(performance.now())));
}

async function flushAllFrames(max = 64) {
  for (let attempt = 0; attempt < max && frames.size > 0; attempt += 1) {
    await flushFrames();
  }
}

// Native-geometry stand-ins: the settle loop reads/writes these like a real
// scroller, while the virtual list is a plain scrollTo sink.
const scrollerElement = dom.window.document.createElement("div");
let scrollTopValue = 0;
let scrollHeightValue = 0;
Object.defineProperty(scrollerElement, "scrollTop", {
  configurable: true,
  get: () => scrollTopValue,
  set: (value: number) => { scrollTopValue = value; },
});
Object.defineProperty(scrollerElement, "scrollHeight", {
  configurable: true,
  get: () => scrollHeightValue,
});
Object.defineProperty(scrollerElement, "clientHeight", { configurable: true, value: 100 });

let scrollWrites: number[] = [];
const virtuosoHandle = {
  scrollTo: (options?: { top?: number }) => {
    if (options?.top === undefined) return;
    scrollWrites.push(options.top);
    scrollTopValue = options.top;
  },
  scrollBy: () => {},
  scrollToIndex: () => {},
} as unknown as VirtuosoHandle;

type PaneApi = {
  reaim: () => void;
  gesture: () => void;
  grow: () => void;
};
let api: PaneApi | null = null;
const setApi = (next: PaneApi) => { api = next; };

function Harness({ enabled = true }: { enabled?: boolean }) {
  const virtuosoRef = useRef<VirtuosoHandle | null>(null);
  const scrollerRef = useRef<HTMLDivElement | null>(null);
  const [version, setVersion] = useState(0);
  const { onUserGesture, reaim } = usePaneTailFollow({
    virtuosoRef,
    scrollerRef,
    contentVersion: version,
    enabled,
  });
  useEffect(() => {
    virtuosoRef.current = virtuosoHandle;
    scrollerRef.current = scrollerElement;
    setApi({ reaim, gesture: onUserGesture, grow: () => setVersion((current) => current + 1) });
  }, [onUserGesture, reaim]);
  return null;
}

let root: Root | null = null;
async function mount(enabled = true): Promise<void> {
  api = null;
  const container = dom.window.document.createElement("div");
  dom.window.document.body.appendChild(container);
  root = createRoot(container);
  await act(async () => {
    root?.render(<Harness enabled={enabled} />);
  });
  await flushAllFrames();
  if (!api) throw new Error("harness never published its api");
}

async function unmount(): Promise<void> {
  if (!root) return;
  await act(async () => root?.unmount());
  root = null;
  frames.clear();
  api = null;
  scrollWrites = [];
}

// Scenario 1: streaming growth converges to the true bottom.
scrollHeightValue = 1000;
scrollTopValue = 900; // at bottom: 1000 - clientHeight(100)
await mount();
check(scrollWrites.length === 0, "mount at the bottom stays pinned without a write");
scrollHeightValue = 1200;
await act(() => api?.grow());
await flushAllFrames();
check(scrollWrites.length === 1, "tail growth emits exactly one native write");
check(scrollWrites[0] === 1100, "the write targets the native bottom (1200 - 100)");
check(scrollTopValue === 1100, "the scroller lands on the true bottom");
await unmount();

// Scenario 2: a user gesture releases the tail; growth stops writing.
scrollHeightValue = 1200;
scrollTopValue = 1100;
await mount();
scrollTopValue = 500; // user scrolled up mid-session
await act(() => api?.gesture());
scrollHeightValue = 1400;
await act(() => api?.grow());
await flushAllFrames();
check(scrollWrites.length === 0, "manual reading never receives tail writes");
check(scrollTopValue === 500, "manual reading leaves the offset untouched");
await unmount();

// Scenario 3: a reasoning fold shrinks the tail without a tug, then growth
// reconverges to the folded bottom.
scrollHeightValue = 1400;
scrollTopValue = 1300;
await mount();
scrollHeightValue = 800;
scrollTopValue = 700; // browser clamps scrollTop to the new bottom
await act(() => api?.grow());
await flushAllFrames();
check(scrollWrites.length === 0, "a fold shrink at the tail does not tug the viewport");
scrollHeightValue = 1000;
await act(() => api?.grow());
await flushAllFrames();
check(scrollWrites.length === 1, "growth after a fold reconverges once");
check(scrollWrites[0] === 900, "reconvergence lands on the folded bottom (1000 - 100)");
check(scrollTopValue === 900, "the viewport ends at the folded bottom");
await unmount();

// Scenario 4: hydration disables the tail writer entirely.
scrollHeightValue = 1000;
scrollTopValue = 900;
await mount(false);
scrollHeightValue = 1300;
await act(() => api?.grow());
await flushAllFrames();
check(scrollWrites.length === 0, "hydration leaves the viewport to restoration");
await unmount();

console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
