package board

import "testing"

// A by-product delivers nothing re-checkable, so a board that demands evidence on every exit
// leaves it open for ever: reachable by everybody, closable by nobody (F86, 2026-10-05).
func TestWaiveClosesAByproductWithoutEvidence(t *testing.T) {
	st := &State{Nodes: map[string]*Node{
		"probe": {ID: "probe", Title: "a probe", State: StateOpen},
	}}
	if err := applyOp(st, Op{Verb: VerbWaive, Node: "probe", Actor: "alice", ID: "op-1", Seq: 4, Reason: "nothing to deliver"}); err != nil {
		t.Fatalf("waive: %v", err)
	}
	probe := st.Nodes["probe"]
	if probe.State != StateAbandoned || probe.Outcome != OutcomeByproduct {
		t.Fatalf("probe = %s/%s, want abandoned/byproduct", probe.State, probe.Outcome)
	}
	if !DepSettled(probe) {
		t.Fatal("a waived node does not settle its dependency: it still pins whatever required it")
	}

	// An assertion with no ref is not a reading, so it does not block a waive: the empty-shell
	// probe F45 found is exactly the shape a by-product has.
	st.Nodes["shell"] = &Node{ID: "shell", State: StateOpen, Asserts: []Assertion{{Actor: "carol"}}}
	if err := applyOp(st, Op{Verb: VerbWaive, Node: "shell", Actor: "alice", ID: "op-2", Seq: 5, Reason: "an empty probe"}); err != nil {
		t.Fatalf("waive over an unevidenced assert: %v", err)
	}
}

// The reason is the whole record of a waive — no evidence rides with it — so it cannot be empty.
func TestWaiveNeedsAReason(t *testing.T) {
	st := &State{Nodes: map[string]*Node{"probe": {ID: "probe", State: StateOpen}}}
	err := applyOp(st, Op{Verb: VerbWaive, Node: "probe", Actor: "alice", ID: "op-1", Seq: 4})
	reason, ok := IsReject(err)
	if !ok || reason != ReasonMissingReason {
		t.Fatalf("waive without a reason = (%v, %t), want %s", err, ok, ReasonMissingReason)
	}
	if got := st.Nodes["probe"].State; got != StateOpen {
		t.Fatalf("a refused waive moved the node to %q", got)
	}
}

// A node that already delivered something re-checkable is not a by-product: closing it that way
// would throw a real reading away, so the refusal names the artifact instead (F86, 2026-10-05).
func TestWaiveRefusesANodeThatAlreadyCarriesReadings(t *testing.T) {
	st := &State{Nodes: map[string]*Node{
		"cited": {ID: "cited", State: StateClaimed, Owner: "carol",
			Asserts: []Assertion{{Actor: "carol", Evidence: []Evidence{{Ref: "log:attempts"}}}}},
	}}
	err := applyOp(st, Op{Verb: VerbWaive, Node: "cited", Actor: "alice", ID: "op-1", Seq: 4, Reason: "not mine"})
	reason, ok := IsReject(err)
	if !ok || reason != ReasonArtifactPresent {
		t.Fatalf("waive over a reading = (%v, %t), want %s", err, ok, ReasonArtifactPresent)
	}
	// And a node that is already closed cannot be waived twice.
	st.Nodes["cited"].Asserts = nil
	st.Nodes["cited"].State = StateDone
	err = applyOp(st, Op{Verb: VerbWaive, Node: "cited", Actor: "alice", ID: "op-2", Seq: 5, Reason: "late"})
	if reason, ok := IsReject(err); !ok || reason != ReasonIllegalTransition {
		t.Fatalf("waive on a done node = (%v, %t), want %s", err, ok, ReasonIllegalTransition)
	}
}
