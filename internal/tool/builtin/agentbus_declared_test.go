package builtin

import (
	"context"
	"strings"
	"testing"

	"reasonix/internal/agentbus"
)

// The roster's declared columns reach the model: who is on the board, what they speak as, and
// where they work — the three facts a reader needs before handing out work (F48, 2026-10-05).
func TestAgentBusToolRendersTheDeclaredFacts(t *testing.T) {
	port := &fakeBoardPort{
		dir: "/tmp/board/default", participant: "bob",
		roster: []agentbus.ParticipantRef{{
			Participant: "carol", SessionPath: "/sessions/carol",
			Model: "deepseek-flash", Workspace: "/work/beta", Role: "reviewer",
		}},
	}

	out, err := NewAgentBusTool(port).Execute(context.Background(), boardArgs(t, `{"action":"participants"}`))
	if err != nil {
		t.Fatalf("participants: %v", err)
	}
	for _, want := range []string{"model=deepseek-flash", "workspace=/work/beta", "role=reviewer"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the roster does not carry %q:\n%s", want, out)
		}
	}
}
