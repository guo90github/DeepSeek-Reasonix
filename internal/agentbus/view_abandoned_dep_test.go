package agentbus

import (
	"strings"
	"testing"

	"reasonix/internal/agentbus/board"
)

// The row has to agree with itself: a dependency that was abandoned is settled, so it must not
// be counted as open while the same row calls the node startable (F22/F44, 2026-10-05).
func TestARowCountsAnAbandonedDependencyAsSettled(t *testing.T) {
	st := board.NewState()
	st.Nodes["container"] = &board.Node{
		ID: "container", Title: "assembly", State: board.StateBlocked, Deps: []string{"child"},
		Requesters: []string{"bob"}, LastSeq: 2,
	}
	st.Nodes["child"] = &board.Node{ID: "child", State: board.StateAbandoned, Outcome: board.OutcomeAbandoned, LastSeq: 1}

	rendered := BuildView(st, ViewSpec{Board: "default", Participant: "bob"}).Render()
	if !strings.Contains(rendered, "deps_open=0") {
		t.Fatalf("an abandoned dependency is still counted as open:\n%s", rendered)
	}
	if !strings.Contains(rendered, "startable=true") {
		t.Fatalf("a container whose child was abandoned is not startable:\n%s", rendered)
	}
}
