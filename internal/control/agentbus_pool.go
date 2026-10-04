package control

import (
	"time"

	"reasonix/internal/agentbus"
)

// AgentBusPool lists the board's common pool: the steps anybody could pick up. Nothing
// broadcasts them, so this is the pull side of discovery — the only way a session that was
// never woken learns there is something it could take (F53/F40, 2026-10-05).
func (c *Controller) AgentBusPool(now time.Time) ([]agentbus.PoolEntry, error) {
	bus, st := c.agentBusSnapshot(now)
	if bus == nil {
		return nil, errAgentBusUnwired
	}
	return agentbus.BuildPool(st), nil
}
