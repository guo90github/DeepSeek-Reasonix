package agentbus

import (
	"testing"
	"time"

	"reasonix/internal/agentbus/board"
)

func parkEntry(node, subtree string, seq uint64) QueueEntry {
	return QueueEntry{
		Node: node, Subtree: subtree,
		EnqueuedAt: talkBase.Add(time.Duration(seq) * time.Second), Seq: seq,
	}
}

func claimOpSteps(node string, steps int) board.Op {
	return board.Op{
		Verb: board.VerbClaim, Node: node, Actor: "alice",
		Bounds: &board.Bounds{Steps: steps}, Deadline: time.Now().UTC().Add(time.Hour),
	}
}

func rankedNodes(ranked []RankedEntry) []string {
	out := make([]string, 0, len(ranked))
	for _, entry := range ranked {
		out = append(out, entry.Entry.Node)
	}
	return out
}

func sameOrder(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// chain is root <- mid <- leaf: finishing root unblocks two nodes, finishing leaf none.
func chainState(t *testing.T) *board.State {
	t.Helper()
	return testState(t,
		assertOp("root", "alice"),
		assertOp("mid", "alice"),
		requireOp("mid", "root"),
		assertOp("leaf", "alice"),
		requireOp("leaf", "mid"),
	)
}

func TestRankPrefersTheMostUnblockingWork(t *testing.T) {
	st := chainState(t)
	entries := []QueueEntry{parkEntry("leaf", "root", 1), parkEntry("mid", "root", 2), parkEntry("root", "root", 3)}

	ranked := Rank(st, entries, nil)
	if !sameOrder(rankedNodes(ranked), []string{"root", "mid", "leaf"}) {
		t.Fatalf("order = %v, want the most unblocking first", rankedNodes(ranked))
	}
	if ranked[0].Critical != 2 {
		t.Fatalf("critical(root) = %d, want two dependents", ranked[0].Critical)
	}
	if ranked[2].Critical != 0 {
		t.Fatalf("critical(leaf) = %d, want none", ranked[2].Critical)
	}

	// Equal advice falls back to arrival: mid and leaf are only separable by it once
	// affinity is out of the picture.
	tied := []QueueEntry{parkEntry("leaf", "", 1), parkEntry("mid", "", 2)}
	if !sameOrder(rankedNodes(Rank(st, tied, nil)), []string{"mid", "leaf"}) {
		t.Fatalf("order = %v, want criticality to outrank arrival", rankedNodes(Rank(st, tied, nil)))
	}
}

func TestRankPrefersAffinityOverCriticality(t *testing.T) {
	st := testState(t,
		assertOp("root", "alice"),
		assertOp("mid", "alice"),
		requireOp("mid", "root"),
		assertOp("leaf", "alice"),
		requireOp("leaf", "mid"),
		// hub unblocks two nodes but sits in its own subtree.
		assertOp("hub", "alice"),
		assertOp("dep1", "alice"),
		requireOp("dep1", "hub"),
		assertOp("dep2", "alice"),
		requireOp("dep2", "hub"),
	)
	entries := []QueueEntry{parkEntry("hub", "hub", 1), parkEntry("mid", "root", 2)}

	// Holding "leaf" means the claimant is already inside the "root" subtree.
	ranked := Rank(st, entries, []string{"leaf"})
	if !sameOrder(rankedNodes(ranked), []string{"mid", "hub"}) {
		t.Fatalf("order = %v, want the same-subtree entry first", rankedNodes(ranked))
	}
	if !ranked[0].Affinity || ranked[0].Critical != 1 {
		t.Fatalf("first = %+v, want affinity even though hub unblocks more", ranked[0])
	}
	if ranked[1].Affinity || ranked[1].Critical != 2 {
		t.Fatalf("second = %+v, want the more critical but foreign subtree", ranked[1])
	}
}

func TestRankPrefersTheHardestWhenNothingElseSeparates(t *testing.T) {
	st := testState(t,
		board.Op{Verb: board.VerbAssert, Node: "small", Actor: "alice", Evidence: ev("small")},
		board.Op{Verb: board.VerbAssert, Node: "big", Actor: "alice", Evidence: ev("big")},
		claimOpSteps("small", 2),
		claimOpSteps("big", 9),
	)
	entries := []QueueEntry{parkEntry("small", "", 1), parkEntry("big", "", 2)}

	ranked := Rank(st, entries, nil)
	if !sameOrder(rankedNodes(ranked), []string{"big", "small"}) {
		t.Fatalf("order = %v, want the larger step ceiling first", rankedNodes(ranked))
	}
	if ranked[0].Steps != 9 || ranked[1].Steps != 2 {
		t.Fatalf("steps = %d/%d, want the declared ceilings", ranked[0].Steps, ranked[1].Steps)
	}
}

func TestRankNeverLosesOrInventsWork(t *testing.T) {
	st := chainState(t)
	entries := []QueueEntry{parkEntry("root", "root", 1), parkEntry("mid", "root", 2), parkEntry("leaf", "root", 3)}
	for _, held := range [][]string{nil, {"root"}, {"mid", "leaf"}} {
		ranked := Rank(st, entries, held)
		if len(ranked) != len(entries) {
			t.Fatalf("ranked %d entries from %d: advice must return the same set", len(ranked), len(entries))
		}
		seen := map[string]bool{}
		for _, entry := range ranked {
			if seen[entry.Entry.Node] {
				t.Fatalf("duplicate entry %q in %v", entry.Entry.Node, rankedNodes(ranked))
			}
			seen[entry.Entry.Node] = true
		}
		for _, entry := range entries {
			if !seen[entry.Node] {
				t.Fatalf("entry %q vanished from the advice", entry.Node)
			}
		}
	}
}

func TestRankFallsBackToArrivalWithoutABoard(t *testing.T) {
	entries := []QueueEntry{parkEntry("first", "", 1), parkEntry("second", "", 2), parkEntry("third", "", 3)}
	ranked := Rank(nil, entries, []string{"anything"})
	if !sameOrder(rankedNodes(ranked), []string{"first", "second", "third"}) {
		t.Fatalf("order = %v, want arrival order when nothing is known", rankedNodes(ranked))
	}
}

// The four levels are one chain, not four independent rules: a tie at one level has to fall
// through to the next. These cases make the first two levels equal *in substance* (same subtree,
// same number of dependents) rather than only trivially empty, and the last case puts all three
// in play at once — where affinity has to win over both criticality and a larger step ceiling.
func TestRankBreaksTiesDownTheWholeChain(t *testing.T) {
	// root <- mid <- leaf, and root <- mid2 <- leaf2: mid and mid2 sit in the same subtree and
	// each unblocks exactly one node, so only the declared step ceiling can separate them.
	st := testState(t,
		assertOp("root", "alice"),
		assertOp("mid", "alice"), requireOp("mid", "root"),
		assertOp("mid2", "alice"), requireOp("mid2", "root"),
		assertOp("leaf", "alice"), requireOp("leaf", "mid"),
		assertOp("leaf2", "alice"), requireOp("leaf2", "mid2"),
		// root has to be done before mid/mid2 can be claimed — and the claim is what carries the
		// declared step ceiling the ranking reads.
		board.Op{Verb: board.VerbDecide, Node: "root", Actor: "alice", Outcome: board.OutcomeDone,
			Evidence: ev("root"), ReproducedBy: "bob"},
		claimOpSteps("mid", 2), claimOpSteps("mid2", 9),
		// hub unblocks two nodes but lives in its own subtree.
		assertOp("hub", "alice"), assertOp("dep1", "alice"), requireOp("dep1", "hub"),
		assertOp("dep2", "alice"), requireOp("dep2", "hub"),
	)
	held := []string{"leaf"}

	// Same subtree, same dependents, different ceilings: the hardest one leads.
	bySteps := []QueueEntry{parkEntry("mid", "root", 1), parkEntry("mid2", "root", 2)}
	if !sameOrder(rankedNodes(Rank(st, bySteps, held)), []string{"mid2", "mid"}) {
		t.Fatalf("order = %v, want the larger ceiling after two equal levels", rankedNodes(Rank(st, bySteps, held)))
	}

	// Affinity equal *in substance* (both entries are already inside the root subtree) and only
	// criticality separating them: mid unblocks leaf, leaf unblocks nothing.
	byCritical := []QueueEntry{parkEntry("leaf", "root", 5), parkEntry("mid", "root", 6)}
	rankedByCritical := Rank(st, byCritical, held)
	if !sameOrder(rankedNodes(rankedByCritical), []string{"mid", "leaf"}) {
		t.Fatalf("order = %v, want criticality to decide once affinity ties", rankedNodes(rankedByCritical))
	}
	if !rankedByCritical[0].Affinity || !rankedByCritical[1].Affinity {
		t.Fatalf("affinity = %+v, want both entries inside the held subtree", rankedNodes(rankedByCritical))
	}

	// All three levels in play: affinity outranks both the more critical node and its larger ceiling.
	mixed := []QueueEntry{parkEntry("hub", "hub", 1), parkEntry("mid2", "root", 2)}
	ranked := Rank(st, mixed, held)
	if !sameOrder(rankedNodes(ranked), []string{"mid2", "hub"}) {
		t.Fatalf("order = %v, want affinity over criticality and steps", rankedNodes(ranked))
	}
	if !ranked[0].Affinity || ranked[0].Critical != 1 || ranked[0].Steps != 9 {
		t.Fatalf("first = %+v, want the affine node with its own numbers", ranked[0])
	}
	if ranked[1].Affinity || ranked[1].Critical != 2 || ranked[1].Steps >= 9 {
		t.Fatalf("second = %+v, want the foreign subtree that unblocks more", ranked[1])
	}
}
