package board

import (
	"testing"
	"time"
)

// The id of the op that moved a node belongs to the folded state, not only to the live one: a
// cold fold has to agree with the cached state, or a reader sees the marker in one and not the
// other (F64, 2026-10-05).
func TestTheFoldRemembersTheOpThatMovedTheNode(t *testing.T) {
	st := &State{Nodes: map[string]*Node{"n1": {ID: "n1", State: StateOpen}}}
	evidence := []Evidence{{Kind: "test", Ref: "go test ./..."}}

	if err := applyOp(st, Op{Verb: VerbAssert, Node: "n1", Actor: "alice", Title: "one", Evidence: evidence, ID: "op-abc", Seq: 1}); err != nil {
		t.Fatal(err)
	}
	if got := st.Nodes["n1"].LastOpID; got != "op-abc" {
		t.Fatalf("folded LastOpID = %q, want the op that moved it", got)
	}

	handed := "agentbus-dispatch:default/n1/9"
	if err := applyOp(st, Op{
		Verb: VerbClaim, Node: "n1", Actor: "bob", ID: handed, Seq: 2,
		Bounds: &Bounds{Steps: 1}, Deadline: time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if got := st.Nodes["n1"].LastOpID; got != handed {
		t.Fatalf("LastOpID = %q, want the newest op, whoever wrote it", got)
	}
}
