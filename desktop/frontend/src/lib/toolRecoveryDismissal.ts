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

// Keyed by the session the notice belongs to, never by the frontend session
// key: that one carries a load generation (sessionIdentityKey) and, for remote
// surfaces, a tab id, so A -> B -> A arrives under a different key and a store
// keyed by it forgets the dismissal.
export function readToolRecoveryDismissal(sessionPath: string): string {
  return dismissed.get(sessionPath) ?? "";
}

export function writeToolRecoveryDismissal(sessionPath: string, noticeKey: string): void {
  dismissed.delete(sessionPath);
  dismissed.set(sessionPath, noticeKey);
  while (dismissed.size > MAX_SESSIONS) dismissed.delete(dismissed.keys().next().value!);
}

export function clearToolRecoveryDismissalForTest(sessionPath: string): void {
  dismissed.delete(sessionPath);
}
