// Rollback for a version switch that never came up. The Go host writes
// desktop-upgrade.json before it restarts into another version; a shell that
// cannot reach its service within the grace period puts current.json back on the
// previous version and hands it back so it can be launched in its place.

import { readFileSync, renameSync, unlinkSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";

export const UPGRADE_STATE_FILE = "desktop-upgrade.json";
export const UPGRADE_ROLLBACK_GRACE_MS = 10 * 60 * 1000;

export interface UpgradeState {
  schemaVersion: number;
  phase: string;
  from?: string;
  to: string;
  activatedAt?: string;
}

export function readUpgradeState(home: string): UpgradeState | null {
  if (!home) return null;
  try {
    const parsed = JSON.parse(readFileSync(join(home, UPGRADE_STATE_FILE), "utf8")) as Partial<UpgradeState>;
    if (typeof parsed?.to !== "string" || typeof parsed?.phase !== "string") return null;
    const schemaVersion = Number(parsed.schemaVersion ?? 0);
    if (schemaVersion > 1) return null;
    return {
      schemaVersion,
      phase: parsed.phase,
      to: parsed.to,
      ...(typeof parsed.from === "string" ? { from: parsed.from } : {}),
      ...(typeof parsed.activatedAt === "string" ? { activatedAt: parsed.activatedAt } : {}),
    };
  } catch {
    return null;
  }
}

/** Walks upward from an executable to the install root that owns current.json. */
export function findInstallRoot(fromPath: string): string | null {
  if (!fromPath) return null;
  let dir = dirname(fromPath);
  for (;;) {
    try {
      readFileSync(join(dir, "current.json"), "utf8");
      return dir;
    } catch {
      // Keep climbing.
    }
    const parent = dirname(dir);
    if (parent === dir) return null;
    dir = parent;
  }
}

function writeCurrentPointer(root: string, version: string): void {
  const body = JSON.stringify({ schemaVersion: 1, activeVersion: version, activeDir: `versions/${version}` }, null, 2);
  const target = join(root, "current.json");
  const temp = join(root, `.current.json.rollback-${process.pid}`);
  writeFileSync(temp, body, "utf8");
  try {
    renameSync(temp, target);
  } catch (error) {
    try {
      unlinkSync(temp);
    } catch {
      // The temp file is best-effort cleanup.
    }
    throw error;
  }
}

/**
 * Rolls a stuck switch back and returns the desktop binary to relaunch into, or
 * null when there is nothing to roll back — no switch, another version, or a
 * switch young enough to still be starting.
 */
export function rollbackUpgrade(home: string, fromExecPath: string, graceMs = UPGRADE_ROLLBACK_GRACE_MS): string | null {
  const state = readUpgradeState(home);
  if (!state || state.phase !== "pending" || !state.from) return null;
  const activatedAt = Date.parse(state.activatedAt ?? "");
  if (Number.isFinite(activatedAt) && Date.now() - activatedAt < graceMs) return null;
  const root = findInstallRoot(fromExecPath);
  if (!root) return null;
  writeCurrentPointer(root, state.from);
  return join(root, "versions", state.from, "reasonix-desktop.exe");
}
