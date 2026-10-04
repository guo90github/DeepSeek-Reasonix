package board

import "testing"

// unassign used to be written as an assign op carrying an empty name, so the op trail could
// not tell "addressed to nobody" from "addressed to somebody": they are two different
// decisions and now two verbs (2026-10-05).
func TestUnassignIsItsOwnVerbAndClearsOnlyTheAssignee(t *testing.T) {
	st := &State{Nodes: map[string]*Node{"n1": {ID: "n1", State: StateOpen, Assignee: "bob", LastSeq: 1}}}

	if err := applyOp(st, Op{Verb: VerbAssign, Node: "n1", Actor: "alice", Assignee: "carol", Seq: 2}); err != nil {
		t.Fatalf("assign: %v", err)
	}
	if st.Nodes["n1"].Assignee != "carol" {
		t.Fatalf("assign did not address the step, got %q", st.Nodes["n1"].Assignee)
	}

	if err := applyOp(st, Op{Verb: VerbUnassign, Node: "n1", Actor: "alice", Seq: 3}); err != nil {
		t.Fatalf("unassign: %v", err)
	}
	if st.Nodes["n1"].Assignee != "" {
		t.Fatalf("unassign left the step addressed to %q", st.Nodes["n1"].Assignee)
	}
	if st.Nodes["n1"].State != StateOpen {
		t.Fatalf("unassign changed the state, got %q", st.Nodes["n1"].State)
	}
}
