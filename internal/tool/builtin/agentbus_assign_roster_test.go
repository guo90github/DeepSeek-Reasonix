package builtin

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/agentbus"
)

// Assigning to somebody who is not on the board leaves the step reachable by nobody: the board
// refuses a stranger's claim, the wake skips it (it has an assignee), and the queue keeps an
// entry no participant will ever take. A typo is the usual cause, so the tool checks the name
// against the roster it can already read (F39/F50, 2026-10-05).
func TestAssigningToSomebodyNotOnTheBoardIsRefusedWithTheRoster(t *testing.T) {
	port := &fakeBoardPort{
		dir: "/tmp/board/default", participant: "bob",
		roster: []agentbus.ParticipantRef{{Participant: "bob"}, {Participant: "carol"}},
	}

	_, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t, `{"action":"assign","node":"step","assignee":"ghost-xyz"}`))
	if err == nil {
		t.Fatal("assigning to a ghost reached the board")
	}
	for _, want := range []string{"ghost-xyz", "bob", "carol"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal does not name %q: %v", want, err)
		}
	}
	if len(port.applied) != 0 {
		t.Fatalf("a refused assign reached the board: %+v", port.applied)
	}

	// Somebody who is on the board goes through.
	if _, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t, `{"action":"assign","node":"step","assignee":"carol"}`)); err != nil {
		t.Fatalf("assigning to a participant on the board: %v", err)
	}
	if len(port.applied) != 1 {
		t.Fatalf("applied %d ops, want the one assign", len(port.applied))
	}
}

// An empty or unreadable roster is no evidence that a name is wrong: a board with nobody
// announced yet must still accept an assignment (2026-10-05).
func TestAssigningOnABoardWithNoRosterIsNotRefused(t *testing.T) {
	port := &fakeBoardPort{dir: "/tmp/board/default", participant: "bob"}
	tool := NewAgentBusTool(port)

	for _, assignee := range []string{"carol", "ghost-xyz"} {
		args := `{"action":"assign","node":"step","assignee":"` + assignee + `"}`
		if _, err := tool.Execute(context.Background(), boardArgs(t, args)); err != nil {
			t.Fatalf("assigning %q on a board with no roster: %v", assignee, err)
		}
	}
	if len(port.applied) != 2 {
		t.Fatalf("applied %d ops, want both assigns to reach the board", len(port.applied))
	}
}
