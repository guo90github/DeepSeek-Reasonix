package builtin

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/agentbus"
)

// The roster has to reach the model: "who is on this board" was answerable nowhere until the
// tool could read the address book beside it (F48/F49, 2026-10-05).
func TestAgentBusToolListsWhoIsOnTheBoard(t *testing.T) {
	port := &fakeBoardPort{
		dir: "/tmp/board/default", participant: "bob",
		roster: []agentbus.ParticipantRef{
			{Participant: "bob", SessionPath: "/sessions/bob"},
			{Participant: "carol", Host: "http://127.0.0.1:9", SessionPath: "/sessions/carol"},
		},
	}

	out, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t, `{"action":"participants"}`))
	if err != nil {
		t.Fatalf("participants: %v", err)
	}
	for _, want := range []string{"2 on this board now", "bob (you)", "carol", "/sessions/carol", "host=http://127.0.0.1:9"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the roster does not carry %q:\n%s", want, out)
		}
	}
}

// An empty roster says so rather than showing nothing: "nobody announced" and "nothing to see"
// are different facts, and only one of them is a bug (2026-10-05).
func TestAgentBusToolSaysWhenNobodyIsOnTheBoard(t *testing.T) {
	port := &fakeBoardPort{dir: "/tmp/board/default", participant: "bob"}

	out, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t, `{"action":"participants"}`))
	if err != nil {
		t.Fatalf("participants: %v", err)
	}
	if !strings.Contains(out, "nobody is on this board") {
		t.Fatalf("the empty roster reads %q, want it to say so", out)
	}
}
