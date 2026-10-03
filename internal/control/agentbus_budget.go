package control

import (
	"log/slog"
	"path/filepath"

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
		return err
	}
	return nil
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
