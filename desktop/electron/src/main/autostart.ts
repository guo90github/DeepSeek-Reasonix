// Opt-in login item, so a crashed or rebooted machine brings the desktop back
// without anyone watching. It stays off unless the operator writes
// desktop-autostart.json in the Reasonix home: nothing here registers itself on
// a machine that never asked, and disabling the file removes the registration.

import { lstatSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { findInstallRoot } from "./upgradeRollback.js";

export const AUTOSTART_FILE = "desktop-autostart.json";

// The install-root launchers, portable alias first. Both resolve current.json
// on every run. Mirrors internal/installlayout.StableRelaunchPath.
const STABLE_LAUNCHER_NAMES =
  process.platform === "win32" ? ["Reasonix.exe", "reasonix-launcher.exe"] : ["reasonix-launcher"];

export interface AutostartPolicy {
  /** False when no policy file was read: that machine is never touched. */
  present: boolean;
  enabled: boolean;
  /** Kept as a login item only while this many launches have been clean. */
  args?: string[];
  /** OS watchdog registration; absent means on whenever the policy is enabled. */
  watchdog?: boolean;
}

export interface LoginItemApp {
  setLoginItemSettings(settings: {
    openAtLogin: boolean;
    openAsHidden?: boolean;
    args?: string[];
    path?: string;
  }): void;
}

export function readAutostartPolicy(home: string): AutostartPolicy {
  if (!home) return { present: false, enabled: false };
  try {
    const parsed = JSON.parse(readFileSync(join(home, AUTOSTART_FILE), "utf8")) as Partial<AutostartPolicy>;
    return {
      present: true,
      enabled: parsed?.enabled === true,
      ...(Array.isArray(parsed?.args) ? { args: parsed.args } : {}),
      ...(typeof parsed?.watchdog === "boolean" ? { watchdog: parsed.watchdog } : {}),
    };
  } catch {
    return { present: false, enabled: false };
  }
}

/**
 * Applies the policy and reports whether the login item is now registered. The
 * version directory grows with every build, so an enabled policy rewrites the
 * entry on each launch instead of trusting whatever is registered: a stale one
 * would keep starting a version that may no longer exist.
 */
export function applyAutostart(app: LoginItemApp, home: string, execPath = process.execPath): boolean {
  const policy = readAutostartPolicy(home);
  if (!policy.present) return false;
  if (!policy.enabled) {
    app.setLoginItemSettings({ openAtLogin: false });
    return false;
  }
  const target = loginItemTarget(execPath);
  app.setLoginItemSettings({
    openAtLogin: true,
    openAsHidden: true,
    args: policy.args ?? ["--unattended-boot"],
    ...(target ? { path: target } : {}),
  });
  return true;
}

/**
 * The executable a login item must start. The Electron app in
 * app/ cannot find its service on its own, so the active version's desktop
 * binary comes first: it bootstraps that shell and hands itself over as its
 * --host-rpc service. A superseded version hands off to the active one.
 */
export function loginItemTarget(execPath: string): string | null {
  const root = findInstallRoot(execPath);
  if (!root) return null;
  const active = activeDesktopPath(root);
  if (active) return active;
  for (const name of STABLE_LAUNCHER_NAMES) {
    const candidate = join(root, name);
    try {
      if (lstatSync(candidate).isFile()) return candidate;
    } catch {
      // Try the next name.
    }
  }
  return null;
}

/** Resolves versions/<active>/<desktop> from the install's current.json. */
function activeDesktopPath(root: string): string | null {
  try {
    const pointer = JSON.parse(readFileSync(join(root, "current.json"), "utf8")) as { activeDir?: unknown };
    if (typeof pointer?.activeDir !== "string" || pointer.activeDir.trim() === "") return null;
    const name = process.platform === "win32" ? "reasonix-desktop.exe" : "reasonix-desktop";
    const candidate = join(root, ...pointer.activeDir.trim().split("/"), name);
    return lstatSync(candidate).isFile() ? candidate : null;
  } catch {
    return null;
  }
}
