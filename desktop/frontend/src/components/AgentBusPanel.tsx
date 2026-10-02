// The collaboration panel: only the subtrees that need attention, with the signals
// behind them. Healthy work is a number, not a card (AGENT_BUS §13.5).

import { useT } from "../lib/i18n";
import type { DictKey } from "../locales/en";

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
  cards: AgentBusCardView[];
  signals: AgentBusSignalView[];
  hidden: number;
  hiddenCards: number;
  healthySubtrees: number;
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

export function AgentBusPanel({ view, onOpenNode }: {
  view: AgentBusBriefingView;
  /** Opens a node; absent until the host can navigate to one. */
  onOpenNode?: (node: string) => void;
}) {
  const t = useT();
  const cards = [...view.cards].sort((left, right) => {
    const bySeverity = severityOf(left.worst) - severityOf(right.worst);
    if (bySeverity !== 0) return bySeverity;
    if (left.signals !== right.signals) return right.signals - left.signals;
    return left.subtree.localeCompare(right.subtree);
  });
  const who = view.participant || t("agentbus.unwired");

  return (
    <section className="agentbus-panel" aria-label={t("agentbus.title")}>
      <header className="agentbus-panel__head" style={{ display: "flex", gap: 8, alignItems: "baseline" }}>
        <strong>{t("agentbus.title")}</strong>
        <span>{t("agentbus.participant", { who })}</span>
      </header>
      {cards.length === 0 ? (
        <p className="agentbus-panel__clear">{t("agentbus.clear", { n: view.healthySubtrees })}</p>
      ) : null}
      <ul className="agentbus-panel__cards" style={{ listStyle: "none", margin: 0, padding: 0 }}>
        {cards.map((card) => {
          const signals = view.signals.filter((signal) => signal.subtree === card.subtree);
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
      {view.hidden > 0 || view.hiddenCards > 0 ? (
        <p className="agentbus-panel__hidden">
          {t("agentbus.hidden", { cards: view.hiddenCards, signals: view.hidden })}
        </p>
      ) : null}
    </section>
  );
}
