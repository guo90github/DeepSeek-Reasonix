package agentbus

import (
	"testing"

	"reasonix/internal/agentbus/board"
)

// TestCrossSubtreeVisibilityGoesThroughTheBoundaryNode is T4-3: two subtrees meet
// at the node one of them required, and nothing else crosses.
func TestCrossSubtreeVisibilityGoesThroughTheBoundaryNode(t *testing.T) {
	st := testState(t,
		assertOp("a-task", "alice"),
		board.Op{Verb: board.VerbRequire, Node: "a-task", Actor: "alice", Dep: &board.NodeSpec{ID: "b-boundary"}},
		assertOp("a-internal", "alice"),
		assertOp("b-boundary", "bob"),
		assertOp("b-internal", "bob"),
	)

	alice := BuildView(st, ViewSpec{Board: "b1", Participant: "alice"})
	if !hasID(alice, "a-task") || !hasID(alice, "a-internal") {
		t.Fatalf("alice lost her own subtree: %v", nodeIDs(alice))
	}
	if !hasID(alice, "b-boundary") {
		t.Fatalf("alice must see the boundary node she requires: %v", nodeIDs(alice))
	}
	if hasID(alice, "b-internal") {
		t.Fatalf("the other subtree's internals leaked into alice's view: %v", nodeIDs(alice))
	}

	bob := BuildView(st, ViewSpec{Board: "b1", Participant: "bob"})
	if !hasID(bob, "b-boundary") || !hasID(bob, "b-internal") {
		t.Fatalf("bob lost his own subtree: %v", nodeIDs(bob))
	}
	if !hasID(bob, "a-task") {
		t.Fatalf("bob must see the node blocked on his boundary node: %v", nodeIDs(bob))
	}
	if hasID(bob, "a-internal") {
		t.Fatalf("the other subtree's internals leaked into bob's view: %v", nodeIDs(bob))
	}
	if bob.Waiting != 1 {
		t.Fatalf("waiting = %d, want 1 (the node stalled on the boundary)", bob.Waiting)
	}
}
