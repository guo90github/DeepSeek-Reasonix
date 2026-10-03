package main

import (
	"strings"
	"testing"

	"reasonix/internal/agentbus"
)

// A wake waits in the queue until the turn ends, so its lists are a snapshot: the prompt has
// to send the reader back to the board, or a model acts on work that was claimed or finished
// after the wake was sent — on a real machine a wake arrived 4.5 minutes after its list had
// stopped being true (2026-10-03).
func TestAgentBusWakePromptSaysItsListsAreASnapshot(t *testing.T) {
	woken := agentBusWakePrompt(agentbus.WakeTarget{
		Participant: "bob",
		Key:         "agentbus-wake:default/bob",
		Ready:       []string{"schema"},
	})
	if !strings.Contains(woken, "read the board before acting") {
		t.Fatalf("wake prompt = %q, want it to send the reader back to the board", woken)
	}

	assigned := agentBusWakePrompt(agentbus.WakeTarget{
		Participant: "bob",
		Key:         agentbus.DispatchKey("default", "schema"),
		Ready:       []string{"schema"},
	})
	if !strings.Contains(assigned, "re-read the board before acting") {
		t.Fatalf("dispatch prompt = %q, want the same caveat on an assignment", assigned)
	}
}
