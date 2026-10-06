package control

import (
	"strings"
	"sync"

	"reasonix/internal/agent"
	"reasonix/internal/evidence"
)

// Turn-outcome record (docs/70 §2.1): what a finished turn delivered and still
// owed. The sink captures the readiness verdict in memory only (it runs on the
// agent's goroutine and must not do I/O); the record is written at turn end.

// readinessState holds the latest readiness verdict. One audit field, so "none"
// is Result == "" rather than another flag.
type readinessState struct {
	mu    sync.Mutex
	audit evidence.ReadinessAudit
}

func (r *readinessState) record(a evidence.ReadinessAudit) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.audit = a
	r.mu.Unlock()
}

func (r *readinessState) last() (evidence.ReadinessAudit, bool) {
	if r == nil {
		return evidence.ReadinessAudit{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.audit, r.audit.Result != ""
}

// clear drops the previous turn's verdict so a turn that produces no audit is not
// recorded with its predecessor's.
func (r *readinessState) clear() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.audit = evidence.ReadinessAudit{}
	r.mu.Unlock()
}

// turnVerdict maps the readiness gate's result onto the record's verdict.
func turnVerdict(result evidence.ReadinessAuditResult) string {
	switch result {
	case evidence.ReadinessBlocked:
		return agent.TurnVerdictBlocked
	case evidence.ReadinessErrored:
		return agent.TurnVerdictAborted
	default:
		return agent.TurnVerdictDelivered
	}
}

// missingObligationCount sums the audit's per-class counters: the record carries
// counts, never the obligations' text.
func missingObligationCount(a evidence.ReadinessAudit) int {
	return a.MissingProjectChecks + a.IncompleteTodos + a.CommandMismatchMissing +
		a.MissingAcceptanceCriteria + a.MissingVerification + a.MissingReview +
		a.MissingSignoff + a.MissingActionEvidence + a.MissingMutation + a.MissingCapabilities
}

// recordTurnOutcome writes the finished turn's outcome to the session sidecar.
// Best-effort: no session path, no write.
func (c *Controller) recordTurnOutcome() {
	if c == nil {
		return
	}
	path := strings.TrimSpace(c.SessionPath())
	if path == "" {
		return
	}
	turn := c.Turn()
	if turn < 1 {
		// A turn the counter never saw (synthetic or pre-submit work) has no
		// number to record, and a "turn 0" line in the next turn's progress block
		// would be noise.
		return
	}
	outcome := agent.TurnOutcome{TurnSeq: turn}
	if audit, ok := c.readiness.last(); ok {
		outcome.Verdict = turnVerdict(audit.Result)
		outcome.MissingCount = missingObligationCount(audit)
		if audit.Recovered {
			outcome.Recovered = 1
		}
	}
	if c.executor != nil {
		outcome.MissingIDs = c.executor.LastMissingObligations()
	}
	reply := lastAssistantText(c.History())
	_ = agent.UpdateBranchMeta(path, false, func(meta *agent.BranchMeta) error {
		agent.AppendTurnOutcome(meta, outcome)
		c.markRecallUse(meta, turn, reply)
		return nil
	})
}
