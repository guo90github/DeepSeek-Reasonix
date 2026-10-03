// 第十六: the per-turn recall record as a module of the bottom-band card
// ("面板") — the same record the review page shows, next to the memory family.
// The card's rule is that an inapplicable module renders nothing at all, so the
// section only appears once the host says a record exists for this tab.
//
// The record carries ids only, so the module also reads the tab's fact list to
// put readable names next to them (the same list the memory panel shows).

import { useEffect, useState } from "react";

import type { MemoryFact, RecallRecordView } from "../generated/desktopContract.generated";
import { app } from "../lib/bridge";
import { FooterPanelSection, type FooterPanelModuleProps } from "./FooterPanel";
import { RecapRecallStrip } from "./RecapRecallStrip";

export function FooterRecallModule({ tabId }: FooterPanelModuleProps) {
  const [record, setRecord] = useState<RecallRecordView | null>(null);
  const [facts, setFacts] = useState<readonly MemoryFact[]>([]);

  useEffect(() => {
    if (!tabId) {
      setRecord(null);
      setFacts([]);
      return;
    }
    let cancelled = false;
    // Deferred calls: a host (or a test stub) without a command must leave the
    // module hidden, not fail the whole card.
    void Promise.resolve()
      .then(() => app.RecallRecordForTab(tabId))
      .then((next) => {
        if (!cancelled) setRecord(next);
      })
      .catch(() => {
        if (!cancelled) setRecord(null);
      });
    void Promise.resolve()
      .then(() => app.MemoryForTab(tabId))
      .then((view) => {
        if (!cancelled) setFacts(view.facts);
      })
      .catch(() => {
        if (!cancelled) setFacts([]);
      });
    return () => {
      cancelled = true;
    };
  }, [tabId]);

  if (!tabId || record === null || record.available !== true) return null;
  return (
    <FooterPanelSection title="memory.activity">
      <RecapRecallStrip record={record} facts={facts} />
    </FooterPanelSection>
  );
}
