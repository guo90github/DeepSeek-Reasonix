// 第十六: the per-turn recall record as a module of the bottom-band card
// ("面板") — the same record the review page shows, next to the memory family.
// The card's rule is that an inapplicable module renders nothing at all, so the
// section only appears once the host says a record exists for this tab.

import { useEffect, useState } from "react";

import type { RecallRecordView } from "../generated/desktopContract.generated";
import { app } from "../lib/bridge";
import { FooterPanelSection, type FooterPanelModuleProps } from "./FooterPanel";
import { RecapRecallStrip } from "./RecapRecallStrip";

export function FooterRecallModule({ tabId }: FooterPanelModuleProps) {
  const [record, setRecord] = useState<RecallRecordView | null>(null);

  useEffect(() => {
    if (!tabId) {
      setRecord(null);
      return;
    }
    let cancelled = false;
    // Deferred call: a host (or a test stub) without this command must leave the
    // module hidden, not fail the whole card.
    void Promise.resolve()
      .then(() => app.RecallRecordForTab(tabId))
      .then((next) => {
        if (!cancelled) setRecord(next);
      })
      .catch(() => {
        if (!cancelled) setRecord(null);
      });
    return () => {
      cancelled = true;
    };
  }, [tabId]);

  if (!tabId || record === null || record.available !== true) return null;
  return (
    <FooterPanelSection title="memory.activity">
      <RecapRecallStrip record={record} />
    </FooterPanelSection>
  );
}
