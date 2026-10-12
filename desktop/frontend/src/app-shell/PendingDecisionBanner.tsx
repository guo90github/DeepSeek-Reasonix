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
  // A question queued behind the one on screen is not a hidden session, but it
  // blocks the same way, so the banner reports both instead of staying silent.
  const queued = snapshot?.sessions.find((session) => session.tabId === activeTabId)?.state?.queuedPrompts ?? 0;
  const queuedHint = queued > 0 ? t("pendingDecisions.queuedInSession", { count: queued }) : "";
  if (waiting.length === 0 && !queuedHint) return null;

  const named = waiting.slice(0, MAX_NAMED_TABS);
  const overflow = waiting.length - named.length;
  return (
    <div className="banner banner--warning banner--actionable" role="status">
      <span className="banner__msg">{waiting.length > 0 ? t("pendingDecisions.title") : queuedHint}</span>
      {waiting.length > 0 ? <span className="banner__hint">{t("pendingDecisions.count", { count: waiting.length })}</span> : null}
      {waiting.length > 0 && queuedHint ? <span className="banner__hint">{queuedHint}</span> : null}
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
