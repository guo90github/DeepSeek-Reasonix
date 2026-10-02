// The collaboration panel: only the subtrees that need attention, with the signals
// behind them. Healthy work is a number, not a card (AGENT_BUS §13.5).

import { useT } from "../lib/i18n";
import type { DictKey } from "../locales/en";
import type { ReactNode } from "react";

export type AgentBusCardView = {
  subtree: string;
  nodes: number;
  atWork: number;
  parked: number;
  done: number;
  worst: string;
  signals: number;
  orphans: number;
  stalled: number;
  disputed: number;
};

export type AgentBusSignalView = {
  kind: string;
  subtree: string;
  node: string;
  detail: string;
};

export type AgentBusBriefingView = {
  participant: string;
  // Null is possible on the wire: Go marshals a nil slice as null. The panel treats
  // that as an empty board rather than crashing on it.
  cards: AgentBusCardView[] | null;
  signals: AgentBusSignalView[] | null;
  hidden: number;
  hiddenCards: number;
  healthySubtrees: number;
};

export type AgentBusAuthorizationView = {
  actor: string;
  reason: string;
  seq?: number;
};

export type AgentBusRefutationView = {
  actor: string;
  reason: string;
};

export type AgentBusNodeDetailView = {
  node: string;
  title: string;
  state: string;
  owner: string;
  ready: boolean;
  // Null on the wire for the same reason as the briefing's lists.
  deps: string[] | null;
  noProgress: number;
  refutations: AgentBusRefutationView[] | null;
  authorizations: AgentBusAuthorizationView[] | null;
  deliberating: boolean;
  verdict: string;
};

// Order mirrors the kernel's severity: what cannot ever run, then what nobody will
// notice on its own, then the rest.
const SEVERITY: Record<string, number> = { orphan: 0, stalled: 1, escalated: 2, disputed: 3, undecided: 4 };

const KIND_LABEL: Record<string, DictKey> = {
  orphan: "agentbus.kind.orphan",
  stalled: "agentbus.kind.stalled",
  escalated: "agentbus.kind.escalated",
  disputed: "agentbus.kind.disputed",
  undecided: "agentbus.kind.undecided",
};

function severityOf(kind: string): number {
  return SEVERITY[kind] ?? 9;
}

// AgentBusNodeDetail is what the record says about one step: what it waits for, what was
// disputed, and who authorized it. Read-only on purpose — the panel decides nothing here
// (§13.8): the authorizations are the part only the op log can answer.
function AgentBusNodeDetail({ detail, onClose }: {
  detail: AgentBusNodeDetailView;
  onClose?: () => void;
}) {
  const t = useT();
  return (
    <div className="agentbus-panel__node" data-node={detail.node} data-state={detail.state}>
      <div className="agentbus-panel__node-head" style={{ display: "flex", gap: 8, alignItems: "baseline" }}>
        <strong>{t("agentbus.detail", { node: detail.node })}</strong>
        <span>{t("agentbus.detail.state", { state: detail.state })}</span>
        {detail.ready ? <span>{t("agentbus.detail.ready")}</span> : null}
        {onClose ? (
          <button type="button" onClick={onClose}>
            {t("agentbus.detail.close")}
          </button>
        ) : null}
      </div>
      {(detail.deps ?? []).length > 0 ? (
        <p className="agentbus-panel__node-deps">{t("agentbus.detail.deps", { deps: (detail.deps ?? []).join(", ") })}</p>
      ) : null}
      {detail.deliberating ? <p className="agentbus-panel__node-deliberating">{t("agentbus.detail.deliberating")}</p> : null}
      {detail.verdict ? <p className="agentbus-panel__node-verdict">{t("agentbus.detail.verdict", { verdict: detail.verdict })}</p> : null}
      <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
        {(detail.refutations ?? []).map((refutation) => (
          <li key={`refute:${refutation.actor}`} className="agentbus-panel__refutation">
            {t("agentbus.detail.disputed", { actor: refutation.actor, reason: refutation.reason })}
          </li>
        ))}
        {(detail.authorizations ?? []).map((authorization) => (
          <li key={`grant:${authorization.actor}`} className="agentbus-panel__authorization">
            {t("agentbus.detail.authorized", { actor: authorization.actor, reason: authorization.reason })}
          </li>
        ))}
      </ul>
    </div>
  );
}

