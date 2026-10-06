package control

import (
	"context"

	"reasonix/internal/agent"
	"reasonix/internal/sessioncontext"
	"reasonix/internal/skill"
)

// withTurnContext attaches current role-specific snapshots to a host turn.
// Synthetic turns receive bootstrap-only context: they may repair an upgraded
// legacy session with no snapshot, but never advertise a mid-session update.
func (c *Controller) withTurnContext(ctx context.Context, realUserTurn bool) context.Context {
	if c == nil {
		return ctx
	}
	executorSections, plannerSections := c.turnContextSections()
	bundle := agent.TurnContextBundle{
		Executor:      sessioncontext.Build(executorSections),
		Planner:       sessioncontext.Build(plannerSections),
		BootstrapOnly: !realUserTurn,
	}
	return agent.WithTurnContextBundle(ctx, bundle)
}

// turnContextSections assembles this moment's executor and planner section sets.
// One caller sends them to the provider; the recall record fingerprints the
// executor digest, so both describe the same snapshot.
func (c *Controller) turnContextSections() (executor, planner sessioncontext.Sections) {
	executor = c.sessionContextStatic
	if mem := c.memory.current(); mem != nil {
		executor.BackgroundMemory = mem.BackgroundDataBlock()
	}
	planner = executor
	if !c.disableImplicitSkillInvocation {
		sk := c.skills.list()
		executor.SkillsCatalog = skill.CatalogBlock(sk)
		planner.SkillsCatalog = skill.ReadOnlyCatalogBlock(sk)
	}
	return executor, planner
}

// executorTurnDigest fingerprints the session-context the provider sees this turn.
func (c *Controller) executorTurnDigest() string {
	executor, _ := c.turnContextSections()
	return sessioncontext.Build(executor).Digest
}
