package control

import (
	"context"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
)

// AgentBusNodeDetail reads one board node for the human side: what it is, what it waits for,
// what was disputed and who authorized it (§13.8). Authorizations come from the op log, which
// is the only place that says which assertion was a grant. It reads; it never writes.
func (c *Controller) AgentBusNodeDetail(node string) (agentbus.NodeDetail, bool) {
	ctx := context.Background()
	bus, hearings, state, err := c.agentBusHearingSnapshot(ctx)
	if err != nil || bus == nil || state == nil {
		return agentbus.NodeDetail{}, false
	}
	brd, err := board.Open(bus.dir)
	if err != nil {
		return agentbus.NodeDetail{}, false
	}
	ops, err := brd.Ops(ctx)
	if err != nil {
		return agentbus.NodeDetail{}, false
	}
	return agentbus.DescribeNode(state, hearings, ops, node)
}
