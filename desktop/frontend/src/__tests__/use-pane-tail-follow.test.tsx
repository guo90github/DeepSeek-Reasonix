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
globalThis.MutationObserver = dom.window.MutationObserver;

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

// Simulate a native scroll (scrollbar drag / keyboard) at the given offset.
async function nativeScrollTo(top: number): Promise<void> {
  scrollTopValue = top;
  await act(async () => {
    scrollerElement.dispatchEvent(new dom.window.Event("scroll"));
  });
}

// Native-geometry stand-ins: the settle loop reads/writes these like a real
// scroller, while the virtual list handle is a plain no-op sink. pinTail
// writes go to the scroller element directly (it bypasses Virtuoso's stale
// size-tree lane, see transcript-scroll-writer.test.ts), so the recorded
// "write" is the native scrollTo, not the Virtuoso handle.
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
// Simulates Virtuoso's stale size tree clamping a physical pin write: the
// browser accepts the call but the scroll position never moves.
let clampWrites = false;
scrollerElement.scrollTo = (options?: ScrollToOptions | number, y?: number) => {
  const top = typeof options === "number" ? (y ?? scrollTopValue) : (options?.top ?? scrollTopValue);
  if (clampWrites) return;
  scrollWrites.push(top);
  scrollTopValue = top;
};

// The MutationObserver growth detector watches the pane scroller's first
// child (Virtuoso's viewport layer), which in the real panes hosts the
// mounted card bodies. Give the fake scroller the same child so the detector
// arms, and keep it in the document like the real panes.
const contentNode = dom.window.document.createElement("div");
scrollerElement.appendChild(contentNode);
dom.window.document.body.appendChild(scrollerElement);

const virtuosoHandle = {
  scrollTo: () => {},
  scrollBy: () => {},
  scrollToIndex: () => {},
} as unknown as VirtuosoHandle;

type PaneApi = {
  reaim: () => void;
  gesture: () => void;
  grow: () => void;
  setEnabled: (next: boolean) => void;
  armScroller: () => void;
};
let api: PaneApi | null = null;
const setApi = (next: PaneApi) => { api = next; };

function Harness({ enabled = true, delayedScroller = false }: { enabled?: boolean; delayedScroller?: boolean }) {
  const virtuosoRef = useRef<VirtuosoHandle | null>(null);
  const scrollerRef = useRef<HTMLDivElement | null>(null);
  const [version, setVersion] = useState(0);
  const [isEnabled, setIsEnabled] = useState(enabled);
  const { onUserGesture, reaim } = usePaneTailFollow({
    virtuosoRef,
    scrollerRef,
    contentVersion: version,
    enabled: isEnabled,
  });
  useEffect(() => {
    setApi({
      reaim,
      gesture: onUserGesture,
      grow: () => setVersion((current) => current + 1),
      setEnabled: setIsEnabled,
      armScroller: () => { scrollerRef.current = scrollerElement as unknown as HTMLDivElement; },
    });
  }, [onUserGesture, reaim]);
  // Callback refs assign during commit, before effects — the hook's scroll
  // listener attaches on the mounted scroller, like the real panes. With
  // delayedScroller the ref is left null so tests can reproduce Virtuoso's
  // late scroller commit (one render after the pane).
  return (
    <span
      ref={(node) => {
        virtuosoRef.current = virtuosoHandle;
        if (!delayedScroller) scrollerRef.current = node ? (scrollerElement as unknown as HTMLDivElement) : null;
      }}
    />
  );
}