// Head is the panel's own title row, shared by the board view and the "not on a board
// yet" view so the entry always looks like the same section.
function Head({ who, enrol }: { who: string; enrol?: ReactNode }) {
  const t = useT();
  return (
    <header className="agentbus-panel__head" style={{ display: "flex", gap: 8, alignItems: "baseline" }}>
      <strong>{t("agentbus.title")}</strong>
      <span>{t("agentbus.participant", { who })}</span>
      {enrol}
    </header>
  );
}

export function AgentBusPanel({ view, onOpenNode, detail, detailNotice, onCloseDetail, enrol, notice }: {
  /** The board's first screen; null means this session is not on a board yet. */
  view?: AgentBusBriefingView | null;
  /** Opens a node; the host decides what opening means (fetch, navigate, both). */
  onOpenNode?: (node: string) => void;
  /** The step that was opened, once its record has been read. */
  detail?: AgentBusNodeDetailView | null;
  /** Shown while that record is being read, or when it could not be read. */
  detailNotice?: string;
  onCloseDetail?: () => void;
  /** The join/leave control: the host owns the enrolment, so it owns this too. */
  enrol?: ReactNode;
  /** One line about the section's own state — joining, pending identity, a failure. */
  notice?: string;
}) {
  const t = useT();
  const who = (view?.participant ?? "") || t("agentbus.unwired");
  // A host that sends null instead of [] must not take the app down: the board simply
  // reads as empty (that regression already shipped once, in v0.0.0-dev.101).
  const cards = view ? [...(view.cards ?? [])].sort((left, right) => {
    const bySeverity = severityOf(left.worst) - severityOf(right.worst);
    if (bySeverity !== 0) return bySeverity;
    if (left.signals !== right.signals) return right.signals - left.signals;
    return left.subtree.localeCompare(right.subtree);
  }) : [];

  if (!view) {
    return (
      <section className="agentbus-panel" aria-label={t("agentbus.title")}>
        <Head who={who} enrol={enrol} />
        {notice ? <p className="agentbus-panel__notice">{notice}</p> : null}
      </section>
    );
  }

  return (
    <section className="agentbus-panel" aria-label={t("agentbus.title")}>
      <Head who={who} enrol={enrol} />
      {notice ? <p className="agentbus-panel__notice">{notice}</p> : null}
      {cards.length === 0 ? (
        <p className="agentbus-panel__clear">{t("agentbus.clear", { n: view.healthySubtrees })}</p>
      ) : null}
      <ul className="agentbus-panel__cards" style={{ listStyle: "none", margin: 0, padding: 0 }}>
        {cards.map((card) => {
          const signals = (view.signals ?? []).filter((signal) => signal.subtree === card.subtree);
          return (
            <li key={card.subtree} className="agentbus-panel__card" data-worst={card.worst}>
              <div className="agentbus-panel__title" style={{ display: "flex", gap: 8, alignItems: "baseline" }}>
                <strong>{card.subtree}</strong>
                <span>{t(KIND_LABEL[card.worst] ?? "agentbus.kind.disputed")}</span>
                <span>
                  {t("agentbus.counts", {
                    atWork: card.atWork,
                    parked: card.parked,
                    done: card.done,
                    n: card.nodes,
                  })}
                </span>
              </div>
              <ul className="agentbus-panel__signals" style={{ listStyle: "none", margin: 0, padding: 0 }}>
                {signals.map((signal) => (
                  <li key={`${signal.kind}:${signal.node}`} className="agentbus-panel__signal" data-kind={signal.kind}>
                    {onOpenNode ? (
                      <button type="button" onClick={() => onOpenNode(signal.node)}>
                        {signal.node}
                      </button>
                    ) : (
                      <span>{signal.node}</span>
                    )}
                    <span className="agentbus-panel__detail"> {signal.detail}</span>
                  </li>
                ))}
              </ul>
            </li>
          );
        })}
      </ul>
      {detail ? (
        <AgentBusNodeDetail detail={detail} onClose={onCloseDetail} />
      ) : detailNotice ? (
        <p className="agentbus-panel__node-notice">{detailNotice}</p>
      ) : null}
      {view.hidden > 0 || view.hiddenCards > 0 ? (
        <p className="agentbus-panel__hidden">
          {t("agentbus.hidden", { cards: view.hiddenCards, signals: view.hidden })}
        </p>
      ) : null}
    </section>
  );
}
