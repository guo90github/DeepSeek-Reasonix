package builtin

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/agentbus"
)

// A session learns who it is working beside from the read it already makes: wakes carry work,
// never company, and the roster was reachable only by a caller that already knew to ask for it
// (F53, 2026-10-05).
func TestTheViewPointsAtTheRosterWhenSomebodyElseIsOnTheBoard(t *testing.T) {
	port := &fakeBoardPort{
		dir: "/tmp/board/default", participant: "bob",
		roster: []agentbus.ParticipantRef{{Participant: "bob"}, {Participant: "carol"}},
	}

	out, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t, `{"action":"view"}`))
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if !strings.Contains(out, "1 other session(s)") || !strings.Contains(out, "action=participants") {
		t.Fatalf("the view does not point at the roster:\n%s", out)
	}

	// Alone on the board the view says nothing extra: a pointer to nobody is noise.
	alone := &fakeBoardPort{
		dir: "/tmp/board/default", participant: "bob",
		roster: []agentbus.ParticipantRef{{Participant: "bob"}},
	}
	out, err = NewAgentBusTool(alone).Execute(context.Background(), boardArgs(t, `{"action":"view"}`))
	if err != nil {
		t.Fatalf("view: %v", err)
	}
	if strings.Contains(out, "action=participants") {
		t.Fatalf("a session alone on the board was pointed at peers:\n%s", out)
	}
}
