package agentbus

import (
	"context"
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
)

// The migration table (§11.1) lets `decide(done)` run from any state but abandoned or stale,
// and that is deliberate: done is a verdict, not an assembly check. The dependency gate sits
// on `claim`, and an assembly decided before its parts are done is caught by the landing
// assessment instead. Pinned here so nobody "fixes" a dependency gate into the transition —
// it was hit for real while assembling this board's own plan.
func TestDecideDoneIsAVerdictAndNotADependencyGate(t *testing.T) {
	ctx := context.Background()
	brd, err := board.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	evidence := []board.Evidence{{Kind: "test", Ref: "go test ./internal/agentbus/"}}
	if _, err := brd.ApplyAll(ctx,
		board.Op{Verb: board.VerbAssert, Node: "part", Actor: "alice", Evidence: evidence},
		board.Op{Verb: board.VerbAssert, Node: "assembly", Actor: "alice", Evidence: evidence},
		board.Op{Verb: board.VerbRequire, Node: "assembly", Actor: "alice", Dep: &board.NodeSpec{ID: "part"}},
	); err != nil {
		t.Fatalf("plan: %v", err)
	}

	// The part is wide open, and the assembly can still be decided: that is the spec.
	if _, err := brd.Apply(ctx, board.Op{
		Verb: board.VerbDecide, Node: "assembly", Actor: "alice", Outcome: board.OutcomeDone,
		Evidence: evidence, ReproducedBy: "bob",
	}); err != nil {
		t.Fatalf("deciding an assembly while its part is open: %v", err)
	}
	state, err := brd.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if got := state.Nodes["assembly"].Outcome; got != board.OutcomeDone {
		t.Fatalf("assembly outcome = %q, want the verdict stored", got)
	}

	// The state machine stored the verdict; the landing check is what refuses to call the task
	// landed, which is why an early container verdict is a mistake made visible, not a hole.
	landing := AssessLanding(state, nil)
	if landing.Landed {
		t.Fatalf("an assembly decided before its part is done must not land: %s", landing.Reason)
	}
	if len(landing.Blockers) == 0 || landing.Blockers[0].Node != "part" {
		t.Fatalf("blockers = %+v, want the unfinished part named", landing.Blockers)
	}
}
