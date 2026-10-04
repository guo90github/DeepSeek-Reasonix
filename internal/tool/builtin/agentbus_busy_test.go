package builtin

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/agentbus"
)

// "What is it busy with" is the column a peer wants before handing over work, and it used to
// live only in each session's private inbox (F57, 2026-10-05).
func TestAgentBusToolRendersWhatAPeerIsBusyWith(t *testing.T) {
	port := &fakeBoardPort{
		dir: "/tmp/board/default", participant: "bob",
		roster: []agentbus.ParticipantRef{
			{Participant: "carol", Model: "deepseek-flash", Busy: "running a turn"},
			{Participant: "dave"},
		},
	}

	out, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t, `{"action":"participants"}`))
	if err != nil {
		t.Fatalf("participants: %v", err)
	}
	if !strings.Contains(out, "busy=running a turn") {
		t.Fatalf("the roster does not carry what carol is doing:\n%s", out)
	}
	if got := strings.Count(out, "busy="); got != 1 {
		t.Fatalf("%d rows carry a busy column, want only the busy peer:\n%s", got, out)
	}
}
