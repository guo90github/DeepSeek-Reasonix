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
		Bounds: &board.Bounds{Steps: steps}, Deadline: talkBase.Add(time.Hour),
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
