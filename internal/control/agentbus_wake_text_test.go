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

// A question and a deliberation are both talk-side: neither becomes a board row, so a block that
// sends its reader to the board leaves it looking for a row that is not there — the woken
// session's own view was empty while its wake named the correlation (2026-10-04).
func TestAgentBusWakePromptSendsTalkSideWorkToTheTalkSurface(t *testing.T) {
	cases := []struct {
		name   string
		target agentbus.WakeTarget
		want   []string
		absent []string
	}{
		{
			name:   "only a question",
			target: agentbus.WakeTarget{Participant: "bob", Key: "k", Asks: []string{"ask-1234"}},
			want:   []string{"questions addressed to you: ask-1234", "action=answer", "correlation="},
			absent: []string{"read the board before acting", "hearing_answer"},
		},
		{
			name:   "only an owed deliberation",
			target: agentbus.WakeTarget{Participant: "bob", Key: "k", Owes: []string{"schema"}},
			want:   []string{"deliberations you owe an answer about: schema", "action=hearing_answer", "node="},
			absent: []string{"read the board before acting", "action=answer"},
		},
		{
			name: "a question and a deliberation",
			target: agentbus.WakeTarget{Participant: "bob", Key: "k",
				Asks: []string{"ask-1234"}, Owes: []string{"schema"}},
			want:   []string{"action=answer", "correlation=", "action=hearing_answer", "node="},
			absent: []string{"read the board before acting"},
		},
		{
			name: "board work alongside a question",
			target: agentbus.WakeTarget{Participant: "bob", Key: "k",
				Ready: []string{"schema"}, Asks: []string{"ask-1234"}},
			want: []string{"read the board before acting", "startable now: schema"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rendered := AgentBusWakePrompt(tc.target)
			for _, want := range tc.want {
				if !strings.Contains(rendered, want) {
					t.Fatalf("block = %q, want %q", rendered, want)
				}
			}
			for _, absent := range tc.absent {
				if strings.Contains(rendered, absent) {
					t.Fatalf("block = %q, want no %q", rendered, absent)
				}
			}
		})
	}
}
