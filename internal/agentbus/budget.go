package agentbus

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"reasonix/internal/agentbus/board"
)

// BudgetLimits is the four-level allowance. Zero means no bound at that level: the
// kernel invents no ceilings on the operator's behalf.
type BudgetLimits struct {
	Board   int64
	Subtree int64
	Node    int64
	Turn    int64
	// Slots is this host's ceiling on participants working at once. It is the
	// machine's ceiling, not one board's, so the ledger lives per host.
	Slots int
}

// Refusals name the ceiling that refused, so an operator knows which one to raise.
// A refusal means "not now": the caller parks and keeps its scene — the node keeps
// its state, its claim and its work; nothing is abandoned and nothing queues here.
const (
	RefuseBudgetBoard   = "budget_board"
	RefuseBudgetSubtree = "budget_subtree"
	RefuseBudgetNode    = "budget_node"
	RefuseBudgetTurn    = "budget_turn"
	RefuseBudgetNoNode  = "budget_unknown_node"
	RefuseNotAccepted   = "not_accepted"
	RefuseSlots         = "slots_exhausted"
)

// ChargeRequest identifies what is being paid for. Work is charged at the node and
// turn levels; only an accepted outcome reaches the board and subtree levels, so
// doing the work never spends the total allowance. Turn names whose allowance pays:
// the host passes the claimant, so the ceiling is per participant rather than one
// bucket every session on the host drains together.
type ChargeRequest struct {
	Board   string
	Subtree string
	Node    string
	Turn    string
	Amount  int64
}

// Charge is the outcome of one charge. Duplicate reports that this settlement had
// already been paid: the account is charged once, like a replayed op appends once.
type Charge struct {
	Amount    int64
	Duplicate bool
	Remaining int64
}

// BudgetReject explains a refused charge.
type BudgetReject struct {
	Level  string
	Key    string
	Reason string
}

func (e *BudgetReject) Error() string {
	return fmt.Sprintf("agentbus budget: %s (%s) refused: %s", e.Level, e.Key, e.Reason)
}

// IsBudgetReject reports whether err is a budget refusal and returns its reason.
func IsBudgetReject(err error) (string, bool) {
	var rej *BudgetReject
	if !errors.As(err, &rej) {
		return "", false
	}
	return rej.Reason, true
}

// Ledger is one host's spending account. The durable truth stays the board log;
// this is the running account a scheduler consults before starting work.
type Ledger struct {
	limits  BudgetLimits
	spend   map[string]int64
	settled map[string]bool
	slots   map[string]bool
}

// NewLedger opens an empty account under these limits.
func NewLedger(limits BudgetLimits) *Ledger {
	return &Ledger{
		limits:  limits,
		spend:   map[string]int64{},
		settled: map[string]bool{},
		slots:   map[string]bool{},
	}
}

// AcquireSlot reserves this holder's one concurrent slot on the host. It is
// idempotent for a holder that already has one, so a scheduler may call it before
// every piece of work; a refused acquire means "wait", never "fail".
func (l *Ledger) AcquireSlot(holder string) error {
	if l.limits.Slots <= 0 {
		return nil
	}
	if l.slots[holder] {
		return nil
	}
	if len(l.slots) >= l.limits.Slots {
		return &BudgetReject{Level: "slots", Key: holder, Reason: RefuseSlots}
	}
	l.slots[holder] = true
	return nil
}

// ReleaseSlot frees a holder's slot. Releasing one nobody holds is a no-op.
func (l *Ledger) ReleaseSlot(holder string) {
	delete(l.slots, holder)
}

// SlotsInUse reports how many of this host's slots are taken.
func (l *Ledger) SlotsInUse() int {
	return len(l.slots)
}

// SlotHolders names who holds a slot, sorted. A host that has to give the slot of somebody
// who stopped working back needs to know whose slots to look at.
func (l *Ledger) SlotHolders() []string {
	return slices.Sorted(maps.Keys(l.slots))
}

// Limits reports the ceilings this account was opened with.
func (l *Ledger) Limits() BudgetLimits { return l.limits }

// Charge spends from the node and turn allowances. It refuses at the first level
// that cannot pay, naming it, and a refusal leaves the account untouched.
func (l *Ledger) Charge(req ChargeRequest) (Charge, error) {
	if err := l.check(RefuseBudgetNode, nodeKey(req), l.limits.Node, req.Amount); err != nil {
		return Charge{}, err
	}
	if err := l.check(RefuseBudgetTurn, turnKey(req), l.limits.Turn, req.Amount); err != nil {
		return Charge{}, err
	}
	l.spend[nodeKey(req)] += req.Amount
	l.spend[turnKey(req)] += req.Amount
	return Charge{Amount: req.Amount, Remaining: l.Remaining(req)}, nil
}

