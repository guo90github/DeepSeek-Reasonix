// Restart pacing for the desktop service. An attended app keeps the small
// budget in restartBudget.ts; an unattended one retries forever, but never in a
// hot loop — the delay doubles up to a minute so a broken build cannot spin.

export const UNATTENDED_RESTART_MAX_DELAY_MS = 60_000;
const UNATTENDED_RESTART_BASE_DELAY_MS = 1_000;

/** Exponential backoff for unattended restarts: 1s, 2s, 4s … capped at 60s. */
export function unattendedRestartDelayMs(attempt: number, maxMs = UNATTENDED_RESTART_MAX_DELAY_MS): number {
  const step = Number.isFinite(attempt) ? Math.max(0, Math.min(Math.floor(attempt), 16)) : 0;
  return Math.min(maxMs, UNATTENDED_RESTART_BASE_DELAY_MS * 2 ** step);
}
