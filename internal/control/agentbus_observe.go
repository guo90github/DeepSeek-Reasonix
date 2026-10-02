package control

import (
	"context"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
)

// AgentBusBriefing folds this session's board, queue and deliberations into the
// human first screen: only the subtrees with something wrong with them, and the
// signals behind them. It reads; it never writes a surface or moves a cursor.
func (c *Controller) AgentBusBriefing(now time.Time) (agentbus.Briefing, bool) {
	bus, err := c.agentBusForTalk()
	if err != nil {
		return agentbus.Briefing{}, false
	}
	ctx := context.Background()
	brd, err := board.Open(bus.dir)
	if err != nil {
		return agentbus.Briefing{}, false
	}
	state, err := brd.Snapshot(ctx, now)
	if err != nil {
		return agentbus.Briefing{}, false
	}
	queueLog, err := agentbus.OpenQueueLog(bus.dir)
	if err != nil {
		return agentbus.Briefing{}, false
	}
	queue, _, err := queueLog.Read()
	if err != nil {
		return agentbus.Briefing{}, false
	}
	hearingLog, err := agentbus.OpenHearingLog(bus.dir)
	if err != nil {
		return agentbus.Briefing{}, false
	}
	hearings, _, err := hearingLog.Read()
	if err != nil {
		return agentbus.Briefing{}, false
	}
	briefing := agentbus.Observe(state, queue, hearings, now, bus.limitsForObserve())
	return *briefing, true
}

// SetAgentBusObserveLimits tunes how much the first screen carries. Zero keeps the
// kernel defaults.
func (c *Controller) SetAgentBusObserveLimits(limits agentbus.ObserveLimits) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.agentBus != nil {
		c.agentBus.observeLimits = limits
	}
}

func (b *agentBusState) limitsForObserve() agentbus.ObserveLimits {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.observeLimits
}

// AgentBusParticipant reports the id this session speaks under, so a host can route
// a wake to the session that owns it. Empty means this session has no standing on
// any board, and a host should route to nobody.
func (c *Controller) AgentBusParticipant() string {
	state, err := c.agentBusForTalk()
	if err != nil {
		return ""
	}
	return state.participantID(c)
}

// AgentBusDir reports the board this session is enrolled on, so a host can hand the
// board to its waker: a wake that no local tab owns must be routed using that board's
// address book, not another one's. Empty means this session is off the board.
func (c *Controller) AgentBusDir() string {
	state, err := c.agentBusForTalk()
	if err != nil {
		return ""
	}
	return state.dir
}
