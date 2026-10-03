package control

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
)

// agentBusDispatchLease is how long a dispatched step stays with its participant before a
// later writer or tick may reclaim it: long enough for one turn, short enough to recover.
const agentBusDispatchLease = 30 * time.Minute

// agentBusDispatchTries is how many times auto-dispatch hands out one step. A lapsed lease
// gets one more try — that is the crash the reclaim exists for — but a step that keeps
// lapsing will not move by being handed out again: the human screen already flags it
// (observe reports "no progress recorded N times"), so the loop stops there.
const agentBusDispatchTries = 2

// AgentBusDispatch gives one named participant the next startable step, takes it out of the
// queue, and asks the host to deliver that assignment to the participant's own session.
// Naming the claimant here is what keeps an assignment from being a guess.
//
// deliver is the host's routing: it must reach the claimant's session and no other, and it
// must fail rather than fall back when that session cannot be reached. A claim nobody was
// told about is released again — leaving it would hand work to a participant that never
// heard of it.
func (c *Controller) AgentBusDispatch(ctx context.Context, claimant string, deliver func(context.Context, agentbus.WakeTarget) error) (int, error) {
	claimant = strings.TrimSpace(claimant)
	bus, st := c.agentBusSnapshot(time.Now().UTC())
	if bus == nil || st == nil || claimant == "" {
		return 0, nil
	}
	now := time.Now().UTC()
	queueLog, err := agentbus.OpenQueueLog(bus.dir)
	if err != nil {
		return 0, err
	}
	if err := parkStartableWork(ctx, queueLog, st, now); err != nil {
		return 0, err
	}
	ledger, boardName := bus.budget()
	if ledger == nil {
		ledger = agentbus.NewLedger(agentbus.BudgetLimits{})
	}
	releaseIdleSlots(ledger, st)
	taken, err := agentbus.TakeRanked(ctx, queueLog, ledger, st, claimant, 1,
		agentBusHeldNodes(st, claimant), agentbus.QueueLimits{})
	if err != nil {
		// The host is full: the step stays parked until a slot frees, which is what
		// "queue rather than fail" means.
		if reason, refused := agentbus.IsBudgetReject(err); refused && reason == agentbus.RefuseSlots {
			return 0, nil
		}
		return 0, err
	}
	dispatched := 0
	for _, entry := range taken {
		node := entry.Entry.Node
		deadline := now.Add(agentBusDispatchLease)
		op := board.Op{
			Verb: board.VerbClaim, Node: node, Actor: claimant,
			Bounds: boundsForClaim(st, node), Deadline: deadline,
		}
		// One dispatch is one attempt, not one intent forever: the derived id covers node, actor
		// and bounds but never time, so handing the same step out again after its claim lapsed
		// would collapse onto the first op and never land. The deadline names the attempt.
		op.ID = fmt.Sprintf("agentbus-dispatch:%s/%s/%d", boardName, node, deadline.UnixNano())
		if _, err := c.ApplyAgentBusOp(ctx, op); err != nil {
			return dispatched, err
		}
		if deliver == nil {
			dispatched++
			continue
		}
		target := agentbus.WakeTarget{
			Participant: claimant,
			Key:         agentbus.DispatchKey(boardName, node),
			Ready:       []string{node},
		}
		if err := deliver(ctx, target); err != nil {
			// Nobody was told, so nobody owns it: give the step back rather than leave it
			// claimed by a session that never heard about it.
			_, _ = c.ApplyAgentBusOp(ctx, board.Op{Verb: board.VerbRelease, Node: node, Actor: claimant})
			return dispatched, fmt.Errorf("agentbus: deliver %s to %q: %w", node, claimant, err)
		}
		dispatched++
	}
	return dispatched, nil
}

// releaseIdleSlots gives back the slot of a holder with no claim on the board. The board is
// what says who is working: a holder whose work was released, settled or swept owns nothing,
// so keeping its slot would fill the host once and park every later dispatch for good.
func releaseIdleSlots(ledger *agentbus.Ledger, st *board.State) {
	if ledger == nil || st == nil {
		return
	}
	holders := ledger.SlotHolders()
	if len(holders) == 0 {
		return
	}
	working := map[string]bool{}
	for _, node := range st.Nodes {
		if node.Owner != "" && node.State == board.StateClaimed {
			working[node.Owner] = true
		}
	}
	for _, holder := range holders {
		if !working[holder] {
			ledger.ReleaseSlot(holder)
		}
	}
}

// parkStartableWork records startable work nobody has taken in the queue. An entry is not
// a promise to start now: it is the host's "waiting for a slot" list, and it is also what
// the human briefing counts as parked.
func parkStartableWork(ctx context.Context, queueLog *agentbus.QueueLog, st *board.State, now time.Time) error {
	if st == nil {
		return nil
	}
	for _, target := range agentbus.WakeTargets(agentbus.WakeInput{State: st, Now: now}) {
		// Addressed work parks like pooled work: the queue is what the take rule reads,
		// whichever group named the node.
		for _, node := range append(append([]string(nil), target.Ready...), target.Assigned...) {
			if !dispatchable(st, node) {
				continue
			}
			if _, _, err := queueLog.Enqueue(ctx, agentbus.QueueEntry{
				Node: node, Subtree: agentbus.SubtreeRoot(st, node), Participant: target.Participant,
			}, agentbus.QueueLimits{}); err != nil {
				return err
			}
		}
	}
	return nil
}

// dispatchable reports whether auto-dispatch may offer this step at all: work that has
// spent its retry budget is left to the human signal instead of being handed out forever.
func dispatchable(st *board.State, node string) bool {
	n := st.Nodes[node]
	return n != nil && n.NoProgress < agentBusDispatchTries
}

// agentBusHeldNodes are the nodes a participant already owns or has asserted: the affinity
// signal that keeps a dispatched step inside the subtree the participant is working in.
func agentBusHeldNodes(st *board.State, participant string) []string {
	if st == nil || participant == "" {
		return nil
	}
	ids := make([]string, 0, len(st.Nodes))
	for id := range st.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		n := st.Nodes[id]
		if n.Owner == participant {
			out = append(out, id)
			continue
		}
		for _, a := range n.Asserts {
			if a.Actor == participant {
				out = append(out, id)
				break
			}
		}
	}
	return out
}

// boundsForClaim keeps a step's declared ceiling when it had one and gives it one step when
// it did not, because a claim has to declare bounds.
func boundsForClaim(st *board.State, node string) *board.Bounds {
	if st != nil {
		if n := st.Nodes[node]; n != nil && n.Bounds != nil && n.Bounds.Steps > 0 {
			declared := *n.Bounds
			return &declared
		}
	}
	return &board.Bounds{Steps: 1}
}
