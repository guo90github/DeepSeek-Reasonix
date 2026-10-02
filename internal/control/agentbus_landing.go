package control

import (
	"context"
	"strings"

	"reasonix/internal/agent"
	"reasonix/internal/agentbus"
)

// agentBusLandingID is the readiness category the board contributes. It is deliberately
// not one of the ids a repeated complete claim may wave through
// (repeatedCompleteMayFinish): a board that has not landed is not a leftover check.
const agentBusLandingID = "agentbus_landing"

// withBoardLanding adds the board's task-level verdict to the host readiness: a deliverable
// left unfinished, or a conclusion still in doubt, is a missing requirement like any other,
// so it rides the readiness the Goal already answers to instead of a second gate beside it
// (AGENT_BUS §13.10, T9-5). A session that is not on a board is returned unchanged, and an
// enrolled session whose board cannot be read is refused rather than waved through.
func (c *Controller) withBoardLanding(ctx context.Context, base agent.ReadinessResult) agent.ReadinessResult {
	bus, hearings, state, err := c.agentBusHearingSnapshot(ctx)
	if bus == nil {
		return base
	}
	if err != nil || state == nil {
		return boardRequirement(base, "the board could not be read")
	}
	landed := agentbus.AssessLanding(state, hearings)
	if landed.Landed {
		return base
	}
	return boardRequirement(base, landed.Reason)
}

// boardRequirement folds one board requirement into a readiness result without losing
// what the host already found missing.
func boardRequirement(base agent.ReadinessResult, reason string) agent.ReadinessResult {
	out := base
	out.Ready = false
	out.Missing = append(append([]string(nil), base.Missing...), agentBusLandingID)
	if strings.TrimSpace(base.Reason) == "" {
		out.Reason = reason
	} else {
		out.Reason = strings.TrimSpace(base.Reason) + "; board: " + reason
	}
	return out
}
