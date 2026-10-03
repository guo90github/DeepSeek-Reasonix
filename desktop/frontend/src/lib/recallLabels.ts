// 召回记录里的命中项按设计只有事实 id（`desktop/recall_record_view.go` "content-free
// by construction"），给人看时看不懂。这里把 id 解析成人能读的标签——标签取自宿主的
// facts 清单（`MemoryForTab(tabId)`，同一份数据记忆面板本来就在用），**不往记录里另存内容**，
// 所以历史会话的记录仍是纯指纹，只是显示时按 id 去当前事实里找名字。

import type { MemoryFact } from "../generated/desktopContract.generated";

export type RecallLabel = { label: string; hint?: string };
export type RecallLabels = ReadonlyMap<string, RecallLabel>;

/** 同时按 id 与 name 建索引：侧车写的是写者手上有的那个，两种都要能命中；查不到就回落显示 id。 */
export function buildRecallLabels(facts: readonly MemoryFact[]): RecallLabels {
  const labels = new Map<string, RecallLabel>();
  for (const fact of facts) {
    const label = (fact.title ?? "").trim() || fact.name;
    const hint = [fact.description?.trim(), fact.type, fact.scope, fact.freshness]
      .filter((part) => part !== undefined && part !== "")
      .join(" · ");
    const entry: RecallLabel = hint === "" ? { label } : { label, hint };
    if (fact.id) labels.set(fact.id, entry);
    if (fact.name) labels.set(fact.name, entry);
  }
  return labels;
}
