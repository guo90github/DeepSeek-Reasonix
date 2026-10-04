package agentbus

import (
	"testing"

	"reasonix/internal/agentbus/board"
)

// The pool is what nobody holds and nobody waits on. The wake names requesters and the
// dispatcher takes only work with a waiter, so exactly these steps were invisible to every
// session on the board (F53/F40, 2026-10-05).
func TestThePoolListsWhatNobodyHoldsAndNobodyWaitsOn(t *testing.T) {
	st := board.NewState()
	st.Nodes["free"] = &board.Node{ID: "free", Title: "nobody wants it", State: board.StateOpen, LastSeq: 1}
	st.Nodes["waited"] = &board.Node{ID: "waited", State: board.StateOpen, LastSeq: 2}
	st.Nodes["held"] = &board.Node{ID: "held", State: board.StateClaimed, Owner: "bob", LastSeq: 3}
	st.Nodes["addressed"] = &board.Node{ID: "addressed", State: board.StateOpen, Assignee: "carol", LastSeq: 4}
	st.Nodes["blocked"] = &board.Node{ID: "blocked", State: board.StateBlocked, Deps: []string{"free"}, LastSeq: 5}

	pool := BuildPool(st)
	waiters := map[string]int{}
	for _, entry := range pool {
		waiters[entry.ID] = entry.Waiters
	}

	if got, ok := waiters["free"]; !ok || got != 1 {
		t.Fatalf("free = (%d, %t), want it in the pool with the one node waiting on it", got, ok)
	}
	if got, ok := waiters["waited"]; !ok || got != 0 {
		t.Fatalf("waited = (%d, %t), want it in the pool with no waiter", got, ok)
	}
	for _, id := range []string{"held", "addressed", "blocked"} {
		if _, ok := waiters[id]; ok {
			t.Fatalf("%s belongs to somebody or cannot start: %+v", id, pool)
		}
	}
	if pool[0].ID != "free" || pool[1].ID != "waited" {
		t.Fatalf("pool order = %+v, want the oldest move first", pool)
	}
}
