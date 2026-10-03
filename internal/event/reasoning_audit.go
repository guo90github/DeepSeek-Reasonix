package event

import "reasonix/internal/nilutil"

// AuditFinding is one flagged issue from the audit verdict: the failure class
// and the quoted excerpt from the audited chain it refers to.
type AuditFinding struct {
	Type  string `json:"type"`  // contradiction | factual_error | invalid_inference | redundancy | instruction_drift | omission
	Quote string `json:"quote"` // quoted excerpt from the audited chain (or a short description for omission)
}

// ReasoningAuditTotals summarizes one reasoning-quality audit for a turn. The
// numbers are the summary; `Explanation` and `Findings` are the basis for the
// verdict, written for the person who asked for the audit and read off the value
// App.AuditTurn returns. They never reach a sink: RecordReasoningAudit strips
// them on the way out, so the audit channel stays content-free like every other
// one (see ContractShadowAuditSink's comment on the family rule).
type ReasoningAuditTotals struct {
	Audited          bool  `json:"audited"`       // analyser produced a verdict (vs. unavailable)
	ElapsedMs        int64 `json:"elapsedMs"`     // evaluator call duration
	Contradiction    int   `json:"contradiction"` // flagged issues per kind
	FactualError     int   `json:"factualError"`
	InvalidInference int   `json:"invalidInference"`
	Redundancy       int   `json:"redundancy"`
	InstructionDrift int   `json:"instructionDrift"`
	Omission         int   `json:"omission"`
	// Hallucination is retained for backward compatibility with earlier
	// four-class evaluator outputs; the current prompt reports factual_error.
	Hallucination int            `json:"hallucination"`
	Issues        int            `json:"issues"`      // total flagged items (all kinds)
	Score         float64        `json:"score"`       // 0..1 aggregate quality
	EvalTokens    int            `json:"evalTokens"`  // evaluator-model tokens
	EvalCost      float64        `json:"evalCost"`    // evaluator-model spend (USD)
	Explanation   string         `json:"explanation"` // why the score is what it is (reader-facing)
	Findings      []AuditFinding `json:"findings"`    // per-issue excerpts from the audited chain (reader-facing)
}

// CountsOnly drops the reader-facing basis for the verdict, leaving the numbers
// the audit channel is allowed to carry. The receiver is a copy: the caller's
// own totals keep their explanation and findings.
func (t ReasoningAuditTotals) CountsOnly() ReasoningAuditTotals {
	t.Explanation = ""
	t.Findings = nil
	return t
}

// ReasoningAuditSink is an optional sink capability for the reasoning-quality
// axis. Content-free only, like every other audit channel — see the comment on
// ProtocolRecoveryAuditSink.
type ReasoningAuditSink interface {
	RecordReasoningAudit(ReasoningAuditTotals)
}

// RecordReasoningAudit forwards a turn's reasoning-quality summary only to
// sinks that opt in. Ordinary UI sinks receive nothing.
//
// This is the choke point where the channel's content-free rule is kept: what a
// sink sees is CountsOnly, so an excerpt of the audited chain cannot end up in a
// record that outlives the run (trajectory, stats, turn events).
func RecordReasoningAudit(s Sink, t ReasoningAuditTotals) {
	if nilutil.IsNil(s) {
		return
	}
	if ra, ok := s.(ReasoningAuditSink); ok {
		ra.RecordReasoningAudit(t.CountsOnly())
	}
}
