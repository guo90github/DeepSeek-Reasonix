import type { WireRecoveryApproval } from "./types";

export interface WireWriteAccessApproval {
  directories?: string[];
  display_directories?: string[];
  justification?: string;
  broad_home_access?: boolean;
  ordinary_permission_needed?: boolean;
  persist_allowed?: boolean;
}

export interface WireApproval {
  id: string;
  tool: string;
  subject: string;
  reason?: string;
  fresh?: boolean;
  kind?: "tool" | "plan" | "recovery" | "write_access" | string;
  recovery?: WireRecoveryApproval;
  write_access?: WireWriteAccessApproval;
  turnId?: string;
  runtimeEpoch?: string;
}

// Ask-timeout wire fields. They live beside the approval fields rather than in
// lib/types.ts, whose line budget the repo ratchets; WireAsk and AgentView extend
// them so the settings panel and the ask card share one definition each.
export interface AskTimeoutWireFields {
  // Relative wait until the host auto-answers with the recommended option.
  // Omitted or <= 0: the host never auto-answers, so no countdown is shown.
  autoAnswerInMs?: number;
}

export interface AskTimeoutAgentViewFields {
  askTimeoutMinutes?: number; // minutes; 0 = default (10), negative = wait forever
}
