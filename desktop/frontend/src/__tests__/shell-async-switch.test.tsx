// Run: node --import ./scripts/css-stub-register.mjs --import tsx src/__tests__/shell-async-switch.test.tsx

// 需求第九的档位控件现在挂在输入框底部（composer 的 meta 行），样式是滑动开关：
// 三个停位 + 一条跟着当前档位滑动的轨。这里钉住三件事：滑动形态（data-tier 决定轨的位置）、
// 点击写下的正是配置值、以及"读不到就不显示"（不谎报档位）。再补一条接线断言，
// 因为 composer 里少挂一个可选控件在类型检查上是无声的。

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { JSDOM } from "jsdom";
import React from "react";
import { act } from "react";
import { createRoot } from "react-dom/client";

import { ShellAsyncTierSwitch } from "../components/ShellAsyncTierSwitch";
import type { ShellAsyncTier } from "../components/ShellAsyncTierField";
import { LocaleProvider } from "../lib/i18n";

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

const dom = new JSDOM("<!doctype html><html><body></body></html>", { url: "http://localhost/" });
(globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
globalThis.window = dom.window as unknown as Window & typeof globalThis;
globalThis.document = dom.window.document;
Object.defineProperty(globalThis, "navigator", { configurable: true, value: dom.window.navigator });
globalThis.HTMLElement = dom.window.HTMLElement;
globalThis.Node = dom.window.Node;
globalThis.Event = dom.window.Event;
globalThis.MouseEvent = dom.window.MouseEvent;

async function mount(options: {
  value?: string;
  fail?: boolean;
  pending?: boolean;
  disabled?: boolean;
}) {
  const saved: ShellAsyncTier[] = [];
  const container = document.createElement("div");
  document.body.appendChild(container);
  const root = createRoot(container);
  await act(async () => {
    root.render(
      <LocaleProvider>
        <ShellAsyncTierSwitch
          disabled={options.disabled ?? false}
          load={async () => {
            if (options.fail) throw new Error("host unavailable");
            return options.value;
          }}
          save={async (tier: ShellAsyncTier) => {
            saved.push(tier);
            if (options.pending) await new Promise(() => {});
          }}
        />
      </LocaleProvider>,
    );
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  return { root, saved, container };
}

function stops(container: HTMLElement): HTMLButtonElement[] {
  return Array.from(container.querySelectorAll<HTMLButtonElement>(".shell-switch__stop"));
}

const mid = await mount({ value: "balanced" });
const buttons = stops(mid.container);
const shell = mid.container.querySelector(".shell-switch");
ok(buttons.length === 3, "the switch offers exactly three stops");
ok(buttons.map((b) => b.getAttribute("aria-pressed")).join(",") === "false,true,false", "the stored tier is the pressed stop");
ok((shell?.getAttribute("data-tier") ?? "") === "balanced", "the rail is positioned from the stored tier");
ok(mid.container.querySelector(".shell-switch__rail") !== null, "the sliding rail is part of the switch");
ok(buttons.every((b) => b.type === "button"), "stops never submit a composer form");
const labels = buttons.map((b) => (b.textContent ?? "").trim());
ok(labels.every((label) => label.length > 0 && !label.startsWith("settings.")), `stops are labelled from the dictionary (${labels.join(" / ")})`);

await act(async () => {
  buttons[2].dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true, cancelable: true }));
});
ok(mid.saved.join(",") === "fast", "clicking the fastest stop writes exactly the config value fast");
ok((mid.container.querySelector(".shell-switch")?.getAttribute("data-tier") ?? "") === "fast", "the rail follows the click before the write returns");
ok(!(mid.container.firstChild as HTMLElement).textContent?.includes("settings."), "no raw dictionary key leaks into the switch");
await act(async () => mid.root.unmount());

const pending = await mount({ value: "off", pending: true });
await act(async () => {
  stops(pending.container)[2].dispatchEvent(new dom.window.MouseEvent("click", { bubbles: true, cancelable: true }));
});
ok(stops(pending.container).every((b) => b.disabled), "while a write is in flight every stop is disabled");
await act(async () => pending.root.unmount());

const unknown = await mount({ value: "turbo" });
ok(stops(unknown.container).map((b) => b.getAttribute("aria-pressed")).join(",") === "true,false,false", "an unknown stored value falls back to off");
await act(async () => unknown.root.unmount());

const unreadable = await mount({ fail: true });
ok(unreadable.container.querySelector(".shell-switch") === null, "a failed read shows no switch rather than a guessed tier");
await act(async () => unreadable.root.unmount());

const testDir = fileURLToPath(new URL(".", import.meta.url));
const source = (relative: string) => readFileSync(resolve(testDir, relative), "utf8");
const composer = source("../components/Composer.tsx");
const mountAt = composer.indexOf("<ShellAsyncTierSwitch");
const sendAt = composer.indexOf("composer-toolbar-send");
ok(composer.includes('from "./ShellAsyncTierSwitch"'), "the composer imports the switch");
ok(composer.includes("app.SetShellAsyncSpeedTier(tier)"), "the composer writes through the same host command the settings field uses");
ok(mountAt > 0 && sendAt > 0 && mountAt < sendAt, "the switch sits in the composer's bottom row, not elsewhere");

dom.window.close();

console.log(`\nshell async switch: ${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
