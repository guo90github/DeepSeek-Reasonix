package main

import (
	"fmt"
	"unicode/utf8"

	"reasonix/internal/control"
)

// sessionAuditChunkMax caps one re-batched chunk payload; a very long verdict
// then arrives as several events instead of one large frame.
const sessionAuditChunkMax = 4000

// AuditSession runs the whole-session reasoning audit over the turns the
// frontend currently holds. It is user-triggered, one-shot, and never
// persisted; the audit model is resolved lazily by the controller. The whole
// pipeline is streamed so the run is not a black box:
//   - sessionaudit:event (tabID, control.SessionAuditEvent) before/while each
//     evaluator call runs, with deltas batched per step
//   - sessionaudit:done  (tabID, totals) once the verdict is final
//
// The resolved promise only means "no error"; sessionaudit:done is the
// completion signal.
func (a *App) AuditSession(turns []control.SessionAuditTurn) (control.SessionAuditTotals, error) {
	if len(turns) == 0 {
		return control.SessionAuditTotals{}, fmt.Errorf("没有可审计的轮次")
	}
	tab, ctrlAPI := a.activeTabAndCtrl()
	ctrl, _ := ctrlAPI.(*control.Controller)
	if ctrl == nil {
		return control.SessionAuditTotals{}, fmt.Errorf("no active session")
	}
	tabID := ""
	if tab != nil {
		tabID = tab.ID
	}

	// Deltas are re-batched per step: the emit path must not see one frame per
	// token, and a step change has to flush before its text is attributed to
	// the next step.
	var pending control.SessionAuditEvent
	flush := func() {
		for pending.Text != "" {
			ev := pending
			chunk, rest := splitAuditChunk(pending.Text, sessionAuditChunkMax)
			ev.Text = chunk
			pending.Text = rest
			a.runtimeEvents.Emit(a.ctx, "sessionaudit:event", tabID, ev)
		}
	}
	onEvent := func(ev control.SessionAuditEvent) {
		if ev.Kind == "reasoning" || ev.Kind == "text" {
			if pending.Stage != ev.Stage || pending.Index != ev.Index || pending.Kind != ev.Kind {
				flush()
				pending = control.SessionAuditEvent{Stage: ev.Stage, Index: ev.Index, Total: ev.Total, Kind: ev.Kind}
			}
			pending.Text += ev.Text
			return
		}
		flush()
		a.runtimeEvents.Emit(a.ctx, "sessionaudit:event", tabID, ev)
	}

	totals, err := ctrl.AuditSessionReasoning(a.bootContext(), turns, onEvent)
	flush()
	if err != nil {
		return control.SessionAuditTotals{}, err
	}
	a.runtimeEvents.Emit(a.ctx, "sessionaudit:done", tabID, totals)
	return totals, nil
}

// splitAuditChunk cuts text before max bytes without landing inside a rune, so
// a CJK verdict is never split into an invalid UTF-8 frame.
func splitAuditChunk(text string, max int) (string, string) {
	if len(text) <= max {
		return text, ""
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	if cut == 0 {
		_, size := utf8.DecodeRuneInString(text)
		cut = size
	}
	return text[:cut], text[cut:]
}
