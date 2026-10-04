package board

import "testing"

// Whoever writes structure has to be readable off the node they touched: a split recorded its
// author only on the children, so the one session that made the tree could not see the tree
// itself in its own view (F19, 2026-10-05).
func TestASplitRecordsItsAuthorOnTheContainerToo(t *testing.T) {
	st := &State{Nodes: map[string]*Node{"root": {ID: "root", Title: "the tree", State: StateOpen}}}

	if err := applyOp(st, Op{
		Verb: VerbSplit, Node: "root", Actor: "carol", ID: "op-1", Seq: 1,
		Children: []NodeSpec{{ID: "kid", Title: "kid"}},
	}); err != nil {
		t.Fatal(err)
	}

	if !requesters(st.Nodes["root"], "carol") {
		t.Fatalf("the container's requesters = %v, want the session that split it", st.Nodes["root"].Requesters)
	}
	if !requesters(st.Nodes["kid"], "carol") {
		t.Fatalf("the child's requesters = %v, want the session that created it", st.Nodes["kid"].Requesters)
	}
}

// A require touches two nodes: the dependency it names and the node that now waits on it. Both
// have to name the author, or the author cannot read back what it changed (F19, 2026-10-05).
func TestARequireRecordsItsAuthorOnBothNodes(t *testing.T) {
	st := &State{Nodes: map[string]*Node{
		"assembly": {ID: "assembly", Title: "assembly", State: StateOpen},
		"part":     {ID: "part", Title: "part", State: StateOpen},
	}}

	if err := applyOp(st, Op{
		Verb: VerbRequire, Node: "assembly", Actor: "dave", ID: "op-2", Seq: 2,
		Dep: &NodeSpec{ID: "part", Title: "part"},
	}); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"assembly", "part"} {
		if !requesters(st.Nodes[id], "dave") {
			t.Fatalf("%s requesters = %v, want the session that required it", id, st.Nodes[id].Requesters)
		}
	}
}

func requesters(n *Node, participant string) bool {
	for _, requester := range n.Requesters {
		if requester == participant {
			return true
		}
	}
	return false
}
