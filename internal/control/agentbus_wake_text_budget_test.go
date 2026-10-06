package control

import (
	"fmt"
	"strings"
	"testing"

	"reasonix/internal/agentbus"
)

// F7 第 3 条: a wake block used to render every list in full, so a board with hundreds of nodes
// put the whole board into the prompt (a wake is a pointer to work). The cap must bite on the
// long case, say what it left out, and cost the ordinary case nothing.
func TestAWakeNamingALongListStaysInsideItsBudget(t *testing.T) {
	items := make([]string, 0, 400)
	for i := range 400 {
		items = append(items, fmt.Sprintf("scn-step-%03d", i))
	}
	block := AgentBusWakePrompt(agentbus.WakeTarget{Participant: "bob", Ready: items})
	if len(block) > agentBusWakeMaxBytes+128 {
		t.Fatalf("block = %d bytes, want it bounded near %d", len(block), agentBusWakeMaxBytes)
	}
	if !strings.Contains(block, "…and") || !strings.Contains(block, "action=view") {
		t.Fatalf("a cut list must say how much it left out and where the rest is:\n%s", block)
	}
	if strings.Contains(block, "scn-step-399") {
		t.Fatalf("the cap cut nothing: the last item is still in the block")
	}

	small := AgentBusWakePrompt(agentbus.WakeTarget{Participant: "bob", Ready: []string{"step-1", "step-2"}})
	if !strings.Contains(small, "step-1, step-2") {
		t.Fatalf("an ordinary list must render as it always did:\n%s", small)
	}
	if strings.Contains(small, "…and") {
		t.Fatalf("nothing was omitted, so nothing may claim otherwise:\n%s", small)
	}
}
