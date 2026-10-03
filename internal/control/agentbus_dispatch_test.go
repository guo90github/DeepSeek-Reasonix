package control

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
)

// The host hands out startable work: one step per claimant per tick, taken out of the queue
// (which is where it waited for a slot), assigned to the participant it names, and
// delivered to that participant and nobody else.
func TestAgentBusDispatchAssignsStartableWorkToTheNamedParticipant(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	c := newAgentBusTalkController(t, dir, "host")
	if _, err := c.ApplyAgentBusOp(ctx, busAssert("design", "alice")); err != nil {
		t.Fatalf("assert design: %v", err)
	}
	if _, err := c.ApplyAgentBusOp(ctx, busRequire("design", "alice", "step")); err != nil {
		t.Fatalf("require step: %v", err)
	}

	var delivered []string
	var assigned []string
	deliver := func(_ context.Context, target agentbus.WakeTarget) error {
		delivered = append(delivered, target.Participant+"->"+strings.Join(target.Ready, ","))
		assigned = append(assigned, target.Key)
		return nil
	}
	n, err := c.AgentBusDispatch(ctx, "worker", deliver)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if n != 1 {
		t.Fatalf("dispatched %d, want the one startable step", n)
	}
	if len(delivered) != 1 || delivered[0] != "worker->step" {
		t.Fatalf("delivered %v, want the step assigned to worker only", delivered)
	}
	// The wake carries the assignment key: that is what tells the recipient the node is
	// already claimed in its name, so the message is not read as a plain wake.
	if len(assigned) != 1 || !agentbus.IsDispatchKey(assigned[0]) || !strings.HasSuffix(assigned[0], "/step") {
		t.Fatalf("keys = %v, want one assignment key naming step", assigned)
	}
	state := agentBusTickState(t, dir)
	step := state.Nodes["step"]
	if step.State != board.StateClaimed || step.Owner != "worker" {
		t.Fatalf("step = %+v, want it claimed by the participant that was named", step)
	}

	// It was taken out of the queue, not left waiting in it.
	queueLog, err := agentbus.OpenQueueLog(dir)
	if err != nil {
		t.Fatalf("open queue: %v", err)
	}
	queue, _, err := queueLog.Read()
	if err != nil {
		t.Fatalf("read queue: %v", err)
	}
	if queue.Depth() != 0 {
		t.Fatalf("queue depth = %d, want the dispatched step taken out of it", queue.Depth())
	}
}

// A host that is full parks the work instead of failing: the queue is the list of steps
// waiting for a slot, and the slot ceiling is what makes it non-empty.
func TestAgentBusDispatchLeavesWorkParkedWhenTheHostIsFull(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	c := newAgentBusTalkController(t, dir, "host")
	c.SetAgentBusLedger(agentbus.NewLedger(agentbus.BudgetLimits{Slots: 1}))
	if _, err := c.ApplyAgentBusOp(ctx, busAssert("design", "alice")); err != nil {
		t.Fatalf("assert design: %v", err)
	}
	for _, spec := range []struct{ node, dep string }{
		{"design", "step-1"},
		{"design", "step-2"},
	} {
		if _, err := c.ApplyAgentBusOp(ctx, busRequire(spec.node, "alice", spec.dep)); err != nil {
			t.Fatalf("require %s: %v", spec.dep, err)
		}
	}

	deliver := func(context.Context, agentbus.WakeTarget) error { return nil }
	if n, err := c.AgentBusDispatch(ctx, "worker", deliver); err != nil || n != 1 {
		t.Fatalf("first dispatch = %d (%v), want the one slot to take one step", n, err)
	}
	if n, err := c.AgentBusDispatch(ctx, "other", deliver); err != nil || n != 0 {
		t.Fatalf("second dispatch = %d (%v), want the full host to hand out nothing", n, err)
	}

	queueLog, err := agentbus.OpenQueueLog(dir)
	if err != nil {
		t.Fatalf("open queue: %v", err)
	}
	queue, _, err := queueLog.Read()
	if err != nil {
		t.Fatalf("read queue: %v", err)
	}
	if queue.Depth() != 1 {
		t.Fatalf("queue depth = %d, want the step the host had no slot for still parked", queue.Depth())
	}
}

// A session that cannot be reached is a refusal, never a silent assignment: the claim is
// released again so the step is nobody's rather than owned by a session that never heard.
func TestAgentBusDispatchReleasesTheStepWhenItsSessionCannotBeReached(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	c := newAgentBusTalkController(t, dir, "host")
	if _, err := c.ApplyAgentBusOp(ctx, busAssert("design", "alice")); err != nil {
		t.Fatalf("assert design: %v", err)
	}
	if _, err := c.ApplyAgentBusOp(ctx, busRequire("design", "alice", "step")); err != nil {
		t.Fatalf("require: %v", err)
	}

	deliver := func(_ context.Context, target agentbus.WakeTarget) error {
		return fmt.Errorf("no tab and no address owns agentbus participant %q", target.Participant)
	}
	n, err := c.AgentBusDispatch(ctx, "worker", deliver)
	if err == nil {
		t.Fatal("an unreachable session must fail the dispatch, not assign the work anyway")
	}
	if n != 0 {
		t.Fatalf("dispatched %d to an unreachable session, want 0", n)
	}
	step := agentBusTickState(t, dir).Nodes["step"]
	if step.State != board.StateOpen || step.Owner != "" {
		t.Fatalf("step = %+v, want the claim released again", step)
	}
}
