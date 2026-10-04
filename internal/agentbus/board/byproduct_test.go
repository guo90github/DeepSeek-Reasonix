package board

import (
	"testing"
	"time"
)

// A child that was given up on can never become done, so treating it as "still waiting" pinned
// its container for ever — the by-product terminal state the board was missing. An abandoned
// dependency has to count as settled (F22, 2026-10-05).
func TestAnAbandonedChildSettlesItsParentsDependency(t *testing.T) {
	st := &State{Nodes: map[string]*Node{
		"container": {ID: "container", State: StateBlocked, Deps: []string{"child"}},
		"child":     {ID: "child", State: StateAbandoned, Outcome: OutcomeAbandoned},
	}}

	container := st.Nodes["container"]
	if !container.Ready(st) {
		t.Fatal("a container whose only child was abandoned is not ready: it is pinned for ever")
	}

	// A dependency that is merely late still holds it back: open, claimed and blocked are all
	// "something is still going to happen here".
	for _, state := range []NodeState{StateOpen, StateClaimed, StateBlocked} {
		st.Nodes["child"].State = state
		if container.Ready(st) {
			t.Fatalf("a %s dependency let the container start", state)
		}
	}

	// And it can actually be taken, which is what "not pinned" has to mean.
	st.Nodes["child"].State = StateAbandoned
	if err := applyOp(st, Op{
		Verb: VerbClaim, Node: "container", Actor: "alice", ID: "op-1", Seq: 9,
		Bounds: &Bounds{Steps: 1}, Deadline: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("claiming a container whose child was abandoned: %v", err)
	}
	if got := st.Nodes["container"].State; got != StateClaimed {
		t.Fatalf("container state = %q, want it claimed", got)
	}
}
