package agent

// TurnOutcome (docs/70 §2.1): what a finished turn delivered and still owed, so
// the next turn's progress block and the review page read one record. Content-free:
// verdicts, counts and obligation ids — never command text, paths, or conclusions.

// TurnOutcomeLimit caps the recorded turns; the oldest fall off.
const TurnOutcomeLimit = 200

// Turn verdicts. They mirror the final-readiness gate: a turn that finished with
// obligations outstanding is recorded as blocked, not delivered.
const (
	TurnVerdictDelivered = "delivered"
	TurnVerdictBlocked   = "blocked"
	TurnVerdictAborted   = "aborted"
)

// TurnOutcome is one turn's result fingerprint.
type TurnOutcome struct {
	TurnSeq int    `json:"turn_seq"`
	Verdict string `json:"verdict"`
	// MissingCount/Counts summarise the readiness gate's verdict; MissingIDs name
	// the obligations it refused to close (ids only, never their text).
	MissingCount int      `json:"missing_count,omitempty"`
	MissingIDs   []string `json:"missing_ids,omitempty"`
	// Recovered counts protocol recoveries in the turn; ChangedFiles counts the
	// files this turn wrote (a count, not a list).
	Recovered    int `json:"recovered,omitempty"`
	ChangedFiles int `json:"changed_files,omitempty"`
}

// AppendTurnOutcome records a turn's outcome, replacing an earlier entry for the
// same turn and trimming the oldest.
func AppendTurnOutcome(meta *BranchMeta, outcome TurnOutcome) {
	if meta == nil || outcome.TurnSeq < 0 {
		return
	}
	if outcome.Verdict == "" {
		outcome.Verdict = TurnVerdictDelivered
	}
	kept := meta.TurnOutcome[:0]
	for _, existing := range meta.TurnOutcome {
		if existing.TurnSeq != outcome.TurnSeq {
			kept = append(kept, existing)
		}
	}
	kept = append(kept, outcome)
	if len(kept) > TurnOutcomeLimit {
		kept = kept[len(kept)-TurnOutcomeLimit:]
	}
	meta.TurnOutcome = kept
}

// LatestTurnOutcome returns the newest outcome, or false when none exists.
func LatestTurnOutcome(meta BranchMeta) (TurnOutcome, bool) {
	if len(meta.TurnOutcome) == 0 {
		return TurnOutcome{}, false
	}
	return meta.TurnOutcome[len(meta.TurnOutcome)-1], true
}

// LastMissingObligations returns the unclosed obligations of the last readiness
// verdict this session produced. The controller records them (ids only) on the
// turn outcome; a nil result means the gate named none.
func (a *Agent) LastMissingObligations() []string {
	if a == nil {
		return nil
	}
	return append([]string(nil), a.turn.missingObligations...)
}
