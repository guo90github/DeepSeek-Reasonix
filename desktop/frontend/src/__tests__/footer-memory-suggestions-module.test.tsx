// Run: tsx src/__tests__/footer-memory-suggestions-module.test.tsx

// The suggestions module: candidates from the local-history scan can be confirmed
// straight from the panel, and a session with nothing pending shows no section.

import React, { act } from "react";
import { createRoot } from "react-dom/client";
import { FooterMemorySuggestionsModule } from "../components/FooterMemorySuggestionsModule";
import type { AppBindings } from "../lib/bridge";
import { LocaleProvider } from "../lib/i18n";
import type { MemorySuggestion, MemorySuggestionsView, SkillSuggestion } from "../lib/types";
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

const memoryCandidate: MemorySuggestion = {
  id: "mem-1",
  name: "split-panel-width",
  title: "分栏面板宽度",
  description: "面板宽度比例存在 localStorage 的 footerPanelShare。",
  type: "project",
  scope: "project",
  body: "…",
  reason: "本轮讨论过",
  evidence: ["会话记录"],
};

const skillCandidate: SkillSuggestion = {
  id: "skill-1",
  name: "footer-panel",
  description: "把底部面板当插拔板用。",
  scope: "project",
  body: "…",
  reason: "重复出现",
  evidence: ["会话记录"],
};

function suggestionsView(memories: MemorySuggestion[], skills: SkillSuggestion[]): MemorySuggestionsView {
  return { memories, skills, generatedAt: "2026-09-29T13:00:00Z", available: true, source: "local-history" };
}

async function renderModule(view: MemorySuggestionsView) {
  const dom = installDom();
  const accepted = { memories: [] as string[], skills: [] as string[] };
  installDesktopHostStub(({
    main: {
      App: {
        MemorySuggestionsForTab: async () => view,
        AcceptMemorySuggestionForTab: async (_tabID: string, candidate: MemorySuggestion) => {
          accepted.memories.push(candidate.id);
          return candidate.name;
        },
        AcceptSkillSuggestionForTab: async (_tabID: string, candidate: SkillSuggestion) => {
          accepted.skills.push(candidate.id);
          return candidate.name;
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
        <FooterMemorySuggestionsModule tabId="tab-a" workspaceScopeKey="scope-a" />
      </LocaleProvider>,
    );
    await flushPromises();
  });
  return { dom, root, accepted };
}

console.log("\nfooter memory suggestions module");

{
  const { dom, root, accepted } = await renderModule(suggestionsView([memoryCandidate], [skillCandidate]));
  await act(async () => {
    await waitFor("suggestions answer", () => document.querySelector(".footer-suggest") !== null);
  });
  ok(document.querySelectorAll(".footer-suggest__row").length === 2, "both candidate kinds are listed");
  ok(document.body.textContent?.includes("分栏面板宽度") === true, "a memory candidate shows its title");

  const buttons = document.querySelectorAll<HTMLButtonElement>(".footer-suggest__accept");
  await act(async () => {
    buttons[0]?.click();
    await flushPromises();
  });
  ok(accepted.memories.length === 1 && accepted.memories[0] === "mem-1", "confirming the memory candidate writes it");
  ok(document.querySelector(".footer-suggest__accepted") !== null, "the confirmed row reports success");

  await act(async () => {
    document.querySelectorAll<HTMLButtonElement>(".footer-suggest__accept")[0]?.click();
    await flushPromises();
  });
  ok(accepted.skills.length === 1 && accepted.skills[0] === "skill-1", "confirming the skill candidate uses the skill store");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

{
  const { dom, root } = await renderModule(suggestionsView([], []));
  await act(async () => {
    await flushPromises();
  });
  ok(document.querySelector(".footer-suggest") === null, "a session with nothing pending renders no section");
  await act(async () => {
    root.unmount();
  });
  dom.window.close();
}

process.stdout.write(`\n${passed} passed, ${failed} failed\n`);
if (failed > 0) process.exitCode = 1;