// Settle charges an accepted node against the board and subtree allowances. Work
// that nobody accepted never spends the total budget (T7-4), and settling the same
// node twice pays once.
func (l *Ledger) Settle(st *board.State, node string, req ChargeRequest) (Charge, error) {
	if st == nil {
		return Charge{}, &BudgetReject{Level: "node", Key: node, Reason: RefuseBudgetNoNode}
	}
	n, ok := st.Nodes[node]
	if !ok {
		return Charge{}, &BudgetReject{Level: "node", Key: node, Reason: RefuseBudgetNoNode}
	}
	if n.Outcome != board.OutcomeDone {
		return Charge{}, &BudgetReject{Level: "node", Key: node, Reason: RefuseNotAccepted}
	}
	if l.settled[node] {
		return Charge{Amount: req.Amount, Duplicate: true, Remaining: l.Remaining(req)}, nil
	}
	req.Node = node
	if req.Subtree == "" {
		req.Subtree = SubtreeRoot(st, node)
	}
	if err := l.check(RefuseBudgetBoard, boardKey(req), l.limits.Board, req.Amount); err != nil {
		return Charge{}, err
	}
	if err := l.check(RefuseBudgetSubtree, subtreeKey(req), l.limits.Subtree, req.Amount); err != nil {
		return Charge{}, err
	}
	l.spend[boardKey(req)] += req.Amount
	l.spend[subtreeKey(req)] += req.Amount
	l.settled[node] = true
	return Charge{Amount: req.Amount, Remaining: l.Remaining(req)}, nil
}

func (l *Ledger) check(reason, key string, limit, amount int64) error {
	if limit <= 0 {
		return nil
	}
	if l.spend[key]+amount > limit {
		level := "board"
		switch reason {
		case RefuseBudgetSubtree:
			level = "subtree"
		case RefuseBudgetNode:
			level = "node"
		case RefuseBudgetTurn:
			level = "turn"
		}
		return &BudgetReject{Level: level, Key: key, Reason: reason}
	}
	return nil
}

// NodeSpent reports what one node has been charged for its work.
func (l *Ledger) NodeSpent(board, node string) int64 {
	return l.spend[nodeKey(ChargeRequest{Board: board, Node: node})]
}

// Remaining is the tightest allowance left across all four levels: the number a
// scheduler may actually rely on.
func (l *Ledger) Remaining(req ChargeRequest) int64 {
	remaining := int64(-1)
	for _, level := range []struct {
		limit int64
		spent int64
	}{
		{l.limits.Board, l.spend[boardKey(req)]},
		{l.limits.Subtree, l.spend[subtreeKey(req)]},
		{l.limits.Node, l.spend[nodeKey(req)]},
		{l.limits.Turn, l.spend[turnKey(req)]},
	} {
		if level.limit <= 0 {
			continue
		}
		left := level.limit - level.spent
		if remaining < 0 || left < remaining {
			remaining = left
		}
	}
	return remaining
}

// BoardSpent reports what the whole board has been charged for accepted work.
func (l *Ledger) BoardSpent(board string) int64 {
	return l.spend["board:"+board]
}

func boardKey(req ChargeRequest) string   { return "board:" + req.Board }
func subtreeKey(req ChargeRequest) string { return "subtree:" + req.Board + "/" + req.Subtree }
func nodeKey(req ChargeRequest) string    { return "node:" + req.Board + "/" + req.Node }
func turnKey(req ChargeRequest) string    { return "turn:" + req.Board + "/" + req.Turn }

// SubtreeRoot names the subtree a node belongs to: the topmost node reachable by
// walking dependencies upwards. A node with two independent ancestors resolves to
// the smaller id, so two readers agree on the same subtree.
func SubtreeRoot(st *board.State, node string) string {
	if st == nil {
		return ""
	}
	root := node
	seen := map[string]bool{}
	for {
		n, ok := st.Nodes[root]
		if !ok || len(n.Deps) == 0 || seen[root] {
			return root
		}
		seen[root] = true
		next := ""
		for _, dep := range n.Deps {
			// A missing dependency is not a subtree: a broken edge must never move a
			// node into a subtree nobody can act on.
			if _, ok := st.Nodes[dep]; !ok {
				continue
			}
			if next == "" || dep < next {
				next = dep
			}
		}
		if next == "" {
			return root
		}
		root = next
	}
}
