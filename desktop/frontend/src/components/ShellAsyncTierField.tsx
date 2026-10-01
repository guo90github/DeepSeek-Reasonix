// The speed tier for long shell calls: off keeps every command in the
// foreground; balanced lifts the checks and builds the host recognizes; fast
// lifts any shell call that shares its batch with a later call. The value is a
// config key, so the hint says when a change takes effect.

import { Gauge } from "lucide-react";

import { useT, type DictKey } from "../lib/i18n";
import { SettingsField } from "./SettingsForm";
import { SettingsOptions } from "./SettingsOptions";

export const SHELL_ASYNC_TIERS = ["off", "balanced", "fast"] as const;

export type ShellAsyncTier = (typeof SHELL_ASYNC_TIERS)[number];

const TIER_LABELS: Record<ShellAsyncTier, DictKey> = {
  off: "settings.shellAsync.off",
  balanced: "settings.shellAsync.balanced",
  fast: "settings.shellAsync.fast",
};

/** Anything the config may hold, narrowed to a tier the control can show. */
export function normalizeShellAsyncTier(value: string | undefined): ShellAsyncTier {
  return value === "balanced" || value === "fast" ? value : "off";
}

export function ShellAsyncTierField({
  value,
  busy = false,
  onSelect,
}: {
  value: string | undefined;
  busy?: boolean;
  onSelect: (tier: ShellAsyncTier) => void;
}) {
  const t = useT();
  const current = normalizeShellAsyncTier(value);
  return (
    <SettingsField label={t("settings.shellAsync")} hint={t("settings.shellAsyncHint")} icon={<Gauge size={18} />}>
      <SettingsOptions layout="field" className="set-seg" aria-label={t("settings.shellAsync")}>
        {SHELL_ASYNC_TIERS.map((tier) => (
          <button
            key={tier}
            type="button"
            className={`set-seg__btn${current === tier ? " set-seg__btn--on" : ""}`}
            aria-pressed={current === tier}
            disabled={busy}
            onClick={() => onSelect(tier)}
          >
            {t(TIER_LABELS[tier])}
          </button>
        ))}
      </SettingsOptions>
    </SettingsField>
  );
}
