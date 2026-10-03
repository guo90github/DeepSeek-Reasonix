package control

import (
	"context"
	"fmt"
	"testing"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
)

func agentBusDispatchFixture(t *testing.T, dir string) *Controller {
	t.Helper()
	ctx := context.Background()
	c := newAgentBusTalkController(t, dir, "host")
	if _, err := c.ApplyAgentBusOp(ctx, busAssert("design", "alice")); err != nil {
		t.Fatalf("assert design: %v", err)
	}
	if _, err := c.ApplyAgentBusOp(ctx, busRequire("design", "alice", "step")); err != nil {
		t.Fatalf("require step: %v", err)
	}
	return c
}

// Handing the same step out again after its claim went away is a new attempt, not a replay
// of the first one. The derived op id covers node, actor and bounds but never time, so
// without an id of its own the second claim collapsed onto the first and was dropped as a
// duplicate: the queue grew by a pair every tick while the board never moved.
func TestAgentBusDispatchReassignsAfterTheClaimIsGivenBack(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	c := agentBusDispatchFixture(t, dir)
	deliver := func(context.Context, agentbus.WakeTarget) error { return nil }

	if n, err := c.AgentBusDispatch(ctx, "worker", deliver); err != nil || n != 1 {
		t.Fatalf("first dispatch = %d (%v), want the step handed out", n, err)
	}
	// A crash lapses the lease; a person gives it back. Either way the step is startable
	// again, which is exactly when a second assignment has to land.
	if _, err := c.ApplyAgentBusOp(ctx, board.Op{Verb: board.VerbRelease, Node: "step", Actor: "worker"}); err != nil {
		t.Fatalf("release: %v", err)
	}
	if n, err := c.AgentBusDispatch(ctx, "worker", deliver); err != nil || n != 1 {
		t.Fatalf("second dispatch = %d (%v), want the step handed out again", n, err)
	}
	step := agentBusTickState(t, dir).Nodes["step"]
	if step.State != board.StateClaimed || step.Owner != "worker" {
		t.Fatalf("step = %+v after the second dispatch, want it claimed again", step)
	}
}

// Work that keeps lapsing stops being handed out: the reclaim counter is the kernel's own
// "needs a human" signal, so auto-dispatch spends its budget and then leaves it there.
func TestAgentBusDispatchStopsHandingOutWorkThatKeepsLapsing(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	c := agentBusDispatchFixture(t, dir)
	brd, err := board.Open(dir)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	for attempt := 0; attempt < agentBusDispatchTries; attempt++ {
		// Each attempt is an op of its own: a replayed id would be refused as a duplicate
		// rather than counted as a lapsed claim.
		if _, err := c.ApplyAgentBusOp(ctx, board.Op{
			ID:   fmt.Sprintf("test-lapse-%d", attempt),
			Verb: board.VerbClaim, Node: "step", Actor: "worker",
			Bounds:   &board.Bounds{Steps: 1},
			Deadline: time.Now().UTC().Add(30 * time.Millisecond),
		}); err != nil {
			t.Fatalf("claim %d: %v", attempt, err)
		}
		time.Sleep(60 * time.Millisecond)
		if _, err := brd.Sweep(ctx, time.Now().UTC()); err != nil {
			t.Fatalf("sweep %d: %v", attempt, err)
		}
	}
	spent := agentBusTickState(t, dir).Nodes["step"].NoProgress
	if spent < agentBusDispatchTries {
		t.Fatalf("no_progress = %d, want the retry budget spent (%d)", spent, agentBusDispatchTries)
	}

	deliver := func(context.Context, agentbus.WakeTarget) error { return nil }
	if n, err := c.AgentBusDispatch(ctx, "worker", deliver); err != nil || n != 0 {
		t.Fatalf("dispatch = %d (%v), want nothing handed out once the budget is spent", n, err)
	}
}
