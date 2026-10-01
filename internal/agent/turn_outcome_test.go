package agent

import "testing"

// BA1 (docs/70 §2.1): the turn-outcome record is bounded, one entry per turn, and
// carries verdicts and counts only.
func TestAppendTurnOutcomeCapsAndReplaces(t *testing.T) {
	meta := &BranchMeta{}
	for turn := 1; turn <= TurnOutcomeLimit+3; turn++ {
		AppendTurnOutcome(meta, TurnOutcome{TurnSeq: turn, Verdict: TurnVerdictDelivered})
	}
	if len(meta.TurnOutcome) != TurnOutcomeLimit {
		t.Fatalf("records = %d, want the cap %d", len(meta.TurnOutcome), TurnOutcomeLimit)
	}
	if meta.TurnOutcome[0].TurnSeq != 4 {
		t.Fatalf("oldest kept turn = %d, want the oldest three dropped", meta.TurnOutcome[0].TurnSeq)
	}

	last := meta.TurnOutcome[len(meta.TurnOutcome)-1].TurnSeq
	AppendTurnOutcome(meta, TurnOutcome{TurnSeq: last, Verdict: TurnVerdictBlocked, MissingCount: 2, MissingIDs: []string{"run go test ./... after the latest write"}})
	count := 0
	for _, outcome := range meta.TurnOutcome {
		if outcome.TurnSeq == last {
			count++
		}
	}
	if count != 1 || len(meta.TurnOutcome) != TurnOutcomeLimit {
		t.Fatalf("turn %d appears %d times over %d records", last, count, len(meta.TurnOutcome))
	}
	latest, ok := LatestTurnOutcome(*meta)
	if !ok || latest.Verdict != TurnVerdictBlocked || latest.MissingCount != 2 || len(latest.MissingIDs) != 1 {
		t.Fatalf("latest = %+v, want the replacement to win", latest)
	}

	// An empty verdict is the delivered default; an empty meta has no latest.
	AppendTurnOutcome(meta, TurnOutcome{TurnSeq: last + 1})
	if latest, _ := LatestTurnOutcome(*meta); latest.Verdict != TurnVerdictDelivered {
		t.Fatalf("an unset verdict = %q, want delivered", latest.Verdict)
	}
	if _, ok := LatestTurnOutcome(BranchMeta{}); ok {
		t.Fatal("an empty meta has no latest outcome")
	}
	if nil == meta.TurnOutcome {
		t.Fatal("the record must stay allocated")
	}
}
