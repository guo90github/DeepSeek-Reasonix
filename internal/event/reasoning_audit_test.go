package event

import "testing"

// auditRecorder is a sink that opts into the reasoning-quality axis, so a test can
// read exactly what the channel was handed.
type auditRecorder struct {
	FuncSink
	records []ReasoningAuditTotals
}

func (r *auditRecorder) RecordReasoningAudit(t ReasoningAuditTotals) {
	r.records = append(r.records, t)
}

func auditTotalsWithReaderBasis() ReasoningAuditTotals {
	return ReasoningAuditTotals{
		Audited: true, ElapsedMs: 900, Contradiction: 1, Redundancy: 2, Issues: 3,
		Score: 0.4, EvalTokens: 1200, EvalCost: 0.0031,
		Explanation: "the chain contradicts itself about the deadline",
		Findings: []AuditFinding{
			{Type: "contradiction", Quote: "the deadline is Friday"}, // an excerpt of the audited chain
		},
	}
}

// The audit channel is content-free like every other one: the explanation and the
// per-issue excerpts are written for the person who asked for the audit, so a record
// that outlives the run — trajectory, stats, turn events — must not receive them,
// while every number the reader sees has to survive the trip.
func TestTheAuditChannelReceivesCountsWithoutTheAuditedChain(t *testing.T) {
	totals := auditTotalsWithReaderBasis()
	recorder := &auditRecorder{FuncSink: func(Event) {}}
	RecordReasoningAudit(recorder, totals)

	if len(recorder.records) != 1 {
		t.Fatalf("records = %d, want exactly one delivery to the opt-in sink", len(recorder.records))
	}
	got := recorder.records[0]
	if got.Explanation != "" || got.Findings != nil {
		t.Fatalf("the channel received the reader's basis: explanation=%q findings=%+v", got.Explanation, got.Findings)
	}
	// A slice lives in the struct, so the numbers are compared field by field.
	if got.Audited != true || got.ElapsedMs != 900 || got.Contradiction != 1 || got.Redundancy != 2 ||
		got.Issues != 3 || got.Score != 0.4 || got.EvalTokens != 1200 || got.EvalCost != 0.0031 {
		t.Fatalf("the numbers a reader relies on did not survive: %+v", got)
	}
}

// The other half of the same rule: stripping the channel's copy must not take the
// basis away from the reader — the value App.AuditTurn returns, and the modal
// renders, still carries it.
func TestTheReaderKeepsTheBasisForTheVerdict(t *testing.T) {
	totals := auditTotalsWithReaderBasis()
	RecordReasoningAudit(&auditRecorder{FuncSink: func(Event) {}}, totals)

	if totals.Explanation == "" || len(totals.Findings) != 1 || totals.Findings[0].Quote == "" {
		t.Fatalf("the caller's copy lost the basis: explanation=%q findings=%+v", totals.Explanation, totals.Findings)
	}
	if counts := totals.CountsOnly(); counts.Explanation != "" || counts.Findings != nil {
		t.Fatalf("CountsOnly kept the basis: %+v", counts)
	}
}

// A sink that does not opt in hears nothing at all, so the axis stays invisible to
// the ordinary UI sinks it was designed to bypass.
func TestASinkThatDoesNotOptInHearsNothing(t *testing.T) {
	emitted := 0
	RecordReasoningAudit(FuncSink(func(Event) { emitted++ }), auditTotalsWithReaderBasis())
	if emitted != 0 {
		t.Fatalf("emitted = %d, want the summary to reach no ordinary sink", emitted)
	}
}
