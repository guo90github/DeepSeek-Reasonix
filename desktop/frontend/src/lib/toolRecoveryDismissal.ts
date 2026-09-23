import type { RecoveryCall } from "./toolRecovery";

const MAX_SESSIONS = 12;
const dismissed = new Map<string, string>();

// Identity is the calls the notice names, not the server revision: revision
// hashes statistics and the runtime epoch too, so unrelated churn used to give
// the same unresolved call a new identity and reopen the closed notice.
export function toolRecoveryNoticeKey(sessionPath: string, calls: readonly RecoveryCall[]): string {
  const attempts = calls.map(call => call.identity.attempt_id ?? "").filter(Boolean).sort();
  return `${sessionPath}\u0000${attempts.join(",")}`;
}

export function readToolRecoveryDismissal(sessionKey: string): string {
  const existing = dismissed.get(sessionKey);
  if (existing === undefined) return "";
  dismissed.delete(sessionKey);
  dismissed.set(sessionKey, existing);
  return existing;
}

export function writeToolRecoveryDismissal(sessionKey: string, noticeKey: string): void {
  dismissed.delete(sessionKey);
  dismissed.set(sessionKey, noticeKey);
  while (dismissed.size > MAX_SESSIONS) dismissed.delete(dismissed.keys().next().value!);
}

export function clearToolRecoveryDismissalForTest(sessionKey: string): void {
  dismissed.delete(sessionKey);
}
