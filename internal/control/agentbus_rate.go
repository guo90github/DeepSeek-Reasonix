package control

import (
	"log/slog"
	"sync"
	"sync/atomic"

	"reasonix/internal/agentbus/board"
	"reasonix/internal/config"
)

// SetAgentBusNodeRate arms the board's ceiling on how often one node may move. Zero leaves
// it off, as every other kernel limit does.
func (c *Controller) SetAgentBusNodeRate(limits board.Limits) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.agentBus != nil {
		c.agentBus.nodeRate = limits
	}
}

// AgentBusNodeRate maps the operator's knob onto the kernel's allowance. The window is the
// minute the ceiling names, so there is no second knob to keep in step with it.
func AgentBusNodeRate(cfg config.AgentBusConfig) board.Limits {
	return board.Limits{NodeRatePerMinute: cfg.NodeRatePerMinute}
}

// nodeRateLimits reads this session's ceiling under the state lock.
func (b *agentBusState) nodeRateLimits() board.Limits {
	if b == nil {
		return board.Limits{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.nodeRate
}

// NodeRateRefusals is the host's record of moves the rate ceiling turned down: how many, and
// which node last. A refused move lands nothing, so it leaves no op, no claim and nothing in
// any session's transcript — the same invisible brake a spent ceiling is (G3).
type NodeRateRefusals struct {
	Count int64
	Last  string
}

// nodeRateRefusals is process-wide like the other refusal counts: the ceiling is the board's,
// and every session this host builds writes to it.
var nodeRateRefusals = struct {
	mu    sync.Mutex
	count atomic.Int64
	last  string
}{}

// AgentBusNodeRateRefusals reports the moves this process's board ceiling turned down.
func AgentBusNodeRateRefusals() NodeRateRefusals {
	nodeRateRefusals.mu.Lock()
	defer nodeRateRefusals.mu.Unlock()
	return NodeRateRefusals{Count: nodeRateRefusals.count.Load(), Last: nodeRateRefusals.last}
}

func noteNodeRateRefusal(node string) {
	nodeRateRefusals.mu.Lock()
	nodeRateRefusals.count.Add(1)
	nodeRateRefusals.last = node
	count := nodeRateRefusals.count.Load()
	nodeRateRefusals.mu.Unlock()
	slog.Warn("controller: agentbus move refused by the node rate ceiling", "node", node, "refusals", count)
}
