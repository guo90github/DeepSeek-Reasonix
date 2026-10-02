package agentbus

import "context"

// Take hands parked work to a claimant that has a free slot, oldest first, and
// stops at the first refusal instead of failing. When the host is full the work
// stays parked — that is what "queue rather than fail" means, and nothing about the
// parked work's scene changes while it waits (AGENT_BUS §13.4).
func Take(ctx context.Context, log *QueueLog, ledger *Ledger, claimant string, want int, lim QueueLimits) ([]QueueEntry, error) {
	if err := ledger.AcquireSlot(claimant); err != nil {
		return nil, err
	}
	state, _, err := log.Read()
	if err != nil {
		return nil, err
	}
	taken := make([]QueueEntry, 0, want)
	for _, entry := range state.Next(want) {
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
