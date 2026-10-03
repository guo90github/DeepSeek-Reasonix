package control

import (
	"errors"
	"log/slog"
	"path/filepath"
	"sync/atomic"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
)

// SetAgentBusLedger shares a host's spending account with this controller. The ceilings
// belong to the machine — that is what BudgetLimits.Slots says — so a host installs the
// same account on every controller it builds, and without one nothing is charged.
func (c *Controller) SetAgentBusLedger(ledger *agentbus.Ledger) {
	if ledger == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.agentBus != nil {
		c.agentBus.ledger = ledger
	}
}

// chargeClaim pays for work as it starts: a claim is the moment work begins, so that is
// where a node's allowance is spent. A refusal comes back before the op lands, so an
// over-budget claim leaves neither a record nor a charge.
//
// A node is charged once — a re-claim or a replayed delivery of the same intent must not
// spend twice. The order is charge-then-write, the kernel's "refusal means not now": if
// the board then refuses the claim for its own reason, the spend stands.
func (b *agentBusState) chargeClaim(op board.Op) error {
	if op.Verb != board.VerbClaim {
		return nil
	}
	ledger, boardName := b.budget()
	if ledger == nil || ledger.NodeSpent(boardName, op.Node) > 0 {
		return nil
	}
	amount := int64(0)
	if op.Bounds != nil {
		amount = int64(op.Bounds.Steps)
	}
	if _, err := ledger.Charge(agentbus.ChargeRequest{
		Board: boardName, Node: op.Node, Amount: amount,
	}); err != nil {
		recordBudgetRefusal(err, ledger, boardName, op.Node)
		return err
	}
	return nil
}

// budgetRefusals counts the claims this host had to turn down, so one refusal and a pattern
// look different in the log. G3's point is that a brake nobody can see is not a brake: which
// ceiling refused belongs in the host's own record, not only in the tool result (T12-3).
var budgetRefusals atomic.Int64

// budgetRefusalCounts says which ceiling refused, not only how often. A single total answers
// "is a brake biting"; the level is what a person has to move, and it is what the panel shows
// (G3's "which layer topped out", 2026-10-03).
var budgetRefusalCounts = struct {
	board, subtree, node, turn, slots atomic.Int64
}{}

// BudgetRefusalCounts is the host's refusal record, by the level that refused.
type BudgetRefusalCounts struct {
	Board   int64
	Subtree int64
	Node    int64
	Turn    int64
	Slots   int64
}

// Total is how many claims were refused at any level.
func (c BudgetRefusalCounts) Total() int64 {
	return c.Board + c.Subtree + c.Node + c.Turn + c.Slots
}

// AgentBusBudgetRefusals reports the host's refusals. Process-wide, like the account itself:
// the ceilings belong to the machine, so the count does.
func AgentBusBudgetRefusals() BudgetRefusalCounts {
	return BudgetRefusalCounts{
		Board:   budgetRefusalCounts.board.Load(),
		Subtree: budgetRefusalCounts.subtree.Load(),
		Node:    budgetRefusalCounts.node.Load(),
		Turn:    budgetRefusalCounts.turn.Load(),
		Slots:   budgetRefusalCounts.slots.Load(),
	}
}

// noteBudgetRefusal records one refusal against the level that made it.
func noteBudgetRefusal(level string) {
	switch level {
	case "board":
		budgetRefusalCounts.board.Add(1)
	case "subtree":
		budgetRefusalCounts.subtree.Add(1)
	case "node":
		budgetRefusalCounts.node.Add(1)
	case "turn":
		budgetRefusalCounts.turn.Add(1)
	case "slots":
		budgetRefusalCounts.slots.Add(1)
	}
}

func recordBudgetRefusal(err error, ledger *agentbus.Ledger, boardName, node string) {
	var reject *agentbus.BudgetReject
	if !errors.As(err, &reject) {
		return
	}
	noteBudgetRefusal(reject.Level)
	limit := int64(0)
	if ledger != nil {
		limits := ledger.Limits()
		switch reject.Level {
		case "board":
			limit = limits.Board
		case "subtree":
			limit = limits.Subtree
		case "node":
			limit = limits.Node
		case "turn":
			limit = limits.Turn
		case "slots":
			limit = int64(limits.Slots)
		}
	}
	attrs := []any{
		"level", reject.Level, "reason", reject.Reason, "key", reject.Key, "limit", limit,
		"board", boardName, "node", node, "refusals", budgetRefusals.Add(1),
	}
	if reject.Level == "node" && ledger != nil {
		// The node level is the only one whose spend has a reader, so it is the only level
		// that can report what is left as well as what the ceiling was.
		attrs = append(attrs, "remaining", limit-ledger.NodeSpent(boardName, node))
	}
	slog.Warn("controller: agentbus claim refused by a budget ceiling", attrs...)
}

// settleBudget charges an accepted node against the board and subtree allowances. The
// verdict has already landed, so a refusal here is bookkeeping and not a gate: it says
// stop starting new work, never undo the work that was accepted.
func (b *agentBusState) settleBudget(state *board.State, node string) {
	ledger, boardName := b.budget()
	if ledger == nil || state == nil {
		return
	}
	n := state.Nodes[node]
	if n == nil || n.Outcome != board.OutcomeDone {
		return
	}
	amount := int64(0)
	if n.Bounds != nil {
		amount = int64(n.Bounds.Steps)
	}
	if _, err := ledger.Settle(state, node, agentbus.ChargeRequest{
		Board: boardName, Node: node, Amount: amount,
	}); err != nil {
		slog.Warn("controller: agentbus settle", "node", node, "err", err)
	}
}

// budget reads the account and this board's name under the state lock.
func (b *agentBusState) budget() (*agentbus.Ledger, string) {
	if b == nil {
		return nil, ""
	}
	b.mu.Lock()
	ledger := b.ledger
	b.mu.Unlock()
	return ledger, filepath.Base(b.dir)
}
