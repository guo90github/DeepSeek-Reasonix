package control

import (
	"context"
	"log/slog"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
)

// SetAgentBusWaker installs the host's routing for ready-work wakes. The kernel
// decides who has work waiting and why; only the host knows how to reach them — a
// sibling controller's inbox in this process, a tab, or another machine. Without a
// waker the kernel wakes nobody, which is the safe default: the host's own tick
// scan stays the fallback (AGENT_BUS §13.2).
func (c *Controller) SetAgentBusWaker(fn func(context.Context, agentbus.WakeTarget) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.agentBus != nil {
		c.agentBus.waker = fn
	}
}

// WakeAgentBus wakes the participants who have work waiting on them and reports
// how many were woken. The wake is keyed by the work, not by the clock, so an
// unchanged board wakes nobody however often a host ticks; a new work set wakes
// exactly once. A failed wake releases its key so the next tick retries it.
func (c *Controller) WakeAgentBus(ctx context.Context) int {
	bus, input, err := c.agentBusWakeSnapshot(ctx)
	if err != nil {
		return 0
	}
	waker := bus.currentWaker()
	if waker == nil {
		return 0
	}
	me := bus.participantID(c)
	woken := 0
	for _, target := range agentbus.WakeTargets(input) {
		// Waking ourselves is pointless: this session's next turn already carries
		// its own work.
		if target.Participant == "" || target.Participant == me {
			continue
		}
		if !bus.claimWake(target.Participant, target.Key) {
			continue
		}
		if err := waker(ctx, target); err != nil {
			slog.Warn("controller: agentbus wake", "participant", target.Participant, "err", err)
			bus.releaseWake(target.Participant, target.Key)
			continue
		}
		woken++
	}
	return woken
}

func (c *Controller) agentBusWakeSnapshot(ctx context.Context) (*agentBusState, agentbus.WakeInput, error) {
	bus, err := c.agentBusForTalk()
	if err != nil {
		return nil, agentbus.WakeInput{}, err
	}
	brd, err := board.Open(bus.dir)
	if err != nil {
		return nil, agentbus.WakeInput{}, err
	}
	ops, err := brd.Ops(ctx)
	if err != nil {
		return nil, agentbus.WakeInput{}, err
	}
	talkLog, err := agentbus.OpenTalkLog(bus.dir)
	if err != nil {
		return nil, agentbus.WakeInput{}, err
	}
	talk, _, err := talkLog.Read()
	if err != nil {
		return nil, agentbus.WakeInput{}, err
	}
	hearingLog, err := agentbus.OpenHearingLog(bus.dir)
	if err != nil {
		return nil, agentbus.WakeInput{}, err
	}
	hearings, _, err := hearingLog.Read()
	if err != nil {
		return nil, agentbus.WakeInput{}, err
	}
	return bus, agentbus.WakeInput{
		Ops:      ops,
		Talk:     talk,
		Hearings: hearings,
		Limits:   bus.limitsForHearing(),
		Now:      time.Now().UTC(),
	}, nil
}

func (b *agentBusState) currentWaker() func(context.Context, agentbus.WakeTarget) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.waker
}

// claimWake records that this participant was woken for exactly this work set.
// One entry per participant is enough: a participant is only ever owed its newest
// wake, and the key makes an older one irrelevant.
func (b *agentBusState) claimWake(participant, key string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.woken == nil {
		b.woken = map[string]string{}
	}
	if b.woken[participant] == key {
		return false
	}
	b.woken[participant] = key
	return true
}

func (b *agentBusState) releaseWake(participant, key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.woken[participant] == key {
		delete(b.woken, participant)
	}
}
