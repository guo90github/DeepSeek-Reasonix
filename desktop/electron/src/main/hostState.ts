// The Go host's launch marker (desktop-host-state.json in the Reasonix home).
// It is written on every launch and removed only on a clean exit, so its
// presence while the host is gone means the last run died. The restart policy
// reads the unattended flag it carries: an unattended run must keep itself up,
// an attended one keeps today's small restart budget.

import { readFileSync } from "node:fs";
import { join } from "node:path";

export const HOST_STATE_FILE = "desktop-host-state.json";

export interface HostState {
  schemaVersion: number;
  pid: number;
  phase: string;
  unattended: boolean;
}

export function hostStatePath(home: string): string {
  return join(home, HOST_STATE_FILE);
}

export function readHostState(home: string): HostState | null {
  if (!home) return null;
  try {
    const parsed = JSON.parse(readFileSync(hostStatePath(home), "utf8")) as Partial<HostState>;
    if (typeof parsed?.unattended !== "boolean") return null;
    return {
      schemaVersion: Number(parsed.schemaVersion ?? 0),
      pid: Number(parsed.pid ?? 0),
      phase: String(parsed.phase ?? ""),
      unattended: parsed.unattended,
    };
  } catch {
    return null;
  }
}

/** True while the last recorded launch asked for unattended driving. */
export function unattendedDesired(home: string): boolean {
  return readHostState(home)?.unattended === true;
}
