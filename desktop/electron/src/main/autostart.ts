// Opt-in login item, so a crashed or rebooted machine brings the desktop back
// without anyone watching. It stays off unless the operator writes
// desktop-autostart.json in the Reasonix home: nothing here registers itself on
// a machine that never asked, and disabling the file removes the registration.

import { readFileSync } from "node:fs";
import { join } from "node:path";

export const AUTOSTART_FILE = "desktop-autostart.json";

export interface AutostartPolicy {
  enabled: boolean;
  /** Kept as a login item only while this many launches have been clean. */
  args?: string[];
}

export interface LoginItemApp {
  setLoginItemSettings(settings: { openAtLogin: boolean; openAsHidden?: boolean; args?: string[] }): void;
  getLoginItemSettings(): { openAtLogin: boolean };
}

export function readAutostartPolicy(home: string): AutostartPolicy {
  if (!home) return { enabled: false };
  try {
    const parsed = JSON.parse(readFileSync(join(home, AUTOSTART_FILE), "utf8")) as Partial<AutostartPolicy>;
    return { enabled: parsed?.enabled === true, ...(Array.isArray(parsed?.args) ? { args: parsed.args } : {}) };
  } catch {
    return { enabled: false };
  }
}

/** Applies the policy and reports whether the login item is now registered. */
export function applyAutostart(app: LoginItemApp, home: string): boolean {
  const policy = readAutostartPolicy(home);
  if (app.getLoginItemSettings().openAtLogin === policy.enabled) return policy.enabled;
  app.setLoginItemSettings({
    openAtLogin: policy.enabled,
    openAsHidden: policy.enabled,
    ...(policy.enabled ? { args: policy.args ?? ["--unattended-boot"] } : {}),
  });
  return policy.enabled;
}
