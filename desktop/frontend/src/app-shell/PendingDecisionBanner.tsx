import { useSyncExternalStore } from "react";

import type { Translator } from "../lib/i18n";
import { selectPendingDecisions } from "../lib/pendingDecisions";
import { runtimeStateStore } from "../lib/runtimeStateStore";

export type PendingDecisionBannerProps = {
  t: Translator;
  activeTabId: string;
  onOpen: (tabId: string) => void;
};

const MAX_NAMED_TABS = 3;

/** Renders the sessions a user cannot see are blocked on a question or an approval. */
export function PendingDecisionBanner({ t, activeTabId, onOpen }: PendingDecisionBannerProps) {
  const snapshot = useSyncExternalStore(runtimeStateStore.subscribe, runtimeStateStore.getSnapshot);
  const waiting = selectPendingDecisions(snapshot, activeTabId);
  if (waiting.length === 0) return null;

  const named = waiting.slice(0, MAX_NAMED_TABS);
  const overflow = waiting.length - named.length;
  return (
    <div className="banner banner--warning banner--actionable" role="status">
      <span className="banner__msg">{t("pendingDecisions.title")}</span>
      <span className="banner__hint">{t("pendingDecisions.count", { count: waiting.length })}</span>
      <span className="banner__spacer" />
      {named.map((item) => (
        <button
          key={item.tabId}
          type="button"
          className="btn btn--small pending-decisions__open"
          title={item.label}
          aria-label={t("pendingDecisions.open", { label: item.label })}
          onClick={() => onOpen(item.tabId)}
        >
          {item.label}
        </button>
      ))}
      {overflow > 0 ? <span className="banner__hint">{`+${overflow}`}</span> : null}
    </div>
  );
}
