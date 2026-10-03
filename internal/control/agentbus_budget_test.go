package control

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
)

func agentBusClaim(t *testing.T, c *Controller, node string, steps int) error {
	t.Helper()
	_, err := c.ApplyAgentBusOp(context.Background(), board.Op{
		Verb: board.VerbClaim, Node: node, Actor: "me",
		Bounds:   &board.Bounds{Steps: steps},
		Deadline: time.Now().UTC().Add(time.Minute),
	})
	return err
}

// Work is paid for as it starts: a claim that cannot pay its own node allowance is
// refused before it lands, so neither a record nor a charge is left behind.
func TestAgentBusBudgetRefusesAnOverBudgetClaimBeforeItLands(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	c := newAgentBusTalkController(t, dir, "me")
	budget := agentbus.NewLedger(agentbus.BudgetLimits{Node: 2})
	c.SetAgentBusLedger(budget)
	boardName := filepath.Base(dir)

	if _, err := c.ApplyAgentBusOp(ctx, busAssert("work", "me")); err != nil {
		t.Fatalf("assert: %v", err)
	}
	err := agentBusClaim(t, c, "work", 3)
	reason, refused := agentbus.IsBudgetReject(err)
	if !refused || reason != agentbus.RefuseBudgetNode {
		t.Fatalf("claim of 3 against a node allowance of 2: err = %v, want %s", err, agentbus.RefuseBudgetNode)
	}
	if spent := budget.NodeSpent(boardName, "work"); spent != 0 {
		t.Fatalf("a refused claim spent %d, want the account untouched", spent)
	}
	state := agentBusTickState(t, dir)
	if got := state.Nodes["work"].State; got != board.StateOpen {
		t.Fatalf("work = %q after a refused claim, want it unclaimed", got)
	}

	// Within the allowance it goes through, and the node is charged exactly once: paying
	// again for the same node would let a re-claim spend a budget nobody asked for.
	if err := agentBusClaim(t, c, "work", 2); err != nil {
		t.Fatalf("claim within the allowance: %v", err)
	}
	if spent := budget.NodeSpent(boardName, "work"); spent != 2 {
		t.Fatalf("work spent %d, want 2", spent)
	}
	if _, err := c.ApplyAgentBusOp(ctx, board.Op{Verb: board.VerbRelease, Node: "work", Actor: "me"}); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := agentBusClaim(t, c, "work", 2); err != nil {
		t.Fatalf("re-claim: %v", err)
	}
	if spent := budget.NodeSpent(boardName, "work"); spent != 2 {
		t.Fatalf("work spent %d after a re-claim, want the node charged once", spent)
	}
}

// Only accepted work reaches the board allowance: a finished step settles, one that would
// pass the total is refused — and the verdict that already landed stays landed.
func TestAgentBusBudgetSettlesAcceptedWorkAndStopsAtTheBoardCeiling(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	c := newAgentBusTalkController(t, dir, "me")
	budget := agentbus.NewLedger(agentbus.BudgetLimits{Board: 4})
	c.SetAgentBusLedger(budget)
	boardName := filepath.Base(dir)

	finish := func(node string) {
		t.Helper()
		if _, err := c.ApplyAgentBusOp(ctx, busAssert(node, "me")); err != nil {
			t.Fatalf("assert %s: %v", node, err)
		}
		if err := agentBusClaim(t, c, node, 3); err != nil {
			t.Fatalf("claim %s: %v", node, err)
		}
		if _, err := c.ApplyAgentBusOp(ctx, board.Op{
			Verb: board.VerbDecide, Node: node, Actor: "me", Outcome: board.OutcomeDone,
			Evidence:     []board.Evidence{{Kind: "test", Ref: "go test ./internal/control/"}},
			ReproducedBy: "go test ./internal/control/",
		}); err != nil {
			t.Fatalf("decide %s: %v", node, err)
		}
	}

	finish("first")
	if spent := budget.BoardSpent(boardName); spent != 3 {
		t.Fatalf("board spent %d after one accepted step of 3, want 3", spent)
	}
	// The second accepted step would pass the board's allowance: the account is left
	// untouched and the verdict stands — the ceiling says stop starting work, not undo it.
	finish("second")
	if spent := budget.BoardSpent(boardName); spent != 3 {
		t.Fatalf("board spent %d after an over-ceiling settlement, want it untouched at 3", spent)
	}
	if got := agentBusTickState(t, dir).Nodes["second"].Outcome; got != board.OutcomeDone {
		t.Fatalf("second = %q, want the accepted verdict kept", got)
	}
}

// Without a host account nothing is charged: a session on its own board must keep working.
func TestAgentBusBudgetIsOffWithoutAHostAccount(t *testing.T) {
	dir := t.TempDir()
	c := newAgentBusTalkController(t, dir, "me")
	if _, err := c.ApplyAgentBusOp(context.Background(), busAssert("work", "me")); err != nil {
		t.Fatalf("assert: %v", err)
	}
	if err := agentBusClaim(t, c, "work", 1000000); err != nil {
		t.Fatalf("with no account installed a claim must not be refused: %v", err)
	}
}
