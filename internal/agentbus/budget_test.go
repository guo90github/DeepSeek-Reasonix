package agentbus

import (
	"testing"

	"reasonix/internal/agentbus/board"
)

func TestChargeRefusesAtTheLevelThatCannotPay(t *testing.T) {
	cases := []struct {
		name    string
		limits  BudgetLimits
		amount  int64
		preload func(l *Ledger)
		want    string
	}{
		{"within every level", BudgetLimits{Node: 10, Turn: 10}, 5, nil, ""},
		{"node ceiling", BudgetLimits{Node: 5, Turn: 100}, 6, nil, RefuseBudgetNode},
		{"turn ceiling", BudgetLimits{Node: 100, Turn: 5}, 6, nil, RefuseBudgetTurn},
		{
			"a second charge against the same node", BudgetLimits{Node: 10, Turn: 100}, 6,
			func(l *Ledger) { l.Charge(ChargeRequest{Board: "b", Node: "n1", Turn: "t1", Amount: 5}) },
			RefuseBudgetNode,
		},
		{
			"a second charge in the same turn", BudgetLimits{Node: 100, Turn: 10}, 6,
			func(l *Ledger) { l.Charge(ChargeRequest{Board: "b", Node: "n1", Turn: "t1", Amount: 5}) },
			RefuseBudgetTurn,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := NewLedger(tc.limits)
			if tc.preload != nil {
				tc.preload(l)
			}
			before := l.NodeSpent("b", "n1")
			_, err := l.Charge(ChargeRequest{Board: "b", Node: "n1", Turn: "t1", Amount: tc.amount})
			if tc.want == "" {
				if err != nil {
					t.Fatalf("expected the charge to land, got %v", err)
				}
				return
			}
			reason, ok := IsBudgetReject(err)
			if !ok || reason != tc.want {
				t.Fatalf("refusal = (%v, %q), want %q", ok, reason, tc.want)
			}
			if after := l.NodeSpent("b", "n1"); after != before {
				t.Fatalf("a refusal must leave the account untouched: %d -> %d", before, after)
			}
		})
	}
}

func TestWorkDoesNotSpendTheBoardAllowance(t *testing.T) {
	l := NewLedger(BudgetLimits{Board: 100, Subtree: 100, Node: 1000, Turn: 1000})
	for i := range 20 {
		if _, err := l.Charge(ChargeRequest{Board: "b", Node: "n1", Turn: "t1", Amount: 50}); err != nil {
			t.Fatalf("charge %d: %v", i, err)
		}
	}
	if got := l.BoardSpent("b"); got != 0 {
		t.Fatalf("board spend = %d after plain work, want 0 (T7-4)", got)
	}
}

func TestSettleOnlyPaysForAnAcceptedNode(t *testing.T) {
	st := testState(t, assertOp("n1", "alice"))
	l := NewLedger(BudgetLimits{Board: 10, Subtree: 10})

	reason, ok := IsBudgetReject(reasonOf(l.Settle(st, "n1", ChargeRequest{Board: "b", Amount: 5})))
	if !ok || reason != RefuseNotAccepted {
		t.Fatalf("settling an unaccepted node = (%v, %q), want not_accepted", ok, reason)
	}
	if _, err := l.Settle(st, "missing", ChargeRequest{Board: "b", Amount: 1}); err == nil {
		t.Fatal("an unknown node cannot be settled")
	}

	accepted := testState(t, assertOp("n1", "alice"), claimOp("n1", "alice"), doneOp("n1", "alice", "bob"))
	charge, err := l.Settle(accepted, "n1", ChargeRequest{Board: "b", Amount: 5})
	if err != nil {
		t.Fatalf("settle an accepted node: %v", err)
	}
	if charge.Duplicate {
		t.Fatal("the first settlement is not a duplicate")
	}
	if got := l.BoardSpent("b"); got != 5 {
		t.Fatalf("board spend = %d, want the accepted work", got)
	}

	again, err := l.Settle(accepted, "n1", ChargeRequest{Board: "b", Amount: 5})
	if err != nil {
		t.Fatalf("re-settle: %v", err)
	}
	if !again.Duplicate {
		t.Fatal("settling the same node twice must not pay twice")
	}
	if got := l.BoardSpent("b"); got != 5 {
		t.Fatalf("board spend = %d after a duplicate settlement, want it unchanged", got)
	}
}

func TestSettleRefusesAtTheTotalCeilings(t *testing.T) {
	accepted := func() *board.State {
		return testState(t,
			assertOp("root", "alice"), requireOp("root", "mid"),
			assertOp("mid", "alice"), claimOp("mid", "alice"), doneOp("mid", "alice", "bob"),
		)
	}
	l := NewLedger(BudgetLimits{Board: 4, Subtree: 100})
	if _, err := l.Settle(accepted(), "mid", ChargeRequest{Board: "b", Amount: 4}); err != nil {
		t.Fatalf("first settlement: %v", err)
	}
	state := accepted()
	state.Nodes["other"] = &board.Node{ID: "other", State: board.StateDone, Outcome: board.OutcomeDone}
	reason, ok := IsBudgetReject(reasonOf(l.Settle(state, "other", ChargeRequest{Board: "b", Amount: 1})))
	if !ok || reason != RefuseBudgetBoard {
		t.Fatalf("settling past the board ceiling = (%v, %q), want budget_board", ok, reason)
	}

	tight := NewLedger(BudgetLimits{Board: 100, Subtree: 2})
	reason, ok = IsBudgetReject(reasonOf(tight.Settle(accepted(), "mid", ChargeRequest{Board: "b", Amount: 3})))
	if !ok || reason != RefuseBudgetSubtree {
		t.Fatalf("settling past the subtree ceiling = (%v, %q), want budget_subtree", ok, reason)
	}
}

func TestRemainingReportsTheTightestLevel(t *testing.T) {
	l := NewLedger(BudgetLimits{Board: 100, Subtree: 50, Node: 20, Turn: 5})
	req := ChargeRequest{Board: "b", Subtree: "s", Node: "n1", Turn: "t1"}
	if got := l.Remaining(req); got != 5 {
		t.Fatalf("remaining = %d, want the tightest level (turn, 5)", got)
	}
	if _, err := l.Charge(ChargeRequest{Board: "b", Subtree: "s", Node: "n1", Turn: "t1", Amount: 3}); err != nil {
		t.Fatalf("charge: %v", err)
	}
	if got := l.Remaining(req); got != 2 {
		t.Fatalf("remaining = %d, want 2 once the turn has spent 3", got)
	}
	if got := NewLedger(BudgetLimits{}).Remaining(req); got != -1 {
		t.Fatalf("remaining = %d without any ceiling, want -1 (unbounded)", got)
	}
}

func TestSubtreeRootWalksDependenciesUp(t *testing.T) {
	st := testState(t,
		assertOp("root", "alice"),
		assertOp("mid", "alice"),
		requireOp("mid", "root"),
		assertOp("leaf", "alice"),
		requireOp("leaf", "mid"),
	)
	if got := SubtreeRoot(st, "leaf"); got != "root" {
		t.Fatalf("subtree root of leaf = %q, want root", got)
	}
	if got := SubtreeRoot(st, "root"); got != "root" {
		t.Fatalf("subtree root of root = %q, want itself", got)
	}
	if got := SubtreeRoot(st, "missing"); got != "missing" {
		t.Fatalf("subtree root of an unknown node = %q, want the name it was given", got)
	}
}

// reasonOf drops the value half of a settle result so a test can read the reason.
func reasonOf(_ Charge, err error) error { return err }
