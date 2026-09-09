import { useEffect, useState } from "react";
import { useT } from "../lib/i18n";

export function formatAwaitSeconds(since: number, now: number): number {
  return Math.max(0, Math.floor((now - since) / 1000));
}

// Ticks locally so a long pre-first-token gap reads as progress, not a hang:
// only this subtree re-renders each second instead of the whole pane list.
export function LiveAwaitElapsed({ since }: { since: number }) {
  const t = useT();
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(id);
  }, []);
  return <span className="conversation-pane__await-elapsed">{t("split.awaitingElapsed", { s: formatAwaitSeconds(since, now) })}</span>;
}
