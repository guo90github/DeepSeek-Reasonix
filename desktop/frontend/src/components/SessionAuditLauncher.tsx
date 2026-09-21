// SessionAuditLauncher owns the whole-session audit trigger and its modal. It
// renders the trigger in the tab strip next to the new-session button, so the
// shell view stays a pure assembly and the run's state lives with its owner.
// The audit covers the turns the transcript currently holds (loaded turns only)
// and is one-shot: nothing is persisted.
import { lazy, Suspense, useMemo, useState } from "react";
import { BrainCircuit } from "lucide-react";
import { useT } from "../lib/i18n";
import { collectSessionAuditTurns, countAuditableTurns } from "../lib/sessionAuditTurns";
import type { Item } from "../lib/useController";
import type { SessionAuditTurn } from "../generated/desktopContract.generated";
import { Tooltip } from "./Tooltip";

const SessionAuditModal = lazy(() => import("./SessionAuditModal").then((module) => ({ default: module.SessionAuditModal })));

export function SessionAuditLauncher({ items, turnBase, tabId, enabled = true }: {
  items: readonly Item[];
  turnBase: number;
  tabId?: string;
  enabled?: boolean;
}) {
  const t = useT();
  const [turns, setTurns] = useState<SessionAuditTurn[] | null>(null);
  const auditable = useMemo(() => countAuditableTurns(items, turnBase), [items, turnBase]);
  const available = enabled && auditable > 0;
  return (
    <>
      <Tooltip label={available ? t("sessionAudit.triggerHint") : t("sessionAudit.empty")} className="tabbar__icon-trigger">
        <button
          type="button"
          className="tabbar__new tabbar__audit"
          aria-label={t("sessionAudit.trigger")}
          disabled={!available}
          onClick={() => {
            const collected = collectSessionAuditTurns(items, turnBase);
            if (collected.length > 0) setTurns(collected);
          }}
        >
          <BrainCircuit size={13} />
        </button>
      </Tooltip>
      {turns && (
        <Suspense fallback={null}>
          <SessionAuditModal turns={turns} tabId={tabId} onClose={() => setTurns(null)} />
        </Suspense>
      )}
    </>
  );
}
