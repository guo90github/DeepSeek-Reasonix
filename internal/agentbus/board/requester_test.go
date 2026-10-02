package board

import (
	"testing"
)

// Who asked for a node is otherwise only in the op trail. The fold records it: the verb that
// created the node names its requester, and every later require names the dependency it wants
// (AGENT_BUS §13.2, §13.3).
func TestRequestersComeFromTheOpThatCreatedTheNode(t *testing.T) {
	b := openTestBoard(t)
	mustApply(t, b, opAssert("design", "planner"))
	mustApply(t, b, Op{Verb: VerbRequire, Node: "design", Actor: "browser", Dep: &NodeSpec{ID: "schema"}})
	mustApply(t, b, Op{Verb: VerbSplit, Node: "design", Actor: "splitter",
		Children: []NodeSpec{{ID: "part-a"}, {ID: "part-b"}}})
	st := snapshot(t, b)

	cases := []struct {
		node string
		want []string
	}{
		{"design", []string{"planner"}},  // asserted by the planner; nobody asked for it as a step
		{"schema", []string{"browser"}},  // created by that require: browser wants it done
		{"part-a", []string{"splitter"}}, // created by the split
		{"part-b", []string{"splitter"}}, //
	}
	for _, tc := range cases {
		got := st.Nodes[tc.node].Requesters
		if len(got) != len(tc.want) {
			t.Fatalf("%s requesters = %v, want %v", tc.node, got, tc.want)
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("%s requesters = %v, want %v", tc.node, got, tc.want)
			}
		}
	}
}

// Asking twice is asking once, and the order is the order they asked in.
func TestARequesterIsRecordedOnceInTheOrderTheyAsked(t *testing.T) {
	b := openTestBoard(t)
	mustApply(t, b, opAssert("n", "alice"))
	mustApply(t, b, opAssert("n", "alice"))
	mustApply(t, b, opAssert("n", "bob"))
	got := snapshot(t, b).Nodes["n"].Requesters
	if len(got) != 2 || got[0] != "alice" || got[1] != "bob" {
		t.Fatalf("requesters = %v, want alice then bob, each once", got)
	}
}

// Every node is created by an op that names who asked for it, so a node on a real board never
// has an empty requester list — that is what makes the fold able to answer the question.
func TestEveryCreatedNodeCarriesItsRequester(t *testing.T) {
	b := openTestBoard(t)
	mustApply(t, b, opAssert("root", "planner"))
	mustApply(t, b, Op{Verb: VerbRequire, Node: "root", Actor: "worker", Dep: &NodeSpec{ID: "step"}})
	st := snapshot(t, b)
	if len(st.Nodes) != 2 {
		t.Fatalf("nodes = %d, want 2", len(st.Nodes))
	}
	for id, n := range st.Nodes {
		if len(n.Requesters) == 0 {
			t.Fatalf("%s has no requester: the fold cannot say who wants it", id)
		}
	}
}

// The fold hands out clones; a slice added here must be copied like the others, or a reader
// could edit the state through the snapshot it was given.
func TestCloneDoesNotShareTheRequesterList(t *testing.T) {
	b := openTestBoard(t)
	mustApply(t, b, opAssert("n", "alice"))

	st := snapshot(t, b)
	clone := st.Clone()
	clone.Nodes["n"].Requesters[0] = "someone-else"

	if got := st.Nodes["n"].Requesters[0]; got != "alice" {
		t.Fatalf("editing a clone changed the state it came from: %q", got)
	}
	if again := snapshot(t, b); again.Nodes["n"].Requesters[0] != "alice" {
		t.Fatalf("editing a clone changed the cached fold: %v", again.Nodes["n"].Requesters)
	}
}
