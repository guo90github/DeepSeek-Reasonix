package control

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
)

// An assigned step reaches the participant it names and nobody else: any other
// participant's tick leaves it parked, and the assignee's own tick is what takes it.
func TestAgentBusDispatchDeliversAnAssignedStepOnlyToItsAssignee(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	c := newAgentBusTalkController(t, dir, "host")
	if _, err := c.ApplyAgentBusOp(ctx, busAssert("design", "alice")); err != nil {
		t.Fatalf("assert design: %v", err)
	}
	if _, err := c.ApplyAgentBusOp(ctx, busRequire("design", "alice", "step")); err != nil {
		t.Fatalf("require step: %v", err)
	}
	assign := board.Op{Verb: board.VerbAssign, Node: "step", Actor: "alice", Assignee: "bob"}
	if _, err := c.ApplyAgentBusOp(ctx, assign); err != nil {
		t.Fatalf("assign step: %v", err)
	}

	var delivered []string
	deliver := func(_ context.Context, target agentbus.WakeTarget) error {
		delivered = append(delivered, target.Participant+"->"+strings.Join(target.Ready, ","))
		return nil
	}
	if n, err := c.AgentBusDispatch(ctx, "alice", deliver); err != nil || n != 0 {
		t.Fatalf("alice's dispatch = %d (%v), want the assigned step left for bob", n, err)
	}
	if len(delivered) != 0 {
		t.Fatalf("delivered %v, want nobody but the assignee told", delivered)
	}
	if n, err := c.AgentBusDispatch(ctx, "bob", deliver); err != nil || n != 1 {
		t.Fatalf("bob's dispatch = %d (%v), want his assigned step", n, err)
	}
	if len(delivered) != 1 || delivered[0] != "bob->step" {
		t.Fatalf("delivered %v, want the step assigned to bob only", delivered)
	}
	step := agentBusTickState(t, dir).Nodes["step"]
	if step.State != board.StateClaimed || step.Owner != "bob" {
		t.Fatalf("step = %+v, want it claimed by the assignee", step)
	}
}
