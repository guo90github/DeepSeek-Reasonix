package agentbus

import (
	"context"
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
)

func capApply(t *testing.T, brd *board.Board, ops ...board.Op) {
	t.Helper()
	if _, err := brd.ApplyAll(context.Background(), ops...); err != nil {
		t.Fatalf("apply %d ops: %v", len(ops), err)
	}
}

func capOps(t *testing.T, brd *board.Board) []board.Op {
	t.Helper()
	ops, err := brd.Ops(context.Background())
	if err != nil {
		t.Fatalf("ops: %v", err)
	}
	return ops
}

// capGapReport is T9-7's opening move: a worker takes the step, finds it cannot do it, and
// reports what is missing instead of failing silently.
func capGapReport(t *testing.T, dir string) *board.Board {
	t.Helper()
	brd, err := board.Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	capApply(t, brd,
		assertOp("publish", "planner"),
		claimOp("publish", "worker"),
		board.Op{
			Verb: board.VerbCapabilityGap, Node: "publish", Actor: "worker",
			Reason: "no signing key on this host",
		},
	)
	return brd
}

// capObtainStep derives §13.6's 「获取能力」 node as an ordinary dependency and finishes it.
func capObtainStep(t *testing.T, brd *board.Board) {
	t.Helper()
	capApply(t, brd, board.Op{
		Verb: board.VerbRequire, Node: "publish", Actor: "facilitator",
		Dep: &board.NodeSpec{ID: "obtain-signing-key", Title: "obtain the signing key publish needs"},
	})
	capApply(t, brd,
		assertOp("obtain-signing-key", "keeper"),
		claimOp("obtain-signing-key", "keeper"),
		doneOp("obtain-signing-key", "keeper", "verifier"),
	)
}

// T9-7: the capability chain exists end to end. A worker finds it cannot do a step and
// says so; somebody derives the step that obtains what is missing; that step is finished
// like any other; and only then, with a fresh authorization from somebody else, does the
// original step run to its end. §13.6 puts the gate after the fact — evidence plus
// hearing — so what this holds the code to is that the authorization is real and
// checkable rather than that it is minimal.
func TestACapabilityGapIsClosedByAnObtainedStep(t *testing.T) {
	dir := t.TempDir()
	brd := capGapReport(t, dir)

	// The lease went back with the gap, and the reason stayed on the node.
	gapped := e2eSnapshot(t, dir)
	if got := gapped.Nodes["publish"].State; got != board.StateCapabilityGap {
		t.Fatalf("publish = %q, want capability_gap", got)
	}
	if owner := gapped.Nodes["publish"].Owner; owner != "" {
		t.Fatalf("a reported gap must release the lease, owner = %q", owner)
	}
	if gapped.Nodes["publish"].Ready(gapped) {
		t.Fatal("a step nobody can do yet must not read as ready")
	}

	capObtainStep(t, brd)
	obtained := e2eSnapshot(t, dir)
	if got := obtained.Nodes["obtain-signing-key"].State; got != board.StateDone {
		t.Fatalf("obtain-signing-key = %q, want done", got)
	}
	// Having what it lacked is not the same as being allowed to use it.
	if Authorized(capOps(t, brd), obtained, "publish") {
		t.Fatal("obtaining the dependency is not an authorization")
	}

	// A different participant issues the authorization, with evidence to check.
	capApply(t, brd, grantOp("publish", "operator", "the key is installed and scoped", e2eEvidence()))
	authorized, err := brd.Snapshot(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if !Authorized(capOps(t, brd), authorized, "publish") {
		t.Fatal("the operator's authorization should let the step proceed")
	}

	// Only now does the original step run to its end.
	capApply(t, brd,
		claimOp("publish", "worker"),
		doneOp("publish", "worker", "verifier"),
	)
	finished := e2eSnapshot(t, dir)
	if got := finished.Nodes["publish"].State; got != board.StateDone {
		t.Fatalf("publish = %q, want done", got)
	}

	// Who approved it stays readable, and replaying the log reaches the same ending.
	grants := AuthorizedGrants(capOps(t, brd), finished, "publish")
	if len(grants) != 1 || grants[0].Actor != "operator" {
		t.Fatalf("authorized grants = %+v, want the operator's", grants)
	}
	if again := e2eSnapshot(t, dir); again.Nodes["publish"].State != finished.Nodes["publish"].State {
		t.Fatalf("replaying the log changed the ending: %q then %q",
			finished.Nodes["publish"].State, again.Nodes["publish"].State)
	}
}

// The gap clears the lease, so "nobody authorizes themselves" cannot be decided by the
// folded owner: a worker would sign off on its own ability to proceed exactly when the
// step needs its first real authorization. Who the producers are comes from the log.
func TestACapabilityGapDoesNotLetTheWorkerAuthorizeItself(t *testing.T) {
	dir := t.TempDir()
	brd := capGapReport(t, dir)
	capObtainStep(t, brd)
	capApply(t, brd, grantOp("publish", "worker", "I installed the key myself", e2eEvidence()))

	state := e2eSnapshot(t, dir)
	if got := state.Nodes["publish"].State; got != board.StateBlocked {
		t.Fatalf("setup: publish = %q, want it still waiting for the dependency", got)
	}
	ops := capOps(t, brd)
	if Authorized(ops, state, "publish") {
		t.Fatal("a worker must not authorize itself, and a reported gap must not erase who the worker is")
	}
	if grants := AuthorizedGrants(ops, state, "publish"); len(grants) != 0 {
		t.Fatalf("authorized grants = %+v, want none", grants)
	}
}
