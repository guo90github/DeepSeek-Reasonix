package control

import (
	"context"
	"testing"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
)

// A host whose slots are all taken parks work rather than failing it: the step stays
// startable, nothing about its scene changes, and the next dispatch after a slot frees
// hands it out. The take rule is the kernel's (T7-1); this pins the host's side of the
// refusal (T12-3②).
//
// Only work somebody is waiting on is dispatchable (agentbus/wake.go), so both steps hang
// off one asserted deliverable — the same shape the retry cases use.
func TestAFullHostParksWorkUntilASlotFrees(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	c := newAgentBusTalkController(t, dir, "host")
	if _, err := c.ApplyAgentBusOp(ctx, busAssert("design", "alice")); err != nil {
		t.Fatalf("assert design: %v", err)
	}
	for _, node := range []string{"step", "step-2"} {
		if _, err := c.ApplyAgentBusOp(ctx, busRequire("design", "alice", node)); err != nil {
			t.Fatalf("require %s: %v", node, err)
		}
	}
	ledger := agentbus.NewLedger(agentbus.BudgetLimits{Slots: 1})
	c.SetAgentBusLedger(ledger)
	deliver := func(context.Context, agentbus.WakeTarget) error { return nil }

	if n, err := c.AgentBusDispatch(ctx, "worker", deliver); err != nil || n != 1 {
		t.Fatalf("first dispatch = %d (%v), want one step handed out", n, err)
	}
	if n, err := c.AgentBusDispatch(ctx, "other", deliver); err != nil || n != 0 {
		t.Fatalf("dispatch on a full host = %d (%v), want the step parked rather than failed", n, err)
	}
	state := agentBusTickState(t, dir)
	handed, parked := "step", "step-2"
	if state.Nodes[handed].Owner == "" {
		handed, parked = parked, handed
	}
	if state.Nodes[handed].State != board.StateClaimed {
		t.Fatalf("%s = %+v, want the step the host handed out claimed", handed, state.Nodes[handed])
	}
	// A refusal is "wait", never "fail": the parked step keeps its scene unchanged.
	if n := state.Nodes[parked]; n.State != board.StateOpen || n.Owner != "" || n.NoProgress != 0 {
		t.Fatalf("%s = %+v, want it still open, unowned and unspent while the host is full", parked, n)
	}
	if inUse := ledger.SlotsInUse(); inUse != 1 {
		t.Fatalf("slots in use = %d, want the one the first dispatch took", inUse)
	}

	// The worker's turn ended, so a slot frees; the parked step is handed out then.
	ledger.ReleaseSlot("worker")
	if n, err := c.AgentBusDispatch(ctx, "other", deliver); err != nil || n != 1 {
		t.Fatalf("dispatch once a slot freed = %d (%v), want the parked step handed out", n, err)
	}
	if n := agentBusTickState(t, dir).Nodes[parked]; n.State != board.StateClaimed || n.Owner != "other" {
		t.Fatalf("%s = %+v, want it claimed by the participant that was queued for it", parked, n)
	}
}

// The control for the case above: without a slot ceiling the same fixture hands out a step
// to each claimant in turn, so the parked step there is the ceiling's doing and not an empty
// queue.
func TestWithoutASlotCeilingEachClaimantTakesAStep(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	c := newAgentBusTalkController(t, dir, "host")
	if _, err := c.ApplyAgentBusOp(ctx, busAssert("design", "alice")); err != nil {
		t.Fatalf("assert design: %v", err)
	}
	for _, node := range []string{"step", "step-2"} {
		if _, err := c.ApplyAgentBusOp(ctx, busRequire("design", "alice", node)); err != nil {
			t.Fatalf("require %s: %v", node, err)
		}
	}
	deliver := func(context.Context, agentbus.WakeTarget) error { return nil }

	for attempt, claimant := range []string{"worker", "other"} {
		if n, err := c.AgentBusDispatch(ctx, claimant, deliver); err != nil || n != 1 {
			t.Fatalf("dispatch %d for %s = %d (%v), want a step handed out", attempt, claimant, n, err)
		}
	}
}

// A slot is not a scar. The board decides who is working, so a holder whose claim is gone —
// given back by a person, swept after a crash, or settled — has to give its slot back;
// without that the host fills up once and parks every later dispatch for good.
func TestASlotComesBackOnceItsHolderHasNoWork(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	c := newAgentBusTalkController(t, dir, "host")
	if _, err := c.ApplyAgentBusOp(ctx, busAssert("design", "alice")); err != nil {
		t.Fatalf("assert design: %v", err)
	}
	for _, node := range []string{"step", "step-2"} {
		if _, err := c.ApplyAgentBusOp(ctx, busRequire("design", "alice", node)); err != nil {
			t.Fatalf("require %s: %v", node, err)
		}
	}
	ledger := agentbus.NewLedger(agentbus.BudgetLimits{Slots: 1})
	c.SetAgentBusLedger(ledger)
	deliver := func(context.Context, agentbus.WakeTarget) error { return nil }

	if n, err := c.AgentBusDispatch(ctx, "worker", deliver); err != nil || n != 1 {
		t.Fatalf("first dispatch = %d (%v), want one step handed out", n, err)
	}
	claimed := "step"
	if agentBusTickState(t, dir).Nodes[claimed].Owner == "" {
		claimed = "step-2"
	}
	if _, err := c.ApplyAgentBusOp(ctx, board.Op{Verb: board.VerbRelease, Node: claimed, Actor: "worker"}); err != nil {
		t.Fatalf("release %s: %v", claimed, err)
	}

	// The worker owns nothing now, so the host may take its slot for whoever is next.
	if n, err := c.AgentBusDispatch(ctx, "other", deliver); err != nil || n != 1 {
		t.Fatalf("dispatch once the holder's work went away = %d (%v), want the step handed out", n, err)
	}
	if inUse := ledger.SlotsInUse(); inUse != 1 {
		t.Fatalf("slots in use = %d, want the idle holder's slot back and the next one taken", inUse)
	}
	after := agentBusTickState(t, dir)
	owned := 0
	for _, node := range []string{"step", "step-2"} {
		if after.Nodes[node].Owner == "other" {
			owned++
		}
	}
	if owned != 1 {
		t.Fatalf("steps owned by the next claimant = %d, want the one it was queued for", owned)
	}
}
