package agentbus

import (
	"sort"

	"reasonix/internal/agentbus/board"
)

// RankedEntry is one parked entry plus the signals it was ranked by, so a human or
// a log can see why one piece of work was suggested before another.
type RankedEntry struct {
	Entry QueueEntry
	// Affinity reports that this entry sits in a subtree the claimant is already
	// working in: taking it keeps one participant inside one subtree (and is what
	// makes a batch coherent).
	Affinity bool
	// Critical is how many nodes wait, directly or transitively, on this one.
	Critical int
	// Steps is the declared work ceiling of the node: the hardest-first signal.
	Steps int64
}

// Rank orders parked work for one claimant: stay inside the subtree you are already
// in, then unblock the most, then take the hardest, and otherwise keep arrival
// order. It is advice only — the same entries, reordered — so `Next` stays the
// queue's visible truth and no entry can be starved by a hint.
func Rank(state *board.State, entries []QueueEntry, held []string) []RankedEntry {
	roots := make(map[string]bool, len(held))
	for _, node := range held {
		if root := SubtreeRoot(state, node); root != "" {
			roots[root] = true
		}
	}
	index := dependentIndex(state)

	out := make([]RankedEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, RankedEntry{
			Entry:    entry,
			Affinity: entry.Subtree != "" && roots[entry.Subtree],
			Critical: transitiveDependents(index, entry.Node),
			Steps:    nodeSteps(state, entry.Node),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		left, right := out[i], out[j]
		if left.Affinity != right.Affinity {
			return left.Affinity
		}
		if left.Critical != right.Critical {
			return left.Critical > right.Critical
		}
		if left.Steps != right.Steps {
			return left.Steps > right.Steps
		}
		// Equal advice falls back to the input's order, which is arrival order:
		// that tie-break is what keeps the queue starvation-free.
		return false
	})
	return out
}

// dependentIndex inverts the dependency edges once, so ranking many entries costs
// one pass over the board rather than one per entry.
func dependentIndex(st *board.State) map[string][]string {
	if st == nil {
		return nil
	}
	index := make(map[string][]string, len(st.Nodes))
	for id, node := range st.Nodes {
		for _, dep := range node.Deps {
			index[dep] = append(index[dep], id)
		}
	}
	return index
}

// transitiveDependents counts what finishing one node would unblock.
func transitiveDependents(index map[string][]string, node string) int {
	if len(index) == 0 {
		return 0
	}
	seen := map[string]bool{}
	queue := append([]string(nil), index[node]...)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if seen[current] || current == node {
			continue
		}
		seen[current] = true
		queue = append(queue, index[current]...)
	}
	return len(seen)
}

func nodeSteps(st *board.State, node string) int64 {
	if st == nil {
		return 0
	}
	n, ok := st.Nodes[node]
	if !ok || n.Bounds == nil {
		return 0
	}
	return int64(n.Bounds.Steps)
}
