package agentbus

import (
	"context"

	"reasonix/internal/agentbus/board"
)

// Take hands parked work to a claimant that has a free slot, oldest first, and
// stops at the first refusal instead of failing. When the host is full the work
// stays parked — that is what "queue rather than fail" means, and nothing about the
// parked work's scene changes while it waits (AGENT_BUS §13.4).
func Take(ctx context.Context, log *QueueLog, ledger *Ledger, st *board.State, claimant string, want int, lim QueueLimits) ([]QueueEntry, error) {
	if err := ledger.AcquireSlot(claimant); err != nil {
		return nil, err
	}
	state, _, err := log.Read()
	if err != nil {
		return nil, err
	}
	entries := takeableFor(st, state.Next(0), claimant)
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

// takeableFor drops parked work the board addressed to somebody else: an assignment is a
// promise to one participant, so taking it in another name is the mis-delivery the wake
// routing exists to prevent. No board, or no assignee, leaves the entry in the pool.
func takeableFor(st *board.State, entries []QueueEntry, claimant string) []QueueEntry {
	if st == nil {
		return entries
	}
	out := make([]QueueEntry, 0, len(entries))
	for _, entry := range entries {
		if n := st.Nodes[entry.Node]; n != nil && n.Assignee != "" && n.Assignee != claimant {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// TakeRanked takes with advice: the same slot rule and the same queue, but the batch
// is chosen by Rank instead of by arrival. The queue's visible order does not move —
// only which parked entries this claimant picks up first.
func TakeRanked(ctx context.Context, log *QueueLog, ledger *Ledger, state *board.State, claimant string, want int, held []string, lim QueueLimits) ([]RankedEntry, error) {
	if err := ledger.AcquireSlot(claimant); err != nil {
		return nil, err
	}
	queue, _, err := log.Read()
	if err != nil {
		return nil, err
	}
	ranked := Rank(state, takeableFor(state, queue.Next(0), claimant), held)
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
