package control

import (
	"context"
	"path/filepath"
	"testing"

	"reasonix/internal/agentbus"
)

// A step the spending ceiling refused is not handed out again on the next tick: retrying every
// 30s produced a WARN + INFO pair every 30s for as long as the ceiling stood, with no bound
// (measured 2026-10-04: refusals 1…11 over six minutes for one unchanged node).
func TestABudgetRefusedStepIsNotHandedOutEveryTick(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	c := newAgentBusTalkController(t, dir, "host")
	ledger := agentbus.NewLedger(agentbus.BudgetLimits{Turn: 5})
	c.SetAgentBusLedger(ledger)
	// Spend the claimant's whole turn allowance up front, so the dispatch's own charge is the
	// one that gets refused. The account is keyed by the board's own name, which is the board
	// directory's base: charging "default" here filled a different bucket and the dispatch paid.
	if _, err := ledger.Charge(agentbus.ChargeRequest{
		Board: filepath.Base(dir), Node: "spent", Turn: "worker", Amount: 5,
	}); err != nil {
		t.Fatalf("pre-charge: %v", err)
	}
	if _, err := c.ApplyAgentBusOp(ctx, busAssert("design", "alice")); err != nil {
		t.Fatalf("assert design: %v", err)
	}
	if _, err := c.ApplyAgentBusOp(ctx, busRequire("design", "alice", "step")); err != nil {
		t.Fatalf("require step: %v", err)
	}
	deliver := func(context.Context, agentbus.WakeTarget) error { return nil }

	// First tick: the ceiling refuses, and the step goes into the memo.
	if _, err := c.AgentBusDispatch(ctx, "worker", deliver); err == nil {
		t.Fatal("the first dispatch should report the refused claim")
	}
	// Second tick: nothing is taken, so nothing is refused — no second pair of log lines.
	n, err := c.AgentBusDispatch(ctx, "worker", deliver)
	if err != nil {
		t.Fatalf("second dispatch = %v, want the refused step skipped rather than retried", err)
	}
	if n != 0 {
		t.Fatalf("dispatched %d on the second tick, want 0 while the refusal stands", n)
	}
}
