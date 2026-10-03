package boot

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
	"reasonix/internal/control"
)

func agentBusSweepBuild(t *testing.T) (*control.Controller, string) {
	t.Helper()
	ctrl, _ := agentBusToolBuild(t, "boot-effect-agentbus-sweep")
	dir := filepath.Join(t.TempDir(), "agentbus", "default")
	ctrl.SetAgentBus(dir, "me")
	return ctrl, dir
}

func agentBusApply(t *testing.T, ctrl *control.Controller, op board.Op) {
	t.Helper()
	if _, err := ctrl.ApplyAgentBusOp(context.Background(), op); err != nil {
		t.Fatalf("apply %s on %s: %v", op.Verb, op.Node, err)
	}
}

func agentBusState(t *testing.T, dir string) *board.State {
	t.Helper()
	brd, err := board.Open(dir)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	state, err := brd.Snapshot(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	return state
}

func agentBusEvidence() []board.Evidence {
	return []board.Evidence{{Kind: "test", Ref: "go test ./internal/boot/"}}
}

// A lease nobody renewed has to be reclaimed by the write path itself. Nothing
// scans the board, and a claimed node is invisible to the wake surface
// (Node.Ready only admits open/blocked), so without this the work simply stops:
// the holder is gone, the node still reads claimed, and no participant is ever
// told to take it over.
func TestEffectAgentBusWriteReclaimsAnExpiredClaim(t *testing.T) {
	ctrl, dir := agentBusSweepBuild(t)
	evidence := agentBusEvidence()

	agentBusApply(t, ctrl, board.Op{Verb: board.VerbAssert, Node: "work", Actor: "me", Evidence: evidence})
	agentBusApply(t, ctrl, board.Op{Verb: board.VerbClaim, Node: "work", Actor: "me",
		Bounds:   &board.Bounds{Steps: 1},
		Deadline: time.Now().UTC().Add(50 * time.Millisecond)})
	time.Sleep(120 * time.Millisecond)

	// The next write is the only thing that can notice the lapse.
	agentBusApply(t, ctrl, board.Op{Verb: board.VerbAssert, Node: "other", Actor: "me", Evidence: evidence})

	state := agentBusState(t, dir)
	if got := state.Nodes["work"].State; got != board.StateOpen {
		t.Fatalf("work = %q after its lease lapsed and another write landed, want it reclaimed to open", got)
	}
	if got := state.Nodes["work"].NoProgress; got != 1 {
		t.Fatalf("work no_progress = %d, want the reclamation recorded once", got)
	}

	// Reclaiming is idempotent: the record already says it lapsed, so later writes
	// must not pile on.
	agentBusApply(t, ctrl, board.Op{Verb: board.VerbAssert, Node: "third", Actor: "me", Evidence: evidence})
	if got := agentBusState(t, dir).Nodes["work"].NoProgress; got != 1 {
		t.Fatalf("work no_progress = %d after another write, want the reclamation counted once", got)
	}

	// Finished work is never reclaimed, whatever its old lease said: a sweeper that
	// keeps taking back done nodes turns bookkeeping into a storm.
	agentBusApply(t, ctrl, board.Op{Verb: board.VerbClaim, Node: "work", Actor: "me",
		Bounds:   &board.Bounds{Steps: 1},
		Deadline: time.Now().UTC().Add(50 * time.Millisecond)})
	agentBusApply(t, ctrl, board.Op{Verb: board.VerbAssert, Node: "work", Actor: "me", Evidence: evidence})
	agentBusApply(t, ctrl, board.Op{Verb: board.VerbDecide, Node: "work", Actor: "me",
		Outcome: board.OutcomeDone, Evidence: evidence, ReproducedBy: "go test ./internal/boot/"})
	time.Sleep(120 * time.Millisecond)
	agentBusApply(t, ctrl, board.Op{Verb: board.VerbAssert, Node: "fourth", Actor: "me", Evidence: evidence})

	finished := agentBusState(t, dir)
	if got := finished.Nodes["work"].State; got != board.StateDone {
		t.Fatalf("work = %q after a later write, want the finished verdict kept", got)
	}
	if got := finished.Nodes["work"].NoProgress; got != 1 {
		t.Fatalf("work no_progress = %d after it was finished, want it untouched", got)
	}
}
