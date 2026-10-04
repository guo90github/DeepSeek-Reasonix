package agentbus

import (
	"strings"
	"testing"

	"reasonix/internal/agentbus/board"
)

// The op that moved a node is what tells a reader who wrote it: the host's dispatch writes
// agentbus-dispatch:…, a session's own tool writes op-…, and no surface showed either. The row
// carries the source, not the whole id — the id itself stays in the board's op log, and a row
// that spelled it out blew the byte cap (F64/F46, 2026-10-05).
func TestAViewRowNamesWhoWroteTheOpThatMovedTheNode(t *testing.T) {
	st := board.NewState()
	st.Nodes["handed"] = &board.Node{
		ID: "handed", Title: "handed over by the host", State: board.StateOpen,
		Requesters: []string{"bob"}, LastOpID: "agentbus-dispatch:default/handed/123", LastSeq: 4,
	}
	st.Nodes["mine"] = &board.Node{ID: "mine", State: board.StateOpen, Owner: "bob", LastSeq: 5}

	rendered := BuildView(st, ViewSpec{Board: "default", Participant: "bob"}).Render()
	if !strings.Contains(rendered, "op=dispatch") {
		t.Fatalf("the row does not name who handed the work over:\n%s", rendered)
	}
	if got := strings.Count(rendered, "op="); got != 1 {
		t.Fatalf("%d rows carry an op, want only the node that has one:\n%s", got, rendered)
	}
	// A node whose op is not known keeps the row it had before this column existed.
	if !strings.Contains(rendered, "title=\"\"\n") {
		t.Fatalf("a row without an op is not byte-stable:\n%s", rendered)
	}
}

// The three sources the board mints are the three the row has to tell apart.
func TestTheOpSourceNamesEachWriter(t *testing.T) {
	cases := map[string]string{
		"op-1234":                             "hand",
		"agentbus-dispatch:default/step/1728": "dispatch",
		"sweep-node-1699999999":               "sweep",
		"":                                    "other",
		"something-else":                      "other",
	}
	for id, want := range cases {
		if got := opSource(id); got != want {
			t.Errorf("opSource(%q) = %q, want %q", id, got, want)
		}
	}
}
