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
  /** Nobody who asked for this subtree can be reached from this host: history, not work. */
  leftover?: boolean;
};

export type AgentBusSignalView = {
  kind: string;
  subtree: string;
  node: string;
  detail: string;
};

export type AgentBusMemberView = {
  participant: string;
  /** The session's own display name, which is what a reader recognises. */
  label: string;
  /** This is the session reading the panel. */
  self: boolean;
};

export type AgentBusBriefingView = {
  participant: string;
  /** Who is here with me: the sessions this host speaks as (2026-10-05). */
  members?: AgentBusMemberView[] | null;
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
// notice on its own, then the rest. A refused claim means work is parked for budget
// rather than for a reason the board knows, so it ranks with "nobody will notice".
const SEVERITY: Record<string, number> = { orphan: 0, stalled: 1, escalated: 2, disputed: 3, undecided: 4, budget: 5, rate_limited: 6 };

const KIND_LABEL: Record<string, DictKey> = {
  orphan: "agentbus.kind.orphan",
  stalled: "agentbus.kind.stalled",
  escalated: "agentbus.kind.escalated",
  disputed: "agentbus.kind.disputed",
  undecided: "agentbus.kind.undecided",
  budget: "agentbus.kind.budget",
  rate_limited: "agentbus.kind.rateLimited",
  wake_undelivered: "agentbus.kind.wakeUndelivered",
  wake_unreachable: "agentbus.kind.wakeUnreachable",
  node_rate: "agentbus.kind.nodeRate",
  other: "agentbus.kind.other",
};

// An unknown kind reads as "other signal", never as a known one: a row about an undelivered
// wake used to be labelled "under deliberation" because everything unlisted fell back to
// disputed, which told the reader the opposite of what the row said (2026-10-05).
function labelKeyOf(kind: string): DictKey {
  return KIND_LABEL[kind] ?? "agentbus.kind.other";
}

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

export function AgentBusPanel({ view, onOpenNode, detail, detailNotice, onCloseDetail, enrol, notice, onRetire, onVerb }: {
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
  /** Drops one leftover subtree; the host runs the board's own two-step protocol. */
  onRetire?: (subtree: string) => void;
  /** A verb the human picked on a node: the form fills in the verb and the node. */
  onVerb?: (action: string, node: string) => void;
}) {
  const t = useT();
  // Who is here with me, as this host knows it; a host that sends nothing draws no roster.
  const members = view?.members ?? [];
  const who = (view?.participant ?? "") || t("agentbus.unwired");
  // A host that sends null instead of [] must not take the app down: the board simply
  // reads as empty (that regression already shipped once, in v0.0.0-dev.101).
  const cards = view ? [...(view.cards ?? [])].sort((left, right) => {
    const bySeverity = severityOf(left.worst) - severityOf(right.worst);
    if (bySeverity !== 0) return bySeverity;
    if (left.signals !== right.signals) return right.signals - left.signals;
    return left.subtree.localeCompare(right.subtree);
  }) : [];
  // Leftover work is history, not a call to act: its participants are not on this host at all, so
  // it folds away by default and stays countable by hand (2026-10-05).
  const live = cards.filter((card) => !card.leftover);
  const leftover = cards.filter((card) => card.leftover);

  // One card, drawn the same way in either list. A leftover card additionally offers to retire it:
  // the host runs the board's own two-step protocol (abandon, then the decision), never a side door.
  const cardRow = (card: AgentBusCardView, leftoverCard = false) => {
    const signals = (view?.signals ?? []).filter((signal) => signal.subtree === card.subtree);
    return (
      <li key={card.subtree} className="agentbus-panel__card" data-worst={card.worst} data-leftover={leftoverCard || undefined}>
        <div className="agentbus-panel__title">
          <span className="agentbus-panel__card-name" title={card.subtree}>
            {card.subtree}
          </span>
          <span className={`agentbus-panel__chip agentbus-panel__chip--${card.worst}`}>{t(labelKeyOf(card.worst))}</span>
          {leftoverCard && onRetire ? (
            <button
              type="button"
              className="agentbus-panel__card-action"
              title={t("agentbus.leftover.retire.hint")}
              aria-label={t("agentbus.leftover.retire.hint")}
              onClick={() => onRetire(card.subtree)}
            >
              {t("agentbus.leftover.retire")}
            </button>
          ) : null}
        </div>
        <p className="agentbus-panel__card-counts">
          {t("agentbus.counts", {
            atWork: card.atWork,
            parked: card.parked,
            done: card.done,
            n: card.nodes,
          })}
        </p>
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
              <span className="agentbus-panel__detail">{signal.detail}</span>
              {onVerb && signal.node ? (
                <span className="agentbus-panel__quick">
                  {(["claim", "decide", "refute"] as const).map((verb) => (
                    <button key={verb} type="button" onClick={() => onVerb(verb, signal.node)}>
                      {t(`agentbus.quick.${verb}` as "agentbus.quick.claim")}
                    </button>
                  ))}
                </span>
              ) : null}
            </li>
          ))}
        </ul>
      </li>
    );
  };

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
      {members.length > 0 ? (
        <p className="agentbus-panel__members">
          <span className="agentbus-panel__detail">{t("agentbus.members", { n: members.length })}</span>
          {members.map((member) => {
            const name = member.label || member.participant;
            return (
              <span key={member.participant} className="agentbus-panel__chip" title={member.participant}>
                {member.self ? t("agentbus.members.you", { label: name }) : name}
              </span>
            );
          })}
        </p>
      ) : null}
      {notice ? <p className="agentbus-panel__notice">{notice}</p> : null}
      {cards.length === 0 ? (
        <p className="agentbus-panel__clear">{t("agentbus.clear", { n: view.healthySubtrees })}</p>
      ) : null}
      {(view.signals ?? []).some((signal) => !signal.subtree) ? (
        <ul className="agentbus-panel__signals agentbus-panel__host-signals" style={{ listStyle: "none", margin: 0, padding: 0 }}>
          {(view.signals ?? [])
            .filter((signal) => !signal.subtree)
            .map((signal) => (
              <li key={`${signal.kind}:${signal.detail}`} className="agentbus-panel__signal" data-kind={signal.kind}>
                <span className={`agentbus-panel__chip agentbus-panel__chip--${signal.kind}`}>{t(labelKeyOf(signal.kind))}</span>
                <span className="agentbus-panel__detail">{signal.detail}</span>
              </li>
            ))}
        </ul>
      ) : null}
      <ul className="agentbus-panel__cards" style={{ listStyle: "none", margin: 0, padding: 0 }}>
        {live.map((card) => cardRow(card))}
      </ul>
      {leftover.length > 0 ? (
        <details className="agentbus-panel__leftover" data-leftover={leftover.length}>
          <summary>{t("agentbus.leftover.summary", { n: leftover.length })}</summary>
          <ul className="agentbus-panel__cards" style={{ listStyle: "none", margin: 0, padding: 0 }}>
            {leftover.map((card) => cardRow(card, true))}
          </ul>
        </details>
      ) : null}
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
