// The sub-agent progress vocabulary: the reserved ToolProgress channel names the Go tracker
// emits, the phases they carry, and the small pure helpers the reducer uses to read them.
// `ToolStatus` lives here too because `terminalStatusOf` answers in it.

export type ToolStatus = "running" | "done" | "error" | "stopped";

// Reserved ToolProgress channel names for sub-agent progress previews (the Go
// tracker emits these; ordinary tool progress must never use them).
export const SUBAGENT_PROGRESS_STATUS = "reasonix.subagent.status";
export const SUBAGENT_PROGRESS_REASONING = "reasonix.subagent.reasoning";
export const SUBAGENT_PROGRESS_TEXT = "reasonix.subagent.text";
export const SUBAGENT_PROGRESS_NOTICE = "reasonix.subagent.notice";
// Reserved names are matched by prefix so a future channel never falls back
// to ordinary tool output on older frontends.
const SUBAGENT_PROGRESS_PREFIX = "reasonix.subagent.";
export const SUBAGENT_PROGRESS_PHASES = new Set(["queued", "running", "reasoning", "responding", "tool", "retrying", "completed", "partial", "failed", "cancelled"]);
// Tool names that initialize a sub-agent progress card. parallel_tasks/fleet
// are group cards: they settle when their whole child progress tree is
// terminal, since they never receive a terminal status of their own.
export const SUBAGENT_PROGRESS_TOOLS = new Set(["task", "read_only_task", "parallel_tasks", "fleet"]);
// Per-channel preview retention. The backend already bounds what it sends
// (8 KiB pending per child); these caps keep one hot card from dominating the
// live conversation memory.
export const SUBAGENT_PREVIEW_REASONING_LIMIT = 8 << 10;
export const SUBAGENT_PREVIEW_TEXT_LIMIT = 8 << 10;
export const SUBAGENT_PREVIEW_NOTICE_LIMIT = 2 << 10;

export type SubagentPhase = "queued" | "running" | "reasoning" | "responding" | "tool" | "retrying" | "completed" | "partial" | "failed" | "cancelled";

// In-memory-only sub-agent progress preview. Never persisted: history
// hydration rebuilds tool items from the transcript without these fields, and
// the full sub-agent transcript stays the source of truth after a restart.
export type SubagentProgress = {
  phase: SubagentPhase;
  reasoning: string;
  text: string;
  notice: string;
  lastActivityAt: number;
  truncated: boolean;
  durationMs?: number;
  startedAt: number;
};

export function isSubagentProgressName(name: string | undefined): boolean {
  return !!name && name.startsWith(SUBAGENT_PROGRESS_PREFIX);
}

export function isTerminalSubagentPhase(phase: string | undefined): boolean {
  return phase === "completed" || phase === "partial" || phase === "failed" || phase === "cancelled";
}

export function isGroupSubagentTool(name: string): boolean {
  return name === "parallel_tasks" || name === "fleet";
}

export function terminalStatusOf(phase: string): ToolStatus {
  switch (phase) {
    case "completed": return "done";
    case "partial": return "error";
    case "failed": return "error";
    case "cancelled": return "stopped";
  }
  return "running";
}

export function freshSubagentProgress(): SubagentProgress {
  const now = Date.now();
  return { phase: "running", reasoning: "", text: "", notice: "", lastActivityAt: now, truncated: false, startedAt: now };
}

/** Keeps the most recent `limit` code points; surrogate pairs stay intact. */
export function tailPreview(text: string, limit: number): string {
  if (text.length <= limit) return text;
  const pts = Array.from(text);
  return pts.slice(pts.length - limit).join("");
}
