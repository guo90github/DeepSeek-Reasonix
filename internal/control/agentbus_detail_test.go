package control

import (
	"context"
	"testing"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
)

// The step's detail is what a human checks it with: its state and dependencies, and — the
// part only the op log knows — who authorized it (§13.8).
func TestAgentBusNodeDetailReadsTheStepAndWhoApprovedIt(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	ctrl := newAgentBusTalkController(t, dir, "orchestrator")

	if _, ok := ctrl.AgentBusNodeDetail("publish"); ok {
		t.Fatal("a node the board does not hold must be reported absent")
	}

	applyBusOps(t, dir, busAssert("publish", "worker"))
	if _, err := ctrl.ApplyAgentBusOp(ctx, board.Op{
		Verb: board.VerbAssert, Node: "publish", Actor: "operator",
		Reason: "the key is installed and scoped", Source: agentbus.GrantSource,
		Evidence: []board.Evidence{busEvidence("verification", "go test ./...")},
	}); err != nil {
		t.Fatalf("grant: %v", err)
	}

	detail, ok := ctrl.AgentBusNodeDetail("publish")
	if !ok {
		t.Fatal("the board holds this node")
	}
	if detail.Node != "publish" || detail.State != board.StateOpen {
		t.Fatalf("detail = %+v, want the open step", detail)
	}
	if len(detail.Authorizations) != 1 || detail.Authorizations[0].Actor != "operator" {
		t.Fatalf("authorizations = %+v, want the operator's", detail.Authorizations)
	}
	if detail.Authorizations[0].Reason == "" {
		t.Fatal("an authorization must keep the reason it was given")
	}
}

func TestAgentBusNodeDetailIsEmptyOffTheBoard(t *testing.T) {
	ctrl := newAgentBusTestController(t)
	if _, ok := ctrl.AgentBusNodeDetail("publish"); ok {
		t.Fatal("a session off the board has no node detail to show")
	}
}
