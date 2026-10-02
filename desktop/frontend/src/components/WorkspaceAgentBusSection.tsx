// The collaboration section of the workspace panel. It owns the fetch and nothing
// else: the card list stays a pure component. A failed fetch says so — an empty
// screen and a working board must never look the same.

import { useEffect, useState } from "react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { AgentBusPanel, type AgentBusBriefingView } from "./AgentBusPanel";

type BriefingState =
  | { kind: "loading" }
  | { kind: "unavailable" }
  | { kind: "ready"; view: AgentBusBriefingView };

export function WorkspaceAgentBusSection({ onOpenNode, refreshKey, load }: {
  /** Opens a node; absent until the host can navigate to one. */
  onOpenNode?: (node: string) => void;
  /** Changes to refetch, e.g. when the active session or its turn changes. */
  refreshKey?: string | number;
  /** Injected so the section's two states can be tested without the host. */
  load?: () => Promise<AgentBusBriefingView>;
}) {
  const t = useT();
  const [state, setState] = useState<BriefingState>({ kind: "loading" });

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

  if (state.kind === "loading") return null;
  if (state.kind === "unavailable") {
    return (
      <p className="workspace-agentbus__unavailable">{t("agentbus.unavailable")}</p>
    );
  }
  return <AgentBusPanel view={state.view} onOpenNode={onOpenNode} />;
}
