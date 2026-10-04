package agentbus

import (
	"strings"
	"testing"

	"reasonix/internal/agentbus/board"
)

// A wake tells the assignee "this step is yours, and nobody else may take it". The view the wake
// points at therefore has to show it, and show what waits on it — before 2026-10-05 all three of
// the view's rules (owned / asserted / needed) missed an assigned-but-unowned node, so a session
// woken for its own step read an empty board (real machine: same-host session pair).
func TestTheViewShowsWorkAddressedToMe(t *testing.T) {
	st := board.NewState()
	st.Nodes["g7-block"] = &board.Node{ID: "g7-block", State: board.StateOpen, Assignee: "bob", LastSeq: 1}
	st.Nodes["g7-drill"] = &board.Node{
		ID: "g7-drill", State: board.StateOpen, Owner: "alice", Deps: []string{"g7-block"},
		Requesters: []string{"alice"}, LastSeq: 2,
	}

	view := BuildView(st, ViewSpec{Board: "default", Participant: "bob"})
	if view.Owned != 1 {
		t.Fatalf("owned = %d, want the step addressed to bob", view.Owned)
	}
	if view.Waiting != 1 {
		t.Fatalf("waiting = %d, want the deliverable that waits on bob's step", view.Waiting)
	}
	rendered := view.Render()
	for _, want := range []string{"g7-block", "g7-drill"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("view = %q, want it to show %q", rendered, want)
		}
	}

	// A step addressed to someone else is still none of this participant's business.
	other := BuildView(st, ViewSpec{Board: "default", Participant: "carol"})
	if other.Owned != 0 || other.Waiting != 0 || other.Needed != 0 || len(other.Lines) != 0 {
		t.Fatalf("carol sees %+v, want nothing: the step is bob's", other)
	}
}
