package control

import (
	"context"
	"log/slog"
	"time"

	"reasonix/internal/agentbus"
)

// busyReport is this session's own answer to "what is it doing, and for how long": a peer reads
// it to decide whether to wait or take the step itself. Until this existed only the session's
// private inbox said "running", and the board its peers read said nothing (F57, 2026-10-05).
func (c *Controller) busyReport() string {
	if !c.Running() {
		return ""
	}
	if started := c.inFlightTurnStartedAt(); !started.IsZero() {
		return "running a turn since " + started.UTC().Format(time.RFC3339)
	}
	return "running a turn"
}

// refreshAgentBusPresence keeps this session on the roster without rewriting the address book
// on every tick: a record is renewed once it has aged past half its TTL, which is well before
// it would be dropped as stale. It re-sends the ref it announced with, so a refresh cannot
// erase an endpoint the way re-announcing with no host would (2026-10-05).
func (c *Controller) refreshAgentBusPresence(ctx context.Context, now time.Time) {
	state, err := c.agentBusForTalk()
	if err != nil {
		return
	}
	state.mu.Lock()
	last := state.announced
	state.mu.Unlock()
	if last.Participant == "" || last.Withdrawn {
		return
	}
	if !last.At.IsZero() && now.Sub(last.At) < agentbus.ParticipantTTL/2 {
		return
	}
	last.At = now
	last.Busy = c.busyReport()
	directory, err := agentbus.OpenParticipantDirectory(state.dir)
	if err != nil {
		return
	}
	if _, err := directory.Announce(ctx, last); err != nil {
		slog.Warn("controller: agentbus presence refresh", "board", state.dir, "err", err)
		return
	}
	state.mu.Lock()
	state.announced = last
	state.mu.Unlock()
}
