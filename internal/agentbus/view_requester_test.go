package agentbus

import (
	"strings"
	"testing"

	"reasonix/internal/agentbus/board"
)

// A split or a require records whoever wrote that structure as the node's requester, and the
// wake names them ("startable now") — but the row used to be missing, so a session that had
// just split a container read a view with none of its own children in it (F17, 2026-10-05).
func TestAViewShowsTheNodesMyOwnStructureCreated(t *testing.T) {
	st := board.NewState()
	st.Nodes["kid"] = &board.Node{
		ID: "kid", Title: "the child I split out", State: board.StateOpen,
		Requesters: []string{"carol"}, LastSeq: 3,
	}

	view := BuildView(st, ViewSpec{Board: "default", Participant: "carol"})
	if !strings.Contains(view.Render(), "kid") {
		t.Fatalf("the splitter cannot see the node it created:\n%s", view.Render())
	}
	if view.Owned != 0 {
		t.Fatalf("owned = %d, want asking for a node not counted as holding it", view.Owned)
	}

	// Being a stranger to the structure still shows nothing.
	stranger := BuildView(st, ViewSpec{Board: "default", Participant: "dave"})
	if len(stranger.Lines) != 0 {
		t.Fatalf("dave sees %+v, want nothing: he neither holds nor asked for the node", stranger)
	}
}
