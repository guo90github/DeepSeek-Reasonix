// Unattended master switch, rendered in the tab strip left of the new-session
// button. It writes the config at once; the running app keeps its startup
// value, so the tooltip is what tells the user a restart applies the change.
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
  const label = t("heartbeat.unattended");
  return (
    <Tooltip label={t("heartbeat.unattendedHint")}>
      <button
        type="button"
        className={`tabbar__mode-badge tabbar__mode-badge--${on ? "yolo" : "plan"}`}
        aria-label={label}
        aria-pressed={on}
        data-unattended={on ? "on" : "off"}
        onClick={flip}
      >
        {label}
      </button>
    </Tooltip>
  );
}
