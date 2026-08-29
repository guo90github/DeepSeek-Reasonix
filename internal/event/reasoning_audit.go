package event

import "reasonix/internal/nilutil"

// AuditFinding is one flagged issue from the audit verdict: the failure class
// and the quoted excerpt from the audited chain it refers to.
type AuditFinding struct {
	Type  string `json:"type"`  // contradiction | hallucination | redundancy | instruction_drift
	Quote string `json:"quote"` // quoted excerpt from the audited chain
}

// ReasoningAuditTotals is a content-free summary of one reasoning-quality
// audit for a turn. Per the audit-channel rule it carries counts and durations
// only — never the reasoning text itself, which belongs on the UI plane (the
// assistant Message's Reasoning field).
type ReasoningAuditTotals struct {
	Audited          bool           `json:"audited"`       // analyser produced a verdict (vs. unavailable)
	ElapsedMs        int64          `json:"elapsedMs"`     // evaluator call duration
	Contradiction    int            `json:"contradiction"` // flagged issues per kind
	Hallucination    int            `json:"hallucination"`
	Redundancy       int            `json:"redundancy"`
	InstructionDrift int            `json:"instructionDrift"`
	Issues           int            `json:"issues"`       // total flagged items
	Score            float64        `json:"score"`        // 0..1 aggregate quality
	EvalTokens       int            `json:"evalTokens"`   // evaluator-model tokens
	EvalCost         float64        `json:"evalCost"`     // evaluator-model spend (USD)
	Explanation      string         `json:"explanation"`  // human-readable basis for the score (audit evidence)
	Findings         []AuditFinding `json:"findings"`     // per-issue excerpts (audit evidence, nullable)
}

// ReasoningAuditSink is an optional sink capability for the reasoning-quality
// axis. Content-free only, like every other audit channel — see the comment on
// ProtocolRecoveryAuditSink.
type ReasoningAuditSink interface {
	RecordReasoningAudit(ReasoningAuditTotals)
}

// RecordReasoningAudit forwards a turn's reasoning-quality summary only to
// sinks that opt in. Ordinary UI sinks receive nothing.
func RecordReasoningAudit(s Sink, t ReasoningAuditTotals) {
	if nilutil.IsNil(s) {
		return
	}
	if ra, ok := s.(ReasoningAuditSink); ok {
		ra.RecordReasoningAudit(t)
	}
}
