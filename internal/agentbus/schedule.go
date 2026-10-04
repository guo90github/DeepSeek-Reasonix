package agentbus

import (
	"context"

	"reasonix/internal/agentbus/board"
)

// Take hands parked work to a claimant that has a free slot, oldest first, and
// stops at the first refusal instead of failing. When the host is full the work
// stays parked — that is what "queue rather than fail" means, and nothing about the
// parked work's scene changes while it waits (AGENT_BUS §13.4).
//
// The queue is read before the slot is spent: a claimant with nothing it may take is
// not refused and holds no slot, so the ceiling only ever answers for work that is
// there to be held back.
func Take(ctx context.Context, log *QueueLog, ledger *Ledger, st *board.State, claimant string, want int, lim QueueLimits) ([]QueueEntry, error) {
	state, _, err := log.Read()
	if err != nil {
		return nil, err
	}
	entries := takeableFor(st, state.Next(0), claimant)
	if len(entries) == 0 {
		return nil, nil
	}
	if err := ledger.AcquireSlot(claimant); err != nil {
		return nil, err
	}
	if want > 0 && len(entries) > want {
		entries = entries[:want]
	}
	taken := make([]QueueEntry, 0, len(entries))
	for _, entry := range entries {
		if _, err := log.Claim(ctx, entry.Node, claimant, lim); err != nil {
			if _, refused := IsQueueReject(err); refused {
				continue
			}
			return taken, err
		}
		taken = append(taken, entry)
	}
	return taken, nil
}

// takeableFor drops parked work the board addressed to somebody else (a mis-delivery to take it
// in another name) and work the board has already finished: a settled node is not waiting for a
// slot, and taking it only made the queue churn (2026-10-04: one done node re-claimed every 30s).
func takeableFor(st *board.State, entries []QueueEntry, claimant string) []QueueEntry {
	if st == nil {
		return entries
	}
	out := make([]QueueEntry, 0, len(entries))
	for _, entry := range entries {
		if n := st.Nodes[entry.Node]; n != nil {
			if nodeSettled(n.State) {
				continue
			}
			if n.Assignee != "" && n.Assignee != claimant {
				continue
			}
		}
		out = append(out, entry)
	}
	return out
}

// nodeSettled reports whether the board has finished with a node, and so filled the slot the queue
// held for it. Refute can pull either state back to contested, and the parker re-enqueues then.
func nodeSettled(state board.NodeState) bool {
	return state == board.StateDone || state == board.StateAbandoned
}

// TakeRanked takes with advice: the same slot rule and the same queue, but the batch
// is chosen by Rank instead of by arrival. The queue's visible order does not move —
// only which parked entries this claimant picks up first.
func TakeRanked(ctx context.Context, log *QueueLog, ledger *Ledger, state *board.State, claimant string, want int, held []string, lim QueueLimits) ([]RankedEntry, error) {
	queue, _, err := log.Read()
	if err != nil {
		return nil, err
	}
	entries := takeableFor(state, queue.Next(0), claimant)
	if len(entries) == 0 {
		return nil, nil
	}
	if err := ledger.AcquireSlot(claimant); err != nil {
		return nil, err
	}
	ranked := Rank(state, entries, held)
	if want > 0 && len(ranked) > want {
		ranked = ranked[:want]
	}
	taken := make([]RankedEntry, 0, len(ranked))
	for _, entry := range ranked {
		if _, err := log.Claim(ctx, entry.Entry.Node, claimant, lim); err != nil {
			if _, refused := IsQueueReject(err); refused {
				continue
			}
			return taken, err
		}
		taken = append(taken, entry)
	}
	return taken, nil
}
