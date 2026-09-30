// Unattended master switch, rendered in the tab strip left of the new-session
// button. It writes the config at once; the running app keeps its startup value,
// so both the label and the hover hint say that a restart applies the change.
import { useCallback, useEffect, useState } from "react";
import { Tooltip } from "../../../components/Tooltip";
import { heartbeatSetUnattended, heartbeatUnattended } from "./heartbeat.bridge";
import { useHeartbeatT } from "./heartbeat.i18n";

export function UnattendedToggle() {
  const t = useHeartbeatT();
  const [on, setOn] = useState<boolean | null>(null);
  const [pending, setPending] = useState(false);

  useEffect(() => {
    let live = true;
    void heartbeatUnattended().then(
      (value) => {
        if (live) setOn(value);
      },
      () => {
        if (live) setOn(null);
      },
    );
    return () => {
      live = false;
    };
  }, []);

  const flip = useCallback(() => {
    if (on === null || pending) return;
    setPending(true);
    void heartbeatSetUnattended(!on)
      .then((value) => setOn(value), () => undefined)
      .finally(() => setPending(false));
  }, [on, pending]);

  if (on === null) return null;
  const state = on ? t("heartbeat.unattendedOn") : t("heartbeat.unattendedOff");
  const hint = on ? t("heartbeat.unattendedOnHint") : t("heartbeat.unattendedOffHint");
  return (
    <Tooltip label={`${t("heartbeat.unattended")} · ${state} — ${hint}`}>
      <button
        type="button"
        className="tabbar__unattended"
        role="switch"
        aria-checked={on}
        aria-label={`${t("heartbeat.unattended")}：${state}`}
        data-unattended={on ? "on" : "off"}
        disabled={pending}
        onClick={flip}
      >
        <span className="tabbar__unattended-track" aria-hidden="true">
          <span className="tabbar__unattended-knob" />
        </span>
        <span className="tabbar__unattended-label">{state}</span>
      </button>
    </Tooltip>
  );
}
