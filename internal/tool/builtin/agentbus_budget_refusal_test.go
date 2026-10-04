package builtin

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/agentbus"
)

// I12/B10: a spending ceiling is the board's answer too, and this tool's contract says a refusal
// comes back as text rather than as a failed call. The host keeps its own typed error for the
// dispatch loop (the terminating skip keys on it), which is why the conversion is here, at the
// tool boundary (2026-10-05).
func TestABudgetRefusalComesBackAsTextNotAFailedCall(t *testing.T) {
	port := &fakeBoardPort{
		dir: "/tmp/board/default", participant: "alice",
		reject: &agentbus.BudgetReject{Level: "turn", Key: "turn:default/alice", Reason: agentbus.RefuseBudgetTurn},
	}
	out, err := NewAgentBusTool(port).Execute(context.Background(),
		boardArgs(t, `{"action":"claim","node":"build","steps":3,"deadline":"2030-01-01T00:00:00Z"}`))
	if err != nil {
		t.Fatalf("a budget refusal must not surface as a tool error: %v", err)
	}
	if !strings.Contains(out, agentbus.RefuseBudgetTurn) || !strings.Contains(out, "refused") {
		t.Fatalf("result = %q, want the reason and a refusal the model can correct", out)
	}
}
