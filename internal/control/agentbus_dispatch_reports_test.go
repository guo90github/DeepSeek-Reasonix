package control

import (
	"context"
	"testing"

	"reasonix/internal/agentbus"
)

// dispatchReportsFor seeds one board, runs one tick, and reports what the dispatcher said
// about the step it handed over.
func dispatchReportsFor(t *testing.T, seed func(t *testing.T, c *Controller)) int {
	t.Helper()
	ctx := context.Background()
	c := newAgentBusTalkController(t, t.TempDir(), "host")
	seed(t, c)

	reports := -1
	deliver := func(_ context.Context, target agentbus.WakeTarget) error {
		for _, node := range target.Ready {
			if node == "step" {
				reports = target.Reports
			}
		}
		return nil
	}
	if n, err := c.AgentBusDispatch(ctx, "worker", deliver); err != nil || n != 1 {
		t.Fatalf("dispatch = %d (%v), want the step handed out", n, err)
	}
	if reports < 0 {
		t.Fatal("the dispatch delivered no target for step")
	}
	return reports
}

// The dispatcher hands a step over with "do it, then decide it" — unless somebody has already
// reported on it, in which case the wake has to ask for the verdict. Only the dispatcher knows
// that at the moment it writes the claim, so it is the one that fills WakeTarget.Reports; the
// rendering on either side of that is covered elsewhere (2026-10-05).
func TestAgentBusDispatchCarriesHowManyReadingsAStepAlreadyHas(t *testing.T) {
	// The dispatcher only takes work somebody waits on, so both cases give "step" a waiter:
	// that is where a step is otherwise parked and handed out at all.
	reported := dispatchReportsFor(t, func(t *testing.T, c *Controller) {
		ctx := context.Background()
		if _, err := c.ApplyAgentBusOp(ctx, busAssert("step", "alice")); err != nil {
			t.Fatalf("assert step: %v", err)
		}
		if _, err := c.ApplyAgentBusOp(ctx, busAssert("design", "alice")); err != nil {
			t.Fatalf("assert design: %v", err)
		}
		if _, err := c.ApplyAgentBusOp(ctx, busRequire("design", "alice", "step")); err != nil {
			t.Fatalf("require step: %v", err)
		}
	})
	if reported != 1 {
		t.Fatalf("Reports for a step that already carries a reading = %d, want 1", reported)
	}

	fresh := dispatchReportsFor(t, func(t *testing.T, c *Controller) {
		ctx := context.Background()
		if _, err := c.ApplyAgentBusOp(ctx, busAssert("design", "alice")); err != nil {
			t.Fatalf("assert design: %v", err)
		}
		if _, err := c.ApplyAgentBusOp(ctx, busRequire("design", "alice", "step")); err != nil {
			t.Fatalf("require step: %v", err)
		}
	})
	if fresh != 0 {
		t.Fatalf("Reports for a step nobody reported on = %d, want 0", fresh)
	}
}
