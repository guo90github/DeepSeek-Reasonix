package main

import (
	"context"
	"testing"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
	"reasonix/internal/config"
	"reasonix/internal/control"
	"reasonix/internal/event"
)

// Every knob maps onto the level it names, and an unconfigured host stays at the kernel's own
// value — zero everywhere, which is the behavior this section did not change.
func TestAgentBusBudgetLimitsMapEveryKnobOntoItsLevel(t *testing.T) {
	limits := agentBusBudgetLimits(config.AgentBusConfig{
		BudgetBoard: 40, BudgetSubtree: 20, BudgetNode: 5, BudgetTurn: 3, DispatchSlots: 2,
	})
	want := agentbus.BudgetLimits{Board: 40, Subtree: 20, Node: 5, Turn: 3, Slots: 2}
	if limits != want {
		t.Fatalf("limits = %+v, want %+v", limits, want)
	}
	if empty := agentBusBudgetLimits(config.AgentBusConfig{}); empty != (agentbus.BudgetLimits{}) {
		t.Fatalf("an unconfigured host = %+v, want the kernel's no-ceiling value", empty)
	}
}

// A configured ceiling has to reach the claim path: an over-budget claim is refused before
// anything lands, which is what "the operator can cap the work" means on a real host.
func TestConfiguredBudgetRefusesAnOverBudgetClaim(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	ctrl := control.New(control.Options{SessionDir: t.TempDir(), Sink: event.Discard})
	ctrl.SetAgentBus(dir, "bob")
	ctrl.SetAgentBusLedger(agentbus.NewLedger(agentBusBudgetLimits(config.AgentBusConfig{BudgetNode: 2})))
	if _, err := ctrl.ApplyAgentBusOp(ctx, board.Op{
		Verb: board.VerbAssert, Node: "step", Actor: "bob",
		Evidence: []board.Evidence{{Kind: "test", Ref: "evidence:step"}},
	}); err != nil {
		t.Fatalf("assert: %v", err)
	}

	_, err := ctrl.ApplyAgentBusOp(ctx, board.Op{
		Verb: board.VerbClaim, Node: "step", Actor: "bob",
		Bounds:   &board.Bounds{Steps: 5},
		Deadline: time.Now().UTC().Add(time.Hour),
	})
	reason, refused := agentbus.IsBudgetReject(err)
	if !refused || reason != agentbus.RefuseBudgetNode {
		t.Fatalf("over-budget claim err = %v (reason %q), want a node-budget refusal", err, reason)
	}

	brd, err := board.Open(dir)
	if err != nil {
		t.Fatalf("open board: %v", err)
	}
	state, err := brd.Snapshot(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	step := state.Nodes["step"]
	if step == nil || step.State != board.StateOpen {
		t.Fatalf("step = %+v, want the refused claim to leave nothing behind", step)
	}
}
