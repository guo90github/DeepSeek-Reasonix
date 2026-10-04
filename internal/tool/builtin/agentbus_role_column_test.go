package builtin

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/agentbus"
)

// The role column is the operator's word, so it appears exactly when a host declared one: an
// empty label is not a missing one, and printing "role=" with nothing after it says the
// opposite of "nothing was declared" (F48, 2026-10-05).
func TestTheRosterShowsARoleOnlyWhenOneWasDeclared(t *testing.T) {
	args := boardArgs(t, `{"action":"participants"}`)

	declared := &fakeBoardPort{
		dir: "/tmp/board/default", participant: "bob",
		roster: []agentbus.ParticipantRef{{Participant: "carol", Role: "reviewer"}},
	}
	out, err := NewAgentBusTool(declared).Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("participants: %v", err)
	}
	if !strings.Contains(out, "role=reviewer") {
		t.Fatalf("a declared role is missing from the roster:\n%s", out)
	}

	undeclared := &fakeBoardPort{
		dir: "/tmp/board/default", participant: "bob",
		roster: []agentbus.ParticipantRef{{Participant: "carol"}},
	}
	out, err = NewAgentBusTool(undeclared).Execute(context.Background(), args)
	if err != nil {
		t.Fatalf("participants: %v", err)
	}
	if strings.Contains(out, "role=") {
		t.Fatalf("an undeclared role was rendered as an empty label:\n%s", out)
	}
}
