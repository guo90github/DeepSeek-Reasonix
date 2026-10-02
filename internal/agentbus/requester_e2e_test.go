package agentbus

import (
	"context"
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
)

// T5-6: "who asked for this node" is answerable from the folded state, not only from the op
// trail. The trail sees `require` alone; the fold also knows the assertion or split that
// created the node, so its answer is a superset — and every wake the trail derives has to be
// among it, which is exactly what makes reading the fold instead of the ops safe later.
func TestTheFoldAnswersWhoAskedForANode(t *testing.T) {
	ctx := context.Background()
	_, brd := detailFixture(t,
		assertOp("publish", "planner"),
		requireOp("publish", "signing-key"),
	)
	state, err := brd.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	wakes := WakeTargets(WakeInput{State: state, Now: time.Now().UTC()})
	if len(wakes) == 0 {
		t.Fatal("the trail says somebody has to be woken")
	}
	for _, wake := range wakes {
		for _, dep := range wake.Ready {
			node, ok := state.Nodes[dep]
			if !ok {
				t.Fatalf("a wake names %q, which the fold does not hold", dep)
			}
			if !containsString(node.Requesters, wake.Participant) {
				t.Fatalf("%s asked for %s in the trail, but the fold says requesters = %v",
					wake.Participant, dep, node.Requesters)
			}
		}
	}

	// And the fold knows one thing the trail's require scan does not: who created the node.
	if !containsString(state.Nodes["publish"].Requesters, "planner") {
		t.Fatalf("requesters of publish = %v, want the participant that asserted it",
			state.Nodes["publish"].Requesters)
	}
	if got := state.Nodes["signing-key"].Requesters; len(got) != 1 || got[0] != "alice" {
		t.Fatalf("requesters of signing-key = %v, want the one that required it", got)
	}
}

// The requester list survives the round trip through a real file, in the same order, because
// it is folded from ops rather than stored.
func TestRequestersFoldTheSameWayFromDisk(t *testing.T) {
	ctx := context.Background()
	dir, brd := detailFixture(t,
		board.Op{Verb: board.VerbAssert, Node: "n", Actor: "first", Evidence: e2eEvidence()},
		board.Op{Verb: board.VerbAssert, Node: "n", Actor: "second", Evidence: e2eEvidence()},
	)
	first, err := brd.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	reopened, err := board.Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	again, err := reopened.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot again: %v", err)
	}
	if len(first.Nodes["n"].Requesters) != 2 || len(again.Nodes["n"].Requesters) != 2 {
		t.Fatalf("requesters = %v then %v, want both readers to see two",
			first.Nodes["n"].Requesters, again.Nodes["n"].Requesters)
	}
	for i := range first.Nodes["n"].Requesters {
		if first.Nodes["n"].Requesters[i] != again.Nodes["n"].Requesters[i] {
			t.Fatalf("order drifted: %v then %v", first.Nodes["n"].Requesters, again.Nodes["n"].Requesters)
		}
	}
}
