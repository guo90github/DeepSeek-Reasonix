package main

import (
	"fmt"
	"strings"

	"reasonix/internal/control"
	"reasonix/internal/event"
)

// defaultAuditThreshold is the quality score below which an audit result is
// considered low. It serves only as the config fallback in GetAuditThreshold;
// results are one-shot and never aggregated across tabs or sessions.
const defaultAuditThreshold = 0.6

// auditRequestPayload is the first event of an audit run: the exact request
// parameters sent to the audit model (system prompt + the audited reasoning).
type auditRequestPayload struct {
	SystemPrompt string `json:"systemPrompt"`
	Input        string `json:"input"`
	Truncated    bool   `json:"truncated"` // input was cut to reasoningAuditMaxChars
}

// auditChunkEvent is one streamed model delta. Kind is "reasoning" (thinking,
// only when the audit model runs with effort enabled) or "text" (the verdict
// being assembled — always streamed live).
type auditChunkEvent struct {
	Kind  string `json:"kind"`
	Chunk string `json:"chunk"`
}

// AuditTurn manually audits the given reasoning chain with the configured audit
// model. It is user-triggered only (no background auto-run) and streams the
// whole run to the webview over events so the frontend is not a black box:
//   - audit:request (tabID, {systemPrompt, input}) before the model call
//   - audit:chunk   (tabID, {kind, chunk}) live as the model produces output
//   - audit:done    (tabID, totals) once the verdict is final
//
// The resolved promise only means "no error"; audit:done is the completion
// signal. Results are one-shot and never persisted.
func (a *App) AuditTurn(reasoning string) (event.ReasoningAuditTotals, error) {
	reasoning = strings.TrimSpace(reasoning)
	if reasoning == "" {
		return event.ReasoningAuditTotals{}, fmt.Errorf("该回复没有可审计的思考过程")
	}
	tab, ctrlAPI := a.activeTabAndCtrl()
	ctrl, _ := ctrlAPI.(*control.Controller)
	if ctrl == nil {
		return event.ReasoningAuditTotals{}, fmt.Errorf("no active session")
	}
	tabID := ""
	if tab != nil {
		tabID = tab.ID
	}
	// Emit with a.ctx (not the request ctx): the async emitter flushes queued
	// events after this call returns, and a canceled request ctx would drop them.
	reasoningFlush := newChunkFlusher(func(chunk string) {
		a.runtimeEvents.Emit(a.ctx, "audit:chunk", tabID, auditChunkEvent{Kind: "reasoning", Chunk: chunk})
	})
	textFlush := newChunkFlusher(func(chunk string) {
		a.runtimeEvents.Emit(a.ctx, "audit:chunk", tabID, auditChunkEvent{Kind: "text", Chunk: chunk})
	})
	totals, err := ctrl.AuditStream(
		a.bootContext(),
		reasoning,
		func(systemPrompt, input string, truncated bool) {
			a.runtimeEvents.Emit(a.ctx, "audit:request", tabID, auditRequestPayload{SystemPrompt: systemPrompt, Input: input, Truncated: truncated})
		},
		reasoningFlush.push,
		textFlush.push,
	)
	if err != nil {
		return event.ReasoningAuditTotals{}, err
	}
	reasoningFlush.flushNow()
	textFlush.flushNow()
	a.runtimeEvents.Emit(a.ctx, "audit:done", tabID, totals)
	return totals, nil
}
