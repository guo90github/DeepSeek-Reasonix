package control

import (
	"context"
	"fmt"
	"strings"
	"time"

	"reasonix/internal/agentbus"
	"reasonix/internal/agentbus/board"
	"reasonix/internal/sessioninbox"
)

// agentBusWakeKeyPrefix is what a generic wake's key starts with (wake.go:186). A wake is
// recognised by its key, not by its source: one delivered from another host arrives
// without the sending host's source name.
const agentBusWakeKeyPrefix = "agentbus-wake:"

// agentBusWakeInjectionRewrite rebuilds a wake from the board as it is when the turn is
// assembled. A wake is rendered when it is sent and injected when the turn ends — minutes
// later on a long turn — so a frozen list can name work that is already claimed or done
// (measured on a real machine: 4.5 minutes stale, 2026-10-03).
//
// It reports false for anything that is not a wake: every other queued item is injected
// exactly as it was queued.
func (c *Controller) agentBusWakeInjectionRewrite(ctx context.Context, meta sessioninbox.InboxItemMeta) (string, bool) {
	key := strings.TrimSpace(meta.Idempotency)
	if !isAgentBusWakeKey(key) {
		return "", false
	}
	bus, input, err := c.agentBusWakeSnapshot(ctx)
	if err != nil {
		// An unreadable board is not a reason to lose the wake: inject it as sent, which is
		// what the block itself already says it is.
		return "", false
	}
	age := agentBusWakeAge(meta, time.Now().UTC())
	me := bus.participantID(c)
	if _, node, ok := splitAgentBusDispatchKey(key); ok {
		return agentBusDispatchAtInjection(input, node, me, age, key), true
	}
	for _, target := range agentbus.WakeTargets(input) {
		if target.Participant == me {
			return agentBusWakeRebuiltNote(age) + AgentBusWakePrompt(target), true
		}
	}
	return agentBusWakeStaleNote(age) + agentBusWakeRefusedBlock("nothing is waiting on you ("+me+") now"), true
}

// agentBusDispatchAtInjection re-checks one assignment: while the wake waited, its node may
// have been decided or taken over, and acting on a lapsed assignment wastes the turn.
func agentBusDispatchAtInjection(input agentbus.WakeInput, node, me, age, key string) string {
	var n *board.Node
	if input.State != nil {
		n = input.State.Nodes[node]
	}
	switch {
	case n == nil:
		return agentBusWakeStaleNote(age) + agentBusWakeRefusedBlock(node+" is not on the board any more")
	case n.Outcome == board.OutcomeDone:
		return agentBusWakeStaleNote(age) + agentBusWakeRefusedBlock(node+" is already done")
	case n.Owner == me:
		return agentBusWakeRebuiltNote(age) + AgentBusWakePrompt(agentbus.WakeTarget{
			Participant: me, Key: key, Ready: []string{node},
		})
	case n.Owner == "":
		return agentBusWakeStaleNote(age) + agentBusWakeRefusedBlock(node+" is unclaimed now")
	default:
		return agentBusWakeStaleNote(age) + agentBusWakeRefusedBlock(node+" is held by "+n.Owner+" now")
	}
}

func isAgentBusWakeKey(key string) bool {
	return agentbus.IsDispatchKey(key) || strings.HasPrefix(key, agentBusWakeKeyPrefix)
}

func splitAgentBusDispatchKey(key string) (string, string, bool) {
	if !agentbus.IsDispatchKey(key) {
		return "", "", false
	}
	boardName, node, ok := strings.Cut(strings.TrimPrefix(key, agentbus.DispatchKeyPrefix), "/")
	if !ok || node == "" {
		return "", "", false
	}
	return boardName, node, true
}

// agentBusWakeAge names how long the wake waited, so a reader can weigh a list it cannot
// see the age of: the marker line is where that already lives for other wake kinds.
func agentBusWakeAge(meta sessioninbox.InboxItemMeta, now time.Time) string {
	if meta.CreatedAt.IsZero() {
		return "an unknown time"
	}
	return now.Sub(meta.CreatedAt).Round(time.Second).String()
}

func agentBusWakeRebuiltNote(age string) string {
	return "[agentbus wake queued " + age + " ago; rebuilt against the board as it is now]\n"
}

func agentBusWakeStaleNote(age string) string {
	return "[agentbus wake queued " + age + " ago; its list no longer holds]\n"
}

func agentBusWakeRefusedBlock(reason string) string {
	return fmt.Sprintf("<agentbus-wake>\nThe board woke you earlier with work that no longer holds: %s.\nRe-read the board before acting on anything that wake named.\n</agentbus-wake>\n", reason)
}
