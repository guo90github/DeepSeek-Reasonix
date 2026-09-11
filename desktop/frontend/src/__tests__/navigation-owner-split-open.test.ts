// The navigation owner is the single open path for topics, blanks and history.
// Split lists several sessions at once, so it must use the add/reuse
// primitives; only the one-surface styles may collapse to activateTopic /
// ensureBlankSurface, which prune every other open session.
import assert from "node:assert/strict";
import {
  executeDesktopNavigation,
  type DesktopNavigationCapture,
  type DesktopNavigationIntent,
  type DesktopNavigationPorts,
} from "../app-runtime/desktopNavigationOwner";

function harness(intent: DesktopNavigationIntent, singleSurface: boolean) {
  const calls: string[] = [];
  const tab = { id: "tab-1", scope: "project", workspaceRoot: "/repo", topicId: "topic-1" };
  const record = (name: string) => () => {
    calls.push(name);
    return tab;
  };
  const ports = {
    isNavigationIntentCurrent: () => true,
    activateTopic: record("activateTopic"),
    openTopicSession: record("openTopicSession"),
    openGlobalTab: record("openGlobalTab"),
    openProjectTab: record("openProjectTab"),
    ensureBlankSurface: record("ensureBlankSurface"),
    ensureBlankTab: record("ensureBlankTab"),
    createIsolatedWorktree: record("createIsolatedWorktree"),
    registeredNavigationIntent: async () => "token",
    switchRemoteTab: record("switchRemoteTab"),
    openChannelSession: record("openChannelSession"),
    resumeSession: record("resumeSession"),
    listTabs: async () => [],
    openRemoteProject: record("openRemoteProject"),
    applyTabs: () => undefined,
    seedTab: () => undefined,
    reveal: () => undefined,
    projectChanged: () => undefined,
    closeHistory: () => undefined,
    listSessions: async () => [],
    applyHistorySessions: () => undefined,
    notice: () => undefined,
  } as unknown as DesktopNavigationPorts;
  const authority = { checkpoint: () => undefined } as unknown as Parameters<typeof executeDesktopNavigation>[1];
  const capture = { intent, navigationIntentSeq: 1, singleSurface, ports } satisfies DesktopNavigationCapture;
  return { capture, authority, calls };
}

async function route(intent: DesktopNavigationIntent, singleSurface: boolean): Promise<string[]> {
  const { capture, authority, calls } = harness(intent, singleSurface);
  await executeDesktopNavigation(capture, authority);
  return calls;
}

const projectTopic: DesktopNavigationIntent = { kind: "topic", scope: "project", workspaceRoot: "/repo", topicId: "topic-1" };
const historyTopic: DesktopNavigationIntent = { kind: "topic", scope: "project", workspaceRoot: "/repo", topicId: "topic-1", sessionPath: "/repo/sessions/s1.jsonl" };
const globalTopic: DesktopNavigationIntent = { kind: "topic", scope: "global", workspaceRoot: "", topicId: "topic-2" };
const projectBlank: DesktopNavigationIntent = { kind: "blank", scope: "project", workspaceRoot: "/repo" };

assert.deepEqual(await route(projectTopic, false), ["openProjectTab"], "split opens a project topic as its own tab");
assert.deepEqual(await route(historyTopic, false), ["openTopicSession"], "split resumes a history session into its own tab");
assert.deepEqual(await route(globalTopic, false), ["openGlobalTab"], "split opens a global topic as its own tab");
assert.deepEqual(await route(projectBlank, false), ["ensureBlankTab"], "split adds a blank tab instead of replacing the surface");
assert.deepEqual(await route(projectTopic, true), ["activateTopic"], "a one-surface layout still activates the topic in place");
assert.deepEqual(await route(projectBlank, true), ["ensureBlankSurface"], "a one-surface layout still replaces the surface");
console.log("navigation owner: split uses the add/reuse primitives, one-surface layouts stay in place");
