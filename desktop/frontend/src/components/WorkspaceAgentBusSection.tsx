// The collaboration section of the workspace panel. It owns the fetches and nothing
// else: the card list stays a pure component. A failed fetch says so — an empty
// screen and a working board must never look the same.

import { useEffect, useState } from "react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { AgentBusPanel, type AgentBusBriefingView, type AgentBusNodeDetailView } from "./AgentBusPanel";

type BriefingState =
  | { kind: "loading" }
  | { kind: "unavailable" }
  | { kind: "ready"; view: AgentBusBriefingView };

type DetailState =
  | { kind: "idle" }
  | { kind: "loading"; node: string }
  | { kind: "unavailable"; node: string }
  | { kind: "ready"; view: AgentBusNodeDetailView };

export function WorkspaceAgentBusSection({ onOpenNode, refreshKey, load, loadDetail }: {
  /** Opens a node; called in addition to showing the step here, for a host that navigates. */
  onOpenNode?: (node: string) => void;
  /** Changes to refetch, e.g. when the active session or its turn changes. */
  refreshKey?: string | number;
  /** Injected so the section's two states can be tested without the host. */
  load?: () => Promise<AgentBusBriefingView>;
  /** Injected for the same reason: reading one step's record is a second fetch. */
  loadDetail?: (node: string) => Promise<AgentBusNodeDetailView>;
}) {
  const t = useT();
  const [state, setState] = useState<BriefingState>({ kind: "loading" });
  const [detail, setDetail] = useState<DetailState>({ kind: "idle" });

  useEffect(() => {
    let alive = true;
    void (async () => {
      try {
        const view = await (load ? load() : app.AgentBusBriefing());
        if (alive) setState({ kind: "ready", view });
      } catch {
        if (alive) setState({ kind: "unavailable" });
      }
    })();
    return () => {
      alive = false;
    };
  }, [refreshKey, load]);

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

  if (state.kind === "loading") return null;
  if (state.kind === "unavailable") {
    return (
      <p className="workspace-agentbus__unavailable">{t("agentbus.unavailable")}</p>
    );
  }
  const notice =
    detail.kind === "loading"
      ? t("agentbus.detail.loading", { node: detail.node })
      : detail.kind === "unavailable"
        ? t("agentbus.detail.unavailable", { node: detail.node })
        : "";
  return (
    <AgentBusPanel
      view={state.view}
      onOpenNode={(node) => void openNode(node)}
      detail={detail.kind === "ready" ? detail.view : null}
      detailNotice={notice}
      onCloseDetail={() => setDetail({ kind: "idle" })}
    />
  );
}
