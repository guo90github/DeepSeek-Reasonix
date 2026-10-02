// The status bar's collaboration entry: always on screen, and it carries a count when
// the board has something a human should look at. Clicking it opens the same section
// the workspace panel shows, so the entry and the surface cannot disagree — enrolment
// included, which is why the popover hosts the section rather than a card list of its
// own. Unenrolled sessions are never polled: reading a board nobody joined would be
// work for nothing (AGENT_BUS §13.5).

import { useCallback, useEffect, useRef, useState } from "react";
import { Network } from "lucide-react";
import { app } from "../lib/bridge";
import { useT } from "../lib/i18n";
import { AnchoredPopover } from "./AnchoredPopover";
import { Tooltip } from "./Tooltip";
import { WorkspaceAgentBusSection, type AgentBusStatusView } from "./WorkspaceAgentBusSection";
import type { AgentBusBriefingView } from "./AgentBusPanel";

/** The badge is a convenience, not a live view: one cheap local board read per tick. */
const BADGE_POLL_MS = 30000;

export function AgentBusStatusItem({ loadBriefing, loadStatus, join, leave }: {
  /** Injected so the chip's two states can be tested without the host. */
  loadBriefing?: () => Promise<AgentBusBriefingView>;
  loadStatus?: () => Promise<AgentBusStatusView>;
  join?: () => Promise<AgentBusStatusView>;
  leave?: () => Promise<AgentBusStatusView>;
}) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const [attention, setAttention] = useState<number | null>(null);
  const anchorRef = useRef<HTMLButtonElement | null>(null);

  const readAttention = useCallback(async () => {
    try {
      const briefing = await (loadBriefing ? loadBriefing() : app.AgentBusBriefing());
      setAttention(briefing.signals.length);
    } catch {
      setAttention(null);
    }
  }, [loadBriefing]);

  useEffect(() => {
    void readAttention();
  }, [readAttention, open]);

  const polling = attention !== null;
  useEffect(() => {
    if (!polling) return undefined;
    const timer = window.setInterval(() => {
      if (document.visibilityState === "visible") void readAttention();
    }, BADGE_POLL_MS);
    return () => window.clearInterval(timer);
  }, [polling, readAttention]);

  useEffect(() => {
    const onFocus = () => void readAttention();
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, [readAttention]);

  const label = attention === null
    ? t("agentbus.statusIdle")
    : t("agentbus.statusAttention", { n: attention });
  return (
    <>
      <Tooltip label={label} className="statusbar__metric statusbar__metric--collab">
        <button
          ref={anchorRef}
          type="button"
          className="stat statusbar__collab"
          aria-label={label}
          aria-expanded={open}
          data-attention={attention ?? 0}
          onClick={() => setOpen((was) => !was)}
        >
          <span className="stat__label stat__label--icon" aria-hidden="true"><Network size={12} /></span>
          <span className="statusbar__collab-label">{t("agentbus.title")}</span>
          {attention !== null && attention > 0 ? (
            <span className="statusbar__collab-badge">{attention}</span>
          ) : null}
        </button>
      </Tooltip>
      <AnchoredPopover
        open={open}
        anchorRef={anchorRef}
        onClose={() => setOpen(false)}
        className="statusbar__collab-popover"
        align="end"
      >
        {open ? (
          <WorkspaceAgentBusSection
            loadStatus={loadStatus}
            join={join}
            leave={leave}
            onEnrolmentChange={() => void readAttention()}
          />
        ) : null}
      </AnchoredPopover>
    </>
  );
}
