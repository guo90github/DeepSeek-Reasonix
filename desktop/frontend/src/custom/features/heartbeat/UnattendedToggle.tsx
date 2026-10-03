// Unattended master switch, rendered in the tab strip left of the new-session
// button. It writes the config at once; the running app keeps its startup value,
// so both the label and the hover hint say that a restart applies the change.
import { useCallback, useEffect, useState } from "react";
import { Tooltip } from "../../../components/Tooltip";
import {
  heartbeatSetUnattended,
  heartbeatUnattended,
  heartbeatWatchdogStatus,
  type HeartbeatUnattendedState,
} from "./heartbeat.bridge";
import { useHeartbeatT } from "./heartbeat.i18n";
import { unattendedPresentation, type UnattendedWatchdogState } from "./heartbeat.presentation";

export function UnattendedToggle() {
  const t = useHeartbeatT();
  const [state, setState] = useState<HeartbeatUnattendedState | null>(null);
  const [watchdog, setWatchdog] = useState<UnattendedWatchdogState | null>(null);
  const [pending, setPending] = useState(false);

  useEffect(() => {
    let live = true;
    void heartbeatUnattended().then(
      (value) => {
        if (live) setState(value);
      },
      () => {
        if (live) setState(null);
      },
    );
    void heartbeatWatchdogStatus().then((value) => {
      if (live) setWatchdog(value);
    });
    return () => {
      live = false;
    };
  }, []);

  const flip = useCallback(() => {
    if (state === null || pending) return;
    setPending(true);
    void heartbeatSetUnattended(!state.on)
      .then((value) => setState(value), () => undefined)
      .finally(() => setPending(false));
  }, [state, pending]);

  if (state === null) return null;
  const presentation = unattendedPresentation({ ...state, watchdog });
  const label = t(presentation.stateKey);
  const hint = t(presentation.hintKey, presentation.hintParams);
  return (
    <Tooltip label={`${t("heartbeat.unattended")} · ${label} — ${hint}`}>
      <button
        type="button"
        className="tabbar__unattended"
        role="switch"
        aria-checked={state.on}
        aria-label={`${t("heartbeat.unattended")}：${label}`}
        data-unattended={state.on ? "on" : "off"}
        disabled={pending}
        onClick={flip}
      >
        <span className="tabbar__unattended-track" aria-hidden="true">
          <span className="tabbar__unattended-knob" />
        </span>
        <span className="tabbar__unattended-label">{label}</span>
      </button>
    </Tooltip>
  );
}
