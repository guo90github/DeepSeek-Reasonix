package control

import (
	"context"
	"slices"
	"testing"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
)

func agentBusTickState(t *testing.T, dir string) *board.State {
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

// The host tick is what moves work while nobody writes: a holder that went away leaves
// a lapsed lease behind, and with no writer nothing else would ever notice. The wake
// half only fires for a work set nobody has been told about, which is why alice asks
// for the step *while* it is claimed — the tick is her first notice of it. Both
// requesters of that step (bob, who asserted it, and alice, who waits on it) are owed.
func TestAgentBusTickReclaimsAndWakesWithNothingBeingWritten(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	c := newAgentBusTalkController(t, dir, "host")
	var woken []string
	c.SetAgentBusWaker(func(_ context.Context, target agentbus.WakeTarget) error {
		woken = append(woken, target.Participant)
		return nil
	})

	// bob takes the step, then he is gone and the lease lapses.
	if _, err := c.ApplyAgentBusOp(ctx, busAssert("step", "bob")); err != nil {
		t.Fatalf("assert step: %v", err)
	}
	if _, err := c.ApplyAgentBusOp(ctx, board.Op{Verb: board.VerbClaim, Node: "step", Actor: "bob",
		Bounds:   &board.Bounds{Steps: 1},
		Deadline: time.Now().UTC().Add(50 * time.Millisecond)}); err != nil {
		t.Fatalf("claim step: %v", err)
	}
	// alice needs that step, and asking while it is claimed wakes nobody.
	if _, err := c.ApplyAgentBusOp(ctx, busAssert("design", "alice")); err != nil {
		t.Fatalf("assert design: %v", err)
	}
	if _, err := c.ApplyAgentBusOp(ctx, busRequire("design", "alice", "step")); err != nil {
		t.Fatalf("require step: %v", err)
	}
	if len(woken) != 0 {
		t.Fatalf("a claimed step owes nobody a wake yet, got %v", woken)
	}
	time.Sleep(120 * time.Millisecond)

	// Nothing is written from here on: the tick has to do this alone.
	if n := c.AgentBusTick(ctx); n != 2 {
		t.Fatalf("the tick woke %d, want both requesters of the reclaimed step: %v", n, woken)
	}
	if !slices.Contains(woken, "alice") || !slices.Contains(woken, "bob") {
		t.Fatalf("woken = %v, want alice and bob", woken)
	}
	state := agentBusTickState(t, dir)
	if got := state.Nodes["step"].State; got != board.StateOpen {
		t.Fatalf("step = %q after the tick, want the lapsed lease reclaimed", got)
	}
	if got := state.Nodes["step"].NoProgress; got != 1 {
		t.Fatalf("step no_progress = %d, want the reclamation recorded once", got)
	}

	// Nothing is lapsed now, so another tick is silent.
	if n := c.AgentBusTick(ctx); n != 0 {
		t.Fatalf("a second tick woke %d with nothing lapsed, want 0", n)
	}
}

// A tick on a session that never joined a board is a no-op, not a panic: the host
// runs this for every tab it has, joined or not.
func TestAgentBusTickOffTheBoardIsANoOp(t *testing.T) {
	c := newAgentBusTestController(t)
	if n := c.AgentBusTick(context.Background()); n != 0 {
		t.Fatalf("a session off the board woke %d, want 0", n)
	}
}
