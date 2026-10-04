package control

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
)

// I8: the costing rule is "a node is charged once, and only accepted work spends the total".
// `chargeClaim` spends the claimant's own allowance the moment work starts (node + turn), and
// `settleBudget` spends the board/subtree levels only when a `decide(done)` lands. Both halves
// matter to a cost model: handing a step back must not buy the session a second allowance, and a
// step nobody accepted must not move the total at all.
func TestANodeIsChargedOnceAndOnlyAcceptedWorkSpendsTheTotal(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	c := newAgentBusTalkController(t, dir, "host")
	ledger := agentbus.NewLedger(agentbus.BudgetLimits{Turn: 5, Board: 4})
	c.SetAgentBusLedger(ledger)
	boardName := filepath.Base(dir)
	apply := func(op board.Op) error {
		t.Helper()
		_, err := c.ApplyAgentBusOp(ctx, op)
		return err
	}
	claim := func(node string) error {
		t.Helper()
		return apply(board.Op{
			Verb: board.VerbClaim, Node: node, Actor: "worker",
			Bounds:   &board.Bounds{Steps: 3},
			Deadline: time.Now().UTC().Add(time.Hour),
		})
	}
	for _, node := range []string{"step-a", "step-b"} {
		if err := apply(busAssert(node, "alice")); err != nil {
			t.Fatalf("assert %s: %v", node, err)
		}
	}
	if err := claim("step-a"); err != nil {
		t.Fatalf("claim step-a: %v", err)
	}
	if got := ledger.NodeSpent(boardName, "step-a"); got != 3 {
		t.Fatalf("step-a spent = %d, want the 3 steps it declared", got)
	}

	// Handing the step back does not refund it, so re-claiming it must not need a second
	// allowance — the node was already paid for when work first started on it.
	if err := apply(board.Op{Verb: board.VerbRelease, Node: "step-a", Actor: "worker"}); err != nil {
		t.Fatalf("release step-a: %v", err)
	}
	if err := claim("step-a"); err != nil {
		t.Fatalf("re-claim step-a: %v", err)
	}
	if got := ledger.NodeSpent(boardName, "step-a"); got != 3 {
		t.Fatalf("step-a spent = %d after the re-claim, want it charged once", got)
	}

	// A different step pays out of the same allowance, and that ceiling is real: 3 + 3 > 5.
	if err := claim("step-b"); err == nil {
		t.Fatal("a second step must pay, and a claim that does not fit must be refused")
	}

	// The total levels see only work that was accepted: a claim alone never reaches them.
	if got := ledger.BoardSpent(boardName); got != 0 {
		t.Fatalf("board spent = %d before anything landed, want 0", got)
	}
	if err := apply(board.Op{
		Verb: board.VerbDecide, Node: "step-a", Actor: "worker",
		Outcome: board.OutcomeDone, ReproducedBy: "bob",
	}); err != nil {
		t.Fatalf("decide step-a done: %v", err)
	}
	if got := ledger.BoardSpent(boardName); got != 3 {
		t.Fatalf("board spent = %d after the step landed, want its 3 steps", got)
	}
}
