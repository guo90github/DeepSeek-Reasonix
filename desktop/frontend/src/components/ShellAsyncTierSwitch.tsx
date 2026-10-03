// The speed tier as a sliding switch for the composer's bottom row (需求第九的
// "档位控件"): three stops, one rail that slides under the current one. The
// settings page keeps the detailed field (with its hint); this is the quick
// switch, so the two read and write the same config key.

import { useCallback, useEffect, useRef, useState } from "react";

import { useT, type DictKey } from "../lib/i18n";
import { SHELL_ASYNC_TIERS, normalizeShellAsyncTier, type ShellAsyncTier } from "./ShellAsyncTierField";
import "./ShellAsyncTierSwitch.css";

const TIER_LABELS: Record<ShellAsyncTier, DictKey> = {
  off: "settings.shellAsync.off",
  balanced: "settings.shellAsync.balanced",
  fast: "settings.shellAsync.fast",
};

export function ShellAsyncTierSwitch({
  load,
  save,
  disabled = false,
}: {
  load: () => Promise<string | undefined>;
  save: (tier: ShellAsyncTier) => Promise<void>;
  disabled?: boolean;
}) {
  const t = useT();
  const [tier, setTier] = useState<ShellAsyncTier | null>(null);
  const [busy, setBusy] = useState(false);
  // Callers pass inline closures; the switch reads once on mount so a new
  // closure identity cannot re-ask the host on every composer render.
  const loadRef = useRef(load);
  loadRef.current = load;

  const fetchTier = useCallback(async () => {
    try {
      setTier(normalizeShellAsyncTier(await loadRef.current()));
    } catch {
      setTier(null);
    }
  }, []);

  useEffect(() => {
    void fetchTier();
  }, [fetchTier]);

  const pick = (next: ShellAsyncTier) => {
    if (busy || disabled || next === tier) return;
    setTier(next);
    setBusy(true);
    void Promise.resolve()
      .then(() => save(next))
      .catch(() => fetchTier())
      .finally(() => setBusy(false));
  };

  // No host answer yet (or the read failed): showing a tier we did not read
  // would be a lie, so the switch stays away until it knows.
  if (tier === null) return null;

  return (
    <div
      className={`shell-switch shell-switch--${tier}`}
      data-tier={tier}
      role="group"
      aria-label={t("settings.shellAsync")}
      title={t("settings.shellAsyncHint")}
    >
      <span className="shell-switch__rail" aria-hidden="true" />
      {SHELL_ASYNC_TIERS.map((value) => (
        <button
          key={value}
          type="button"
          className={`shell-switch__stop${tier === value ? " shell-switch__stop--on" : ""}`}
          aria-pressed={tier === value}
          disabled={busy || disabled}
          onClick={() => pick(value)}
        >
          {t(TIER_LABELS[value])}
        </button>
      ))}
    </div>
  );
}
