// The collaboration section of the workspace panel. It owns the fetches and nothing
// else: the card list stays a pure component. A failed fetch says so — an empty
// screen and a working board must never look the same. It also owns the one control
// that decides whether there is a board at all, because a session nobody enrolled
// reads nothing (AGENT_BUS §编排：参与者是会话).

import { useEffect, useState } from "react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { AgentBusControls } from "./AgentBusControls";
import { AgentBusPanel, type AgentBusBriefingView, type AgentBusNodeDetailView } from "./AgentBusPanel";
import type { AgentBusApplyArgs } from "../generated/desktopContract.generated";

export type AgentBusStatusView = {
  enrolled: boolean;
  participant: string;
  board: string;
  boardDir: string;
  defaultDir: string;
};

type StatusState =
  | { kind: "loading" }
  | { kind: "unavailable" }
  | { kind: "ready"; view: AgentBusStatusView };

type BriefingState =
  | { kind: "idle" }
  | { kind: "loading" }
  | { kind: "unavailable" }
  | { kind: "ready"; view: AgentBusBriefingView };

type DetailState =
  | { kind: "idle" }
  | { kind: "loading"; node: string }
  | { kind: "unavailable"; node: string }
  | { kind: "ready"; view: AgentBusNodeDetailView };

export function WorkspaceAgentBusSection({ onOpenNode, refreshKey, load, loadDetail, loadStatus, join, leave, apply, onEnrolmentChange }: {
  /** Opens a node; called in addition to showing the step here, for a host that navigates. */
  onOpenNode?: (node: string) => void;
  /** Changes to refetch, e.g. when the active session or its turn changes. */
  refreshKey?: string | number;
  /** Injected so the section's states can be tested without the host. */
  load?: () => Promise<AgentBusBriefingView>;
  /** Injected for the same reason: reading one step's record is a second fetch. */
  loadDetail?: (node: string) => Promise<AgentBusNodeDetailView>;
  /** Injected for the same reason: the enrolment the join/leave button acts on. */
  loadStatus?: () => Promise<AgentBusStatusView>;
  join?: () => Promise<AgentBusStatusView>;
  leave?: () => Promise<AgentBusStatusView>;
  /** Injected for the same reason: the human's board actions. */
  apply?: (args: AgentBusApplyArgs) => Promise<string>;
  /** Joining here changes what any other entry shows, so it is told. */
  onEnrolmentChange?: () => void;
}) {
  const t = useT();
  const [status, setStatus] = useState<StatusState>({ kind: "loading" });
  const [briefing, setBriefing] = useState<BriefingState>({ kind: "idle" });
  const [detail, setDetail] = useState<DetailState>({ kind: "idle" });
  const [trouble, setTrouble] = useState("");
  const [busy, setBusy] = useState(false);

  async function readBriefing() {
    setBriefing({ kind: "loading" });
    try {
      const view = await (load ? load() : app.AgentBusBriefing());
      setBriefing({ kind: "ready", view });
    } catch {
      setBriefing({ kind: "unavailable" });
    }
  }

  useEffect(() => {
    let alive = true;
    void (async () => {
      try {
        const view = await (loadStatus ? loadStatus() : app.AgentBusStatus());
        if (!alive) return;
        setStatus({ kind: "ready", view });
        if (view.enrolled && view.participant) await readBriefing();
      } catch {
        if (alive) setStatus({ kind: "unavailable" });
      }
    })();
    return () => {
      alive = false;
    };
  }, [refreshKey, load, loadStatus]);

  async function changeEnrolment(action: () => Promise<AgentBusStatusView>) {
    setBusy(true);
    setTrouble("");
    setDetail({ kind: "idle" });
    try {
      const view = await action();
      setStatus({ kind: "ready", view });
      setBriefing({ kind: "idle" });
      onEnrolmentChange?.();
      if (view.enrolled && view.participant) await readBriefing();
    } catch (err) {
      setTrouble(t("agentbus.enrol.failed", { err: err instanceof Error ? err.message : String(err) }));
    } finally {
      setBusy(false);
    }
  }

  async function openNode(node: string) {
    onOpenNode?.(node);
    setDetail({ kind: "loading", node });
    try {
      const view = await (loadDetail ? loadDetail(node) : app.AgentBusNodeDetail(node));
      setDetail({ kind: "ready", view });
    } catch {
      setDetail({ kind: "unavailable", node });
    }
  }

  if (status.kind === "loading") return null;
  if (status.kind === "unavailable") {
    return (
      <p className="workspace-agentbus__unavailable">{t("agentbus.unavailable")}</p>
    );
  }

  const button = (label: string, action: () => Promise<AgentBusStatusView>) => (
    <button type="button" disabled={busy} onClick={() => void changeEnrolment(action)}>
      {label}
    </button>
  );
  const leaveAction = leave ?? (() => app.AgentBusLeave());

  if (!status.view.enrolled) {
    return (
      <AgentBusPanel
        view={null}
        enrol={button(t("agentbus.join"), join ?? (() => app.AgentBusJoin()))}
        notice={trouble || t("agentbus.enrol.hint", { dir: status.view.defaultDir })}
      />
    );
  }

  const boardNotice = status.view.board ? t("agentbus.board", { board: status.view.board }) : "";
  const leaveButton = button(t("agentbus.leave"), leaveAction);

  // Enrolled before it had a path: the board is recorded, the identity is not, and
  // saying so is the difference between "joining did nothing" and "it takes effect".
  if (!status.view.participant || briefing.kind === "unavailable") {
    return (
      <AgentBusPanel
        view={null}
        enrol={leaveButton}
        notice={trouble || (status.view.participant ? t("agentbus.unavailable") : t("agentbus.enrol.pending"))}
      />
    );
  }
  if (briefing.kind !== "ready") {
    return <AgentBusPanel view={null} enrol={leaveButton} notice={trouble || boardNotice} />;
  }

  const notice =
    detail.kind === "loading"
      ? t("agentbus.detail.loading", { node: detail.node })
      : detail.kind === "unavailable"
        ? t("agentbus.detail.unavailable", { node: detail.node })
        : "";
  const applyAction = apply ?? ((args: AgentBusApplyArgs) => app.AgentBusApply(args));
  return (
    <>
      <AgentBusPanel
        view={briefing.view}
        enrol={leaveButton}
        notice={trouble || boardNotice}
        onOpenNode={(node) => void openNode(node)}
        detail={detail.kind === "ready" ? detail.view : null}
        detailNotice={notice}
        onCloseDetail={() => setDetail({ kind: "idle" })}
      />
      {/* The human acts on the board too, through the same verbs the model uses. */}
      <AgentBusControls apply={applyAction} onApplied={() => void readBriefing()} />
    </>
  );
}
