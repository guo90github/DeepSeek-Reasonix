import type { Translator } from "./i18n";

export interface RecoveryStatus {
  state?: "recovery_required" | string;
  call_id?: string;
  attempt_id?: string;
  requires_user_decision?: boolean;
  read_only?: boolean;
  phase?: string;
  reason?: string;
  next_attempt_at?: number;
  waited_ms?: number;
  wait_budget_ms?: number;
  waiting?: boolean;
}

export interface RecoveryRetry {
  attempt: number;
  max: number;
  /** Why the retry is happening ("rate_limited", "server_error", …), when the host labels it. */
  reason?: string;
  recovery?: RecoveryStatus;
}

export interface RecoveryEventFields {
  recovery?: RecoveryStatus;
  retryAttempt?: number;
  retryMax?: number;
  retryReason?: string;
}

export function recoveryNextAttemptSeconds(recovery: RecoveryStatus, now: number): number {
  return Math.max(0, Math.ceil(((recovery.next_attempt_at ?? now) - now) / 1000));
}

// Why a retry is happening, in the host's closed set (event.RetryReason). A reason this build
// does not know is left out rather than guessed at: "retrying (3/10)" is honest, a wrong label
// is not.
export function retryReasonLabel(t: Translator, reason?: string): string {
  switch (reason) {
    case "rate_limited": return t("status.retryReasonRateLimited");
    case "server_error": return t("status.retryReasonServer");
    case "timeout": return t("status.retryReasonTimeout");
    case "network": return t("status.retryReasonNetwork");
    default: return "";
  }
}

export function recoveryStatusText(t: Translator, retry: RecoveryRetry, now: number): string {
  if (!retry.recovery?.waiting) {
    const base = t("status.retrying", { attempt: retry.attempt, max: retry.max });
    const reason = retryReasonLabel(t, retry.reason);
    return reason ? `${base} ${reason}` : base;
  }
  const phase = t(retry.recovery.phase === "connect" ? "status.recoveryNetwork" : "status.recoveryProvider");
  return t("status.recoveryWaiting", { seconds: recoveryNextAttemptSeconds(retry.recovery, now), phase });
}
