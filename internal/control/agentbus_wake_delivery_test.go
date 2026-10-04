package control

import (
	"strings"
	"testing"

	"reasonix/internal/agentbus"
)

// The dispatcher hands a step over at the moment it claims it, and that delivery is what an
// idle session reads first: a step somebody has already reported on must not arrive as "do it,
// then decide it" (2026-10-05).
func TestDeliveredAssignmentForAStepThatAlreadyCarriesReadingsAsksForTheVerdict(t *testing.T) {
	reported := AgentBusWakePrompt(agentbus.WakeTarget{
		Participant: "bob", Key: agentbus.DispatchKey("default", "step"), Ready: []string{"step"}, Reports: 1,
	})
	if strings.Contains(reported, "do it, then decide it") {
		t.Fatalf("the delivered wake still tells the holder to redo reported work:\n%s", reported)
	}
	for _, want := range []string{"step", "1 assertion", "decide", "action=view", "heartbeat"} {
		if !strings.Contains(reported, want) {
			t.Fatalf("the delivered wake does not carry %q:\n%s", want, reported)
		}
	}
}

func TestDeliveredAssignmentWithoutReadingsKeepsTheDoItInstruction(t *testing.T) {
	fresh := AgentBusWakePrompt(agentbus.WakeTarget{
		Participant: "bob", Key: agentbus.DispatchKey("default", "step"), Ready: []string{"step"},
	})
	if !strings.Contains(fresh, "do it, then decide it") {
		t.Fatalf("a fresh assignment lost its instruction:\n%s", fresh)
	}
}
