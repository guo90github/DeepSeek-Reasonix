package agentbus

import "testing"

// The turn allowance belongs to the claimant: keyed by the board alone it was one bucket
// every participant drained, so one busy session refused everybody else's next claim for the
// rest of the process (measured 2026-10-05: two claims filled it to 60/60 and a third claim
// for an unrelated node was refused).
func TestTheTurnCeilingIsPerParticipant(t *testing.T) {
	l := NewLedger(BudgetLimits{Turn: 60})
	if _, err := l.Charge(ChargeRequest{Board: "default", Node: "a", Turn: "alice", Amount: 40}); err != nil {
		t.Fatalf("alice's first claim = %v, want it paid", err)
	}
	// Bob's own allowance is untouched; a shared bucket is exactly what refused this charge.
	if _, err := l.Charge(ChargeRequest{Board: "default", Node: "b", Turn: "bob", Amount: 40}); err != nil {
		t.Fatalf("bob = %v, want his own turn allowance to pay", err)
	}
	if _, err := l.Charge(ChargeRequest{Board: "default", Node: "c", Turn: "alice", Amount: 30}); err == nil {
		t.Fatal("alice's bucket should refuse the charge that passes her own ceiling")
	}
}
