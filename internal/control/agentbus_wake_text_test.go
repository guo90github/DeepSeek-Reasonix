package control

import (
	"strings"
	"testing"

	"reasonix/internal/agentbus"
)

// A wake waits in the queue until the turn ends, so its lists are a snapshot: the block has to
// send the reader back to the board, or a model acts on work that was claimed or finished after
// the wake was sent — on a real machine a wake arrived 4.5 minutes after its list had stopped
// being true (2026-10-03).
func TestAgentBusWakePromptSaysItsListsAreASnapshot(t *testing.T) {
	woken := AgentBusWakePrompt(agentbus.WakeTarget{
		Participant: "bob",
		Key:         "agentbus-wake:default/bob",
		Ready:       []string{"schema"},
	})
	if !strings.Contains(woken, "read the board before acting") {
		t.Fatalf("wake block = %q, want it to send the reader back to the board", woken)
	}
	if !strings.Contains(woken, "startable now: schema") {
		t.Fatalf("wake block = %q, want the lists it was woken for", woken)
	}
	if !strings.HasSuffix(woken, "</agentbus-wake>\n") {
		t.Fatalf("wake block = %q, want the block closed", woken)
	}

	assigned := AgentBusWakePrompt(agentbus.WakeTarget{
		Participant: "bob",
		Key:         agentbus.DispatchKey("default", "schema"),
		Ready:       []string{"schema"},
	})
	if !strings.Contains(assigned, "re-read the board before acting") {
		t.Fatalf("dispatch block = %q, want the same caveat on an assignment", assigned)
	}
	// The host writes a long lease and nothing renews it — desktop/agentbus*.go never calls
	// heartbeat — so the block has to tell the worker how to keep the claim it was handed.
	if !strings.Contains(assigned, "heartbeat") {
		t.Fatalf("dispatch block = %q, want it to say how a long-running claim stays live", assigned)
	}
}

// Work the board addressed to one participant has to say so: the wake is the reader's only
// account of why it was woken, and nobody else may take these.
func TestAgentBusWakePromptNamesWorkAddressedToTheReader(t *testing.T) {
	woken := AgentBusWakePrompt(agentbus.WakeTarget{Participant: "bob", Key: "k", Assigned: []string{"schema"}})
	if !strings.Contains(woken, "addressed to you") || !strings.Contains(woken, "schema") {
		t.Fatalf("wake block = %q, want the addressed work named", woken)
	}
	line := AgentBusWakeLine(agentbus.WakeTarget{Participant: "bob", Assigned: []string{"schema"}})
	if !strings.Contains(line, "addressed to you") || !strings.Contains(line, "schema") {
		t.Fatalf("wake line = %q, want a person told the same thing", line)
	}
}

// The block is the model's only account of why it was woken: a target with nothing to say must
// still render a closed block rather than labels with no content under them.
func TestAgentBusWakePromptRendersEmptyListsAsABlock(t *testing.T) {
	empty := AgentBusWakePrompt(agentbus.WakeTarget{Participant: "bob"})
	if strings.Contains(empty, "startable now") {
		t.Fatalf("block = %q, want no label for an empty list", empty)
	}
	if !strings.HasPrefix(empty, "<agentbus-wake>\n") || !strings.HasSuffix(empty, "</agentbus-wake>\n") {
		t.Fatalf("block = %q, want it wrapped and closed", empty)
	}
}
