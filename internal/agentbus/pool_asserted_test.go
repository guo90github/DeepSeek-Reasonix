package agentbus

import (
	"testing"

	"reasonix/internal/agentbus/board"
)

// The pool is built from the folded board, so two steps asserted on the same board both belong in
// it: this pins the derivation the push side reads, independently of any host (F53, 2026-10-05).
func TestBuildPoolListsEveryAssertedStepNobodyHolds(t *testing.T) {
	st := board.Fold([]board.Op{
		{Verb: board.VerbAssert, Node: "loose", Actor: "alice", ID: "op-1", Seq: 1, Evidence: []board.Evidence{{Ref: "r1"}}},
		{Verb: board.VerbAssert, Node: "second", Actor: "alice", ID: "op-2", Seq: 2, Evidence: []board.Evidence{{Ref: "r2"}}},
	})

	pool := BuildPool(st)
	ids := make([]string, 0, len(pool))
	for _, entry := range pool {
		ids = append(ids, entry.ID)
	}
	if len(ids) != 2 || ids[0] != "loose" || ids[1] != "second" {
		t.Fatalf("pool = %v, want both asserted steps in the order they last moved", ids)
	}
}
