package board

import (
	"testing"
	"time"
)

func TestAssignAddressesANodeAndRecordsTheAssignee(t *testing.T) {
	st := Fold([]Op{
		{Verb: VerbAssert, Node: "step", Actor: "alice", Evidence: []Evidence{{Ref: "test:assert"}}},
		{Verb: VerbAssign, Node: "step", Actor: "alice", Assignee: "bob"},
	})
	n := st.Nodes["step"]
	if n.Assignee != "bob" {
		t.Fatalf("assignee = %q, want bob", n.Assignee)
	}
	if len(n.Requesters) != 2 || n.Requesters[1] != "bob" {
		t.Fatalf("requesters = %v, want the assignee recorded too, or the board cannot show them the work", n.Requesters)
	}
}

func TestAssignRefusesALiveClaimAndAnEmptyAssignee(t *testing.T) {
	st := Fold([]Op{
		{Verb: VerbAssert, Node: "step", Actor: "alice", Evidence: []Evidence{{Ref: "test:assert"}}},
		{Verb: VerbClaim, Node: "step", Actor: "alice", Deadline: time.Now().UTC().Add(time.Hour), Bounds: &Bounds{Steps: 1}},
	})
	err := applyOp(st, Op{Verb: VerbAssign, Node: "step", Actor: "alice", Assignee: "bob"})
	if reason, refused := IsReject(err); !refused || reason != ReasonIllegalTransition {
		t.Fatalf("re-addressing a live claim = (%v, %q), want illegal_transition", refused, reason)
	}
	if got := st.Nodes["step"].Assignee; got != "" {
		t.Fatalf("assignee = %q, want the refused op to change nothing", got)
	}
	err = applyOp(st, Op{Verb: VerbAssign, Node: "step", Actor: "alice"})
	if reason, refused := IsReject(err); !refused || reason != ReasonMissingAssignee {
		t.Fatalf("empty assignee = (%v, %q), want missing_assignee", refused, reason)
	}
}
