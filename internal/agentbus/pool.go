package agentbus

import (
	"sort"

	"reasonix/internal/agentbus/board"
)

// PoolEntry is one step anybody could pick up: it can start, nobody holds it, and it is
// addressed to nobody.
type PoolEntry struct {
	ID    string
	Title string
	// Waiters is how many other nodes are waiting on this one. A step with none is the case no
	// surface showed at all: the wake names requesters, and the dispatcher takes only work
	// somebody waits on, so it sat invisible to every session (F53/F40, 2026-10-05).
	Waiters int
	LastSeq uint64
}

// BuildPool lists the board's common pool, ordered by the move that last touched each step so
// the ones sitting longest come first. It reads the fold and no clock, like the view does.
func BuildPool(st *board.State) []PoolEntry {
	if st == nil {
		return nil
	}
	waiting := waitersByDep(st)
	out := make([]PoolEntry, 0, len(st.Nodes))
	for _, id := range sortedNodeIDs(st) {
		n := st.Nodes[id]
		if n.Owner != "" || n.Assignee != "" || !n.Ready(st) {
			continue
		}
		out = append(out, PoolEntry{ID: id, Title: n.Title, Waiters: len(waiting[id]), LastSeq: n.LastSeq})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].LastSeq != out[j].LastSeq {
			return out[i].LastSeq < out[j].LastSeq
		}
		return out[i].ID < out[j].ID
	})
	return out
}
