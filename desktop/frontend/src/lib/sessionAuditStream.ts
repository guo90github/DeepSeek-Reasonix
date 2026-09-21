// Whole-session reasoning-audit streaming. The desktop runs a two-pass audit
// (per-segment scoring, then one cross-turn review) and streams every step so
// the modal is not a black box: "sessionaudit:event" carries one step
// (request / model deltas / step outcome), "sessionaudit:done" the final
// verdict. See desktop/session_audit.go for the emits.
import { desktopHost } from "./desktopHost";
import type { SessionAuditTotals } from "../generated/desktopContract.generated";

export interface SessionAuditEvent {
  stage: "segment" | "review";
  /** 1-based step number; the last step is always the cross-turn review. */
  index: number;
  total: number;
  kind: "request" | "reasoning" | "text" | "step_done" | "step_failed";
  systemPrompt?: string;
  input?: string;
  text?: string;
  turnFrom?: number;
  turnTo?: number;
  error?: string;
}

const eventListeners = new Set<(tabId: string, ev: SessionAuditEvent) => void>();
const doneListeners = new Set<(tabId: string, ev: SessionAuditTotals) => void>();

function subscribeTo<T>(channel: string, listeners: Set<(tabId: string, ev: T) => void>, guard: (ev: T) => boolean) {
  return (cb: (tabId: string, ev: T) => void): (() => void) => {
    const host = desktopHost();
    if (host.kind !== "none") {
      return host.events.on(channel, (tabId?: unknown, payload?: unknown) => {
        const ev = (payload ?? {}) as T;
        if (typeof tabId === "string" && guard(ev)) cb(tabId, ev);
      });
    }
    listeners.add(cb);
    return () => {
      listeners.delete(cb);
    };
  };
}

export const onSessionAuditEvent = subscribeTo<SessionAuditEvent>("sessionaudit:event", eventListeners, (ev) => typeof ev.kind === "string");
export const onSessionAuditDone = subscribeTo<SessionAuditTotals>("sessionaudit:done", doneListeners, () => true);

// Test seams for the session-audit stream. The desktop shell receives the same
// payloads through the host event stream.
export function __emitMockSessionAuditEvent(tabId: string, ev: SessionAuditEvent): void {
  eventListeners.forEach((l) => l(tabId, ev));
}
export function __emitMockSessionAuditDone(tabId: string, ev: SessionAuditTotals): void {
  doneListeners.forEach((l) => l(tabId, ev));
}