let root: Root | null = null;
async function mount(enabled = true, delayedScroller = false): Promise<void> {
  api = null;
  const container = dom.window.document.createElement("div");
  dom.window.document.body.appendChild(container);
  root = createRoot(container);
  await act(async () => {
    root?.render(<Harness enabled={enabled} delayedScroller={delayedScroller} />);
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
  clampWrites = false;
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

// Dragging the scrollbar back to the bottom re-arms the tail: the next growth
// converges to the new physical bottom again.
scrollTopValue = 1300; // bottom of the grown 1400 content
await act(() => api?.gesture());
scrollHeightValue = 1600;
await act(() => api?.grow());
await flushAllFrames();
check(scrollWrites.length === 1, "returning to the bottom restores tail following");
check(scrollWrites[0] === 1500, "restored follow writes the new native bottom (1600 - 100)");
check(scrollTopValue === 1500, "the scroller lands on the bottom after the tail resumes");
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

// Scenario 5: the tail re-arms once the writer turns on — content that grew
// while hydration parked the pane converges to the true bottom on the flip.
scrollHeightValue = 1300;
scrollTopValue = 300; // restoration parked the viewport mid-session
await mount(false);
await flushAllFrames();
check(scrollWrites.length === 0, "disabled pane stays parked during hydration");
scrollHeightValue = 1500;
await act(() => api?.grow());
await flushAllFrames();
check(scrollWrites.length === 0, "growth during hydration never tugs");
await act(() => api?.setEnabled(true));
await flushAllFrames();
check(scrollWrites.length === 1, "enabling the writer re-aims once");
check(scrollWrites[0] === 1400, "re-aim lands on the true bottom (1500 - 100)");
check(scrollTopValue === 1400, "the scroller ends at the bottom after hydration");
await unmount();

// Scenario 6: layout/programmatic scrolls (Virtuoso range shifts, late row
// commits) never release the tail — only real gestures pause it. A non-user
// scroll above the tail must not freeze auto-follow.
scrollHeightValue = 1200;
scrollTopValue = 1100;
await mount();
await nativeScrollTo(400); // Virtuoso-style layout scroll away from the tail
scrollHeightValue = 1400;
await act(() => api?.grow());
await flushAllFrames();
check(scrollWrites.length === 1, "a layout scroll above the tail does not pause following");
check(scrollWrites[0] === 1300, "following still writes the new native bottom (1400 - 100)");
check(scrollTopValue === 1300, "the layout scroll does not freeze the viewport mid-growth");
await unmount();

// Scenario 7: Virtuoso commits row height AFTER the settle loop exits — the
// bounded confirm window re-converges without a new data-change reaim.
scrollHeightValue = 1200;
scrollTopValue = 1100;
await mount();
scrollHeightValue = 1400;
await act(() => api?.grow());
await flushAllFrames();
check(scrollWrites.length === 1 && scrollWrites[0] === 1300, "the first commit pins the interim bottom");
scrollHeightValue = 1700; // late row-measurement commit, no contentVersion bump
await new Promise((resolve) => setTimeout(resolve, 450)); // inside the confirm window
await flushAllFrames();
check(scrollWrites.length === 2, "a late height commit triggers exactly one confirm write");
check(scrollWrites[1] === 1600, "the confirm write lands on the true late bottom (1700 - 100)");
check(scrollTopValue === 1600, "the viewport converges after the late commit");
await unmount();

// Scenario 8: pure-DOM growth with no Virtuoso callback and no
// contentVersion bump (an old folded card expands above the viewport) — the
// MutationObserver detector re-aims the pinned tail exactly once.
scrollHeightValue = 1200;
scrollTopValue = 1100;
await mount();
await flushAllFrames();
check(scrollWrites.length === 0, "mount pinned at the bottom stays silent");
scrollHeightValue = 1700; // the card body mounts, growing the real DOM extent
await act(async () => {
  contentNode.appendChild(dom.window.document.createElement("article"));
});
await new Promise((resolve) => setTimeout(resolve, 0)); // flush MO microtasks
await flushAllFrames();
check(scrollWrites.length === 1, "DOM-only growth emits exactly one write");
check(scrollWrites[0] === 1600, "the DOM-only write lands on the native bottom (1700 - 100)");
check(scrollTopValue === 1600, "the viewport converges on pure-DOM growth");
// A shrink-only mutation (fold) must not tug the pinned viewport.
scrollWrites = [];
scrollHeightValue = 1000;
scrollTopValue = 900;
await act(async () => {
  contentNode.firstElementChild?.remove();
});
await new Promise((resolve) => setTimeout(resolve, 0));
await flushAllFrames();
check(scrollWrites.length === 0, "a fold-only mutation never tugs the pinned tail");
await unmount();

// Scenario 9: Virtuoso commits its scroller one pass after the pane, so the
// hook's first effects see no element and their deps never re-run. The
// detector and the scroll re-arm listener must arm on the LATE arrival.
scrollHeightValue = 1200;
scrollTopValue = 1100;
await mount(true, true); // scroller ref intentionally absent at commit
check(scrollWrites.length === 0, "delayed-mount pane starts silent");
await act(() => api?.armScroller()); // Virtuoso's late scroller commit
await flushAllFrames();
scrollHeightValue = 1500; // pure-DOM growth lands right after arming
await act(async () => {
  contentNode.appendChild(dom.window.document.createElement("article"));
});
await new Promise((resolve) => setTimeout(resolve, 0));
await flushAllFrames();
check(scrollWrites.length === 1, "growth right after the late scroller mount writes once");
check(scrollWrites[0] === 1400, "the late-mount write lands on the native bottom (1500 - 100)");
check(scrollTopValue === 1400, "the viewport converges after the late scroller mount");
// The re-arm listener must be live too: gesture away pauses, returning to
// the bottom resumes with one write.
scrollWrites = [];
scrollTopValue = 300; // user scrolled up mid-session
await act(() => api?.gesture());
scrollHeightValue = 1600;
await act(async () => {
  contentNode.appendChild(dom.window.document.createElement("p"));
});
await new Promise((resolve) => setTimeout(resolve, 0));
await flushAllFrames();
check(scrollWrites.length === 0, "manual mode after a gesture stays silent post-arming");
scrollTopValue = 1500; // bottom of the grown 1600 content
await nativeScrollTo(1500);
scrollHeightValue = 1700;
await act(() => api?.grow());
await flushAllFrames();
check(scrollWrites.length === 1, "returning to the bottom re-arms the late-mounted listener");
check(scrollWrites[0] === 1600, "the re-armed write lands on the new bottom (1700 - 100)");
await unmount();

// Scenario 10: a stale-tree clamp must not kill tail-follow for the rest of
// the session. The pane settle never cycles the ownership epoch that clears
// the failed-pin quarantine, so a large native gap on a fresh growth signal
// has to escape it (bounded: a still-stale tree re-clamps and re-arms it).
scrollHeightValue = 1200;
scrollTopValue = 1100;
clampWrites = true; // Virtuoso's stale tree accepts the write, scroll stays put
await mount();
scrollHeightValue = 1400;
await act(() => api?.grow()); // growth → physical pin → clamped → quarantine
await flushAllFrames();
await new Promise((resolve) => setTimeout(resolve, 200)); // quiet retry (idle #1)
await flushAllFrames();
await new Promise((resolve) => setTimeout(resolve, 200)); // retry spent (idle #2)
await flushAllFrames();
check(scrollWrites.length === 0, "stale-tree clamped pins stay silent");
clampWrites = false; // Virtuoso's size tree catches up
scrollHeightValue = 1700;
await act(() => api?.grow()); // fresh growth — must escape the dead quarantine
await flushAllFrames();
check(scrollWrites.length === 1, "growth after a stale-tree pin escapes the dead quarantine once");
check(scrollWrites[0] === 1600, "the escape write lands on the native bottom (1700 - 100)");
check(scrollTopValue === 1600, "the viewport converges after the stale-tree quarantine");
await unmount();

// Scenario 11: staged growth (Virtuoso commits an expanded card in steps)
// must end AT the bottom. Each growth signal is a new geometry revision; the
// session's frame keys must not exhaust the settle budget with duplicates.
scrollHeightValue = 1200;
scrollTopValue = 1100;
await mount();
for (let step = 1; step <= 9; step += 1) {
  scrollHeightValue = 1200 + step * 200; // 1400 … 3000
  await act(() => api?.grow());
  await flushAllFrames();
}
check(scrollWrites.length === 9, "each staged growth emits its own accepted write");
check(scrollWrites[8] === 2900, "the last staged write lands on the final bottom (3000 - 100)");
check(scrollTopValue === 2900, "the viewport ends exactly at the bottom after staged growth");
await unmount();

// Scenario 12: Virtuoso echoes its own remeasure after a pin (list height
// wobbles ±8 → distance 8 > 4). The tail must not chase that echo at frame
// rate — the visible jitter; growth past the wobble band still writes.
scrollHeightValue = 1200;
scrollTopValue = 1100;
await mount();
scrollHeightValue = 1400;
await act(() => api?.grow());
await flushAllFrames();
check(scrollWrites.length === 1 && scrollWrites[0] === 1300, "the first growth pins the bottom");
scrollHeightValue = 1408; // remeasure echo: +8 inside the quiet window
await act(() => api?.grow());
await flushAllFrames();
check(scrollWrites.length === 1, "an echo wobble inside the quiet window writes nothing");
scrollHeightValue = 1400; // echo settles back down
await act(() => api?.grow());
await flushAllFrames();
check(scrollWrites.length === 1, "the echo settling back writes nothing");
scrollHeightValue = 1700; // real growth past the band, still inside the window
await act(() => api?.grow());
await flushAllFrames();
check(scrollWrites.length === 2, "growth past the wobble band writes immediately");
check(scrollWrites[1] === 1600, "the real-growth write lands on the new bottom (1700 - 100)");
check(scrollTopValue === 1600, "the viewport converges on real growth");
await unmount();

console.log(`\n${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
