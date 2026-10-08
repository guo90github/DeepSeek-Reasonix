import { asArray } from "./array";
import type { RuntimeProjection } from "./runtimeStateStore";
import type { ProjectNode } from "./types";

export type PendingDecision = { tabId: string; label: string };

/** Tabs that are blocked on an ask or an approval while another tab owns the view. */
export function selectPendingDecisions(
  projection: RuntimeProjection | undefined,
  activeTabId: string,
): PendingDecision[] {
  if (!projection) return [];

  const sessionLabels = new Map<string, string>();
  const topicLabels = new Map<string, string>();
  const remember = (node: ProjectNode) => {
    if ((node.kind === "topic" || node.kind === "global_topic") && node.topicId) {
      topicLabels.set(node.topicId, node.label);
    }
    if (node.sessionPath) sessionLabels.set(node.sessionPath, node.label);
    for (const child of asArray(node.children)) remember(child);
  };
  for (const topic of projection.topics) remember(topic.node);

  const waiting: PendingDecision[] = [];
  const seen = new Set<string>();
  for (const session of projection.sessions) {
    if (!session.state.pendingPrompt || session.tabId === "") continue;
    if (session.tabId === activeTabId || seen.has(session.tabId)) continue;
    seen.add(session.tabId);
    waiting.push({
      tabId: session.tabId,
      label: sessionLabels.get(session.sessionPath) ?? topicLabels.get(session.topicId) ?? session.tabId,
    });
  }
  return waiting;
}
