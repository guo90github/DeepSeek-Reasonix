// Reasoning-audit streaming. The desktop streams the whole audit run over
// events so the card is not a black box: audit:request carries the exact request
// params before the model call, audit:chunk carries live model deltas (kind =
// "reasoning" | "text"), and audit:done carries the final verdict. See
// desktop/reasoning_audit.go for the emits. Kept in its own module (only the
// lazily-loaded AuditInlineCard imports it) so the code stays out of the initial
// JS bundle.
import { desktopHost } from "./desktopHost";
import type { ReasoningAuditTotals } from "../generated/desktopContract.generated";

export interface AuditRequestPayload {
  systemPrompt: string;
  input: string;
  truncated: boolean;
}
export interface AuditChunkEvent {
  kind: "reasoning" | "text";
  chunk: string;
}

const auditRequestListeners = new Set<(tabId: string, ev: AuditRequestPayload) => void>();
const auditChunkListeners = new Set<(tabId: string, ev: AuditChunkEvent) => void>();
const auditDoneListeners = new Set<(tabId: string, ev: ReasoningAuditTotals) => void>();

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

export const onAuditRequest = subscribeTo<AuditRequestPayload>("audit:request", auditRequestListeners, (ev) => typeof ev.systemPrompt === "string");
export const onAuditChunk = subscribeTo<AuditChunkEvent>("audit:chunk", auditChunkListeners, (ev) => typeof ev.chunk === "string");
export const onAuditDone = subscribeTo<ReasoningAuditTotals>("audit:done", auditDoneListeners, () => true);

// Test seams for the reasoning-audit stream. The desktop shell receives the same
// payloads through the host event stream.
export function __emitMockAuditRequest(tabId: string, ev: AuditRequestPayload): void {
  auditRequestListeners.forEach((l) => l(tabId, ev));
}
export function __emitMockAuditChunk(tabId: string, ev: AuditChunkEvent): void {
  auditChunkListeners.forEach((l) => l(tabId, ev));
}
export function __emitMockAuditDone(tabId: string, ev: ReasoningAuditTotals): void {
  auditDoneListeners.forEach((l) => l(tabId, ev));
}
