package agentbus

import (
	"strings"
	"testing"

	"reasonix/internal/agentbus/board"
)

// A row has to answer "can I take this step now": the board refuses a claim from anyone but
// the assignee, so the readout names whose step it is and reflects that same gate. Before
// this, a step addressed to somebody else read startable=true while claim refused it, and
// the counters line counted it as ready too (2026-10-05).
func TestAViewRowNamesTheAssigneeAndNarrowsStartable(t *testing.T) {
	st := board.NewState()
	st.Nodes["step"] = &board.Node{ID: "step", Title: "hand it over", State: board.StateOpen, Assignee: "bob", LastSeq: 7}
	st.Nodes["mine"] = &board.Node{ID: "mine", State: board.StateBlocked, Owner: "dave", Deps: []string{"step"}, LastSeq: 8}

	// dave sees the step because his own node waits on it, not because it is his.
	dave := BuildView(st, ViewSpec{Board: "default", Participant: "dave"})
	rendered := dave.Render()
	if !strings.Contains(rendered, "assignee=bob") {
		t.Fatalf("the row does not name the assignee:\n%s", rendered)
	}
	if !strings.Contains(rendered, "startable=false") {
		t.Fatalf("a step addressed to somebody else reads as startable:\n%s", rendered)
	}
	if dave.Ready != 0 {
		t.Fatalf("ready = %d while the only open step is addressed to somebody else", dave.Ready)
	}

	bob := BuildView(st, ViewSpec{Board: "default", Participant: "bob"})
	if !strings.Contains(bob.Render(), "assignee=bob") || !strings.Contains(bob.Render(), "startable=true") {
		t.Fatalf("the assignee's own row should be startable and named:\n%s", bob.Render())
	}
	if bob.Ready != 1 {
		t.Fatalf("ready = %d for the assignee, want its own step counted", bob.Ready)
	}
}
