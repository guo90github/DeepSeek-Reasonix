package board

import (
	"testing"
	"time"
)

// F6: the board calls two different things "blocked", and only one of them has a way back without
// a third party. A dependency block lifts when its dependencies land; a verdict does not. The
// machine read "claim d4-verify refused" as "no self-service re-claim" while the dependency was
// simply not there yet (2026-10-05).
func TestADependencyBlockIsLiftedByItsDependenciesNotByAVerdict(t *testing.T) {
	b := openTestBoard(t)
	mustApply(t, b, opAssert("root", "alice"))
	mustApply(t, b, Op{Verb: VerbSplit, Node: "root", Actor: "alice",
		Children: []NodeSpec{{ID: "a"}, {ID: "b"}}})
	st := snapshot(t, b)
	wantState(t, st, "root", StateBlocked)
	if st.Nodes["root"].Outcome == OutcomeBlocked {
		t.Fatal("a split container carries no verdict: its children block it, nobody decided against it")
	}
	// Blocked, so the claim is refused — exactly the reading the report recorded as a dead end.
	if _, isReject := rejectReason(t, b, opClaim("root", "bob", time.Hour)); !isReject {
		t.Fatal("a container must not be claimable while its children are open")
	}
	for _, child := range []string{"a", "b"} {
		mustApply(t, b, opAssert(child, "bob"))
		mustApply(t, b, opClaim(child, "bob", time.Hour))
		mustApply(t, b, opDecideDone(child, "bob", "alice"))
	}
	// Children done, so the container is ready again and anybody may take it: this is the
	// self-service path the report said was missing, and it was there all along.
	if _, isReject := rejectReason(t, b, opClaim("root", "bob", time.Hour)); isReject {
		t.Fatal("a container whose children are all done must be claimable again")
	}
	// The other kind of block carries a verdict, and a verdict never lifts by itself: no
	// dependency ending makes a node somebody decided against ready to start.
	decided := &Node{ID: "d4-verify", State: StateBlocked, Outcome: OutcomeBlocked}
	if decided.Ready(st) {
		t.Fatal("a node decided against is not ready, however its dependencies end up")
	}
}
